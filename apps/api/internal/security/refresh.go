package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Generate produces a cryptographically random refresh token.
// raw is returned to the client; hash is stored in the DB.
func Generate() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	hash = hashToken(raw)
	return
}

// HashToken returns the SHA-256 hex digest of a raw token.
// Exported so other packages (e.g. http) can compute hashes for lookups.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func hashToken(raw string) string { return HashToken(raw) }

// Store persists a refresh token hash for the given user, pinned to a tenant.
func Store(ctx context.Context, db *pgxpool.Pool, userID, tenantID, hash string, exp time.Time) error {
	_, err := db.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, tenant_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, tenantID, hash, exp)
	return err
}

// Rotate atomically swaps an existing refresh token for a fresh one,
// carrying the stored tenant_id forward to the new token.
// Returns the userID, tenantID, and new raw token.
func Rotate(ctx context.Context, db *pgxpool.Pool, rawToken string, newTTL time.Duration) (userID, tenantID, newRaw string, err error) {
	h := hashToken(rawToken)

	tx, err := db.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`DELETE FROM refresh_tokens WHERE token_hash = $1 RETURNING user_id, tenant_id, expires_at`,
		h).Scan(&userID, &tenantID, &expiresAt)
	if err != nil {
		err = errors.New("invalid refresh token")
		return
	}
	if time.Now().After(expiresAt) {
		err = errors.New("refresh token expired")
		return
	}

	var newHash string
	newRaw, newHash, err = Generate()
	if err != nil {
		return
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, tenant_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, tenantID, newHash, time.Now().Add(newTTL))
	if err != nil {
		return
	}

	err = tx.Commit(ctx)
	return
}

// Switch atomically swaps a refresh token and re-pins it to newTenantID.
// Call only after verifying the user is a member of newTenantID.
// Returns the userID and new raw token.
func Switch(ctx context.Context, db *pgxpool.Pool, rawToken, newTenantID string, newTTL time.Duration) (userID, newRaw string, err error) {
	h := hashToken(rawToken)

	tx, err := db.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`DELETE FROM refresh_tokens WHERE token_hash = $1 RETURNING user_id, expires_at`,
		h).Scan(&userID, &expiresAt)
	if err != nil {
		err = errors.New("invalid refresh token")
		return
	}
	if time.Now().After(expiresAt) {
		err = errors.New("refresh token expired")
		return
	}

	var newHash string
	newRaw, newHash, err = Generate()
	if err != nil {
		return
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, tenant_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, newTenantID, newHash, time.Now().Add(newTTL))
	if err != nil {
		return
	}

	err = tx.Commit(ctx)
	return
}

// Revoke deletes a refresh token, effectively logging out the session.
func Revoke(ctx context.Context, db *pgxpool.Pool, rawToken string) error {
	h := hashToken(rawToken)
	_, err := db.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, h)
	return err
}

// PruneExpired removes all expired tokens. Call periodically (e.g. hourly).
func PruneExpired(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW()`)
	return err
}
