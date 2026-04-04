package http

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"saas-template/api/internal/types"
)

// writeAudit inserts an audit event within the current request's TenantTx transaction.
// The RLS GUC is already set by TenantTx middleware, so the insert is automatically
// scoped to the correct tenant. Best-effort: errors are silently dropped.
func writeAudit(c *fiber.Ctx, actorID string, targetID *string, action string, meta map[string]any) {
	tx := txFromCtx(c)
	if tx == nil {
		return
	}
	tenantID := mustString(c, types.CtxTenantID)

	var metaBytes []byte
	if meta != nil {
		metaBytes, _ = json.Marshal(meta)
	}

	tx.Exec(c.UserContext(),
		`INSERT INTO audit_events (tenant_id, actor_id, target_id, action, meta)
		 VALUES ($1, $2, $3, $4, $5::jsonb)`,
		tenantID, actorID, targetID, action, metaBytes)
}

// writeAuditDirect inserts an audit event outside of a TenantTx request.
// Used by handlers that manage their own DB connection (e.g., handleSwitch,
// handleChangePassword). Acquires a connection, sets the GUC manually, inserts,
// then releases. Best-effort: errors are silently dropped.
func (s *Server) writeAuditDirect(ctx context.Context, tenantID, actorID string, targetID *string, action string, meta map[string]any) {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		return
	}

	var metaBytes []byte
	if meta != nil {
		metaBytes, _ = json.Marshal(meta)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO audit_events (tenant_id, actor_id, target_id, action, meta)
		 VALUES ($1, $2, $3, $4, $5::jsonb)`,
		tenantID, actorID, targetID, action, metaBytes); err != nil {
		return
	}

	tx.Commit(ctx)
}
