package http

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/jackc/pgx/v5/pgxpool"
	"saas-template/api/internal/config"
	"saas-template/api/internal/mail"
)

// Server holds the Fiber app and shared dependencies.
type Server struct {
	app    *fiber.App
	cfg    *config.Config
	db     *pgxpool.Pool
	mailer mail.Mailer
}

// New wires up the Fiber application and registers all routes.
func New(cfg *config.Config, db *pgxpool.Pool, mailer mail.Mailer) *Server {
	s := &Server{cfg: cfg, db: db, mailer: mailer}
	s.app = fiber.New(fiber.Config{
		ErrorHandler: s.errorHandler,
	})
	s.setup()
	return s
}

func (s *Server) setup() {
	// ── Global middleware ──────────────────────────────────────────────────────
	s.app.Use(requestid.New()) // X-Request-Id header on every response
	s.app.Use(s.requestLogger())
	s.app.Use(recover.New(recover.Config{EnableStackTrace: true}))

	v1 := s.app.Group("/api/v1")

	// ── Public: auth ───────────────────────────────────────────────────────────
	auth := v1.Group("/auth")
	auth.Post("/register", s.handleRegister)
	auth.Post("/login", s.handleLogin)
	auth.Post("/refresh", s.handleRefresh)
	auth.Post("/logout", s.handleLogout)
	auth.Post("/switch", AuthRequired(s.cfg), s.handleSwitch)
	auth.Get("/tenants", AuthRequired(s.cfg), s.handleTenants)

	// ── Public: Stripe webhooks (raw body, no auth) ────────────────────────────
	v1.Post("/webhooks/stripe", s.handleStripeWebhook)

	// ── Protected: /me ────────────────────────────────────────────────────────
	me := v1.Group("/me", AuthRequired(s.cfg))
	me.Get("/", s.handleGetMe)
	me.Put("/", s.handleUpdateMe)
	me.Put("/password", s.handleChangePassword)
	me.Get("/invites", s.handleMyInvites)
	me.Get("/billing", s.handleGetBilling)
	me.Post("/billing/portal", s.handleBillingPortal)

	// ── Protected: /members (RLS via TenantTx; memberships filtered by explicit WHERE) ──
	members := v1.Group("/members", AuthRequired(s.cfg), TenantTx(s.db))
	members.Get("/", RequireRole("member"), s.handleListMembers)
	members.Patch("/:uid", RequireRole("admin"), s.handleUpdateMember)
	members.Delete("/:uid", s.handleRemoveMember) // RBAC checked in handler (allows self-leave)

	// ── Protected: accept invite (AuthRequired only — user not yet tenant member) ─
	v1.Post("/invites/accept", AuthRequired(s.cfg), s.handleAcceptInvite)

	// ── Protected: invite management (admin+, RLS via TenantTx) ──────────────
	invites := v1.Group("/invites", AuthRequired(s.cfg), TenantTx(s.db), RequireRole("admin"))
	invites.Post("/", s.handleCreateInvite)
	invites.Get("/", s.handleListInvites)
	invites.Delete("/:id", s.handleRevokeInvite)

	// ── Protected: /projects (RLS-enforced via TenantTx) ──────────────────────
	projects := v1.Group("/projects", AuthRequired(s.cfg), TenantTx(s.db))
	projects.Get("/", RequireRole("member"), s.handleListProjects)
	projects.Post("/", RequireRole("admin"), s.handleCreateProject)
	projects.Get("/:id", RequireRole("member"), s.handleGetProject)
	projects.Put("/:id", RequireRole("admin"), s.handleUpdateProject)
	projects.Delete("/:id", RequireRole("admin"), s.handleDeleteProject)

	// ── Protected: audit log (admin+, RLS via TenantTx) ───────────────────────
	v1.Get("/audit", AuthRequired(s.cfg), TenantTx(s.db), RequireRole("admin"), s.handleListAuditEvents)
}

// Listen starts the HTTP server on the given address (e.g. ":8080").
func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}

// ── Middleware helpers ─────────────────────────────────────────────────────────

// requestLogger returns a slog-based structured request logger.
func (s *Server) requestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		slog.LogAttrs(c.UserContext(), slog.LevelInfo, "request",
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", c.Response().StatusCode()),
			slog.Duration("latency", time.Since(start)),
			slog.String("request_id", c.GetRespHeader("X-Request-Id")),
		)
		return err
	}
}

// errorHandler converts any error into a JSON {"error":"..."} response.
// Fiber errors carry a status code; everything else becomes 500.
func (s *Server) errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal server error"

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
		msg = e.Message
	}

	return c.Status(code).JSON(fiber.Map{"error": msg})
}
