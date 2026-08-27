package api

import (
	"Mesh2Mesh/internal/store"
	"errors"
	"net/http"
	"time"
)

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
