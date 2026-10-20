package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("invalid password")
)

// Service implements authentication rules.
type Service struct {
	repo      *UserRepository
	jwtSecret string
	jwtTTL    time.Duration
}

// Register validates credentials and stores a user with a hashed password.
func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := checkEmail(email); err != nil {
		return User{}, ErrInvalidEmail
	}
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return User{}, ErrInvalidPassword
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	return s.repo.Create(ctx, email, hash)
}

// Login authenticates a user and issues an access token.
func (s *Service) Login(ctx context.Context, email, password string) (TokenResponse, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		return TokenResponse{}, ErrInvalidCredentials
	}
	if err != nil {
		return TokenResponse{}, err
	}
	if err := checkPassword(password, user.passwordHash); err != nil {
		return TokenResponse{}, ErrInvalidCredentials
	}
	return issueToken(user.ID, s.jwtSecret, s.jwtTTL)
}

func checkEmail(email string) error {
	if email == "" {
		return errors.New("empty email")
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return errors.New("invalid email")
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hash), nil
}

func checkPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
