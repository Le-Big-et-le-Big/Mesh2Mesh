package wgcrypt

import (
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"golang.org/x/crypto/chacha20poly1305"
)

const keyInfo = "Mesh2Mesh v1 static-static ChaCha20-Poly1305"

const (
	// Width of the per-packet counter prefixed to every sealed payload, in little-endian.
	CounterLen = 8

	// Overhead is what sealing adds to a packet the counter plus the Poly1305 tag.
	Overhead = CounterLen + chacha20poly1305.Overhead
)

var ErrCounterExhausted = errors.New("wgcrypt: nonce counter exhausted, the session must be replaced")
var ErrShortPayload = errors.New("wgcrypt: payload shorter than the counter and tag")

// Session seals packets for one peer with one derived key. It is safe for concurrent use the counter is atomic, so each Seal claims a distinct nonce.
type Session struct {
	aead    cipher.AEAD
	counter atomic.Uint64
}

func NewSession(privateKey, peerPublicKey string) (*Session, error) {
	priv, err := decodeKey(privateKey, "private key")
	if err != nil {
		return nil, err
	}
	pub, err := decodeKey(peerPublicKey, "peer public key")
	if err != nil {
		return nil, err
	}

	key, err := DeriveKey(priv, pub)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: build AEAD: %w", err)
	}
	return &Session{aead: aead}, nil
}

// DeriveKey computes the ChaCha20-Poly1305 key both ends share
func DeriveKey(privateKey, peerPublicKey []byte) ([]byte, error) {
	curve := ecdh.X25519()

	priv, err := curve.NewPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: invalid private key: %w", err)
	}
	pub, err := curve.NewPublicKey(peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: invalid peer public key: %w", err)
	}

	shared, err := priv.ECDH(pub)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: key agreement failed: %w", err)
	}

	key, err := hkdf.Key(sha256.New, shared, nil, keyInfo, chacha20poly1305.KeySize)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: derive key: %w", err)
	}
	return key, nil
}

// Seal encrypts one packet into the counter, ciphertext and tag, appended to
// dst; pass a reusable buffer so the forwarding path allocates nothing per
// packet but the nonce, which escapes through the cipher.AEAD interface.
//
// plaintext is never read after this returns, so it may alias a read buffer.
func (s *Session) Seal(dst, plaintext []byte) ([]byte, error) {
	// A plain Add would wrap at the limit and reuse nonces, which leaks plaintext
	// with a stream cipher; the CAS loop refuses to wrap.
	var counter uint64
	for {
		counter = s.counter.Load()
		if counter == ^uint64(0) {
			return nil, ErrCounterExhausted
		}
		if s.counter.CompareAndSwap(counter, counter+1) {
			break
		}
	}

	dst = binary.LittleEndian.AppendUint64(dst[:0], counter)
	return s.aead.Seal(dst, nonce(counter), plaintext, nil), nil
}

// Open reverses Seal, returning the packet appended to dst.
func (s *Session) Open(dst, payload []byte) ([]byte, error) {
	if len(payload) < Overhead {
		return nil, ErrShortPayload
	}

	counter := binary.LittleEndian.Uint64(payload[:CounterLen])
	out, err := s.aead.Open(dst[:0], nonce(counter), payload[CounterLen:], nil)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: authentication failed for counter %d: %w", counter, err)
	}
	return out, nil
}

// nonce builds the 12-byte ChaACha20-Poly1305 nonce WireGuard uses
func nonce(counter uint64) []byte {
	var n [chacha20poly1305.NonceSize]byte
	binary.LittleEndian.PutUint64(n[4:], counter)
	return n[:]
}

func decodeKey(encoded, what string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("wgcrypt: %s is not valid base64: %w", what, err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("wgcrypt: %s is %d bytes, want 32", what, len(raw))
	}
	return raw, nil
}
