package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"

	"Mesh2Mesh/internal/prober"
	"Mesh2Mesh/internal/relay"
	"Mesh2Mesh/internal/wire"
)

func TestMeshBindSharesSocketWithProbes(t *testing.T) {
	b := &meshBind{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	fns, port, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Close() }()
	bindAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(port)}

	// A probe, then a WireGuard handshake initiation.
	nonce := bytes.Repeat([]byte{7}, wire.NonceLen)
	if _, err := peer.WriteToUDP(wire.Encode(nil, wire.TypeProbe, nonce), bindAddr); err != nil {
		t.Fatal(err)
	}
	wgMsg := []byte{1, 0, 0, 0, 0xaa, 0xbb}
	if _, err := peer.WriteToUDP(wgMsg, bindAddr); err != nil {
		t.Fatal(err)
	}

	packets := [][]byte{make([]byte, 1500)}
	sizes := make([]int, 1)
	eps := make([]conn.Endpoint, 1)
	n, err := fns[0](packets, sizes, eps)
	if err != nil || n != 1 {
		t.Fatalf("receive = %d, %v", n, err)
	}
	if got := packets[0][:sizes[0]]; !bytes.Equal(got, wgMsg) {
		t.Errorf("wireguard got %x, want %x (the probe must not reach it)", got, wgMsg)
	}
	if want := peer.LocalAddr().(*net.UDPAddr).AddrPort(); eps[0].DstToString() != want.String() {
		t.Errorf("endpoint = %s, want %s", eps[0].DstToString(), want)
	}

	_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	rn, err := peer.Read(buf)
	if err != nil {
		t.Fatalf("no probe reply: %v", err)
	}
	typ, payload, err := wire.Decode(buf[:rn])
	if err != nil || typ != wire.TypeProbeReply || !bytes.Equal(payload, nonce) {
		t.Errorf("probe reply = type %d payload %x err %v", typ, payload, err)
	}

	// Sending goes out of the same socket.
	ep, err := b.ParseEndpoint(peer.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Send([][]byte{wgMsg}, ep); err != nil {
		t.Fatal(err)
	}
	rn, from, err := peer.ReadFromUDPAddrPort(buf)
	if err != nil || !bytes.Equal(buf[:rn], wgMsg) || from.Port() != port {
		t.Errorf("send: got %x from %s, err %v", buf[:rn], from, err)
	}

	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fns[0](packets, sizes, eps); err == nil {
		t.Error("receive after Close returned no error")
	}
}

func TestHexKey(t *testing.T) {
	priv, pub, err := newKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{priv, pub} {
		h, err := hexKey(k)
		if err != nil || len(h) != 64 {
			t.Errorf("hexKey(%q) = %q, %v", k, h, err)
		}
	}
	for _, bad := range []string{"", "not base64!", "AAAA"} {
		if _, err := hexKey(bad); err == nil {
			t.Errorf("hexKey(%q) accepted a bad key", bad)
		}
	}
}

func TestMeshBindDiscoversItsMappingFromTheProber(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p, err := prober.Open("127.0.0.1:0", log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	server := p.LocalAddr().(*net.UDPAddr).AddrPort()

	b := &meshBind{log: log}
	fns, port, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	go func() {
		packets := [][]byte{make([]byte, 1500)}
		for {
			if _, err := fns[0](packets, make([]int, 1), make([]conn.Endpoint, 1)); err != nil {
				return
			}
		}
	}()

	mapped, err := b.discoverMapping(context.Background(), server)
	if err != nil {
		t.Fatalf("discoverMapping: %v", err)
	}
	if want := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port); mapped != want {
		t.Errorf("mapped = %s, want %s", mapped, want)
	}
}

func TestMeshBindRelaysThroughTheControlPlane(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p, err := prober.Open("127.0.0.1:0", log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	r := relay.New(log)
	p.SetRelay(r)
	server := p.LocalAddr().(*net.UDPAddr).AddrPort()

	open := func(meshIP string) (*meshBind, chan conn.Endpoint, chan []byte) {
		b := &meshBind{log: log}
		fns, _, err := b.Open(0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		eps, msgs := make(chan conn.Endpoint, 1), make(chan []byte, 1)
		go func() {
			packets, sizes, ep := [][]byte{make([]byte, 1500)}, make([]int, 1), make([]conn.Endpoint, 1)
			for {
				if _, err := fns[0](packets, sizes, ep); err != nil {
					return
				}
				msgs <- bytes.Clone(packets[0][:sizes[0]])
				eps <- ep[0]
			}
		}()

		token, err := r.Issue(meshIP, "tenant", netip.MustParseAddr(meshIP))
		if err != nil {
			t.Fatal(err)
		}
		b.useRelay(server, token)
		if err := b.registerRelay(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for !b.relayUp(time.Now()) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: the relay never acknowledged the registration", meshIP)
			}
			time.Sleep(10 * time.Millisecond)
		}
		return b, eps, msgs
	}
	a, _, _ := open("10.203.0.2")
	_, bEps, bMsgs := open("10.203.0.3")

	ep, err := a.ParseEndpoint("relay:10.203.0.3")
	if err != nil {
		t.Fatal(err)
	}
	wgMsg := []byte{1, 0, 0, 0, 0xaa, 0xbb}
	if err := a.Send([][]byte{wgMsg}, ep); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-bMsgs:
		if !bytes.Equal(got, wgMsg) {
			t.Errorf("b got %x, want %x", got, wgMsg)
		}
		if from := (<-bEps).DstToString(); from != "relay:10.203.0.2" {
			t.Errorf("endpoint = %s, want relay:10.203.0.2", from)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the relayed datagram never arrived")
	}
}

func TestWgPeerTarget(t *testing.T) {
	ip := netip.MustParseAddr("10.203.0.7")
	ep := netip.MustParseAddrPort("198.51.100.4:51820")
	for _, tc := range []struct {
		p    wgPeer
		want string
	}{
		{wgPeer{meshIP: ip}, ""},
		{wgPeer{meshIP: ip, endpoint: ep}, ep.String()},
		{wgPeer{meshIP: ip, endpoint: ep, relayed: true}, "relay:10.203.0.7"},
	} {
		if got := tc.p.target(); got != tc.want {
			t.Errorf("%+v target = %q, want %q", tc.p, got, tc.want)
		}
	}
}
