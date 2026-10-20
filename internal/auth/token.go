package auth

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultAccessTokenTTL = 15 * time.Minute

var ErrInvalidToken = errors.New("invalid access token")

// TokenResponse contains the signed access token and its lifetime in seconds.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func issueToken(userID int64, secret string, ttl time.Duration) (TokenResponse, error) {
	if strings.TrimSpace(secret) == "" {
		return TokenResponse{}, errors.New("JWT secret is required")
	}
	if ttl < time.Second || ttl%time.Second != 0 {
		return TokenResponse{}, errors.New("JWT TTL must be a positive whole number of seconds")
	}
	if userID <= 0 {
		return TokenResponse{}, errors.New("user ID must be positive")
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    "auth-service",
		Audience:  jwt.ClaimStrings{"blog-service"},
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("sign access token: %w", err)
	}
	return TokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: int64(ttl / time.Second)}, nil
}

// ValidateToken verifies an access token and returns its positive user ID.
func (s *Service) ValidateToken(raw string) (int64, error) {
	if strings.TrimSpace(s.jwtSecret) == "" {
		return 0, errors.New("JWT secret is required")
	}
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return []byte(s.jwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(),
		jwt.WithIssuer("auth-service"), jwt.WithAudience("blog-service"), jwt.WithIssuedAt())
	if err != nil {
		return 0, ErrInvalidToken
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 || strconv.FormatInt(userID, 10) != claims.Subject {
		return 0, ErrInvalidToken
	}
	return userID, nil
}
