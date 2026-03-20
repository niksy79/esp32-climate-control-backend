// Package auth provides JWT token generation/validation and HTTP middleware
// for the climate-backend REST API.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"climate-backend/internal/models"
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 7 * 24 * time.Hour
)

// Claims are the JWT payload fields embedded in every token.
type Claims struct {
	UserID   string      `json:"user_id"`
	TenantID string      `json:"tenant_id"`
	Email    string      `json:"email"`
	Role     models.Role `json:"role"`
	jwt.RegisteredClaims
}

// Service signs and validates JWTs using HMAC-SHA256.
type Service struct {
	secret []byte
}

// NewService creates a Service from a non-empty signing secret.
func NewService(secret string) (*Service, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT_SECRET must not be empty")
	}
	return &Service{secret: []byte(secret)}, nil
}

// GenerateAccessToken issues a short-lived access token (15 min).
func (s *Service) GenerateAccessToken(userID, tenantID, email string, role models.Role) (string, error) {
	return s.sign(userID, tenantID, email, role, accessTokenTTL)
}

// GenerateRefreshToken issues a long-lived refresh token (7 days).
func (s *Service) GenerateRefreshToken(userID, tenantID, email string, role models.Role) (string, error) {
	return s.sign(userID, tenantID, email, role, refreshTokenTTL)
}

// ValidateToken parses and validates a token string, returning its claims.
func (s *Service) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("auth: unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("auth: invalid token claims")
	}
	return claims, nil
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func (s *Service) sign(userID, tenantID, email string, role models.Role, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		TenantID: tenantID,
		Email:    email,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}
