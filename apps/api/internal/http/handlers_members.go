package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"saas-template/api/internal/types"
)

// ── GET /members ──────────────────────────────────────────────────────────────

func (s *Server) handleListMembers(c *fiber.Ctx) error {
	tenantID := mustString(c, types.CtxTenantID)
	tx := txFromCtx(c)

	rows, err := tx.Query(c.UserContext(),
		`SELECT m.user_id, u.name, u.email, m.role, m.created_at
		 FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.tenant_id = $1
		 ORDER BY m.created_at ASC`,
		tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type member struct {
		UserID    string    `json:"userId"`
		Name      string    `json:"name"`
		Email     string    `json:"email"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"createdAt"`
	}
	result := []member{}
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &m.CreatedAt); err != nil {
			return err
		}
		result = append(result, m)
	}

	return c.JSON(result)
}

// ── PATCH /members/:uid ───────────────────────────────────────────────────────

func (s *Server) handleUpdateMember(c *fiber.Ctx) error {
	actorID := mustString(c, types.CtxUserID)
	actorRole := mustString(c, types.CtxRole)
	tenantID := mustString(c, types.CtxTenantID)
	targetUID := c.Params("uid")
	tx := txFromCtx(c)

	if actorID == targetUID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot change your own role"})
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil || req.Role == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role is required"})
	}
	if roleWeight := map[string]int{"owner": 4, "admin": 3, "member": 2, "viewer": 1}; roleWeight[req.Role] == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid role"})
	}

	// Only owners can assign the owner role.
	if req.Role == "owner" && actorRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only owners can assign the owner role"})
	}

	// Fetch current role of target.
	var currentRole string
	err := tx.QueryRow(c.UserContext(),
		`SELECT role FROM memberships WHERE user_id = $1 AND tenant_id = $2`,
		targetUID, tenantID).Scan(&currentRole)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "member not found"})
	}
	if err != nil {
		return err
	}

	// Prevent demoting the last owner.
	if currentRole == "owner" && req.Role != "owner" {
		var ownerCount int
		if err := tx.QueryRow(c.UserContext(),
			`SELECT COUNT(*) FROM memberships WHERE tenant_id = $1 AND role = 'owner'`,
			tenantID).Scan(&ownerCount); err != nil {
			return err
		}
		if ownerCount <= 1 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "cannot demote the only owner"})
		}
	}

	if _, err := tx.Exec(c.UserContext(),
		`UPDATE memberships SET role = $1 WHERE user_id = $2 AND tenant_id = $3`,
		req.Role, targetUID, tenantID); err != nil {
		return err
	}

	writeAudit(c, actorID, &targetUID, "role_changed", map[string]any{
		"old_role": currentRole,
		"new_role": req.Role,
	})

	return c.SendStatus(fiber.StatusNoContent)
}

// ── DELETE /members/:uid ──────────────────────────────────────────────────────

func (s *Server) handleRemoveMember(c *fiber.Ctx) error {
	actorID := mustString(c, types.CtxUserID)
	actorRole := mustString(c, types.CtxRole)
	tenantID := mustString(c, types.CtxTenantID)
	targetUID := c.Params("uid")
	tx := txFromCtx(c)

	isSelf := actorID == targetUID

	// Non-self removal requires admin+.
	if !isSelf && !types.RoleAtLeast(actorRole, "admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient permissions"})
	}

	// Fetch target's current role and email (for audit meta).
	var targetRole, targetEmail string
	err := tx.QueryRow(c.UserContext(),
		`SELECT m.role, u.email FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.user_id = $1 AND m.tenant_id = $2`,
		targetUID, tenantID).Scan(&targetRole, &targetEmail)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "member not found"})
	}
	if err != nil {
		return err
	}

	// Admin cannot remove other admins or owners.
	if !isSelf && actorRole == "admin" && types.RoleAtLeast(targetRole, "admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admins can only remove members and viewers"})
	}

	// Prevent removing the last owner.
	if targetRole == "owner" {
		var ownerCount int
		if err := tx.QueryRow(c.UserContext(),
			`SELECT COUNT(*) FROM memberships WHERE tenant_id = $1 AND role = 'owner'`,
			tenantID).Scan(&ownerCount); err != nil {
			return err
		}
		if ownerCount <= 1 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "cannot remove the only owner"})
		}
	}

	if _, err := tx.Exec(c.UserContext(),
		`DELETE FROM memberships WHERE user_id = $1 AND tenant_id = $2`,
		targetUID, tenantID); err != nil {
		return err
	}

	writeAudit(c, actorID, &targetUID, "member_removed", map[string]any{
		"email": targetEmail,
		"role":  targetRole,
	})

	return c.SendStatus(fiber.StatusNoContent)
}
