package http

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ── GET /audit ────────────────────────────────────────────────────────────────

func (s *Server) handleListAuditEvents(c *fiber.Ctx) error {
	tx := txFromCtx(c)

	rows, err := tx.Query(c.UserContext(),
		`SELECT ae.id,
		        ae.actor_id,   ua.name  AS actor_name,
		        ae.target_id,  ut.name  AS target_name,
		        ae.action,     ae.meta, ae.created_at
		 FROM audit_events ae
		 JOIN  users ua ON ua.id = ae.actor_id
		 LEFT JOIN users ut ON ut.id = ae.target_id
		 ORDER BY ae.created_at DESC
		 LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type event struct {
		ID         string          `json:"id"`
		ActorID    string          `json:"actorId"`
		ActorName  string          `json:"actorName"`
		TargetID   *string         `json:"targetId,omitempty"`
		TargetName *string         `json:"targetName,omitempty"`
		Action     string          `json:"action"`
		Meta       json.RawMessage `json:"meta,omitempty"`
		CreatedAt  time.Time       `json:"createdAt"`
	}
	result := []event{}
	for rows.Next() {
		var e event
		var metaBytes []byte
		if err := rows.Scan(
			&e.ID,
			&e.ActorID, &e.ActorName,
			&e.TargetID, &e.TargetName,
			&e.Action, &metaBytes, &e.CreatedAt,
		); err != nil {
			return err
		}
		if len(metaBytes) > 0 {
			e.Meta = json.RawMessage(metaBytes)
		}
		result = append(result, e)
	}

	return c.JSON(result)
}
