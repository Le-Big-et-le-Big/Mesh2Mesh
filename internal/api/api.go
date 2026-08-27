package api

import (
	"log/slog"
	"net/http"
	"time"

	"Mesh2Mesh/internal/prober"
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
	// prober tests reported endpoints for unsolicited inbound reachability. A
	// nil prober disables the test and reports every endpoint as unreachable.
	prober *prober.Prober
}

// New builds a Server.
func New(st *store.Store, log *slog.Logger, p *prober.Prober) *Server {
	return &Server{store: st, log: log, prober: p}
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
	mux.HandleFunc("POST /v1/peers/{id}/endpoint", s.updateEndpoint)
	return mux
}
