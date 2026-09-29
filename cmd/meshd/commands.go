package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Mesh2Mesh/internal/client"
)

const enrollTokenTTL = time.Minute

// enrollFlags are what setup and join share: where the control plane is, where
// the node state lives and what this node is called.
type enrollFlags struct {
	fset  *flag.FlagSet
	api   *string
	state *string
	name  *string
}

func newEnrollFlags(cmd string) enrollFlags {
	fset := flag.NewFlagSet(cmd, flag.ContinueOnError)
	return enrollFlags{
		fset:  fset,
		api:   fset.String("api", env("MESH2MESH_API", ""), "control-plane URL (default: the one this node last used, else "+client.DefaultBaseURL+")"),
		state: fset.String("state", env("MESH2MESH_STATE", defaultStatePath), "node state file"),
		name:  fset.String("name", "", "this node's name in the mesh (default: the hostname)"),
	}
}

// parse reads the flags and the single positional argument, which may come
// before or after them: `meshd setup acme --api x` and `meshd setup --api x acme`.
func (f enrollFlags) parse(args []string, argName string) (string, error) {
	if err := f.fset.Parse(args); err != nil {
		return "", err
	}
	rest := f.fset.Args()
	if len(rest) == 0 {
		return "", fmt.Errorf("%s: expected <%s>", f.fset.Name(), argName)
	}
	arg := strings.TrimSpace(rest[0])
	if err := f.fset.Parse(rest[1:]); err != nil {
		return "", err
	}
	if f.fset.NArg() > 0 {
		return "", fmt.Errorf("%s: unexpected argument %q", f.fset.Name(), f.fset.Arg(0))
	}
	if arg == "" {
		return "", fmt.Errorf("%s: <%s> is empty", f.fset.Name(), argName)
	}
	return arg, nil
}

func setupCmd(ctx context.Context, args []string) error {
	f := newEnrollFlags("setup")
	cidr := f.fset.String("cidr", "", "mesh CIDR (default: the server's 10.203.0.0/16)")
	tenantName, err := f.parse(args, "tenant-name")
	if err != nil {
		return err
	}

	prev, api, err := f.prepare()
	if err != nil {
		return err
	}

	c := client.New(api)
	tenant, err := c.CreateTenant(ctx, tenantName, strings.TrimSpace(*cidr))
	if err != nil {
		return err
	}
	fmt.Printf("created tenant %q (%s, %s)\n", tenant.Name, tenant.ID, tenant.CIDR)

	st, err := f.enroll(ctx, c, api, prev, tenant.ID)
	if err != nil {
		return fmt.Errorf("the tenant was created but this node could not join it (retry with `meshd join %s`): %w", tenant.ID, err)
	}

	fmt.Printf("\nOn every other node:\n  sudo meshd join %s --api %s\nThen, on each node:\n  sudo meshd run\n", st.TenantID, api)
	return nil
}

func joinCmd(ctx context.Context, args []string) error {
	f := newEnrollFlags("join")
	tenantID, err := f.parse(args, "tenant-id")
	if err != nil {
		return err
	}

	prev, api, err := f.prepare()
	if err != nil {
		return err
	}

	if _, err := f.enroll(ctx, client.New(api), api, prev, tenantID); err != nil {
		return err
	}

	fmt.Printf("\nStart (or restart) the mesh with:\n  sudo meshd run\n")
	return nil
}

// prepare loads any existing registration and settles which control plane to
// talk to. It also fails early when the state file cannot be written, so a
// missing sudo does not leave a tenant or peer behind on the control plane.
func (f enrollFlags) prepare() (prev *state, api string, err error) {
	prev, err = loadState(*f.state)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	if err := checkWritable(filepath.Dir(*f.state)); err != nil {
		return nil, "", err
	}

	api = strings.TrimSpace(*f.api)
	switch {
	case api != "":
	case prev != nil && prev.API != "":
		api = prev.API
	default:
		api = client.DefaultBaseURL
	}
	return prev, api, nil
}

// enroll gives a single-use token for tenantID and redeems it, replacing any
// registration this node had while keeping its keypair.
func (f enrollFlags) enroll(ctx context.Context, c *client.Client, api string, prev *state, tenantID string) (*state, error) {
	st := &state{}
	if prev != nil && prev.PrivateKey != "" {
		st.PrivateKey, st.PublicKey = prev.PrivateKey, prev.PublicKey
	} else {
		private, public, err := newKeyPair()
		if err != nil {
			return nil, err
		}
		st.PrivateKey, st.PublicKey = private, public
	}
	if prev != nil {
		// A redirect server is a property of the node, not of the tenant.
		st.ServerEndpoint, st.ServerKey, st.UDPPort = prev.ServerEndpoint, prev.ServerKey, prev.UDPPort
	}

	peerName := strings.TrimSpace(*f.name)
	if peerName == "" {
		host, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("--name not given and the hostname is unreadable: %w", err)
		}
		peerName = host
	}

	token, err := c.CreateToken(ctx, tenantID, enrollTokenTTL, 1)
	if err != nil {
		return nil, err
	}
	reg, err := c.RegisterPeer(ctx, token.Secret, peerName, st.PublicKey)
	if err != nil {
		return nil, err
	}

	st.PeerID = reg.PeerID
	st.TenantID = reg.TenantID
	st.TenantName = reg.TenantName
	st.Name = reg.Name
	st.PublicKey = reg.PublicKey
	st.MeshIP = reg.MeshIP
	st.MeshCIDR = reg.MeshCIDR
	st.API = api
	st.RegisteredAt = time.Now().UTC()

	prefix, err := st.meshPrefix()
	if err != nil {
		return nil, err
	}
	if err := saveState(*f.state, st); err != nil {
		return nil, err
	}

	if prev != nil && prev.TenantID != "" && prev.TenantID != st.TenantID {
		// The control plane has no way to remove a peer yet, so the old record stays.
		fmt.Printf("moved out of tenant %q (its peer record for %s stays on the control plane)\n",
			prev.TenantName, prev.MeshIP)
	}
	fmt.Printf("joined tenant %q as %q with %s\n", st.TenantName, st.Name, prefix)
	return st, nil
}

// checkWritable creates dir if needed and makes sure we can create files in it.
func checkWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w (run with sudo?)", dir, err)
	}
	f, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w (run with sudo?)", dir, err)
	}
	_ = f.Close()
	return os.Remove(f.Name())
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
