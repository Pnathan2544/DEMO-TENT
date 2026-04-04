package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	stripe "github.com/stripe/stripe-go/v76"
	stripebpsession "github.com/stripe/stripe-go/v76/billingportal/session"
	"saas-template/api/internal/security"
	"saas-template/api/internal/types"
)

// ── GET /me ───────────────────────────────────────────────────────────────────

func (s *Server) handleGetMe(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)
	tenantID := mustString(c, types.CtxTenantID)
	role := mustString(c, types.CtxRole)

	var id, email, name string
	var createdAt time.Time
	err := s.db.QueryRow(c.UserContext(),
		`SELECT id, email, name, created_at FROM users WHERE id = $1`, userID).
		Scan(&id, &email, &name, &createdAt)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"id":        id,
		"email":     email,
		"name":      name,
		"role":      role,
		"tenantId":  tenantID,
		"createdAt": createdAt,
	})
}

// ── PUT /me ───────────────────────────────────────────────────────────────────

func (s *Server) handleUpdateMe(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)

	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" && req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provide name or email to update"})
	}

	// COALESCE(NULLIF($1,''), col) leaves the column unchanged when the field is omitted.
	_, err := s.db.Exec(c.UserContext(),
		`UPDATE users
		    SET name      = COALESCE(NULLIF($1, ''), name),
		        email     = COALESCE(NULLIF($2, ''), email),
		        updated_at = NOW()
		  WHERE id = $3`,
		req.Name, req.Email, userID)
	if err != nil {
		if isDuplicateKey(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already in use"})
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── PUT /me/password ──────────────────────────────────────────────────────────

func (s *Server) handleChangePassword(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)
	tenantID := mustString(c, types.CtxTenantID)

	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Current == "" || req.New == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "current and new passwords are required"})
	}
	if len(req.New) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "new password must be at least 8 characters"})
	}

	var hash string
	if err := s.db.QueryRow(c.UserContext(),
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		return err
	}
	if !security.Verify(req.Current, hash) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "current password is incorrect"})
	}

	newHash, err := security.Hash(req.New)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec(c.UserContext(),
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID); err != nil {
		return err
	}

	s.writeAuditDirect(c.UserContext(), tenantID, userID, nil, "password_changed", nil)

	return c.SendStatus(fiber.StatusNoContent)
}

// ── GET /me/invites ───────────────────────────────────────────────────────────

func (s *Server) handleMyInvites(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)

	var email string
	if err := s.db.QueryRow(c.UserContext(),
		`SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		return err
	}

	rows, err := s.db.Query(c.UserContext(),
		`SELECT i.id, i.tenant_id, t.name AS tenant_name, i.role,
		        u.name AS inviter_name, i.expires_at, i.created_at
		 FROM invites i
		 JOIN tenants t ON t.id = i.tenant_id
		 JOIN users   u ON u.id = i.invited_by
		 WHERE i.email = $1 AND i.expires_at > NOW()
		 ORDER BY i.created_at DESC`,
		email)
	if err != nil {
		return err
	}
	defer rows.Close()

	type inviteEntry struct {
		ID          string    `json:"id"`
		TenantID    string    `json:"tenantId"`
		TenantName  string    `json:"tenantName"`
		Role        string    `json:"role"`
		InviterName string    `json:"inviterName"`
		ExpiresAt   time.Time `json:"expiresAt"`
		CreatedAt   time.Time `json:"createdAt"`
	}
	result := []inviteEntry{}
	for rows.Next() {
		var e inviteEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.TenantName, &e.Role,
			&e.InviterName, &e.ExpiresAt, &e.CreatedAt); err != nil {
			return err
		}
		result = append(result, e)
	}

	return c.JSON(result)
}

// ── GET /me/billing ───────────────────────────────────────────────────────────

func (s *Server) handleGetBilling(c *fiber.Ctx) error {
	tenantID := mustString(c, types.CtxTenantID)

	var plan string
	var subID *string
	err := s.db.QueryRow(c.UserContext(),
		`SELECT stripe_plan, stripe_subscription_id FROM tenants WHERE id = $1`, tenantID).
		Scan(&plan, &subID)
	if err != nil {
		return err
	}

	status := "active"
	if subID == nil {
		status = "free"
	}

	return c.JSON(fiber.Map{
		"plan":   plan,
		"status": status,
	})
}

// ── POST /me/billing/portal ───────────────────────────────────────────────────

func (s *Server) handleBillingPortal(c *fiber.Ctx) error {
	tenantID := mustString(c, types.CtxTenantID)

	var customerID *string
	err := s.db.QueryRow(c.UserContext(),
		`SELECT stripe_customer_id FROM tenants WHERE id = $1`, tenantID).
		Scan(&customerID)
	if err != nil || customerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no billing account found"})
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(*customerID),
		ReturnURL: stripe.String(s.cfg.FrontendURL + "/settings/billing"),
	}
	sess, err := stripebpsession.New(params)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not create billing session"})
	}

	return c.JSON(fiber.Map{"url": sess.URL})
}
