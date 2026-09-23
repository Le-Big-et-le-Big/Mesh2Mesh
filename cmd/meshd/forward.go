package main

import (
	"fmt"
	"net"
	"net/netip"
	"sync"

	"Mesh2Mesh/internal/client"
	"Mesh2Mesh/internal/wire"
)

const ipv4HeaderLen = 20

type peerEntry struct {
	name     string
	endpoint netip.AddrPort
	learned  bool
}

type peerTable struct {
	mu    sync.RWMutex
	peers map[netip.Addr]peerEntry
}

func newPeerTable() *peerTable {
	return &peerTable{peers: make(map[netip.Addr]peerEntry)}
}

// refresh replaces the table with what the control plane just reported. Only
// peers it could probe get a routable endpoint and endpoints we learned from
// received traffic survive the refresh.
func (t *peerTable) refresh(peers []client.Peer, selfID string) (routable int) {
	next := make(map[netip.Addr]peerEntry, len(peers))

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, p := range peers {
		if p.PeerID == selfID {
			continue
		}
		meshIP, err := netip.ParseAddr(p.MeshIP)
		if err != nil {
			continue // the control plane sent something we cannot route to
		}

		entry := peerEntry{name: p.Name}
		switch prev, ok := t.peers[meshIP]; {
		case ok && prev.learned:
			entry.endpoint, entry.learned = prev.endpoint, true
		case p.DirectReachable:
			// It answered the control plane's probe, so its port is open to us too.
			if ep, err := netip.ParseAddrPort(p.Endpoint); err == nil {
				entry.endpoint = ep
			}
		}
		if entry.endpoint.IsValid() {
			routable++
		}
		next[meshIP] = entry
	}

	t.peers = next
	return routable
}

func (t *peerTable) lookup(meshIP netip.Addr) (netip.AddrPort, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	entry, ok := t.peers[meshIP]
	return entry.endpoint, ok && entry.endpoint.IsValid()
}

// learn records where a peer's traffic arrives from, which is where replies
// should go. It returns false when meshIP is not a peer of ours.
func (t *peerTable) learn(meshIP netip.Addr, from netip.AddrPort) bool {
	t.mu.RLock()
	entry, ok := t.peers[meshIP]
	unchanged := ok && entry.endpoint == from
	t.mu.RUnlock()

	if !ok || unchanged {
		return ok
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	// Re-check: the table may have been replaced while the lock was released.
	if entry, ok = t.peers[meshIP]; !ok {
		return false
	}
	entry.endpoint, entry.learned = from, true
	t.peers[meshIP] = entry
	return true
}

// read IP packets out of mesh0 and send each one on
func (d *daemon) forwardOutbound() error {
	packet := make([]byte, wire.MaxFrameLen)
	frame := make([]byte, 0, wire.MaxFrameLen)
	sealed := make([]byte, 0, wire.MaxFrameLen)

	for {
		n, err := d.tun.Read(packet)
		if err != nil {
			return fmt.Errorf("read %s: %w", meshIface, err)
		}

		dst, ok := destAddr(packet[:n])
		if !ok {
			continue // not IPv4, the mesh is v4-only for now
		}

		if d.session != nil {
			frame, sealed, err = d.forwardToServer(frame, sealed, packet[:n])
			if err != nil {
				d.log.Warn("could not send to the mesh server", "dst", dst.String(), "err", err)
			}
			continue
		}

		endpoint, ok := d.peers.lookup(dst)
		if !ok {
			d.log.Debug("dropping packet, no reachable peer", "dst", dst.String())
			continue
		}

		frame = wire.Encode(frame, wire.TypeData, packet[:n])
		if _, err := d.conn.WriteToUDP(frame, net.UDPAddrFromAddrPort(endpoint)); err != nil {
			d.log.Warn("could not send to peer", "dst", dst.String(), "endpoint", endpoint.String(), "err", err)
		}
	}
}

// Seals one packet and sends it to the mesh server.
// It returns the frame and seal buffers so the caller can keep reusing them.
func (d *daemon) forwardToServer(frame, sealed, packet []byte) ([]byte, []byte, error) {
	sealed, err := d.session.Seal(sealed, packet)
	if err != nil {
		return frame, sealed, fmt.Errorf("seal packet: %w", err)
	}

	frame = wire.Encode(frame, wire.TypeDataEncrypted, sealed)
	if _, err := d.conn.WriteToUDP(frame, d.server); err != nil {
		return frame, sealed, fmt.Errorf("write to %s: %w", d.server, err)
	}
	return frame, sealed, nil
}

// read frames off the tunnel socket, answer probes, and hand tunnelled packets to the kernel through mesh0.
func (d *daemon) forwardInbound() error {
	buf := make([]byte, wire.MaxFrameLen)
	reply := make([]byte, 0, wire.HeaderLen+wire.NonceLen)

	for {
		n, src, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return fmt.Errorf("read tunnel socket: %w", err)
		}
		from := netip.AddrPortFrom(src.AddrPort().Addr().Unmap(), uint16(src.Port))

		typ, payload, err := wire.Decode(buf[:n])
		if err != nil {
			d.log.Debug("dropping malformed frame", "src", from.String(), "err", err)
			continue
		}

		switch typ {
		case wire.TypeProbe:
			if len(payload) < wire.NonceLen {
				continue
			}
			reply = wire.Encode(reply, wire.TypeProbeReply, payload[:wire.NonceLen])
			if _, err := d.conn.WriteToUDP(reply, src); err != nil {
				d.log.Warn("could not answer probe", "src", from.String(), "err", err)
			}

		case wire.TypeData:
			d.deliver(payload, from)

		case wire.TypeProbeReply:
			// Nothing sends probes from a node yet; holepunching will.
			d.log.Debug("unsolicited probe reply", "src", from.String())

		case wire.TypeDataEncrypted:
			// Nothing should send us one: opening it needs the sender's public
			// key, which the peer table does not carry.
			d.log.Debug("dropping encrypted frame, inbound decryption is not wired up", "src", from.String())
		}
	}
}

func (d *daemon) deliver(packet []byte, from netip.AddrPort) {
	srcMesh, ok := sourceAddr(packet)
	if !ok {
		d.log.Debug("dropping non-IPv4 payload", "src", from.String())
		return
	}

	if !d.peers.learn(srcMesh, from) {
		d.log.Debug("dropping packet from an unknown peer", "src", from.String(), "claimed", srcMesh.String())
		return
	}
	if _, err := d.tun.Write(packet); err != nil {
		d.log.Warn("could not write to "+meshIface, "err", err)
	}
}

func destAddr(packet []byte) (netip.Addr, bool) { return addrAt(packet, 16) }

func sourceAddr(packet []byte) (netip.Addr, bool) { return addrAt(packet, 12) }

func addrAt(packet []byte, offset int) (netip.Addr, bool) {
	if len(packet) < ipv4HeaderLen || packet[0]>>4 != 4 {
		return netip.Addr{}, false
	}
	return netip.AddrFrom4([4]byte(packet[offset : offset+4])), true
}
