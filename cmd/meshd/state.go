package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"time"
)

type state struct {
	PrivateKey   string    `json:"private_key,omitempty"`
	PublicKey    string    `json:"public_key"`
	PeerID       string    `json:"peer_id"`
	TenantID     string    `json:"tenant_id"`
	TenantName   string    `json:"tenant_name"`
	Name         string    `json:"name"`
	MeshIP       string    `json:"mesh_ip"`
	MeshCIDR     string    `json:"mesh_cidr"`
	API          string    `json:"api"`
	RegisteredAt time.Time `json:"registered_at"`
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
	defer os.Remove(tmp.Name()) // no-op once the rename below succeeds

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
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
