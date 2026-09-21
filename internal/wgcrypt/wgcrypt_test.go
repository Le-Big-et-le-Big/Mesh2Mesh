package wgcrypt_test

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"Mesh2Mesh/internal/wgcrypt"
)

// keypair mints one X25519 pair encoded the way the node state file stores it.
func keypair(t *testing.T) (private, public string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()),
		base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
}

// sessions builds the two halves of one static-static agreement: the node
// sealing towards the server, and the server opening what it receives.
func sessions(t *testing.T) (node, server *wgcrypt.Session) {
	t.Helper()
	nodePriv, nodePub := keypair(t)
	serverPriv, serverPub := keypair(t)

	node, err := wgcrypt.NewSession(nodePriv, serverPub)
	if err != nil {
		t.Fatalf("node session: %v", err)
	}
	server, err = wgcrypt.NewSession(serverPriv, nodePub)
	if err != nil {
		t.Fatalf("server session: %v", err)
	}
	return node, server
}

// The whole point of a static-static agreement: both ends reach the same key
// from opposite halves of the two keypairs, with nothing sent between them.
func TestSealOpenRoundTrip(t *testing.T) {
	node, server := sessions(t)

	packet := []byte("\x45\x00\x00\x1c inner IP packet")
	sealed, err := node.Seal(nil, packet)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if bytes.Contains(sealed, packet) {
		t.Fatal("sealed frame still contains the plaintext")
	}
	if want := len(packet) + wgcrypt.Overhead; len(sealed) != want {
		t.Errorf("sealed length = %d, want %d", len(sealed), want)
	}

	opened, err := server.Open(nil, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(opened, packet) {
		t.Errorf("opened = %q, want %q", opened, packet)
	}
}

// A stream cipher that reuses a nonce leaks plaintext, so the counter must
// advance on every packet even when the payloads are identical.
func TestSealUsesAFreshCounter(t *testing.T) {
	node, server := sessions(t)

	packet := []byte("the same packet twice")
	first, err := node.Seal(nil, packet)
	if err != nil {
		t.Fatalf("seal first: %v", err)
	}
	first = bytes.Clone(first)

	second, err := node.Seal(nil, packet)
	if err != nil {
		t.Fatalf("seal second: %v", err)
	}

	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same packet produced identical frames")
	}
	if bytes.Equal(first[:wgcrypt.CounterLen], second[:wgcrypt.CounterLen]) {
		t.Fatal("counter did not advance between packets")
	}

	// Out-of-order delivery is normal on UDP, so the counter travels with the
	// packet and the second frame must open without the first.
	if _, err := server.Open(nil, second); err != nil {
		t.Fatalf("open second before first: %v", err)
	}
	if _, err := server.Open(nil, first); err != nil {
		t.Fatalf("open first after second: %v", err)
	}
}

// Poly1305 is what stops a node from accepting an injected or edited frame --
// the gap that wire v0 left open.
func TestOpenRejectsTamperedFrames(t *testing.T) {
	node, server := sessions(t)

	sealed, err := node.Seal(nil, []byte("authentic packet"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	for _, tc := range []struct {
		name  string
		index int
	}{
		{"counter", 0},
		{"ciphertext", wgcrypt.CounterLen},
		{"tag", len(sealed) - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tampered := bytes.Clone(sealed)
			tampered[tc.index] ^= 0xff
			if _, err := server.Open(nil, tampered); err == nil {
				t.Error("Open accepted a tampered frame")
			}
		})
	}
}

// A frame from a session we share no key with must not open, or the tunnel
// would accept traffic from anyone.
func TestOpenRejectsAStranger(t *testing.T) {
	_, server := sessions(t)
	strangerPriv, _ := keypair(t)
	_, serverPub := keypair(t)

	stranger, err := wgcrypt.NewSession(strangerPriv, serverPub)
	if err != nil {
		t.Fatalf("stranger session: %v", err)
	}
	sealed, err := stranger.Seal(nil, []byte("packet from elsewhere"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if _, err := server.Open(nil, sealed); err == nil {
		t.Error("Open accepted a frame sealed with an unrelated key")
	}
}

func TestOpenRejectsShortPayload(t *testing.T) {
	_, server := sessions(t)

	short := make([]byte, wgcrypt.Overhead-1)
	if _, err := server.Open(nil, short); !errors.Is(err, wgcrypt.ErrShortPayload) {
		t.Errorf("Open(short) error = %v, want ErrShortPayload", err)
	}
}

// Seal runs once for every packet leaving the interface, so it must append to
// the caller's buffer rather than allocate a fresh one. The nonce is the single
// allowed allocation: it escapes through the cipher.AEAD interface call.
func TestSealReusesTheBuffer(t *testing.T) {
	node, _ := sessions(t)

	buf := make([]byte, 0, 2048)
	packet := make([]byte, 1400)

	sealed, err := node.Seal(buf, packet)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if &sealed[:1][0] != &buf[:1][0] {
		t.Error("Seal allocated a new buffer instead of appending to dst")
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := node.Seal(buf, packet); err != nil {
			t.Fatalf("seal: %v", err)
		}
	})
	if allocs > 1 {
		t.Errorf("Seal allocated %v times per call, want at most 1 (the nonce)", allocs)
	}
}

func TestNewSessionRejectsMalformedKeys(t *testing.T) {
	good, goodPub := keypair(t)

	for _, tc := range []struct {
		name       string
		priv, peer string
	}{
		{"private not base64", "not!base64", goodPub},
		{"peer not base64", good, "not!base64"},
		{"private wrong length", base64.StdEncoding.EncodeToString([]byte("short")), goodPub},
		{"peer wrong length", good, base64.StdEncoding.EncodeToString([]byte("short"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := wgcrypt.NewSession(tc.priv, tc.peer); err == nil {
				t.Error("NewSession accepted a malformed key")
			}
		})
	}
}
