package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Mesh2Mesh/internal/wgcrypt"
)

type state struct {
	PrivateKey   string    `json:"private_key,omitempty"`
	PublicKey    string    `json:"public_key"`
	PeerID       string    `json:"peer_id"`
	TenantID     string    `json:"tenant_id"`
	TenantName   string    `json:"tenant_name"`
	Name         string    `json:"name"`
	MeshIP       string    `json:"mesh_ip"`
	UDPPort      int       `json:"udp_port,omitempty"`
	MeshCIDR     string    `json:"mesh_cidr"`
	API          string    `json:"api"`
	RegisteredAt time.Time `json:"registered_at"`

	// ServerEndpoint (host:port) is the mesh server every outbound packet is redirected to
	ServerEndpoint string `json:"server_endpoint,omitempty"`
	ServerKey      string `json:"server_key,omitempty"`
}

func (s *state) applyServerFlags(endpoint, key string) (changed bool) {
	if endpoint = strings.TrimSpace(endpoint); endpoint != "" && endpoint != s.ServerEndpoint {
		s.ServerEndpoint, changed = endpoint, true
	}
	if key = strings.TrimSpace(key); key != "" && key != s.ServerKey {
		s.ServerKey, changed = key, true
	}
	return changed
}

// serverSession builds the sealing session and resolved address for the mesh
// server, or nils when none is configured -- the cleartext peer-to-peer path.
func (s *state) serverSession() (*wgcrypt.Session, *net.UDPAddr, error) {
	switch {
	case s.ServerEndpoint == "" && s.ServerKey == "":
		return nil, nil, nil
	case s.ServerEndpoint == "":
		return nil, nil, errors.New("a server key is set but no server endpoint: pass --server host:port")
	case s.ServerKey == "":
		return nil, nil, errors.New("a server endpoint is set but no server key: pass --server-key <base64 X25519 public key>")
	case s.PrivateKey == "":
		// Registering with --public-key stores a key we hold no private half for.
		return nil, nil, errors.New("this node has no private key, so it cannot derive a session key: re-register without --public-key")
	}

	addr, err := net.ResolveUDPAddr("udp", s.ServerEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve server endpoint %q: %w", s.ServerEndpoint, err)
	}

	session, err := wgcrypt.NewSession(s.PrivateKey, s.ServerKey)
	if err != nil {
		return nil, nil, err
	}
	return session, addr, nil
}

func (s *state) meshPrefix() (netip.Prefix, error) {
	ip, err := netip.ParseAddr(s.MeshIP)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("state has an invalid mesh_ip %q: %w", s.MeshIP, err)
	}
	cidr, err := netip.ParsePrefix(s.MeshCIDR)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("state has an invalid mesh_cidr %q: %w", s.MeshCIDR, err)
	}
	return netip.PrefixFrom(ip, cidr.Bits()), nil
}

func loadState(path string) (*state, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st state
	if err := json.Unmarshal(buf, &st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &st, nil
}

func saveState(path string, st *state) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	buf, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	buf = append(buf, '\n')

	tmp, err := os.CreateTemp(dir, ".peer-*.json")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if _, err := tmp.Write(buf); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}
	return nil
}

func newKeyPair() (private, public string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate keypair: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()),
		base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()),
		nil
}
