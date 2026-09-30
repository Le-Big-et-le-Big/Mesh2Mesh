package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"Mesh2Mesh/internal/client"
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

	// wg encrypts everything on mesh0 end to end and wgPeers is what syncPeers last gave it.
	wg      *device.Device
	wgPeers map[string]wgPeer

	reported bool
}

func runCmd(ctx context.Context, args []string) error {
	fset := flag.NewFlagSet("run", flag.ContinueOnError)
	port := fset.Int("port", 0, "UDP tunnel port (default: the port in the state file, else 51820)")
	verbose := fset.Bool("v", false, "log WireGuard handshakes, dropped frames and other per-packet detail")
	foreground := fset.Bool("foreground", false, "run in this terminal instead of as the systemd service")
	statePath := fset.String("state", env("MESH2MESH_STATE", defaultStatePath), "node state file")
	if err := fset.Parse(args); err != nil {
		return err
	}

	st, err := loadState(*statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("run: this node is in no tenant yet (%s is missing) — run `meshd setup <tenant-name>` or `meshd join <tenant-id>` first", *statePath)
	} else if err != nil {
		return err
	}

	prefix, err := st.meshPrefix()
	if err != nil {
		return err
	}
	if st.PrivateKey == "" {
		return errors.New("run: this node has no private key, so it cannot speak WireGuard — run `meshd join " + st.TenantID + "` again")
	}
	privateKey, err := hexKey(st.PrivateKey)
	if err != nil {
		return fmt.Errorf("run: state has an invalid private_key: %w", err)
	}

	// The flag wins over the state file, and is persisted below so a restart
	// under systemd keeps the port without it.
	dirty := false
	if *port != 0 && *port != st.UDPPort {
		st.UDPPort, dirty = *port, true
	}

	asService := !*foreground && hasSystemd()
	if err := checkNotRunning(asService); err != nil {
		return err
	}

	if asService {
		if dirty {
			if err := saveState(*statePath, st); err != nil {
				return err
			}
		}
		return startService(*statePath, *verbose, st, prefix)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	tunDev, err := tun.CreateTUN(meshIface, meshMTU)
	if err != nil {
		return fmt.Errorf("create %s: %w (is the tun module loaded, and are we root?)", meshIface, err)
	}
	// The device owns tunDev from here, and closes it with the bind.
	bind := &meshBind{log: log}
	wg := device.NewDevice(tunDev, bind, wgLogger(log))
	defer wg.Close()

	if err := setupInterface(prefix.String()); err != nil {
		return err
	}

	udpPort := firstSet(*port, st.UDPPort, defaultUDPPort)
	if err := wg.IpcSet(fmt.Sprintf("private_key=%s\nlisten_port=%d\n", privateKey, udpPort)); err != nil {
		return fmt.Errorf("configure wireguard: %w", err)
	}
	if err := wg.Up(); err != nil {
		return fmt.Errorf("bring wireguard up on udp port %d: %w", udpPort, err)
	}
	udpPort = bind.port()

	if st.UDPPort != udpPort {
		st.UDPPort, dirty = udpPort, true
	}
	if dirty {
		if err := saveState(*statePath, st); err != nil {
			return err
		}
	}

	// Lets `meshd stop` find us when we were not started by systemd.
	if err := writePIDFile(); err != nil {
		return err
	}
	defer removePIDFile()

	d := &daemon{
		log:     log,
		api:     client.New(st.API),
		state:   st,
		port:    udpPort,
		wg:      wg,
		wgPeers: make(map[string]wgPeer),
	}

	log.Info("mesh is up, traffic between peers is encrypted with WireGuard",
		"iface", meshIface, "addr", prefix.String(), "udp_port", udpPort,
		"peer_id", st.PeerID, "tenant", st.TenantName, "public_key", st.PublicKey)

	d.reportEndpoint(ctx)
	d.refreshPeers(ctx)
	go d.maintain(ctx)

	select {
	case <-wg.Wait():
		return errors.New("the wireguard device closed unexpectedly")
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

	routable, err := d.syncPeers(peers)
	if err != nil {
		d.log.Error("could not apply the peer list", "err", err)
		return
	}
	d.log.Debug("peers refreshed", "peers", len(d.wgPeers), "with_endpoint", routable)
}

func firstSet(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
