-- Run as a PostgreSQL superuser (e.g. postgres).
-- The API connects as `saas_app` (non-superuser) so RLS is enforced automatically.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ── Core tables ───────────────────────────────────────────────────────────────

CREATE TABLE tenants (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                   TEXT NOT NULL,
  slug                   TEXT UNIQUE NOT NULL,
  stripe_customer_id     TEXT,
  stripe_subscription_id TEXT,
  stripe_plan            TEXT NOT NULL DEFAULT 'free',
  created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  name          TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users <-> Tenants (many-to-many with role). Supports multi-tenant users.
CREATE TABLE memberships (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'member'
               CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, tenant_id)
);

-- Refresh tokens stored as SHA-256 hashes (rotate on every use).
CREATE TABLE refresh_tokens (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Example tenant-scoped resource.
CREATE TABLE projects (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  description TEXT,
  created_by  UUID NOT NULL REFERENCES users(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Indexes ───────────────────────────────────────────────────────────────────

CREATE INDEX idx_memberships_user_id    ON memberships(user_id);
CREATE INDEX idx_memberships_tenant_id  ON memberships(tenant_id);
CREATE INDEX idx_projects_tenant_id     ON projects(tenant_id);
CREATE INDEX idx_refresh_tokens_user_id    ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

-- ── Row-Level Security ────────────────────────────────────────────────────────
-- Only applied to tenant-scoped resource tables (not auth/identity tables).
-- The app sets `app.tenant_id` per-transaction via:
--   SELECT set_config('app.tenant_id', '<uuid>', true)
-- If the GUC is not set, current_setting returns NULL and all rows are hidden (safe default).

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
CREATE POLICY projects_tenant_isolation ON projects
  USING      (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- ── App role (non-superuser → RLS applies) ────────────────────────────────────

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'saas_app') THEN
    CREATE ROLE saas_app LOGIN PASSWORD 'saas_secret';
  END IF;
END$$;

GRANT CONNECT ON DATABASE saas_db TO saas_app;
GRANT USAGE   ON SCHEMA public TO saas_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO saas_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO saas_app;
