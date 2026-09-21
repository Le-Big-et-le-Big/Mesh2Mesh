// Check for control plane if it can reach a given peer at the endpoint it reported
package prober

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"

	"Mesh2Mesh/internal/wire"
)

// DefaultAddr is where the prober listens when none is configured. Replies come back to this port, so it must be reachable from the peers being probed.
const DefaultAddr = ":51821"

const (
	probeBudget   = 2 * time.Second
	probeRetries  = 3
	retryInterval = 400 * time.Millisecond
)

// Prober owns a UDP socket
type Prober struct {
	conn   *net.UDPConn
	log    *slog.Logger
	budget time.Duration

	mu      sync.Mutex
	waiters map[wire.Nonce]waiter

	closeOnce sync.Once
}

// waiter is one in-flight probe: where it went, and the channel closed on reply.
type waiter struct {
	target  netip.AddrPort
	replied chan struct{}
}

// Open binds the prober socket and starts its read loop. An empty addr uses DefaultAddr.
func Open(addr string, log *slog.Logger) (*Prober, error) {
	if addr == "" {
		addr = DefaultAddr
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("prober: resolve %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("prober: listen %s: %w", addr, err)
	}

	p := &Prober{conn: conn, log: log, budget: probeBudget, waiters: make(map[wire.Nonce]waiter)}
	go p.readLoop()
	return p, nil
}

func (p *Prober) LocalAddr() net.Addr { return p.conn.LocalAddr() }

func (p *Prober) Close() error {
	var err error
	p.closeOnce.Do(func() { err = p.conn.Close() })
	return err
}

// Reachable reports whether target echoed a probe back. false means only no response in time
func (p *Prober) Reachable(ctx context.Context, target netip.AddrPort) (bool, error) {
	if !target.IsValid() || target.Port() == 0 {
		return false, fmt.Errorf("prober: invalid target %s", target)
	}

	nonce, err := wire.NewNonce()
	if err != nil {
		return false, err
	}

	replied := make(chan struct{})
	p.mu.Lock()
	p.waiters[nonce] = waiter{target: target, replied: replied}
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.waiters, nonce)
		p.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, p.budget)
	defer cancel()

	frame := wire.Encode(nil, wire.TypeProbe, nonce[:])
	dst := net.UDPAddrFromAddrPort(target)

	for attempt := range probeRetries {
		if _, err := p.conn.WriteToUDP(frame, dst); err != nil {
			if attempt == probeRetries-1 {
				return false, fmt.Errorf("prober: send probe to %s: %w", target, err)
			}
			p.log.Debug("probe send failed, retrying", "target", target.String(), "err", err)
		}

		select {
		case <-replied:
			return true, nil
		case <-time.After(retryInterval):
		case <-ctx.Done():
			return false, nil
		}
	}

	// Every probe is out; spend what is left of the budget waiting.
	select {
	case <-replied:
		return true, nil
	case <-ctx.Done():
		return false, nil
	}
}

// readLoop dispatches probe replies to whoever waits on their nonce.
func (p *Prober) readLoop() {
	buf := make([]byte, wire.MaxFrameLen)
	for {
		n, src, err := p.conn.ReadFromUDP(buf)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				p.log.Error("prober read failed", "err", err)
			}
			return
		}

		srcAP := netip.AddrPortFrom(src.AddrPort().Addr().Unmap(), uint16(src.Port))

		typ, payload, err := wire.Decode(buf[:n])
		if err != nil || typ != wire.TypeProbeReply || len(payload) < wire.NonceLen {
			p.log.Debug("ignoring unexpected datagram", "src", srcAP.String(), "err", err)
			continue
		}

		var nonce wire.Nonce
		copy(nonce[:], payload)

		p.mu.Lock()
		w, ok := p.waiters[nonce]
		// The nonce went to one endpoint, so a reply from elsewhere proves nothing.
		if ok && srcAP != w.target {
			ok = false
			p.log.Debug("probe reply from an unexpected source",
				"src", srcAP.String(), "expected", w.target.String())
		}
		if ok {
			delete(p.waiters, nonce) // so the close below happens exactly once
		}
		p.mu.Unlock()

		if ok {
			close(w.replied)
		}
	}
}
