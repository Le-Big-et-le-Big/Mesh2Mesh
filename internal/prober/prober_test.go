package prober

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"Mesh2Mesh/internal/wire"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// openTestProber returns a prober with a budget short enough that a negative
// verdict does not slow the suite down.
func openTestProber(t *testing.T) *Prober {
	t.Helper()
	p, err := Open("127.0.0.1:0", testLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.budget = 300 * time.Millisecond
	return p
}

func localAddrPort(t *testing.T, conn *net.UDPConn) netip.AddrPort {
	t.Helper()
	a := conn.LocalAddr().(*net.UDPAddr)
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), uint16(a.Port))
}

// listenUDP opens a loopback socket standing in for a peer's tunnel port.
func listenUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestReachableWhenTheEndpointEchoes(t *testing.T) {
	node := listenUDP(t)
	go func() {
		buf := make([]byte, wire.MaxFrameLen)
		for {
			n, src, err := node.ReadFromUDP(buf)
			if err != nil {
				return
			}
			typ, payload, err := wire.Decode(buf[:n])
			if err != nil || typ != wire.TypeProbe {
				continue
			}
			_, _ = node.WriteToUDP(wire.Encode(nil, wire.TypeProbeReply, payload), src)
		}
	}()

	reachable, err := openTestProber(t).Reachable(context.Background(), localAddrPort(t, node))
	if err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if !reachable {
		t.Error("an endpoint that echoed the probe was reported unreachable")
	}
}

func TestUnreachableWhenNothingAnswers(t *testing.T) {
	// A socket that reads and drops: the "closed port behind a NAT" case, where
	// the probe simply goes nowhere.
	silent := listenUDP(t)

	reachable, err := openTestProber(t).Reachable(context.Background(), localAddrPort(t, silent))
	if err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if reachable {
		t.Error("a silent endpoint was reported reachable")
	}
}

func TestReplyFromAnotherAddressDoesNotCount(t *testing.T) {
	node := listenUDP(t)
	other := listenUDP(t)
	go func() {
		buf := make([]byte, wire.MaxFrameLen)
		for {
			n, src, err := node.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, payload, err := wire.Decode(buf[:n])
			if err != nil {
				continue
			}
			// Right nonce, wrong socket: this says nothing about whether the
			// endpoint we probed accepts traffic.
			_, _ = other.WriteToUDP(wire.Encode(nil, wire.TypeProbeReply, payload), src)
		}
	}()

	reachable, err := openTestProber(t).Reachable(context.Background(), localAddrPort(t, node))
	if err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if reachable {
		t.Error("a reply from a different address was accepted as proof of reachability")
	}
}

func TestReachableRejectsAnInvalidTarget(t *testing.T) {
	if _, err := openTestProber(t).Reachable(context.Background(), netip.AddrPort{}); err == nil {
		t.Error("Reachable accepted a zero endpoint")
	}
}

func TestReachableHonoursContextCancellation(t *testing.T) {
	silent := listenUDP(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reachable, err := openTestProber(t).Reachable(ctx, localAddrPort(t, silent))
	if err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if reachable {
		t.Error("a cancelled probe reported the endpoint reachable")
	}
}
