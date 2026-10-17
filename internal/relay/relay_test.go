package relay

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"Mesh2Mesh/internal/wire"
)

func server(t *testing.T) (*Relay, netip.AddrPort) {
	t.Helper()
	r := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	go func() {
		buf := make([]byte, wire.MaxFrameLen)
		for {
			n, src, err := c.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			r.HandleFrame(c, buf[:n], src)
		}
	}()
	return r, c.LocalAddr().(*net.UDPAddr).AddrPort()
}

type node struct {
	c *net.UDPConn
}

func register(t *testing.T, r *Relay, relay netip.AddrPort, peerID, tenantID, meshIP string) node {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	token, err := r.Issue(peerID, tenantID, netip.MustParseAddr(meshIP))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.WriteToUDPAddrPort(wire.Encode(nil, wire.TypeRelayRegister, token[:]), relay); err != nil {
		t.Fatal(err)
	}
	typ, payload, ok := read(t, c)
	if !ok || typ != wire.TypeRelayAck || !bytes.Equal(payload, token[:]) {
		t.Fatalf("registration of %s: got type %d payload %x, want an ack", peerID, typ, payload)
	}
	return node{c: c}
}

func read(t *testing.T, c *net.UDPConn) (uint8, []byte, bool) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, wire.MaxFrameLen)
	n, err := c.Read(buf)
	if err != nil {
		return 0, nil, false
	}
	typ, payload, err := wire.Decode(buf[:n])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return typ, payload, true
}

func (n node) send(t *testing.T, relay netip.AddrPort, dst string, msg []byte) {
	t.Helper()
	ip := netip.MustParseAddr(dst).As4()
	frame := append(wire.Encode(nil, wire.TypeRelayData, ip[:]), msg...)
	if _, err := n.c.WriteToUDPAddrPort(frame, relay); err != nil {
		t.Fatal(err)
	}
}

func TestRelayForwardsWithinATenant(t *testing.T) {
	r, relay := server(t)
	a := register(t, r, relay, "a", "t1", "10.203.0.2")
	b := register(t, r, relay, "b", "t1", "10.203.0.3")

	msg := []byte{4, 0, 0, 0, 0xca, 0xfe}
	a.send(t, relay, "10.203.0.3", msg)

	typ, payload, ok := read(t, b.c)
	if !ok || typ != wire.TypeRelayData {
		t.Fatalf("b got type %d, ok %v, want relayed data", typ, ok)
	}
	if src := netip.AddrFrom4([4]byte(payload[:4])); src != netip.MustParseAddr("10.203.0.2") {
		t.Errorf("source = %s, want a's mesh address", src)
	}
	if !bytes.Equal(payload[4:], msg) {
		t.Errorf("payload = %x, want %x", payload[4:], msg)
	}
}

func TestRelayNeverCrossesTenants(t *testing.T) {
	r, relay := server(t)
	a := register(t, r, relay, "a", "t1", "10.203.0.2")
	// Same mesh address as a's target, other tenant.
	other := register(t, r, relay, "x", "t2", "10.203.0.3")

	a.send(t, relay, "10.203.0.3", []byte{4, 1, 2, 3})
	if _, _, ok := read(t, other.c); ok {
		t.Error("a peer received traffic from another tenant")
	}
}

func TestRelayDropsUnregisteredSources(t *testing.T) {
	r, relay := server(t)
	b := register(t, r, relay, "b", "t1", "10.203.0.3")

	stranger, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stranger.Close() }()
	node{c: stranger}.send(t, relay, "10.203.0.3", []byte{4, 1, 2, 3})

	if _, _, ok := read(t, b.c); ok {
		t.Error("the relay forwarded for a socket that never registered")
	}
}

func TestRelayIgnoresUnknownAndReplacedTokens(t *testing.T) {
	r, relay := server(t)
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	old, err := r.Issue("a", "t1", netip.MustParseAddr("10.203.0.2"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Issue("a", "t1", netip.MustParseAddr("10.203.0.2")); err != nil {
		t.Fatal(err)
	}
	var forged wire.RelayToken
	for _, tok := range []wire.RelayToken{old, forged} {
		if _, err := c.WriteToUDPAddrPort(wire.Encode(nil, wire.TypeRelayRegister, tok[:]), relay); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := read(t, c); ok {
			t.Errorf("token %x was acknowledged", tok)
		}
	}
}

func TestRelayForgetsSilentPeers(t *testing.T) {
	r, relay := server(t)
	now := time.Now()
	r.mu.Lock()
	r.now = func() time.Time { return now }
	r.mu.Unlock()

	a := register(t, r, relay, "a", "t1", "10.203.0.2")
	b := register(t, r, relay, "b", "t1", "10.203.0.3")

	r.mu.Lock()
	r.byPeer["b"].lastSeen = now.Add(-SessionTTL)
	r.mu.Unlock()

	a.send(t, relay, "10.203.0.3", []byte{4, 1, 2, 3})
	if _, _, ok := read(t, b.c); ok {
		t.Error("the relay forwarded to a registration past its TTL")
	}
}
