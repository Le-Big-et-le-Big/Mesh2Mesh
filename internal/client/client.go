// Package client is a typed HTTP client for the Mesh2Mesh control-plane API.
// It is what the node CLI uses to talk to `cmd/controlplane`.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL matches the control plane's default LISTEN_ADDR.
const DefaultBaseURL = "http://localhost:8090"

// maxResponseBytes caps what we read back.
const maxResponseBytes = 1 << 20

const defaultTimeout = 15 * time.Second

// Client talks to one control-plane instance.
type Client struct {
	baseURL string
	http    *http.Client
}

// New builds a Client. An empty baseURL falls back to DefaultBaseURL.
func New(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// Error is a non-2xx response. Message is the API's `error` field when the
// body carries one, otherwise the HTTP status line.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("control plane: %s (HTTP %d)", e.Message, e.Status)
}

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CIDR      string    `json:"cidr"`
	CreatedAt time.Time `json:"created_at"`
}

type Token struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Secret    string    `json:"token"`
	MaxUses   int       `json:"max_uses"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Registration is what a peer gets back after redeeming an enrollment token.
type Registration struct {
	PeerID     string    `json:"peer_id"`
	TenantID   string    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	Name       string    `json:"name"`
	PublicKey  string    `json:"public_key"`
	MeshIP     string    `json:"mesh_ip"`
	MeshCIDR   string    `json:"mesh_cidr"`
	CreatedAt  time.Time `json:"created_at"`
}

// Peer is one member of a tenant's mesh, as the control plane sees it.
type Peer struct {
	PeerID    string `json:"peer_id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	MeshIP    string `json:"mesh_ip"`
	// Endpoint is "ip:port" on the underlay network, empty until the peer has
	// reported one.
	Endpoint string `json:"endpoint"`
	// DirectReachable is whether the control plane's probe of Endpoint got an
	// answer.
	DirectReachable bool      `json:"direct_reachable"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateTenant creates a tenant; an empty cidr lets the server pick its default.
func (c *Client) CreateTenant(ctx context.Context, name, cidr string) (*Tenant, error) {
	req := struct {
		Name string `json:"name"`
		CIDR string `json:"cidr,omitempty"`
	}{Name: name, CIDR: cidr}

	var out Tenant
	if err := c.do(ctx, http.MethodPost, "/v1/tenants", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateToken mints an enrollment token. The secret it returns is not
// recoverable afterwards.
func (c *Client) CreateToken(ctx context.Context, tenantID string, ttl time.Duration, maxUses int) (*Token, error) {
	req := struct {
		TTLSeconds int `json:"ttl_seconds"`
		MaxUses    int `json:"max_uses"`
	}{TTLSeconds: int(ttl.Seconds()), MaxUses: maxUses}

	var out Token
	if err := c.do(ctx, http.MethodPost, "/v1/tenants/"+tenantID+"/tokens", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RegisterPeer redeems an enrollment token and returns the mesh address to configure
func (c *Client) RegisterPeer(ctx context.Context, token, name, publicKey string) (*Registration, error) {
	req := struct {
		Token     string `json:"token"`
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}{Token: token, Name: name, PublicKey: publicKey}

	var out Registration
	if err := c.do(ctx, http.MethodPost, "/v1/peers/register", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateEndpoint reports the UDP port this node bound.
func (c *Client) UpdateEndpoint(ctx context.Context, peerID string, udpPort int) (*Peer, error) {
	req := struct {
		UDPPort int `json:"udp_port"`
	}{UDPPort: udpPort}

	var out Peer
	if err := c.do(ctx, http.MethodPost, "/v1/peers/"+peerID+"/endpoint", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPeers returns every peer in a tenant, with the endpoints to send to them.
func (c *Client) ListPeers(ctx context.Context, tenantID string) ([]Peer, error) {
	var out struct {
		Peers []Peer `json:"peers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/tenants/"+tenantID+"/peers", nil, &out); err != nil {
		return nil, err
	}
	return out.Peers, nil
}

// do sends one request and decodes the response into out (which may be nil).
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call control plane: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{Status: resp.StatusCode, Message: apiMessage(payload, resp.Status)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// apiMessage pulls the `error` field out of a failure body, falling back to the
// status line.
func apiMessage(payload []byte, status string) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &body); err == nil && body.Error != "" {
		return body.Error
	}
	return status
}
