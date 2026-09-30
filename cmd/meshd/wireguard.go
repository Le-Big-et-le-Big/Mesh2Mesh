package main

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"

	"Mesh2Mesh/internal/client"
	"Mesh2Mesh/internal/wire"
)

// keepaliveSeconds keeps NAT mappings open between peers that have nothing to say.
const keepaliveSeconds = 25

// meshBind is the UDP socket wireguard-go sends and receives on. The control
// plane's probes arrive on it too: a WireGuard message starts with its type
// (1 to 4), a wire frame with wire.Version (0), so the first byte tells them
// apart and probes are answered here without reaching WireGuard.
type meshBind struct {
	log *slog.Logger

	mu   sync.Mutex
	conn *net.UDPConn
}

var _ conn.Bind = (*meshBind)(nil)

func (b *meshBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}

	c, err := net.ListenUDP("udp", &net.UDPAddr{Port: int(port)})
	if err != nil {
		return nil, 0, fmt.Errorf("listen udp on port %d: %w", port, err)
	}
	b.conn = c
	return []conn.ReceiveFunc{b.receive(c)}, uint16(c.LocalAddr().(*net.UDPAddr).Port), nil
}

// port is the UDP port the bind is listening on, 0 when it is closed.
func (b *meshBind) port() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return 0
	}
	return b.conn.LocalAddr().(*net.UDPAddr).Port
}

func (b *meshBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return nil
	}
	err := b.conn.Close()
	b.conn = nil
	return err
}

func (b *meshBind) SetMark(uint32) error { return nil }

func (b *meshBind) BatchSize() int { return 1 }

func (b *meshBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return &conn.StdNetEndpoint{AddrPort: ap}, nil
}

func (b *meshBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	b.mu.Lock()
	c := b.conn
	b.mu.Unlock()
	if c == nil {
		return net.ErrClosed
	}

	dst, ok := ep.(*conn.StdNetEndpoint)
	if !ok {
		return conn.ErrWrongEndpointType
	}
	for _, buf := range bufs {
		if _, err := c.WriteToUDPAddrPort(buf, dst.AddrPort); err != nil {
			return err
		}
	}
	return nil
}

// receive returns WireGuard datagrams one at a time and answers probes in
// passing. wireguard-go calls it from a single goroutine, so reply is not shared.
func (b *meshBind) receive(c *net.UDPConn) conn.ReceiveFunc {
	reply := make([]byte, 0, wire.HeaderLen+wire.NonceLen)

	return func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		for {
			n, src, err := c.ReadFromUDPAddrPort(packets[0])
			if err != nil {
				return 0, err
			}
			from := netip.AddrPortFrom(src.Addr().Unmap(), src.Port())

			if n == 0 {
				continue
			}
			if packets[0][0] == wire.Version {
				reply = b.answerProbe(c, reply, packets[0][:n], from)
				continue
			}

			sizes[0] = n
			eps[0] = &conn.StdNetEndpoint{AddrPort: from}
			return 1, nil
		}
	}
}

func (b *meshBind) answerProbe(c *net.UDPConn, reply, frame []byte, from netip.AddrPort) []byte {
	typ, payload, err := wire.Decode(frame)
	switch {
	case err != nil:
		b.log.Debug("dropping malformed frame", "src", from.String(), "err", err)
		return reply
	case typ == wire.TypeProbeReply:
		// Nothing sends probes from a node yet; holepunching will.
		b.log.Debug("unsolicited probe reply", "src", from.String())
		return reply
	case typ != wire.TypeProbe || len(payload) < wire.NonceLen:
		b.log.Debug("dropping unexpected frame", "src", from.String(), "type", typ)
		return reply
	}

	reply = wire.Encode(reply, wire.TypeProbeReply, payload[:wire.NonceLen])
	if _, err := c.WriteToUDPAddrPort(reply, from); err != nil {
		b.log.Warn("could not answer probe", "src", from.String(), "err", err)
	}
	return reply
}

// wgPeer is what we last configured in WireGuard for one peer.
type wgPeer struct {
	meshIP   netip.Addr
	endpoint netip.AddrPort
}

// syncPeers brings WireGuard's peer list in line with the control plane's,
// keyed by hex public key. Only changes are sent.
func (d *daemon) syncPeers(peers []client.Peer) (routable int, err error) {
	var cfg strings.Builder
	next := make(map[string]wgPeer, len(peers))

	for _, p := range peers {
		if p.PeerID == d.state.PeerID {
			continue
		}
		key, err := hexKey(p.PublicKey)
		if err != nil {
			d.log.Debug("skipping peer with an unusable public key", "peer", p.Name, "err", err)
			continue
		}
		meshIP, err := netip.ParseAddr(p.MeshIP)
		if err != nil || !meshIP.Is4() {
			continue // the control plane sent something we cannot route to
		}

		want := wgPeer{meshIP: meshIP}
		if p.DirectReachable {
			// It answered the control plane's probe, so its port is open to us too.
			if ep, err := netip.ParseAddrPort(p.Endpoint); err == nil {
				want.endpoint = ep
			}
		}
		prev, known := d.wgPeers[key]
		if !want.endpoint.IsValid() {
			// Keep the last endpoint: WireGuard cannot unset one, and it may still work.
			want.endpoint = prev.endpoint
		}
		if want.endpoint.IsValid() {
			routable++
		}
		next[key] = want
		if known && prev == want {
			continue
		}

		fmt.Fprintf(&cfg, "public_key=%s\nreplace_allowed_ips=true\nallowed_ip=%s/32\n", key, meshIP)
		if want.endpoint.IsValid() && want.endpoint != prev.endpoint {
			// Keepalive only with an endpoint: setting it sends at once, and
			// with nowhere to send WireGuard logs an error.
			fmt.Fprintf(&cfg, "endpoint=%s\npersistent_keepalive_interval=%d\n", want.endpoint, keepaliveSeconds)
		}
	}
	for key := range d.wgPeers {
		if _, ok := next[key]; !ok {
			fmt.Fprintf(&cfg, "public_key=%s\nremove=true\n", key)
		}
	}

	if cfg.Len() > 0 {
		if err := d.wg.IpcSet(cfg.String()); err != nil {
			return 0, fmt.Errorf("configure wireguard peers: %w", err)
		}
	}
	d.wgPeers = next
	return routable, nil
}

// wgLogger routes wireguard-go's logs into slog it only shows with -v.
func wgLogger(log *slog.Logger) *device.Logger {
	return &device.Logger{
		Verbosef: func(format string, args ...any) { log.Debug(fmt.Sprintf(format, args...), "component", "wireguard") },
		Errorf:   func(format string, args ...any) { log.Error(fmt.Sprintf(format, args...), "component", "wireguard") },
	}
}

// hexKey turns a base64 X25519 key into the hex form the WireGuard UAPI takes.
func hexKey(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return "", fmt.Errorf("decode key: %w", err)
	}
	if len(raw) != 32 {
		return "", errors.New("key is not 32 bytes")
	}
	return hex.EncodeToString(raw), nil
}
