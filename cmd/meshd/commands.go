package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"Mesh2Mesh/internal/client"
)

// commonFlags are accepted by every command that talks to the control plane or
// touches the node state file.
type commonFlags struct {
	api   *string
	state *string
}

func registerCommonFlags(fs *flag.FlagSet) commonFlags {
	return commonFlags{
		api:   fs.String("api", env("MESH2MESH_API", client.DefaultBaseURL), "control-plane base URL"),
		state: fs.String("state", env("MESH2MESH_STATE", defaultStatePath), "node state file"),
	}
}

func tenantCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("tenant: expected a subcommand (create, token)")
	}
	switch args[0] {
	case "create":
		return tenantCreateCmd(ctx, args[1:])
	case "token":
		return tenantTokenCmd(ctx, args[1:])
	default:
		return fmt.Errorf("tenant: unknown subcommand %q (expected create or token)", args[0])
	}
}

func tenantCreateCmd(ctx context.Context, args []string) error {
	fset := flag.NewFlagSet("tenant create", flag.ContinueOnError)
	name := fset.String("name", "", "tenant name (required)")
	cidr := fset.String("cidr", "", "mesh CIDR (default: the server's 10.203.0.0/16)")
	common := registerCommonFlags(fset)
	if err := fset.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*name) == "" {
		return errors.New("tenant create: --name is required")
	}

	tenant, err := client.New(*common.api).CreateTenant(ctx, strings.TrimSpace(*name), strings.TrimSpace(*cidr))
	if err != nil {
		return err
	}

	fmt.Printf("tenant created\n  id:   %s\n  name: %s\n  cidr: %s\n", tenant.ID, tenant.Name, tenant.CIDR)
	fmt.Printf("\nIssue an enrollment token with:\n  meshd tenant token --tenant %s\n", tenant.ID)
	return nil
}

func tenantTokenCmd(ctx context.Context, args []string) error {
	fset := flag.NewFlagSet("tenant token", flag.ContinueOnError)
	tenantID := fset.String("tenant", "", "tenant id (required)")
	ttl := fset.Duration("ttl", time.Hour, "how long the token stays valid (max 720h)")
	maxUses := fset.Int("max-uses", 1, "how many peers may redeem the token (max 1000)")
	common := registerCommonFlags(fset)
	if err := fset.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("tenant token: --tenant is required")
	}
	if *ttl <= 0 {
		return errors.New("tenant token: --ttl must be positive")
	}

	token, err := client.New(*common.api).CreateToken(ctx, strings.TrimSpace(*tenantID), *ttl, *maxUses)
	if err != nil {
		return err
	}

	fmt.Printf("enrollment token issued\n  token:      %s\n  max uses:   %d\n  expires at: %s\n",
		token.Secret, token.MaxUses, token.ExpiresAt.Format(time.RFC3339))
	fmt.Printf("\nThis is the only time the token is shown. Register a node with:\n  meshd register --token %s\n", token.Secret)
	return nil
}

// registerCmd redeems an enrollment token and persists the identity and address
// the control plane assigned, so `meshd up` can configure the interface from it.
func registerCmd(ctx context.Context, args []string) error {
	fset := flag.NewFlagSet("register", flag.ContinueOnError)
	token := fset.String("token", "", "enrollment token (required)")
	name := fset.String("name", "", "peer name (default: this host's name)")
	publicKey := fset.String("public-key", "", "public key to register (default: reuse or generate the node's own)")
	force := fset.Bool("force", false, "re-register even if this node already has state")
	common := registerCommonFlags(fset)
	if err := fset.Parse(args); err != nil {
		return err
	}

	secret := strings.TrimSpace(*token)
	if secret == "" {
		return errors.New("register: --token is required")
	}

	peerName := strings.TrimSpace(*name)
	if peerName == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("register: --name not given and the hostname is unreadable: %w", err)
		}
		peerName = host
	}

	// Start from what this node already has, so re-registering keeps its keypair.
	st, err := loadState(*common.state)
	switch {
	case err == nil && !*force:
		return fmt.Errorf("register: %s already holds a registration for %q (%s); pass --force to replace it",
			*common.state, st.Name, st.MeshIP)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	case err != nil:
		st = &state{}
	}

	// A caller-supplied key is registered as-is; we hold no private half for it.
	if key := strings.TrimSpace(*publicKey); key != "" {
		st.PrivateKey, st.PublicKey = "", key
	}
	if st.PublicKey == "" {
		private, public, err := newKeyPair()
		if err != nil {
			return err
		}
		st.PrivateKey, st.PublicKey = private, public
	}

	reg, err := client.New(*common.api).RegisterPeer(ctx, secret, peerName, st.PublicKey)
	if err != nil {
		return err
	}

	st.PeerID = reg.PeerID
	st.TenantID = reg.TenantID
	st.TenantName = reg.TenantName
	st.Name = reg.Name
	st.PublicKey = reg.PublicKey
	st.MeshIP = reg.MeshIP
	st.MeshCIDR = reg.MeshCIDR
	st.API = *common.api
	st.RegisteredAt = time.Now().UTC()

	prefix, err := st.meshPrefix()
	if err != nil {
		return err
	}
	if err := saveState(*common.state, st); err != nil {
		return err
	}

	fmt.Printf("registered as %q in tenant %s\n  peer id:  %s\n  mesh addr: %s\n  state:     %s\n",
		st.Name, st.TenantName, st.PeerID, prefix, *common.state)
	fmt.Printf("\nBring the interface up with:\n  sudo meshd up\n")
	return nil
}

// keygenCmd prints an X25519 keypair without touching the node state, for the
// mesh server: it is sealed towards, but never registers as a peer.
func keygenCmd(args []string) error {
	fset := flag.NewFlagSet("keygen", flag.ContinueOnError)
	if err := fset.Parse(args); err != nil {
		return err
	}

	private, public, err := newKeyPair()
	if err != nil {
		return err
	}

	fmt.Printf("private key: %s\npublic key:  %s\n", private, public)
	fmt.Printf("\nKeep the private key on the server. Point nodes at it with:\n"+
		"  sudo meshd run --server <server-host>:%d --server-key %s\n", defaultUDPPort, public)
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
