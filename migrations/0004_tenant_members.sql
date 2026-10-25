-- Who may manage a tenant from the web UI. user_id is the OIDC subject
-- (Keycloak's user id); email and name are copies for display.
CREATE TABLE tenant_members (
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    text NOT NULL,
    email      text,
    name       text,
    role       text NOT NULL CHECK (role IN ('owner', 'member')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);

CREATE INDEX tenant_members_user_id_idx ON tenant_members (user_id);

-- Invites are stored like enrollment tokens: only the code's SHA-256.
CREATE TABLE tenant_invites (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code_hash  bytea NOT NULL UNIQUE,
    role       text NOT NULL CHECK (role IN ('owner', 'member')),
    created_by text NOT NULL,
    max_uses   integer NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    uses       integer NOT NULL DEFAULT 0 CHECK (uses >= 0),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tenant_invites_tenant_id_idx ON tenant_invites (tenant_id);

-- With accounts anyone can create, a global name would let one user squat a
-- name or learn that another user's tenant exists.
ALTER TABLE tenants DROP CONSTRAINT tenants_name_key;
