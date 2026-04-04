package http

import (
	"github.com/gofiber/fiber/v2"
	"saas-template/api/internal/config"
	"saas-template/api/internal/security"
	"saas-template/api/internal/types"
)

// AuthRequired validates the access_token HttpOnly cookie and populates Locals.
func AuthRequired(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies("access_token")
		if token == "" {
			clearAuthCookies(c)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		claims, err := security.ParseAccessToken(token, cfg.JWTSecret)
		if err != nil {
			clearAuthCookies(c)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		// claims.Subject is the standard JWT "sub" field — contains the user ID.
		c.Locals(types.CtxUserID, claims.Subject)
		c.Locals(types.CtxTenantID, claims.TenantID)
		c.Locals(types.CtxRole, claims.Role)
		return c.Next()
	}
}

func clearAuthCookies(c *fiber.Ctx) {
	expired := fiber.Cookie{
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   -1,
		Path:     "/",
	}
	expired.Name = "access_token"
	c.Cookie(&expired)
	expired.Name = "refresh_token"
	c.Cookie(&expired)
}
