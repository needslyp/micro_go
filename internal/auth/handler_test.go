package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLoginRejectsMalformedJSON(t *testing.T) {
	for _, body := range []string{`{`, `{"email":"user@example.com","extra":true}`, `{} {}`, `[]`} {
		t.Run(body, func(t *testing.T) {
			handler := &Handler{}
			response := httptest.NewRecorder()
			handler.Login(response, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("got %d", response.Code)
			}
		})
	}
}

func TestCheckPasswordDoesNotApplyRegistrationLength(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("short"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPassword("short", string(hash)); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if err := checkPassword("wrong", string(hash)); err == nil {
		t.Fatal("wrong password accepted")
	}
}
