package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Port                string
	DatabaseURL         string
	JWTSecret           string
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	StripeSecretKey     string
	StripeWebhookSecret string
	FrontendURL         string
	AppEnv              string
	CookieSecure        bool
	SMTPHost            string
	SMTPPort            string
	SMTPUser            string
	SMTPPassword        string
	SMTPFrom            string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                getenv("APP_PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		FrontendURL:         getenv("FRONTEND_URL", "http://localhost:3000"),
		AppEnv:              getenv("APP_ENV", "development"),
		SMTPHost:            getenv("SMTP_HOST", ""),
		SMTPPort:            getenv("SMTP_PORT", "587"),
		SMTPUser:            os.Getenv("SMTP_USER"),
		SMTPPassword:        os.Getenv("SMTP_PASS"),
		SMTPFrom:            getenv("SMTP_FROM", "noreply@example.com"),
		AccessTokenTTL:      15 * time.Minute,
		RefreshTokenTTL:     7 * 24 * time.Hour,
	}
	cfg.CookieSecure = cfg.AppEnv == "production"

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
