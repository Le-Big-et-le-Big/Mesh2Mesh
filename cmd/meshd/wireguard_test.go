package main

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"

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
