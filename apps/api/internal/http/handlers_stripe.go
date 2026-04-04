package http

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/webhook"
)

// handleStripeWebhook verifies the Stripe signature and updates tenant billing state.
// Must receive the raw (unmodified) request body — Fiber does not buffer it by default.
func (s *Server) handleStripeWebhook(c *fiber.Ctx) error {
	if s.cfg.StripeWebhookSecret == "" {
		// Stripe not configured; silently accept to avoid noise in development.
		return c.SendStatus(fiber.StatusOK)
	}

	sig := c.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(c.Body(), sig, s.cfg.StripeWebhookSecret)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stripe signature"})
	}

	ctx := c.UserContext()

	switch event.Type {

	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unmarshal error"})
		}
		if session.Subscription != nil && session.Customer != nil {
			s.db.Exec(ctx,
				`UPDATE tenants
				    SET stripe_subscription_id = $1
				  WHERE stripe_customer_id = $2`,
				session.Subscription.ID, session.Customer.ID)
		}

	case "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unmarshal error"})
		}
		plan := extractPlan(&sub)
		s.db.Exec(ctx,
			`UPDATE tenants SET stripe_plan = $1 WHERE stripe_subscription_id = $2`,
			plan, sub.ID)

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unmarshal error"})
		}
		s.db.Exec(ctx,
			`UPDATE tenants
			    SET stripe_plan = 'free', stripe_subscription_id = NULL
			  WHERE stripe_subscription_id = $1`,
			sub.ID)
	}

	return c.SendStatus(fiber.StatusOK)
}

// extractPlan reads the plan name from the subscription's first price item.
// Falls back to "paid" if no nickname/lookup_key is set.
func extractPlan(sub *stripe.Subscription) string {
	if len(sub.Items.Data) == 0 {
		return "paid"
	}
	price := sub.Items.Data[0].Price
	if price.Nickname != "" {
		return price.Nickname
	}
	if price.LookupKey != "" {
		return price.LookupKey
	}
	return "paid"
}
