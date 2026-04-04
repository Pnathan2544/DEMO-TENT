-- Run as a PostgreSQL superuser (e.g. postgres).
-- Milestone B: tenant switching, member management, invites, audit log.

-- ── 1. Pin active tenant to each refresh session ──────────────────────────────
ALTER TABLE refresh_tokens
  ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- ── 2. Invites (no RLS — acceptance happens before tenant context is established) ─
CREATE TABLE invites (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  invited_by UUID NOT NULL REFERENCES users(id),
  email      TEXT NOT NULL,
  role       TEXT NOT NULL DEFAULT 'member'
               CHECK (role IN ('admin', 'member', 'viewer')), -- owner not invitable
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One active invite per email per tenant.
CREATE UNIQUE INDEX idx_invites_pending    ON invites (tenant_id, email);
CREATE INDEX        idx_invites_token_hash ON invites (token_hash);
CREATE INDEX        idx_invites_email      ON invites (email);

-- ── 3. Audit events (RLS enforced via app.tenant_id GUC) ─────────────────────
CREATE TABLE audit_events (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  actor_id   UUID NOT NULL REFERENCES users(id),
  target_id  UUID REFERENCES users(id),
  action     TEXT NOT NULL,
  -- actions: invite_created, invite_revoked, role_changed,
  --          member_removed, tenant_switched, password_changed
  meta       JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_events_tenant_isolation ON audit_events
  USING      (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE INDEX idx_audit_events_tenant_id  ON audit_events (tenant_id);
CREATE INDEX idx_audit_events_created_at ON audit_events (created_at DESC);

-- Grant new tables to app role.
GRANT SELECT, INSERT, UPDATE, DELETE ON invites      TO saas_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON audit_events TO saas_app;
