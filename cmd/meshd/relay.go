package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"

	"Mesh2Mesh/internal/wire"
)

const (
	relayEndpointPrefix = "relay:"

	relayAckTimeout = 3 * peerRefreshInterval
	punchBudget     = 20 * time.Second
	// WireGuard rekeys every 2 minutes and drops keys after 3.
	handshakeStale = 3 * time.Minute
)

// relayEndpoint is a peer reached through the relay. WireGuard roams to and
// from it like any other endpoint.
type relayEndpoint struct {
	meshIP netip.Addr
}

var _ conn.Endpoint = (*relayEndpoint)(nil)

func (e *relayEndpoint) ClearSrc()           {}
func (e *relayEndpoint) SrcToString() string { return "" }
func (e *relayEndpoint) DstToString() string { return relayEndpointPrefix + e.meshIP.String() }
func (e *relayEndpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (e *relayEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }

func (e *relayEndpoint) DstToBytes() []byte {
	ip := e.meshIP.As4()
	return append([]byte(relayEndpointPrefix), ip[:]...)
}

func (b *meshBind) sendRelayed(c *net.UDPConn, bufs [][]byte, dst netip.Addr) error {
	b.mu.Lock()
	relay := b.relay
	b.mu.Unlock()
	if !relay.IsValid() {
		return fmt.Errorf("no relay to reach %s through", dst)
	}

	ip := dst.As4()
	for _, buf := range bufs {
		frame := make([]byte, 0, wire.RelayHeaderLen+len(buf))
		frame = append(wire.Encode(frame, wire.TypeRelayData, ip[:]), buf...)
		if _, err := c.WriteToUDPAddrPort(frame, relay); err != nil {
			return err
		}
	}
	return nil
}

func (b *meshBind) unwrapRelayed(frame []byte, from netip.AddrPort) (int, conn.Endpoint, bool) {
	if len(frame) <= wire.RelayHeaderLen || frame[1] != wire.TypeRelayData {
		return 0, nil, false
	}
	b.mu.Lock()
	relay := b.relay
	b.mu.Unlock()
	if from != relay {
		return 0, nil, false
	}

	src := netip.AddrFrom4([4]byte(frame[wire.HeaderLen:wire.RelayHeaderLen]))
	n := copy(frame, frame[wire.RelayHeaderLen:])
	return n, &relayEndpoint{meshIP: src}, true
}

func (b *meshBind) useRelay(relay netip.AddrPort, token wire.RelayToken) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.relay, b.relayToken, b.relayAcked = relay, token, time.Time{}
}

func (b *meshBind) registerRelay() error {
	b.mu.Lock()
	c, relay, token := b.conn, b.relay, b.relayToken
	b.mu.Unlock()
	if c == nil {
		return net.ErrClosed
	}
	if !relay.IsValid() {
		return nil
	}
	_, err := c.WriteToUDPAddrPort(wire.Encode(nil, wire.TypeRelayRegister, token[:]), relay)
	return err
}

func (b *meshBind) ackRelay(payload []byte, from netip.AddrPort) {
	if len(payload) < wire.RelayTokenLen {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if from == b.relay && wire.RelayToken(payload) == b.relayToken {
		b.relayAcked = time.Now()
	}
}

func (b *meshBind) relayUp(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.relayAcked.IsZero() && now.Sub(b.relayAcked) < relayAckTimeout
}

// refreshRelay asks for a new token when the relay stopped acknowledging
// ours, as after a control plane restart.
func (d *daemon) refreshRelay(ctx context.Context) {
	now := time.Now()
	if !d.bind.relayUp(now) && now.Sub(d.relayIssued) > relayAckTimeout {
		if err := d.issueRelayToken(ctx); err != nil {
			d.log.Warn("could not get a relay token, peers we cannot reach directly stay unreachable", "err", err)
			return
		}
		d.relayIssued = now
	}
	if err := d.bind.registerRelay(); err != nil {
		d.log.Warn("could not register with the relay", "err", err)
	}
}

func (d *daemon) issueRelayToken(ctx context.Context) error {
	grant, err := d.api.IssueRelayToken(ctx, d.state.PeerID)
	if err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(grant.Token)
	if err != nil || len(raw) != wire.RelayTokenLen {
		return fmt.Errorf("the control plane sent an invalid relay token")
	}
	server, err := net.ResolveUDPAddr("udp", net.JoinHostPort(d.api.Host(), strconv.Itoa(grant.Port)))
	if err != nil {
		return fmt.Errorf("resolve the relay: %w", err)
	}
	ap := server.AddrPort()
	relay := netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())

	d.bind.useRelay(relay, wire.RelayToken(raw))
	d.log.Debug("relay token issued", "relay", relay)
	return nil
}

func (d *daemon) handshakes() map[string]time.Time {
	out := make(map[string]time.Time)
	dump, err := d.wg.IpcGet()
	if err != nil {
		d.log.Warn("could not read wireguard state", "err", err)
		return out
	}

	var key string
	sc := bufio.NewScanner(strings.NewReader(dump))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), "=")
		switch k {
		case "public_key":
			key = v
		case "last_handshake_time_sec":
			if secs, err := strconv.ParseInt(v, 10, 64); err == nil && secs > 0 && key != "" {
				out[key] = time.Unix(secs, 0)
			}
		}
	}
	return out
}
