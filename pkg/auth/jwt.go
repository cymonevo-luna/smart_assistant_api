// Package auth issues and verifies JWT access and refresh tokens.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role enumerates coarse-grained authorization roles.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// TokenType distinguishes short-lived access tokens from long-lived refresh
// tokens so a refresh token can never be used as an access token.
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Claims is the JWT payload carried by tokens.
type Claims struct {
	UserID string    `json:"uid"`
	Email  string    `json:"email"`
	Role   Role      `json:"role"`
	Type   TokenType `json:"typ"`
	jwt.RegisteredClaims
}

// IsAdmin reports whether the claims grant admin privileges.
func (c *Claims) IsAdmin() bool { return c.Role == RoleAdmin }

// TokenPair bundles an access and refresh token.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// TokenManager issues and parses signed tokens.
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
}

// NewTokenManager builds a TokenManager. refreshTTL is derived as 7x the access
// TTL when not configured separately by the caller.
func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration, issuer string) *TokenManager {
	if refreshTTL <= 0 {
		refreshTTL = accessTTL * 7
	}
	return &TokenManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL, issuer: issuer}
}

// GeneratePair issues a fresh access/refresh token pair for a user.
func (m *TokenManager) GeneratePair(userID, email string, role Role) (TokenPair, error) {
	access, err := m.sign(userID, email, role, AccessToken, m.accessTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := m.sign(userID, email, role, RefreshToken, m.refreshTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(m.accessTTL.Seconds())}, nil
}

func (m *TokenManager) sign(userID, email string, role Role, typ TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse validates a token string and returns its claims.
func (m *TokenManager) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ParseRefresh validates a token and ensures it is a refresh token.
func (m *TokenManager) ParseRefresh(token string) (*Claims, error) {
	claims, err := m.Parse(token)
	if err != nil {
		return nil, err
	}
	if claims.Type != RefreshToken {
		return nil, errors.New("not a refresh token")
	}
	return claims, nil
}
