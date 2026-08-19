// Command meshd is the Mesh2Mesh node CLI: it talks to the control plane to
// create tenants, mint enrollment tokens and register this node, then
// configures the local mesh interface from what it was given.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// unitFile runs `up`, so the service configures the interface rather than
// re-installing itself. It is oneshot because `up` sets the interface and
// exits; RemainAfterExit keeps the unit active afterwards.
const unitFile = `[Unit]
Description=Meshing VPN service
After=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/Mesh2Mesh up

[Install]
WantedBy=multi-user.target
`

const (
	meshIface     = "mesh0"
	serverRelayIp = "192.168.1.12"

	// fallbackAddr is what `up` configures when this node has not registered.
	fallbackAddr = "10.203.0.1/16"

	defaultStatePath = "/etc/mesh2mesh/peer.json"
	unitPath         = "/etc/systemd/system/mesh2mesh.service"
)

const usage = `meshd — Mesh2Mesh node CLI

Usage:
  meshd tenant create --name <name> [--cidr <cidr>]
  meshd tenant token  --tenant <tenant-id> [--ttl <duration>] [--max-uses <n>]
  meshd register      --token <m2m_...> [--name <name>] [--public-key <key>] [--force]
  meshd install
  meshd up

Common flags:
  --api    control-plane base URL   (env MESH2MESH_API, default http://localhost:8090)
  --state  node state file          (env MESH2MESH_STATE, default /etc/mesh2mesh/peer.json)

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
	case "tenant":
		return tenantCmd(ctx, args[1:])
	case "register":
		return registerCmd(ctx, args[1:])
	case "install":
		return installCmd(args[1:])
	case "up":
		return upCmd(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		if strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("flags come after the command, e.g. `meshd tenant create %s ...`", args[0])
		}
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// installCmd writes the systemd unit and starts the service.
func installCmd(args []string) error {
	fset := flag.NewFlagSet("install", flag.ContinueOnError)
	if err := fset.Parse(args); err != nil {
		return err
	}

	if err := os.WriteFile(unitPath, []byte(unitFile), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", unitPath, err)
	}
	if err := runCmd("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := runCmd("systemctl", "enable", "--now", "mesh2mesh.service"); err != nil {
		return err
	}

	fmt.Printf("installed and started %s\n", unitPath)
	return nil
}

// upCmd configures the mesh interface with this node's registered address,
// falling back to fallbackAddr when the node has not registered yet.
func upCmd(args []string) error {
	fset := flag.NewFlagSet("up", flag.ContinueOnError)
	statePath := fset.String("state", env("MESH2MESH_STATE", defaultStatePath), "node state file")
	if err := fset.Parse(args); err != nil {
		return err
	}

	addr := fallbackAddr
	switch st, err := loadState(*statePath); {
	case err == nil:
		prefix, err := st.meshPrefix()
		if err != nil {
			return err
		}
		addr = prefix.String()
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintf(os.Stderr, "meshd: no registration at %s, falling back to %s — run `meshd register` first\n",
			*statePath, fallbackAddr)
	default:
		return err
	}

	if err := setupInterface(addr); err != nil {
		return err
	}

	fmt.Printf("%s is up with %s\n", meshIface, addr)
	return nil
}

// setupInterface creates mesh0 if it does not exist and gives it addr.
func setupInterface(addr string) error {
	if err := exec.Command("ip", "link", "show", meshIface).Run(); err != nil {
		if err := runCmd("ip", "tuntap", "add", "dev", meshIface, "mode", "tun"); err != nil {
			return err
		}
	}

	// "replace" so re-running does not fail with EEXIST.
	if err := runCmd("ip", "addr", "replace", addr, "dev", meshIface); err != nil {
		return err
	}
	return runCmd("ip", "link", "set", "dev", meshIface, "up")
}

// runCmd runs a command and folds its output into the returned error, so a
// failure says what the tool actually complained about.
func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
