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

// maxResponseBytes caps what we read back; every payload here is a few fields.
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

// CreateTenant posts to /v1/tenants. An empty cidr lets the server pick its
// default.
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

// CreateToken mints an enrollment token for a tenant. The plaintext secret it
// returns is not recoverable afterwards.
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

// RegisterPeer redeems an enrollment token and returns the mesh address the
// node should configure.
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
	defer resp.Body.Close()

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
// status line when the body is not the shape we expect.
func apiMessage(payload []byte, status string) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &body); err == nil && body.Error != "" {
		return body.Error
	}
	return status
}
