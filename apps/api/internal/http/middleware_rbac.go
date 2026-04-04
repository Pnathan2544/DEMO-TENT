package http

import (
	"github.com/gofiber/fiber/v2"
	"saas-template/api/internal/types"
)

// RequireRole returns a middleware that gates access by minimum role.
// Uses numeric weights: viewer(1) < member(2) < admin(3) < owner(4).
func RequireRole(minRole string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals(types.CtxRole).(string)
		if !ok || !types.RoleAtLeast(role, minRole) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}
