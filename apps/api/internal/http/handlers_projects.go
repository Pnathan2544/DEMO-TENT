package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"saas-template/api/internal/types"
)

type project struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// All handlers below use the pgx.Tx set by TenantTx middleware.
// RLS automatically scopes every query to the active tenant — no WHERE tenant_id needed
// on SELECT/UPDATE/DELETE. tenant_id is only provided explicitly on INSERT.

func (s *Server) handleListProjects(c *fiber.Ctx) error {
	tx := txFromCtx(c)
	rows, err := tx.Query(c.UserContext(),
		`SELECT id, tenant_id, name, description, created_by, created_at
		   FROM projects
		  ORDER BY created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	result := []project{}
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.CreatedBy, &p.CreatedAt); err != nil {
			return err
		}
		result = append(result, p)
	}
	return c.JSON(result)
}

func (s *Server) handleCreateProject(c *fiber.Ctx) error {
	tx := txFromCtx(c)
	tenantID := mustString(c, types.CtxTenantID)
	userID := mustString(c, types.CtxUserID)

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	var p project
	err := tx.QueryRow(c.UserContext(),
		`INSERT INTO projects (tenant_id, name, description, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, name, description, created_by, created_at`,
		tenantID, req.Name, req.Description, userID).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(p)
}

func (s *Server) handleGetProject(c *fiber.Ctx) error {
	tx := txFromCtx(c)
	id := c.Params("id")

	var p project
	err := tx.QueryRow(c.UserContext(),
		`SELECT id, tenant_id, name, description, created_by, created_at
		   FROM projects WHERE id = $1`, id).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		// pgx returns ErrNoRows when RLS hides the row or it doesn't exist — both look like 404.
		if err == pgx.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "project not found"})
		}
		return err
	}
	return c.JSON(p)
}

func (s *Server) handleUpdateProject(c *fiber.Ctx) error {
	tx := txFromCtx(c)
	id := c.Params("id")

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	tag, err := tx.Exec(c.UserContext(),
		`UPDATE projects
		    SET name = $1, description = $2
		  WHERE id = $3`,
		req.Name, req.Description, id)
	if err != nil {
		return err
	}
	// RowsAffected == 0 means either not found or RLS filtered it out — both → 404.
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "project not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteProject(c *fiber.Ctx) error {
	tx := txFromCtx(c)
	id := c.Params("id")

	tag, err := tx.Exec(c.UserContext(), `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "project not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
