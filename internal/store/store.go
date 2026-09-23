// Package store holds the control-plane persistence layer: tenants, the
// enrollment tokens they hand out, and the peers that registered with one.
package store

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when a uniqueness constraint rejects a write.
	ErrConflict = errors.New("conflict")
	// ErrPoolExhausted is returned when a tenant CIDR has no free address left.
	ErrPoolExhausted = errors.New("address pool exhausted")
	ErrTokenInvalid  = errors.New("invalid enrollment token")
)

// Store is a handle on the control-plane database.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to postgres and verifies the connection is usable.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

type Tenant struct {
	ID        string
	Name      string
	CIDR      netip.Prefix
	CreatedAt time.Time
}

type Peer struct {
	ID        string
	TenantID  string
	Name      string
	PublicKey string
	MeshIP    netip.Addr
	CreatedAt time.Time

	Endpoint          netip.AddrPort
	DirectReachable   bool
	EndpointUpdatedAt time.Time
}

type Token struct {
	ID        string
	TenantID  string
	Secret    string
	MaxUses   int
	Uses      int
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CreateTenant inserts a tenant. It returns ErrConflict if the name is taken.
func (s *Store) CreateTenant(ctx context.Context, name string, cidr netip.Prefix) (*Tenant, error) {
	t := Tenant{Name: name, CIDR: cidr}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tenants (name, cidr) VALUES ($1, $2) RETURNING id, created_at`,
		name, cidr,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &t, nil
}

func (s *Store) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	t := Tenant{ID: id}
	err := s.pool.QueryRow(ctx,
		`SELECT name, cidr, created_at FROM tenants WHERE id = $1`, id,
	).Scan(&t.Name, &t.CIDR, &t.CreatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &t, nil
}

// ListTenants returns every tenant, newest first.
func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, cidr, created_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CIDR, &t.CreatedAt); err != nil {
			return nil, wrap(err)
		}
		out = append(out, t)
	}
	return out, wrap(rows.Err())
}

// CreateToken issues an enrollment token. The plaintext secret is returned once,
// here, and is not recoverable afterwards.
func (s *Store) CreateToken(ctx context.Context, tenantID string, ttl time.Duration, maxUses int) (*Token, error) {
	secret, err := newSecret()
	if err != nil {
		return nil, err
	}

	tok := Token{
		TenantID:  tenantID,
		Secret:    secret,
		MaxUses:   maxUses,
		ExpiresAt: time.Now().Add(ttl),
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO enrollment_tokens (tenant_id, token_hash, max_uses, expires_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, expires_at`,
		tenantID, hashSecret(secret), maxUses, tok.ExpiresAt,
	).Scan(&tok.ID, &tok.CreatedAt, &tok.ExpiresAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &tok, nil
}

// peerColumns is the projection scanPeer expects.
const peerColumns = `id, tenant_id, name, public_key, mesh_ip, created_at,
                     public_ip, udp_port, direct_reachable, endpoint_updated_at`

// row is the part of pgx.Row and pgx.Rows that scanPeer needs.
type row interface{ Scan(dest ...any) error }

// scanPeer reads one peerColumns row, folding the nullable endpoint columns
// into Peer's zero values.
func scanPeer(r row) (Peer, error) {
	var (
		p       Peer
		ip      *netip.Addr
		port    *int32
		updated *time.Time
	)
	if err := r.Scan(&p.ID, &p.TenantID, &p.Name, &p.PublicKey, &p.MeshIP, &p.CreatedAt,
		&ip, &port, &p.DirectReachable, &updated); err != nil {
		return Peer{}, wrap(err)
	}
	if ip != nil && port != nil {
		p.Endpoint = netip.AddrPortFrom(*ip, uint16(*port))
	}
	if updated != nil {
		p.EndpointUpdatedAt = *updated
	}
	return p, nil
}

func (s *Store) GetPeer(ctx context.Context, id string) (*Peer, error) {
	p, err := scanPeer(s.pool.QueryRow(ctx,
		`SELECT `+peerColumns+` FROM peers WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetPeerEndpoint records where a peer says it can be reached, and the verdict
// of probing that endpoint.
func (s *Store) SetPeerEndpoint(ctx context.Context, peerID string, endpoint netip.AddrPort, reachable bool) (*Peer, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE peers
		    SET public_ip = $2, udp_port = $3, direct_reachable = $4, endpoint_updated_at = now()
		  WHERE id = $1`,
		peerID, endpoint.Addr(), int32(endpoint.Port()), reachable)
	if err != nil {
		return nil, wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetPeer(ctx, peerID)
}

// ListPeers returns the peers registered in a tenant, oldest first.
func (s *Store) ListPeers(ctx context.Context, tenantID string) ([]Peer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+peerColumns+` FROM peers WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	var out []Peer
	for rows.Next() {
		p, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, wrap(rows.Err())
}

func (s *Store) RegisterPeer(ctx context.Context, secret, name, publicKey string) (*Peer, *Tenant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, wrap(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tokenID, tenantID string
	err = tx.QueryRow(ctx,
		`SELECT id, tenant_id FROM enrollment_tokens
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > now()
		   AND uses < max_uses
		 FOR UPDATE`,
		hashSecret(secret),
	).Scan(&tokenID, &tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, nil, wrap(err)
	}

	tenant := Tenant{ID: tenantID}
	if err := tx.QueryRow(ctx,
		`SELECT name, cidr, created_at FROM tenants WHERE id = $1`, tenantID,
	).Scan(&tenant.Name, &tenant.CIDR, &tenant.CreatedAt); err != nil {
		return nil, nil, wrap(err)
	}

	used, err := usedAddrs(ctx, tx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	ip, err := nextFreeAddr(tenant.CIDR, used)
	if err != nil {
		return nil, nil, err
	}

	peer := Peer{TenantID: tenantID, Name: name, PublicKey: publicKey, MeshIP: ip}
	err = tx.QueryRow(ctx,
		`INSERT INTO peers (tenant_id, name, public_key, mesh_ip, token_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		tenantID, name, publicKey, ip, tokenID,
	).Scan(&peer.ID, &peer.CreatedAt)
	if err != nil {
		return nil, nil, wrap(err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE enrollment_tokens SET uses = uses + 1 WHERE id = $1`, tokenID,
	); err != nil {
		return nil, nil, wrap(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, wrap(err)
	}
	return &peer, &tenant, nil
}

func usedAddrs(ctx context.Context, tx pgx.Tx, tenantID string) (map[netip.Addr]bool, error) {
	rows, err := tx.Query(ctx, `SELECT mesh_ip FROM peers WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	used := make(map[netip.Addr]bool)
	for rows.Next() {
		var ip netip.Addr
		if err := rows.Scan(&ip); err != nil {
			return nil, wrap(err)
		}
		used[ip] = true
	}
	return used, wrap(rows.Err())
}

// wrap translates pgx and postgres errors into the package's sentinel errors.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
	}
	return err
}
