package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/netip"
)

const tokenPrefix = "m2m_"

// 2 reserved hosts = 1 relay IP for the server + 1 network address IP
const reservedHosts = 2

// newSecret mints a 256-bit enrollment token.
func newSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashSecret derives the value actually stored in the database.
func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// nextFreeAddr returns the lowest address in prefix that is not in used and not
// reserved. The IPv4 broadcast address is skipped.
func nextFreeAddr(prefix netip.Prefix, used map[netip.Addr]bool) (netip.Addr, error) {
	prefix = prefix.Masked()

	addr := prefix.Addr()
	for range reservedHosts {
		addr = addr.Next()
	}

	last := lastAddr(prefix)
	for prefix.Contains(addr) {
		if addr.Is4() && addr == last {
			break // broadcast address
		}
		if !used[addr] {
			return addr, nil
		}
		if addr == last {
			break
		}
		addr = addr.Next()
	}
	return netip.Addr{}, fmt.Errorf("%w: %s", ErrPoolExhausted, prefix)
}

// lastAddr returns the highest address inside prefix.
func lastAddr(prefix netip.Prefix) netip.Addr {
	bytes := prefix.Masked().Addr().AsSlice()
	for i := prefix.Bits(); i < len(bytes)*8; i++ {
		bytes[i/8] |= 1 << (7 - i%8)
	}
	addr, _ := netip.AddrFromSlice(bytes)
	return addr
}
