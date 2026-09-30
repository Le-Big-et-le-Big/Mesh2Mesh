// Command meshd is the Mesh2Mesh node CLI: it enrolls this node into a tenant
// through the control plane, then carries the mesh traffic over mesh0, encrypted with WireGuard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const (
	meshIface = "mesh0"
	// meshMTU leaves room for WireGuard's 80 bytes of overhead on a 1500 underlay.
	meshMTU = 1420

	defaultStatePath = "/etc/mesh2mesh/peer.json"
)

const usage = `meshd — Mesh2Mesh node CLI

Usage:
  meshd setup <tenant-name> [--cidr <cidr>]   create a tenant and join this node to it
  meshd join  <tenant-id>                     join (or move this node to) an existing tenant
  meshd run   [--port <udp-port>] [-v]          bring mesh0 up and carry traffic over
                                              WireGuard, as a systemd service when
                                              there is one
  meshd stop                                  stop meshd and remove mesh0

Flags for setup and join:
  --api    control-plane URL   (env MESH2MESH_API; default: the one this node last used)
  --name   this node's name    (default: the hostname)

setup, join and run take --state (env MESH2MESH_STATE, default /etc/mesh2mesh/peer.json).
Run "meshd <command> --help" for a command's own flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "meshd:", err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return errors.New("expected a command")
	}

	switch args[0] {
	case "setup":
		return setupCmd(ctx, args[1:])
	case "join":
		return joinCmd(ctx, args[1:])
	case "run":
		return runCmd(ctx, args[1:])
	case "stop":
		return stopCmd(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		if strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("flags come after the command, e.g. `meshd run %s ...`", args[0])
		}
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// execCmd runs a command, folding its output into the returned error.
func execCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
