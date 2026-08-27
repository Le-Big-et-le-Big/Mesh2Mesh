// Package wire is the Mesh2Mesh underlay protocol: the framing every datagram
// exchanged over UDP uses, whether it carries a tunnelled IP packet or a
// reachability probe.
//
// SECURITY: v0 frames are not encrypted or authenticated. Payloads travel in
// cleartext and anyone who can reach a node's UDP port can inject a frame. The
// transport is deliberately kept simple until the Noise handshake lands; do not
// run this across an untrusted network before then.
package wire

import (
	"crypto/rand"
	"errors"
	"fmt"
)

// Version is the protocol version carried in byte 0. Receivers drop frames
// that do not match, so a future version can change everything after it.
const Version = 0

// Frame types.
const (
	// TypeData carries one tunnelled IP packet as its payload.
	TypeData uint8 = 1
	// TypeProbe asks the receiver to echo the frame back as a TypeProbeReply.
	// The control plane sends these to test whether a peer's UDP port accepts
	// unsolicited inbound traffic.
	TypeProbe uint8 = 2
	// TypeProbeReply answers a TypeProbe, repeating its nonce unchanged.
	TypeProbeReply uint8 = 3
)

// HeaderLen is the fixed prefix on every frame: version, type, 2 reserved
// bytes that keep the payload 4-byte aligned and leave room to grow.
const HeaderLen = 4

// NonceLen is the payload length of probe and probe-reply frames.
const NonceLen = 16

// MaxFrameLen caps what a receiver will accept, and sizes read buffers. It
// leaves room for a full-MTU inner packet plus the header.
const MaxFrameLen = 2048

// ErrShortFrame is returned when a datagram is too small to hold a header.
var ErrShortFrame = errors.New("wire: frame shorter than header")

// ErrVersion is returned for a frame from an incompatible protocol version.
var ErrVersion = errors.New("wire: unsupported protocol version")

// Nonce identifies one probe exchange.
type Nonce [NonceLen]byte

// NewNonce mints a random probe nonce.
func NewNonce() (Nonce, error) {
	var n Nonce
	if _, err := rand.Read(n[:]); err != nil {
		return Nonce{}, fmt.Errorf("wire: generate nonce: %w", err)
	}
	return n, nil
}

// Encode frames a payload of the given type. The returned slice aliases dst,
// which is grown as needed; pass a reusable buffer to avoid an allocation per
// packet on the forwarding path.
func Encode(dst []byte, typ uint8, payload []byte) []byte {
	dst = append(dst[:0], Version, typ, 0, 0)
	return append(dst, payload...)
}

// Decode splits a received datagram into its type and payload. The payload
// aliases buf.
func Decode(buf []byte) (typ uint8, payload []byte, err error) {
	if len(buf) < HeaderLen {
		return 0, nil, ErrShortFrame
	}
	if buf[0] != Version {
		return 0, nil, fmt.Errorf("%w: %d", ErrVersion, buf[0])
	}
	return buf[1], buf[HeaderLen:], nil
}
