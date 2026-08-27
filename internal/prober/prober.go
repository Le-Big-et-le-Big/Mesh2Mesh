// Package prober answers one question for the control plane: can an
// unsolicited UDP datagram reach a peer at the endpoint it reported?
//
// That is the test behind connectivity strategy 1. A peer whose UDP port is
// open -- because it holds a public IP, or because the port is forwarded --
// echoes the probe back, and other peers can then be told to talk to it
// directly. A peer that stays silent is behind a NAT that drops unsolicited
// inbound, and needs holepunching or a relay instead.
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

// DefaultAddr is the address the prober listens on when none is configured.
// The port matters: replies come back to it, so it must be reachable from the
// peers being probed.
const DefaultAddr = ":51821"

const (
	// probeBudget is the total time one Reachable call spends waiting.
	probeBudget = 2 * time.Second
	// probeRetries is how many datagrams a single call sends. UDP is lossy and
	// one dropped probe should not be reported as "unreachable".
	probeRetries = 3
	// retryInterval spaces those datagrams out.
	retryInterval = 400 * time.Millisecond
)

// Prober owns a UDP socket and correlates probes with their replies.
type Prober struct {
	conn *net.UDPConn
	log  *slog.Logger
	// budget is how long one Reachable call waits. It is a field so tests can
	// shorten it; Open always sets it to probeBudget.
	budget time.Duration

	mu      sync.Mutex
	waiters map[wire.Nonce]waiter

	closeOnce sync.Once
}

// waiter is one in-flight probe: the endpoint it went to, and the channel
// closed when that endpoint answers.
type waiter struct {
	target  netip.AddrPort
	replied chan struct{}
}

// Open binds the prober socket and starts its read loop. An empty addr uses
// DefaultAddr.
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

// LocalAddr reports where the prober is listening.
func (p *Prober) LocalAddr() net.Addr { return p.conn.LocalAddr() }

// Close stops the prober. Calls to Reachable after it fail.
func (p *Prober) Close() error {
	var err error
	p.closeOnce.Do(func() { err = p.conn.Close() })
	return err
}

// Reachable reports whether target echoed a probe back. A false return is a
// verdict, not a failure: it means the endpoint did not answer in time. An
// error is returned only when the probe could not be sent at all.
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
			// A send failure on the last attempt is the caller's problem; an
			// earlier one may still be transient (a full socket buffer, say).
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

// readLoop dispatches probe replies to whoever is waiting on their nonce.
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

		// A dual-stack socket reports IPv4 senders as 4-in-6; unmap so the
		// address compares equal to the IPv4 target we probed.
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
		// The nonce went to exactly one endpoint, so a reply from anywhere else
		// says nothing about whether that endpoint is reachable.
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
