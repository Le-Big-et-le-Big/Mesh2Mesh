package api

import (
	"net/http"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		s.fail(w, r, http.StatusServiceUnavailable, "database unreachable", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
