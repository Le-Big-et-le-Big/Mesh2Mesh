package api

import (
	"Mesh2Mesh/internal/store"
	"errors"
	"net/http"
	"strings"
	"time"
)

type tenantResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CIDR      string    `json:"cidr"`
	CreatedAt time.Time `json:"created_at"`
}

func tenantView(t *store.Tenant) tenantResponse {
	return tenantResponse{ID: t.ID, Name: t.Name, CIDR: t.CIDR.String(), CreatedAt: t.CreatedAt}
}

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
