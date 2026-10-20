package api

import (
	"log/slog"
	"net/http"
	"time"

	"Mesh2Mesh/internal/prober"
	"Mesh2Mesh/internal/relay"
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
	// A nil prober disables the reachability test: every endpoint reports
	// unreachable.
	prober *prober.Prober
	// nil disables relaying.
	relay *relay.Relay
	// empty disables the /v1/me routes.
	webToken string
}

func New(st *store.Store, log *slog.Logger, p *prober.Prober, r *relay.Relay, webToken string) *Server {
	return &Server{store: st, log: log, prober: p, relay: r, webToken: webToken}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/tenants", s.createTenant)
	mux.HandleFunc("GET /v1/tenants", s.webOnly(s.listTenants))
	mux.HandleFunc("GET /v1/tenants/{id}", s.webOnly(s.getTenant))
	mux.HandleFunc("POST /v1/tenants/{id}/tokens", s.createToken)
	mux.HandleFunc("GET /v1/tenants/{id}/peers", s.listPeers)
	mux.HandleFunc("POST /v1/peers/register", s.registerPeer)
	mux.HandleFunc("POST /v1/peers/{id}/endpoint", s.updateEndpoint)
	mux.HandleFunc("POST /v1/peers/{id}/nat-endpoint", s.updateNATEndpoint)
	mux.HandleFunc("POST /v1/peers/{id}/relay", s.issueRelayToken)

	mux.HandleFunc("GET /v1/me/tenants", s.asUser(s.myTenants))
	mux.HandleFunc("POST /v1/me/tenants", s.asUser(s.createMyTenant))
	mux.HandleFunc("GET /v1/me/tenants/{id}", s.asUser(s.getMyTenant))
	mux.HandleFunc("GET /v1/me/tenants/{id}/peers", s.asUser(s.myTenantPeers))
	mux.HandleFunc("GET /v1/me/tenants/{id}/members", s.asUser(s.listMembers))
	mux.HandleFunc("DELETE /v1/me/tenants/{id}/members/{user}", s.asUser(s.removeMember))
	mux.HandleFunc("POST /v1/me/tenants/{id}/invites", s.asUser(s.createInvite))
	mux.HandleFunc("POST /v1/me/tenants/{id}/claim", s.asUser(s.claimTenant))
	mux.HandleFunc("POST /v1/me/invites/accept", s.asUser(s.acceptInvite))
	return mux
}
