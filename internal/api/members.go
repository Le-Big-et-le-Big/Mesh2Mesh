package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"Mesh2Mesh/internal/store"
)

// The web UI's server authenticates users with OIDC and calls the /v1/me
// routes on their behalf, with the shared token and these headers.
const (
	headerUserID    = "X-Mesh2Mesh-User"
	headerUserEmail = "X-Mesh2Mesh-User-Email"
	headerUserName  = "X-Mesh2Mesh-User-Name" // URL-escaped: headers are not UTF-8
)

const (
	defaultInviteTTL = 7 * 24 * time.Hour
	maxInviteTTL     = 30 * 24 * time.Hour
	maxInviteUses    = 100
)

func (s *Server) fromWeb(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && s.webToken != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.webToken)) == 1
}

// webOnly guards the routes that expose every tenant. Nodes never call them;
// without a WEB_API_TOKEN they stay open, as before memberships existed.
func (s *Server) webOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.webToken != "" && !s.fromWeb(r) {
			s.fail(w, r, http.StatusUnauthorized, "this route needs the web UI's token", nil)
			return
		}
		h(w, r)
	}
}

type userHandler func(w http.ResponseWriter, r *http.Request, u store.User)

func (s *Server) asUser(h userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.webToken == "" {
			s.fail(w, r, http.StatusServiceUnavailable, "per-user routes need WEB_API_TOKEN on the control plane", nil)
			return
		}
		if !s.fromWeb(r) {
			s.fail(w, r, http.StatusUnauthorized, "this route needs the web UI's token", nil)
			return
		}
		u := store.User{
			ID:    strings.TrimSpace(r.Header.Get(headerUserID)),
			Email: strings.TrimSpace(r.Header.Get(headerUserEmail)),
		}
		if u.ID == "" {
			s.fail(w, r, http.StatusBadRequest, headerUserID+" is required", nil)
			return
		}
		if name, err := url.QueryUnescape(r.Header.Get(headerUserName)); err == nil {
			u.Name = strings.TrimSpace(name)
		}
		h(w, r, u)
	}
}

// member loads the caller's membership of the {id} tenant, answering 404
// for a tenant they are not in, so tenant ids cannot be probed.
func (s *Server) member(w http.ResponseWriter, r *http.Request, u store.User, ownerOnly bool) (*store.Membership, bool) {
	m, err := s.store.GetMembership(r.Context(), r.PathValue("id"), u.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, r, http.StatusNotFound, "no such tenant", nil)
		return nil, false
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not load tenant", err)
		return nil, false
	case ownerOnly && m.Role != store.RoleOwner:
		s.fail(w, r, http.StatusForbidden, "only owners of this tenant can do that", nil)
		return nil, false
	}
	return m, true
}

func membershipView(m *store.Membership) map[string]any {
	return map[string]any{
		"id":         m.ID,
		"name":       m.Name,
		"cidr":       m.CIDR.String(),
		"created_at": m.CreatedAt,
		"role":       m.Role,
	}
}

func (s *Server) myTenants(w http.ResponseWriter, r *http.Request, u store.User) {
	ms, err := s.store.ListMemberships(r.Context(), u)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not list tenants", err)
		return
	}
	out := make([]map[string]any, 0, len(ms))
	for i := range ms {
		out = append(out, membershipView(&ms[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": out})
}

func (s *Server) createMyTenant(w http.ResponseWriter, r *http.Request, u store.User) {
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

	t, err := s.store.CreateOwnedTenant(r.Context(), name, cidr, u)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not create tenant", err)
		return
	}
	s.log.Info("tenant created", "tenant_id", t.ID, "name", t.Name, "cidr", t.CIDR.String(), "owner", u.ID)
	writeJSON(w, http.StatusCreated, membershipView(&store.Membership{Tenant: *t, Role: store.RoleOwner}))
}

func (s *Server) getMyTenant(w http.ResponseWriter, r *http.Request, u store.User) {
	if m, ok := s.member(w, r, u, false); ok {
		writeJSON(w, http.StatusOK, membershipView(m))
	}
}

func (s *Server) myTenantPeers(w http.ResponseWriter, r *http.Request, u store.User) {
	if _, ok := s.member(w, r, u, false); !ok {
		return
	}
	s.listPeers(w, r)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request, u store.User) {
	if _, ok := s.member(w, r, u, false); !ok {
		return
	}
	members, err := s.store.ListMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not list members", err)
		return
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{
			"user_id":    m.UserID,
			"email":      m.Email,
			"name":       m.Name,
			"role":       m.Role,
			"created_at": m.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, u store.User) {
	target := r.PathValue("user")
	m, ok := s.member(w, r, u, false)
	if !ok {
		return
	}
	if target != u.ID && m.Role != store.RoleOwner {
		s.fail(w, r, http.StatusForbidden, "only owners of this tenant can remove others", nil)
		return
	}

	err := s.store.RemoveMember(r.Context(), m.ID, target)
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, r, http.StatusNotFound, "no such member", nil)
		return
	case errors.Is(err, store.ErrLastOwner):
		s.fail(w, r, http.StatusConflict, "the tenant would be left without an owner: invite another owner first", nil)
		return
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not remove member", err)
		return
	}
	s.log.Info("member removed", "tenant_id", m.ID, "user_id", target, "by", u.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, u store.User) {
	var req struct {
		Role       string `json:"role"`
		TTLSeconds int    `json:"ttl_seconds"`
		MaxUses    int    `json:"max_uses"`
	}
	if !decode(w, r, &req, s) {
		return
	}
	m, ok := s.member(w, r, u, true)
	if !ok {
		return
	}

	role := req.Role
	if role == "" {
		role = store.RoleMember
	}
	if role != store.RoleMember && role != store.RoleOwner {
		s.fail(w, r, http.StatusBadRequest, "role must be member or owner", nil)
		return
	}
	ttl := defaultInviteTTL
	if req.TTLSeconds != 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl <= 0 || ttl > maxInviteTTL {
		s.fail(w, r, http.StatusBadRequest, "ttl_seconds must be between 1 and 2592000", nil)
		return
	}
	maxUses := 1
	if req.MaxUses != 0 {
		maxUses = req.MaxUses
	}
	if maxUses < 1 || maxUses > maxInviteUses {
		s.fail(w, r, http.StatusBadRequest, "max_uses must be between 1 and 100", nil)
		return
	}

	inv, err := s.store.CreateInvite(r.Context(), m.ID, role, u.ID, ttl, maxUses)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not create invite", err)
		return
	}
	s.log.Info("invite created", "tenant_id", m.ID, "invite_id", inv.ID, "role", role, "by", u.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         inv.ID,
		"code":       inv.Code,
		"role":       inv.Role,
		"max_uses":   inv.MaxUses,
		"expires_at": inv.ExpiresAt,
	})
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request, u store.User) {
	var req struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &req, s) {
		return
	}
	m, err := s.store.AcceptInvite(r.Context(), strings.TrimSpace(req.Code), u)
	switch {
	case errors.Is(err, store.ErrInviteInvalid):
		s.fail(w, r, http.StatusNotFound, "this invite is invalid, expired or already used", nil)
		return
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not accept invite", err)
		return
	}
	s.log.Info("invite accepted", "tenant_id", m.ID, "user_id", u.ID, "role", m.Role)
	writeJSON(w, http.StatusOK, membershipView(m))
}

// claimTenant takes ownership of a tenant with no members yet. Knowing its id
// is the proof, as it already is for `meshd join`.
func (s *Server) claimTenant(w http.ResponseWriter, r *http.Request, u store.User) {
	m, err := s.store.ClaimTenant(r.Context(), r.PathValue("id"), u)
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, r, http.StatusNotFound, "no such tenant", nil)
		return
	case errors.Is(err, store.ErrClaimed):
		s.fail(w, r, http.StatusConflict, "this tenant already has members: ask one of its owners for an invite", nil)
		return
	case err != nil:
		s.fail(w, r, http.StatusInternalServerError, "could not claim tenant", err)
		return
	}
	s.log.Info("tenant claimed", "tenant_id", m.ID, "owner", u.ID)
	writeJSON(w, http.StatusOK, membershipView(m))
}
