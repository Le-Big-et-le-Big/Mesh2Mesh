package api

import (
	"errors"
	"net/http"
	"strings"
	"Mesh2Mesh/internal/store"
)

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

