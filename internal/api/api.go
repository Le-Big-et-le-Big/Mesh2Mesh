package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"Mesh2Mesh/internal/store"
)

const (
	defaultTokenTTL = time.Hour
	maxTokenTTL     = 30 * 24 * time.Hour
	maxTokenUses    = 1000
)

// Server wires the store into an http.Handler.
type Server struct {
	store *store.Store
	log   *slog.Logger
}

// New builds a Server.
func New(st *store.Store, log *slog.Logger) *Server {
	return &Server{store: st, log: log}
}

// Routes returns the API handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/tenants", s.createTenant)
	mux.HandleFunc("GET /v1/tenants", s.listTenants)
	mux.HandleFunc("GET /v1/tenants/{id}", s.getTenant)
	mux.HandleFunc("POST /v1/tenants/{id}/tokens", s.createToken)
	mux.HandleFunc("GET /v1/tenants/{id}/peers", s.listPeers)
	mux.HandleFunc("POST /v1/peers/register", s.registerPeer)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		s.fail(w, r, http.StatusServiceUnavailable, "database unreachable", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type tenantResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CIDR      string    `json:"cidr"`
	CreatedAt time.Time `json:"created_at"`
}

func tenantView(t *store.Tenant) tenantResponse {
	return tenantResponse{ID: t.ID, Name: t.Name, CIDR: t.CIDR.String(), CreatedAt: t.CreatedAt}
}

// createTenant handles POST /v1/tenants.
func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		CIDR string `json:"cidr"`
	}
	if !decode(w, r, &req, s) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		s.fail(w, r, http.StatusBadRequest, "name is required", nil)
		return
	}

	cidr, err := parseCIDR(req.CIDR)
	if err != nil {
		s.fail(w, r, http.StatusBadRequest, err.Error(), nil)
		return
	}

	tenant, err := s.store.CreateTenant(r.Context(), name, cidr)
	switch {
	case errors.Is(err, store.ErrConflict):
		s.fail(w, r, http.StatusConflict, "a tenant with that name already exists", err)
		return
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not create tenant", err)
		return
	}

	s.log.Info("tenant created", "tenant_id", tenant.ID, "name", tenant.Name, "cidr", tenant.CIDR.String())
	writeJSON(w, http.StatusCreated, tenantView(tenant))
}

// listTenants handles GET /v1/tenants.
func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := s.store.ListTenants(r.Context())
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not list tenants", err)
		return
	}

	out := make([]tenantResponse, 0, len(tenants))
	for i := range tenants {
		out = append(out, tenantView(&tenants[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": out})
}

// getTenant handles GET /v1/tenants/{id}.
func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.store.GetTenant(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, r, http.StatusNotFound, "no such tenant", err)
		return
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not load tenant", err)
		return
	}
	writeJSON(w, http.StatusOK, tenantView(tenant))
}

// createToken handles POST /v1/tenants/{id}/tokens. The plaintext token is in
// the response and nowhere else — it cannot be read back later.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TTLSeconds int `json:"ttl_seconds"`
		MaxUses    int `json:"max_uses"`
	}
	if !decode(w, r, &req, s) {
		return
	}

	ttl := defaultTokenTTL
	if req.TTLSeconds != 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl <= 0 || ttl > maxTokenTTL {
		s.fail(w, r, http.StatusBadRequest, "ttl_seconds must be between 1 and 2592000", nil)
		return
	}

	maxUses := 1
	if req.MaxUses != 0 {
		maxUses = req.MaxUses
	}
	if maxUses < 1 || maxUses > maxTokenUses {
		s.fail(w, r, http.StatusBadRequest, "max_uses must be between 1 and 1000", nil)
		return
	}

	tenantID := r.PathValue("id")
	// Check the tenant exists so a bad id is a 404.
	if _, err := s.store.GetTenant(r.Context(), tenantID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, http.StatusNotFound, "no such tenant", err)
			return
		}
		s.fail(w, r, http.StatusInternalServerError, "could not load tenant", err)
		return
	}

	token, err := s.store.CreateToken(r.Context(), tenantID, ttl, maxUses)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not create token", err)
		return
	}

	s.log.Info("enrollment token issued", "tenant_id", tenantID, "token_id", token.ID, "max_uses", maxUses)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         token.ID,
		"tenant_id":  token.TenantID,
		"token":      token.Secret,
		"max_uses":   token.MaxUses,
		"expires_at": token.ExpiresAt,
	})
}

// registerPeer handles POST /v1/peers/register: a client redeems an enrollment
// token and gets back the mesh address it should configure.
func (s *Server) registerPeer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token     string `json:"token"`
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if !decode(w, r, &req, s) {
		return
	}

	token := strings.TrimSpace(req.Token)
	name := strings.TrimSpace(req.Name)
	publicKey := strings.TrimSpace(req.PublicKey)
	switch {
	case token == "":
		s.fail(w, r, http.StatusBadRequest, "token is required", nil)
		return
	case name == "":
		s.fail(w, r, http.StatusBadRequest, "name is required", nil)
		return
	case publicKey == "":
		s.fail(w, r, http.StatusBadRequest, "public_key is required", nil)
		return
	}

	peer, tenant, err := s.store.RegisterPeer(r.Context(), token, name, publicKey)
	switch {
	case errors.Is(err, store.ErrTokenInvalid):
		s.fail(w, r, http.StatusUnauthorized, "enrollment token is invalid, expired or already used", err)
		return
	case errors.Is(err, store.ErrConflict):
		s.fail(w, r, http.StatusConflict, "that public key is already registered in this tenant", err)
		return
	case errors.Is(err, store.ErrPoolExhausted):
		s.fail(w, r, http.StatusConflict, "the tenant address pool is exhausted", err)
		return
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not register peer", err)
		return
	}

	s.log.Info("peer registered", "tenant_id", tenant.ID, "peer_id", peer.ID, "mesh_ip", peer.MeshIP.String())
	writeJSON(w, http.StatusCreated, map[string]any{
		"peer_id":     peer.ID,
		"tenant_id":   peer.TenantID,
		"tenant_name": tenant.Name,
		"name":        peer.Name,
		"public_key":  peer.PublicKey,
		"mesh_ip":     peer.MeshIP.String(),
		"mesh_cidr":   tenant.CIDR.String(),
		"created_at":  peer.CreatedAt,
	})
}

// listPeers handles GET /v1/tenants/{id}/peers.
func (s *Server) listPeers(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	if _, err := s.store.GetTenant(r.Context(), tenantID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, http.StatusNotFound, "no such tenant", err)
			return
		}
		s.fail(w, r, http.StatusInternalServerError, "could not load tenant", err)
		return
	}

	peers, err := s.store.ListPeers(r.Context(), tenantID)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not list peers", err)
		return
	}

	out := make([]map[string]any, 0, len(peers))
	for _, p := range peers {
		out = append(out, map[string]any{
			"peer_id":    p.ID,
			"name":       p.Name,
			"public_key": p.PublicKey,
			"mesh_ip":    p.MeshIP.String(),
			"created_at": p.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": out})
}
