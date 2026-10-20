package store

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInviteInvalid = errors.New("invalid invite")
	ErrLastOwner     = errors.New("a tenant needs at least one owner")
	ErrClaimed       = errors.New("tenant already has members")
)

const invitePrefix = "m2mi_"

const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type User struct {
	ID    string
	Email string
	Name  string
}

type Member struct {
	TenantID  string
	UserID    string
	Email     string
	Name      string
	Role      string
	CreatedAt time.Time
}

type Membership struct {
	Tenant
	Role string
}

type Invite struct {
	ID        string
	TenantID  string
	Code      string
	Role      string
	MaxUses   int
	ExpiresAt time.Time
}

func (s *Store) CreateOwnedTenant(ctx context.Context, name string, cidr netip.Prefix, u User) (*Tenant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	t := Tenant{Name: name, CIDR: cidr}
	if err := tx.QueryRow(ctx,
		`INSERT INTO tenants (name, cidr) VALUES ($1, $2) RETURNING id, created_at`,
		name, cidr,
	).Scan(&t.ID, &t.CreatedAt); err != nil {
		return nil, wrap(err)
	}
	if err := addMember(ctx, tx, t.ID, u, RoleOwner); err != nil {
		return nil, err
	}
	return &t, wrap(tx.Commit(ctx))
}

// ListMemberships returns the tenants u belongs to, oldest first, and
// refreshes u's display name and email on them.
func (s *Store) ListMemberships(ctx context.Context, u User) ([]Membership, error) {
	if _, err := s.pool.Exec(ctx,
		`UPDATE tenant_members SET email = $2, name = $3
		  WHERE user_id = $1 AND (email IS DISTINCT FROM $2 OR name IS DISTINCT FROM $3)`,
		u.ID, nullable(u.Email), nullable(u.Name)); err != nil {
		return nil, wrap(err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.name, t.cidr, t.created_at, m.role
		   FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id
		  WHERE m.user_id = $1
		  ORDER BY t.created_at`, u.ID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.ID, &m.Name, &m.CIDR, &m.CreatedAt, &m.Role); err != nil {
			return nil, wrap(err)
		}
		out = append(out, m)
	}
	return out, wrap(rows.Err())
}

// GetMembership returns tenantID with userID's role in it, or ErrNotFound
// when userID is not a member: callers cannot tell "no such tenant" from
// "not yours".
func (s *Store) GetMembership(ctx context.Context, tenantID, userID string) (*Membership, error) {
	var m Membership
	err := s.pool.QueryRow(ctx,
		`SELECT t.id, t.name, t.cidr, t.created_at, m.role
		   FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id
		  WHERE m.tenant_id = $1 AND m.user_id = $2`, tenantID, userID,
	).Scan(&m.ID, &m.Name, &m.CIDR, &m.CreatedAt, &m.Role)
	if err != nil {
		return nil, wrap(err)
	}
	return &m, nil
}

func (s *Store) ListMembers(ctx context.Context, tenantID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT tenant_id, user_id, coalesce(email, ''), coalesce(name, ''), role, created_at
		   FROM tenant_members WHERE tenant_id = $1
		  ORDER BY role = 'owner' DESC, created_at`, tenantID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.TenantID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt); err != nil {
			return nil, wrap(err)
		}
		out = append(out, m)
	}
	return out, wrap(rows.Err())
}

func (s *Store) RemoveMember(ctx context.Context, tenantID, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrap(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the tenant's rows so two owners leaving at once cannot both pass the check.
	var role string
	err = tx.QueryRow(ctx,
		`SELECT role FROM tenant_members WHERE tenant_id = $1 AND user_id = $2 FOR UPDATE`,
		tenantID, userID).Scan(&role)
	if err != nil {
		return wrap(err)
	}
	if role == RoleOwner {
		var owners int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM (SELECT 1 FROM tenant_members
			   WHERE tenant_id = $1 AND role = 'owner' FOR UPDATE) o`, tenantID).Scan(&owners); err != nil {
			return wrap(err)
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM tenant_members WHERE tenant_id = $1 AND user_id = $2`, tenantID, userID); err != nil {
		return wrap(err)
	}
	return wrap(tx.Commit(ctx))
}

// CreateInvite issues an invite code. Like an enrollment token, the code is
// returned once and only its hash is kept.
func (s *Store) CreateInvite(ctx context.Context, tenantID, role, createdBy string, ttl time.Duration, maxUses int) (*Invite, error) {
	code, err := newSecret()
	if err != nil {
		return nil, err
	}
	code = invitePrefix + code[len(tokenPrefix):]

	inv := Invite{TenantID: tenantID, Code: code, Role: role, MaxUses: maxUses}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO tenant_invites (tenant_id, code_hash, role, created_by, max_uses, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, expires_at`,
		tenantID, hashSecret(code), role, createdBy, maxUses, time.Now().Add(ttl),
	).Scan(&inv.ID, &inv.ExpiresAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &inv, nil
}

// AcceptInvite adds u to the invite's tenant. Someone already in it keeps
// their role, and the invite is not used up for them.
func (s *Store) AcceptInvite(ctx context.Context, code string, u User) (*Membership, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inviteID, tenantID, role string
	err = tx.QueryRow(ctx,
		`SELECT id, tenant_id, role FROM tenant_invites
		  WHERE code_hash = $1 AND expires_at > now() AND uses < max_uses
		  FOR UPDATE`, hashSecret(code),
	).Scan(&inviteID, &tenantID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, wrap(err)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO tenant_members (tenant_id, user_id, email, name, role)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		tenantID, u.ID, nullable(u.Email), nullable(u.Name), role)
	if err != nil {
		return nil, wrap(err)
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx,
			`UPDATE tenant_invites SET uses = uses + 1 WHERE id = $1`, inviteID); err != nil {
			return nil, wrap(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrap(err)
	}
	return s.GetMembership(ctx, tenantID, u.ID)
}

// ClaimTenant makes u the owner of a tenant nobody belongs to yet: one made
// with `meshd setup`, or from before memberships existed.
func (s *Store) ClaimTenant(ctx context.Context, tenantID string, u User) (*Membership, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The tenant row lock serializes concurrent claims.
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM tenants WHERE id = $1 FOR UPDATE`, tenantID).Scan(&id); err != nil {
		return nil, wrap(err)
	}
	var taken bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM tenant_members WHERE tenant_id = $1)`, tenantID).Scan(&taken); err != nil {
		return nil, wrap(err)
	}
	if taken {
		return nil, ErrClaimed
	}
	if err := addMember(ctx, tx, tenantID, u, RoleOwner); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrap(err)
	}
	return s.GetMembership(ctx, tenantID, u.ID)
}

func addMember(ctx context.Context, tx pgx.Tx, tenantID string, u User, role string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO tenant_members (tenant_id, user_id, email, name, role) VALUES ($1, $2, $3, $4, $5)`,
		tenantID, u.ID, nullable(u.Email), nullable(u.Name), role)
	return wrap(err)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
