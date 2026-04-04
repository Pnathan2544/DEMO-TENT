package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload. UserID lives in RegisteredClaims.Subject (standard "sub" field)
// to avoid duplicate json:"sub" tags that would break Subject-based validation.
type Claims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tid"`
	Role     string `json:"role"`
}

// SignAccessToken issues a short-lived HS256 access token.
func SignAccessToken(userID, tenantID, role, secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		TenantID: tenantID,
		Role:     role,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseAccessToken validates the token and returns Claims.
// WithValidMethods pins the algorithm to HS256, closing off alg:none attacks.
func ParseAccessToken(tokenStr, secret string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{},
		func(token *jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		return nil, err
	}
	c, ok := t.Claims.(*Claims)
	if !ok || !t.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return c, nil
}
