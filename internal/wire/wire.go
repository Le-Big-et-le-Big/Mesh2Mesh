package wire

import (
	"crypto/rand"
	"errors"
	"fmt"
)

// Version is the protocol version carried in byte 0
const Version = 0

// Frame types.
const (
	TypeData          uint8 = 1
	TypeProbe         uint8 = 2
	TypeProbeReply    uint8 = 3
	TypeDataEncrypted uint8 = 4
)

// Version, type, and 2 reserved
const HeaderLen = 4

const NonceLen = 16

// MaxFrameLen caps
const MaxFrameLen = 2048

var ErrShortFrame = errors.New("wire: frame shorter than header")
var ErrVersion = errors.New("wire: unsupported protocol version")

// Identifies one probe exchange.
type Nonce [NonceLen]byte

// NewNonce mints a random probe nonce.
func NewNonce() (Nonce, error) {
	var n Nonce
	if _, err := rand.Read(n[:]); err != nil {
		return Nonce{}, fmt.Errorf("wire: generate nonce: %w", err)
	}
	return n, nil
}

// Encode frames a payload of the given type.
func Encode(dst []byte, typ uint8, payload []byte) []byte {
	dst = append(dst[:0], Version, typ, 0, 0)
	return append(dst, payload...)
}

// Decode splits a received datagram into its type and payload
func Decode(buf []byte) (typ uint8, payload []byte, err error) {
	if len(buf) < HeaderLen {
		return 0, nil, ErrShortFrame
	}
	if buf[0] != Version {
		return 0, nil, fmt.Errorf("%w: %d", ErrVersion, buf[0])
	}
	return buf[1], buf[HeaderLen:], nil
}
