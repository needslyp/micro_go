package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRegisterRejectsInvalidCredentials(t *testing.T) {
	for _, tt := range []struct {
		name, email, password string
		want                  error
	}{
		{"empty email", "", "password123", ErrInvalidEmail},
		{"invalid email", "not-an-email", "password123", ErrInvalidEmail},
		{"display name", "User <user@example.com>", "password123", ErrInvalidEmail},
		{"empty password", "user@example.com", "", ErrInvalidPassword},
		{"short ASCII password", "user@example.com", "1234567", ErrInvalidPassword},
		{"short Unicode password", "user@example.com", "паролья", ErrInvalidPassword},
		{"over bcrypt byte limit", "user@example.com", strings.Repeat("я", 37), ErrInvalidPassword},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// No repository: invalid input must be rejected before database access.
			s := &Service{}
			if _, err := s.Register(context.Background(), tt.email, tt.password); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}
