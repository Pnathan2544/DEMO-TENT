package http

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"saas-template/api/internal/security"
	"saas-template/api/internal/types"
)

const inviteTTL = 48 * time.Hour

// ── POST /invites ─────────────────────────────────────────────────────────────

func (s *Server) handleCreateInvite(c *fiber.Ctx) error {
	actorID := mustString(c, types.CtxUserID)
	tenantID := mustString(c, types.CtxTenantID)
	tx := txFromCtx(c)

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Email == "" || req.Role == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and role are required"})
	}
	validRoles := map[string]bool{"admin": true, "member": true, "viewer": true}
	if !validRoles[req.Role] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be admin, member, or viewer"})
	}

	// Check email is not already a member.
	var existingCount int
	if err := tx.QueryRow(c.UserContext(),
		`SELECT COUNT(*) FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.tenant_id = $1 AND u.email = $2`,
		tenantID, req.Email).Scan(&existingCount); err != nil {
		return err
	}
	if existingCount > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user is already a member"})
	}

	rawToken, tokenHash, err := security.Generate()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(inviteTTL)
	var inviteID string
	err = tx.QueryRow(c.UserContext(),
		`INSERT INTO invites (tenant_id, invited_by, email, role, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		tenantID, actorID, req.Email, req.Role, tokenHash, expiresAt).Scan(&inviteID)
	if err != nil {
		if isDuplicateKey(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "an invite for this email already exists"})
		}
		return err
	}

	// Fetch inviter name and tenant name for the email body.
	var inviterName, tenantName string
	tx.QueryRow(c.UserContext(),
		`SELECT u.name, t.name FROM users u, tenants t WHERE u.id = $1 AND t.id = $2`,
		actorID, tenantID).Scan(&inviterName, &tenantName)

	inviteURL := fmt.Sprintf("%s/invites/accept?token=%s", s.cfg.FrontendURL, rawToken)
	// Send email best-effort — don't fail the request if mail is misconfigured.
	_ = s.mailer.SendInvite(c.UserContext(), req.Email, inviteURL, tenantName, inviterName)

	writeAudit(c, actorID, nil, "invite_created", map[string]any{
		"invite_email": req.Email,
		"role":         req.Role,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":        inviteID,
		"email":     req.Email,
		"role":      req.Role,
		"expiresAt": expiresAt,
	})
}

// ── GET /invites ──────────────────────────────────────────────────────────────

func (s *Server) handleListInvites(c *fiber.Ctx) error {
	tenantID := mustString(c, types.CtxTenantID)
	tx := txFromCtx(c)

	rows, err := tx.Query(c.UserContext(),
		`SELECT i.id, i.email, i.role, u.name AS inviter_name, i.expires_at, i.created_at
		 FROM invites i JOIN users u ON u.id = i.invited_by
		 WHERE i.tenant_id = $1 AND i.expires_at > NOW()
		 ORDER BY i.created_at DESC`,
		tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type inviteEntry struct {
		ID          string    `json:"id"`
		Email       string    `json:"email"`
		Role        string    `json:"role"`
		InviterName string    `json:"inviterName"`
		ExpiresAt   time.Time `json:"expiresAt"`
		CreatedAt   time.Time `json:"createdAt"`
	}
	result := []inviteEntry{}
	for rows.Next() {
		var e inviteEntry
		if err := rows.Scan(&e.ID, &e.Email, &e.Role, &e.InviterName, &e.ExpiresAt, &e.CreatedAt); err != nil {
			return err
		}
		result = append(result, e)
	}

	return c.JSON(result)
}

// ── POST /invites/accept ──────────────────────────────────────────────────────
// AuthRequired only — no TenantTx (user doesn't belong to the target tenant yet).

func (s *Server) handleAcceptInvite(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)

	var req struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&req); err != nil || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	tokenHash := security.HashToken(req.Token)
	var inviteID, tenantID, inviteEmail, role string
	var expiresAt time.Time
	err := s.db.QueryRow(c.UserContext(),
		`SELECT id, tenant_id, email, role, expires_at FROM invites WHERE token_hash = $1`,
		tokenHash).Scan(&inviteID, &tenantID, &inviteEmail, &role, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invite not found or already used"})
	}
	if err != nil {
		return err
	}
	if time.Now().After(expiresAt) {
		return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "invite has expired"})
	}

	// Verify logged-in user's email matches the invite.
	var userEmail string
	if err := s.db.QueryRow(c.UserContext(),
		`SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail); err != nil {
		return err
	}
	if userEmail != inviteEmail {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "this invite was sent to a different email address"})
	}

	// Insert membership and consume the invite atomically.
	tx, err := s.db.Begin(c.UserContext())
	if err != nil {
		return err
	}
	defer tx.Rollback(c.UserContext())

	_, err = tx.Exec(c.UserContext(),
		`INSERT INTO memberships (user_id, tenant_id, role) VALUES ($1, $2, $3)`,
		userID, tenantID, role)
	if err != nil {
		if isDuplicateKey(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "already a member of this workspace"})
		}
		return err
	}

	if _, err = tx.Exec(c.UserContext(),
		`DELETE FROM invites WHERE id = $1`, inviteID); err != nil {
		return err
	}

	if err = tx.Commit(c.UserContext()); err != nil {
		return err
	}

	var tenantName, tenantSlug string
	s.db.QueryRow(c.UserContext(),
		`SELECT name, slug FROM tenants WHERE id = $1`, tenantID).Scan(&tenantName, &tenantSlug)

	return c.JSON(fiber.Map{
		"tenantId":   tenantID,
		"tenantName": tenantName,
		"tenantSlug": tenantSlug,
		"role":       role,
	})
}

// ── DELETE /invites/:id ───────────────────────────────────────────────────────

func (s *Server) handleRevokeInvite(c *fiber.Ctx) error {
	actorID := mustString(c, types.CtxUserID)
	tenantID := mustString(c, types.CtxTenantID)
	inviteID := c.Params("id")
	tx := txFromCtx(c)

	// Fetch invite email for audit meta before deleting.
	var inviteEmail string
	err := tx.QueryRow(c.UserContext(),
		`SELECT email FROM invites WHERE id = $1 AND tenant_id = $2`,
		inviteID, tenantID).Scan(&inviteEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invite not found"})
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(c.UserContext(),
		`DELETE FROM invites WHERE id = $1 AND tenant_id = $2`,
		inviteID, tenantID); err != nil {
		return err
	}

	writeAudit(c, actorID, nil, "invite_revoked", map[string]any{
		"invite_email": inviteEmail,
	})

	return c.SendStatus(fiber.StatusNoContent)
}
