package api

import (
	"log/slog"
	"net/http"
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
