// Package relay forwards WireGuard ciphertext between peers of a tenant that
// cannot reach each other directly or by hole punching.
package relay

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"

	"Mesh2Mesh/internal/wire"
)

const SessionTTL = 90 * time.Second

type session struct {
	peerID   string
	tenantID string
	meshIP   netip.Addr
	token    wire.RelayToken

	addr     netip.AddrPort
	lastSeen time.Time
}

// Mesh addresses are only unique within a tenant.
type meshKey struct {
	tenantID string
	meshIP   netip.Addr
}

// Relay has no socket of its own: the prober hands it relay frames.
type Relay struct {
	log *slog.Logger
	now func() time.Time

	mu      sync.Mutex
	byPeer  map[string]*session
	byToken map[wire.RelayToken]*session
	byAddr  map[netip.AddrPort]*session
	byMesh  map[meshKey]*session
}

func New(log *slog.Logger) *Relay {
	return &Relay{
		log:     log,
		now:     time.Now,
		byPeer:  make(map[string]*session),
		byToken: make(map[wire.RelayToken]*session),
		byAddr:  make(map[netip.AddrPort]*session),
		byMesh:  make(map[meshKey]*session),
	}
}

// Issue replaces any earlier registration of peerID.
func (r *Relay) Issue(peerID, tenantID string, meshIP netip.Addr) (wire.RelayToken, error) {
	if !meshIP.Is4() {
		return wire.RelayToken{}, fmt.Errorf("relay: mesh address %s is not IPv4", meshIP)
	}
	var token wire.RelayToken
	if _, err := rand.Read(token[:]); err != nil {
		return wire.RelayToken{}, fmt.Errorf("relay: generate token: %w", err)
	}

	s := &session{peerID: peerID, tenantID: tenantID, meshIP: meshIP, token: token}

	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.byPeer[peerID]; ok {
		r.forget(old)
	}
	r.byPeer[peerID] = s
	r.byToken[token] = s
	r.byMesh[meshKey{tenantID, meshIP}] = s
	return token, nil
}

// r.mu must be held.
func (r *Relay) forget(s *session) {
	delete(r.byToken, s.token)
	if r.byPeer[s.peerID] == s {
		delete(r.byPeer, s.peerID)
	}
	if r.byAddr[s.addr] == s {
		delete(r.byAddr, s.addr)
	}
	if k := (meshKey{s.tenantID, s.meshIP}); r.byMesh[k] == s {
		delete(r.byMesh, k)
	}
}

// r.mu must be held.
func (r *Relay) live(s *session, now time.Time) bool {
	return s != nil && s.addr.IsValid() && now.Sub(s.lastSeen) < SessionTTL
}

// HandleFrame may rewrite frame in place.
func (r *Relay) HandleFrame(c *net.UDPConn, frame []byte, src netip.AddrPort) {
	typ, payload, err := wire.Decode(frame)
	if err != nil {
		return
	}
	switch typ {
	case wire.TypeRelayRegister:
		r.register(c, payload, src)
	case wire.TypeRelayData:
		r.forward(c, frame, src)
	default:
		r.log.Debug("relay: ignoring unexpected frame", "src", src.String(), "type", typ)
	}
}

func (r *Relay) register(c *net.UDPConn, payload []byte, src netip.AddrPort) {
	if len(payload) < wire.RelayTokenLen {
		return
	}
	token := wire.RelayToken(payload[:wire.RelayTokenLen])

	r.mu.Lock()
	s, ok := r.byToken[token]
	if !ok {
		r.mu.Unlock()
		r.log.Debug("relay: registration with an unknown token", "src", src.String())
		return
	}
	moved := s.addr != src
	if moved {
		if r.byAddr[s.addr] == s {
			delete(r.byAddr, s.addr)
		}
		if prev, ok := r.byAddr[src]; ok {
			prev.addr = netip.AddrPort{}
		}
		s.addr = src
		r.byAddr[src] = s
	}
	s.lastSeen = r.now()
	peerID := s.peerID
	r.mu.Unlock()

	if moved {
		r.log.Info("relay: peer registered", "peer_id", peerID, "addr", src.String())
	}
	ack := wire.Encode(nil, wire.TypeRelayAck, token[:])
	if _, err := c.WriteToUDPAddrPort(ack, src); err != nil {
		r.log.Warn("relay: could not acknowledge registration", "src", src.String(), "err", err)
	}
}

func (r *Relay) forward(c *net.UDPConn, frame []byte, src netip.AddrPort) {
	if len(frame) <= wire.RelayHeaderLen {
		return
	}
	ipField := frame[wire.HeaderLen:wire.RelayHeaderLen]
	dstIP := netip.AddrFrom4([4]byte(ipField))

	now := r.now()
	r.mu.Lock()
	from := r.byAddr[src]
	if !r.live(from, now) {
		r.mu.Unlock()
		r.log.Debug("relay: data from an unregistered source", "src", src.String())
		return
	}
	from.lastSeen = now
	// Looked up in the sender's tenant only.
	to := r.byMesh[meshKey{from.tenantID, dstIP}]
	if to == from || !r.live(to, now) {
		r.mu.Unlock()
		r.log.Debug("relay: no live registration for destination", "tenant_id", from.tenantID, "dst", dstIP.String())
		return
	}
	srcIP, dst := from.meshIP.As4(), to.addr
	r.mu.Unlock()

	copy(ipField, srcIP[:])
	if _, err := c.WriteToUDPAddrPort(frame, dst); err != nil {
		r.log.Debug("relay: forward failed", "dst", dst.String(), "err", err)
	}
}
