package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	stripe "github.com/stripe/stripe-go/v76"
	"saas-template/api/internal/config"
	"saas-template/api/internal/db"
	httpserver "saas-template/api/internal/http"
	"saas-template/api/internal/mail"
	"saas-template/api/internal/security"
)

func main() {
	// JSON structured logging to stdout (parsed by log aggregators in production).
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Load .env — try CWD (apps/api/.env) first, then project root (../../.env).
	// Production platforms inject env vars directly; both Load calls are no-ops there.
	_ = godotenv.Load()
	_ = godotenv.Load("../../.env")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Set Stripe API key globally (stripe-go reads stripe.Key on every call).
	stripe.Key = cfg.StripeSecretKey

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	// Prune expired refresh tokens in the background (hourly).
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := security.PruneExpired(context.Background(), pool); err != nil {
				slog.Error("prune refresh tokens", "error", err)
			}
		}
	}()

	var mailer mail.Mailer
	if cfg.SMTPHost != "" {
		mailer = &mail.SMTPMailer{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}
	} else {
		slog.Warn("SMTP_HOST not set — invite emails will be logged only (NoopMailer)")
		mailer = mail.NoopMailer{}
	}

	srv := httpserver.New(cfg, pool, mailer)

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("shutdown signal received")
		if err := srv.Shutdown(); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("server starting", "port", cfg.Port)
	if err := srv.Listen(":" + cfg.Port); err != nil {
		slog.Error("server stopped", "error", err)
	}
}
