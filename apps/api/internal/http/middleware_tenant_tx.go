package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"saas-template/api/internal/types"
)

// TenantTx begins a DB transaction, sets the app.tenant_id GUC (local to the
// transaction), and stores the pgx.Tx in c.Locals for handlers to use.
// This is what makes PostgreSQL Row-Level Security work per-request:
//
//	set_config('app.tenant_id', '<uuid>', true)  →  RLS policies filter all queries automatically.
//
// The transaction is committed on success and rolled back on any error.
func TenantTx(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID, ok := c.Locals(types.CtxTenantID).(string)
		if !ok || tenantID == "" {
			return fiber.ErrUnauthorized
		}

		tx, err := db.Begin(c.UserContext())
		if err != nil {
			return err
		}

		// set_config(name, value, is_local=true) — parameterized, safe from injection.
		// is_local=true makes it transaction-scoped, not session-scoped.
		if _, err := tx.Exec(c.UserContext(),
			`SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
			tx.Rollback(c.UserContext())
			return err
		}

		c.Locals(types.CtxTx, tx)

		handlerErr := c.Next()

		if handlerErr != nil {
			tx.Rollback(c.UserContext())
			return handlerErr
		}

		if err := tx.Commit(c.UserContext()); err != nil {
			tx.Rollback(c.UserContext())
			// If the handler already wrote a 4xx response (e.g. 409 from a unique-violation
			// that it caught and turned into a conflict reply), the transaction is in an
			// aborted state and PostgreSQL will always reject the COMMIT. Don't override
			// the handler's intentional 4xx with a 500 from the commit error.
			if code := c.Response().StatusCode(); code >= 400 && code < 500 {
				return nil
			}
			return err
		}
		return nil
	}
}
