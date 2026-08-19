CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL UNIQUE,
    cidr       cidr NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE enrollment_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    max_uses   integer NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    uses       integer NOT NULL DEFAULT 0 CHECK (uses >= 0),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX enrollment_tokens_tenant_id_idx ON enrollment_tokens (tenant_id);

CREATE TABLE peers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name       text NOT NULL,
    public_key text NOT NULL,
    mesh_ip    inet NOT NULL,
    token_id   uuid REFERENCES enrollment_tokens(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, mesh_ip),
    UNIQUE (tenant_id, public_key)
);

CREATE INDEX peers_tenant_id_idx ON peers (tenant_id);
