package wire

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

// Version is the protocol version carried in byte 0
const Version = 0

const (
	TypeProbe      uint8 = 1
	TypeProbeReply uint8 = 2
	// Binding: a node asks the prober which address it is seen from.
	TypeBindingRequest  uint8 = 3
	TypeBindingResponse uint8 = 4
	// Relay: register with a token from the API, then send WireGuard datagrams.
	TypeRelayRegister uint8 = 5
	TypeRelayAck      uint8 = 6
	TypeRelayData     uint8 = 7
)

const RelayTokenLen = 16

type RelayToken [RelayTokenLen]byte

// Header, then the destination mesh IPv4 to the relay, the source one from it.
const RelayHeaderLen = HeaderLen + 4

// Version, type, and 2 reserved
const HeaderLen = 4

const NonceLen = 16

// 16-byte IP, then the port
const AddrPortLen = 18

// Nonce, then the address. Requests are padded to it.
const BindingLen = NonceLen + AddrPortLen

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

func AppendAddrPort(dst []byte, ap netip.AddrPort) []byte {
	ip := ap.Addr().As16()
	dst = append(dst, ip[:]...)
	return binary.BigEndian.AppendUint16(dst, ap.Port())
}

// ParseAddrPort reads AddrPortLen bytes written by AppendAddrPort.
func ParseAddrPort(b []byte) netip.AddrPort {
	ip := netip.AddrFrom16([16]byte(b[:16])).Unmap()
	return netip.AddrPortFrom(ip, binary.BigEndian.Uint16(b[16:AddrPortLen]))
}
