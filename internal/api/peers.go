package api

import (
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"Mesh2Mesh/internal/store"
)

// peerView renders a peer for the API. endpoint is omitted until the peer
// reports one.
func peerView(p *store.Peer) map[string]any {
	out := map[string]any{
		"peer_id":    p.ID,
		"name":       p.Name,
		"public_key": p.PublicKey,
		"mesh_ip":    p.MeshIP.String(),
		"created_at": p.CreatedAt,
		// True when the last probe was answered: others can send to endpoint
		// directly.
		"direct_reachable": p.DirectReachable,
	}
	if p.Endpoint.IsValid() {
		out["endpoint"] = p.Endpoint.String()
		out["endpoint_updated_at"] = p.EndpointUpdatedAt
	}
	return out
}

// registerPeer redeems an enrollment token and returns the mesh address the
// client should configure.
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
	for i := range peers {
		out = append(out, peerView(&peers[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": out})
}

// updateEndpoint takes the UDP port a node bound and decides whether the
// resulting endpoint is usable for direct connections.
func (s *Server) updateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UDPPort int `json:"udp_port"`
	}
	if !decode(w, r, &req, s) {
		return
	}
	if req.UDPPort < 1 || req.UDPPort > 65535 {
		s.fail(w, r, http.StatusBadRequest, "udp_port must be between 1 and 65535", nil)
		return
	}

	peerID := r.PathValue("id")
	if _, err := s.store.GetPeer(r.Context(), peerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, http.StatusNotFound, "no such peer", err)
			return
		}
		s.fail(w, r, http.StatusInternalServerError, "could not load peer", err)
		return
	}

	publicIP, err := remoteAddr(r)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not read the request source address", err)
		return
	}
	endpoint := netip.AddrPortFrom(publicIP, uint16(req.UDPPort))

	reachable := false
	switch {
	case s.prober == nil:
		s.log.Warn("no prober configured, reporting endpoint as unreachable", "peer_id", peerID)
	default:
		reachable, err = s.prober.Reachable(r.Context(), endpoint)
		if err != nil {
			s.log.Error("could not probe endpoint", "err", err, "peer_id", peerID, "endpoint", endpoint.String())
		}
	}

	peer, err := s.store.SetPeerEndpoint(r.Context(), peerID, endpoint, reachable)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, "could not save endpoint", err)
		return
	}

	s.log.Info("peer endpoint updated",
		"peer_id", peer.ID, "endpoint", endpoint.String(), "direct_reachable", reachable)
	writeJSON(w, http.StatusOK, peerView(peer))
}

func remoteAddr(r *http.Request) (netip.Addr, error) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	return ap.Addr().Unmap(), nil
}
