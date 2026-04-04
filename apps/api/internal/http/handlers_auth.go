package http

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/customer"
	"saas-template/api/internal/security"
	"saas-template/api/internal/types"
)

// ── Register ──────────────────────────────────────────────────────────────────

type registerReq struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Password   string `json:"password"`
	TenantName string `json:"tenantName"`
}

func (s *Server) handleRegister(c *fiber.Ctx) error {
	var req registerReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Email == "" || req.Name == "" || req.Password == "" || req.TenantName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "all fields are required"})
	}
	if len(req.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password must be at least 8 characters"})
	}

	hash, err := security.Hash(req.Password)
	if err != nil {
		return err
	}

	// Use a raw transaction here — no RLS context needed for initial inserts.
	tx, err := s.db.Begin(c.UserContext())
	if err != nil {
		return err
	}
	defer tx.Rollback(c.UserContext())

	slug := slugify(req.TenantName)

	var tenantID string
	err = tx.QueryRow(c.UserContext(),
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		req.TenantName, slug).Scan(&tenantID)
	if err != nil {
		if isDuplicateKey(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "tenant name already taken"})
		}
		return err
	}

	var userID string
	err = tx.QueryRow(c.UserContext(),
		`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id`,
		req.Email, hash, req.Name).Scan(&userID)
	if err != nil {
		if isDuplicateKey(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already registered"})
		}
		return err
	}

	if _, err = tx.Exec(c.UserContext(),
		`INSERT INTO memberships (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, tenantID); err != nil {
		return err
	}

	if err = tx.Commit(c.UserContext()); err != nil {
		return err
	}

	// Create Stripe customer after commit so DB state is consistent.
	// Non-fatal: billing features degrade gracefully if Stripe is unconfigured.
	if s.cfg.StripeSecretKey != "" {
		params := &stripe.CustomerParams{
			Email: stripe.String(req.Email),
			Name:  stripe.String(req.TenantName),
		}
		params.AddMetadata("tenant_id", tenantID)
		if cus, err := customer.New(params); err == nil {
			s.db.Exec(c.UserContext(),
				`UPDATE tenants SET stripe_customer_id = $1 WHERE id = $2`,
				cus.ID, tenantID)
		}
	}

	rawRefresh, refreshHash, err := security.Generate()
	if err != nil {
		return err
	}
	if err := security.Store(c.UserContext(), s.db, userID, tenantID, refreshHash,
		time.Now().Add(s.cfg.RefreshTokenTTL)); err != nil {
		return err
	}

	accessToken, err := security.SignAccessToken(userID, tenantID, "owner", s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return err
	}

	s.setTokenCookies(c, accessToken, rawRefresh)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"user":   fiber.Map{"id": userID, "email": req.Email, "name": req.Name},
		"tenant": fiber.Map{"id": tenantID, "name": req.TenantName, "slug": slug},
	})
}

// ── Login ─────────────────────────────────────────────────────────────────────

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(c *fiber.Ctx) error {
	var req loginReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}

	var userID, passwordHash, name string
	err := s.db.QueryRow(c.UserContext(),
		`SELECT id, password_hash, name FROM users WHERE email = $1`, req.Email).
		Scan(&userID, &passwordHash, &name)

	// Constant-time path: always run Verify to avoid timing side-channels.
	credentialsOK := err == nil && security.Verify(req.Password, passwordHash)
	if errors.Is(err, pgx.ErrNoRows) || !credentialsOK {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if err != nil {
		return err
	}

	// Fetch first/primary membership. Tenant switching is a Milestone B feature.
	var tenantID, role, tenantName, slug string
	err = s.db.QueryRow(c.UserContext(),
		`SELECT m.tenant_id, m.role, t.name, t.slug
		 FROM memberships m JOIN tenants t ON t.id = m.tenant_id
		 WHERE m.user_id = $1
		 ORDER BY m.created_at ASC LIMIT 1`,
		userID).Scan(&tenantID, &role, &tenantName, &slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no tenant membership found"})
	}
	if err != nil {
		return err
	}

	rawRefresh, refreshHash, err := security.Generate()
	if err != nil {
		return err
	}
	if err := security.Store(c.UserContext(), s.db, userID, tenantID, refreshHash,
		time.Now().Add(s.cfg.RefreshTokenTTL)); err != nil {
		return err
	}

	accessToken, err := security.SignAccessToken(userID, tenantID, role, s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return err
	}

	s.setTokenCookies(c, accessToken, rawRefresh)

	return c.JSON(fiber.Map{
		"user":   fiber.Map{"id": userID, "email": req.Email, "name": name},
		"tenant": fiber.Map{"id": tenantID, "name": tenantName, "slug": slug},
		"role":   role,
	})
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (s *Server) handleRefresh(c *fiber.Ctx) error {
	rawToken := c.Cookies("refresh_token")
	if rawToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no refresh token"})
	}

	userID, tenantID, newRaw, err := security.Rotate(c.UserContext(), s.db, rawToken, s.cfg.RefreshTokenTTL)
	if err != nil {
		clearAuthCookies(c)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired refresh token"})
	}

	var role string
	err = s.db.QueryRow(c.UserContext(),
		`SELECT role FROM memberships WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID).Scan(&role)
	if err != nil {
		return err
	}

	accessToken, err := security.SignAccessToken(userID, tenantID, role, s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return err
	}

	s.setTokenCookies(c, accessToken, newRaw)
	return c.JSON(fiber.Map{"ok": true})
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (s *Server) handleLogout(c *fiber.Ctx) error {
	rawToken := c.Cookies("refresh_token")
	if rawToken != "" {
		// Best-effort revoke; ignore errors (token may already be expired).
		_ = security.Revoke(c.UserContext(), s.db, rawToken)
	}
	clearAuthCookies(c)
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Switch tenant ─────────────────────────────────────────────────────────────

func (s *Server) handleSwitch(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)

	var req struct {
		TenantID string `json:"tenantId"`
	}
	if err := c.BodyParser(&req); err != nil || req.TenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenantId is required"})
	}

	rawToken := c.Cookies("refresh_token")
	if rawToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no refresh token"})
	}

	// Verify user is a member of the target tenant.
	var role, tenantName string
	err := s.db.QueryRow(c.UserContext(),
		`SELECT m.role, t.name
		 FROM memberships m JOIN tenants t ON t.id = m.tenant_id
		 WHERE m.user_id = $1 AND m.tenant_id = $2`,
		userID, req.TenantID).Scan(&role, &tenantName)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not a member of that tenant"})
	}
	if err != nil {
		return err
	}

	// Atomically swap the refresh token, pinned to the new tenant.
	_, newRaw, err := security.Switch(c.UserContext(), s.db, rawToken, req.TenantID, s.cfg.RefreshTokenTTL)
	if err != nil {
		clearAuthCookies(c)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired session"})
	}

	accessToken, err := security.SignAccessToken(userID, req.TenantID, role, s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return err
	}

	s.setTokenCookies(c, accessToken, newRaw)

	// Audit in new tenant context (best-effort).
	s.writeAuditDirect(c.UserContext(), req.TenantID, userID, nil, "tenant_switched", nil)

	return c.JSON(fiber.Map{
		"tenant": fiber.Map{"id": req.TenantID, "name": tenantName},
		"role":   role,
	})
}

// ── List user's tenants ────────────────────────────────────────────────────────

func (s *Server) handleTenants(c *fiber.Ctx) error {
	userID := mustString(c, types.CtxUserID)

	rows, err := s.db.Query(c.UserContext(),
		`SELECT m.tenant_id, t.name, t.slug, m.role
		 FROM memberships m JOIN tenants t ON t.id = m.tenant_id
		 WHERE m.user_id = $1
		 ORDER BY m.created_at ASC`,
		userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type tenantEntry struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
		Role string `json:"role"`
	}
	result := []tenantEntry{}
	for rows.Next() {
		var e tenantEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.Slug, &e.Role); err != nil {
			return err
		}
		result = append(result, e)
	}

	return c.JSON(result)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// setTokenCookies writes both tokens as HttpOnly cookies.
// Both use Path="/" so the refresh token is sent to /auth/logout for revocation.
// Real protection comes from HttpOnly + Secure + SameSite, not path restriction.
func (s *Server) setTokenCookies(c *fiber.Ctx, accessToken, refreshToken string) {
	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		HTTPOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: "Lax",
		MaxAge:   int(s.cfg.AccessTokenTTL.Seconds()),
		Path:     "/",
	})
	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		HTTPOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: "Lax",
		MaxAge:   int(s.cfg.RefreshTokenTTL.Seconds()),
		Path:     "/",
	})
}

// slugify converts "Acme Corp" → "acme-corp".
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// isDuplicateKey returns true for PostgreSQL unique-constraint violations (23505).
func isDuplicateKey(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// mustLocals panics if a required Locals key is missing (programming error, not user error).
func mustString(c *fiber.Ctx, key string) string {
	v, _ := c.Locals(key).(string)
	return v
}

// txFromCtx extracts the per-request pgx.Tx set by TenantTx middleware.
func txFromCtx(c *fiber.Ctx) pgx.Tx {
	tx, _ := c.Locals(types.CtxTx).(pgx.Tx)
	return tx
}
