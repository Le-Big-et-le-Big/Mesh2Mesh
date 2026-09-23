package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"time"

	"Mesh2Mesh/internal/client"
	"Mesh2Mesh/internal/wgcrypt"
)

const (
	defaultUDPPort          = 51820
	peerRefreshInterval     = 15 * time.Second
	endpointRefreshInterval = 5 * time.Minute
)

type daemon struct {
	log   *slog.Logger
	api   *client.Client
	state *state
	port  int

	tun   *os.File
	conn  *net.UDPConn
	peers *peerTable

	// Set together, or not at all: when they are, every outbound packet is sealed
	// and redirected to server instead of going to the peer in its header.
	session *wgcrypt.Session
	server  *net.UDPAddr

	reported bool
}

func runCmd(ctx context.Context, args []string) error {
	fset := flag.NewFlagSet("run", flag.ContinueOnError)
	port := fset.Int("port", 0, "UDP tunnel port (default: the port in the state file, else 51820)")
	server := fset.String("server", "", "mesh server `host:port`: encrypt every outbound packet and redirect it there")
	serverKey := fset.String("server-key", "", "the mesh server's base64 X25519 public key (required with --server)")
	verbose := fset.Bool("v", false, "log dropped packets and other per-packet detail")
	common := registerCommonFlags(fset)
	if err := fset.Parse(args); err != nil {
		return err
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	st, err := loadState(*common.state)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("run: no registration at %s — run `meshd register --token ...` first", *common.state)
	} else if err != nil {
		return err
	}

	prefix, err := st.meshPrefix()
	if err != nil {
		return err
	}

	// Flags win over the state file, and are persisted below so a restart under
	// systemd keeps redirecting without them.
	dirty := st.applyServerFlags(*server, *serverKey)
	session, serverAddr, err := st.serverSession()
	if err != nil {
		return err
	}

	if err := setupInterface(prefix.String()); err != nil {
		return err
	}

	// Bind before opening the TUN device: if the port is taken, say so first.
	udpPort := firstSet(*port, st.UDPPort, defaultUDPPort)
	conn, err := bindUDP(udpPort)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	udpPort = conn.LocalAddr().(*net.UDPAddr).Port

	if st.UDPPort != udpPort {
		st.UDPPort, dirty = udpPort, true
	}
	if dirty {
		if err := saveState(*common.state, st); err != nil {
			return err
		}
	}

	tun, err := openTUN(meshIface)
	if err != nil {
		return err
	}
	defer func() { _ = tun.Close() }()

	d := &daemon{
		log:     log,
		api:     client.New(st.API),
		state:   st,
		port:    udpPort,
		tun:     tun,
		conn:    conn,
		peers:   newPeerTable(),
		session: session,
		server:  serverAddr,
	}

	log.Info("mesh is up",
		"iface", meshIface, "addr", prefix.String(), "udp_port", udpPort,
		"peer_id", st.PeerID, "tenant", st.TenantName)

	if d.session != nil {
		log.Info("outbound packets are encrypted and redirected to the mesh server",
			"server", d.server.String(), "cipher", "ChaCha20-Poly1305", "key_agreement", "X25519 static-static")
	} else {
		log.Warn("outbound packets are sent to peers in cleartext",
			"hint", "pass --server host:port --server-key <base64> to encrypt and redirect them")
	}

	errc := make(chan error, 2)
	go func() { errc <- d.forwardOutbound() }()
	go func() { errc <- d.forwardInbound() }()

	d.reportEndpoint(ctx)
	d.refreshPeers(ctx)
	go d.maintain(ctx)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("shutting down", "iface", meshIface)
		return nil
	}
}

// maintain keeps the peer table and our own endpoint current until ctx ends.
func (d *daemon) maintain(ctx context.Context) {
	peerTick := time.NewTicker(peerRefreshInterval)
	defer peerTick.Stop()
	endpointTick := time.NewTicker(endpointRefreshInterval)
	defer endpointTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-peerTick.C:
			if !d.reported {
				d.reportEndpoint(ctx)
			}
			d.refreshPeers(ctx)
		case <-endpointTick.C:
			d.reportEndpoint(ctx)
		}
	}
}

// reportEndpoint tells the control plane which UDP port we bound
func (d *daemon) reportEndpoint(ctx context.Context) {
	peer, err := d.api.UpdateEndpoint(ctx, d.state.PeerID, d.port)
	if err != nil {
		d.log.Error("could not report endpoint to the control plane", "err", err)
		return
	}
	d.reported = true

	if peer.DirectReachable {
		d.log.Info("direct connectivity available: our UDP port accepts unsolicited traffic",
			"endpoint", peer.Endpoint)
		return
	}
	d.log.Warn("no direct connectivity: our UDP port is not reachable from outside",
		"endpoint", peer.Endpoint,
		"hint", fmt.Sprintf("forward UDP %d to this host, or wait for holepunching/relay support", d.port))
}

func (d *daemon) refreshPeers(ctx context.Context) {
	peers, err := d.api.ListPeers(ctx, d.state.TenantID)
	if err != nil {
		d.log.Error("could not list peers", "err", err)
		return
	}

	routable := d.peers.refresh(peers, d.state.PeerID)
	d.log.Debug("peer table refreshed", "peers", len(peers)-1, "routable", routable)
}

func firstSet(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
