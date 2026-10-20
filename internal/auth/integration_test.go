package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Each test uses a separate schema and applies the real Auth migration.
// AUTH_TEST_DATABASE_URL must point to a database where schemas may be created.
func authTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AUTH_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{fmt.Sprintf("auth_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("clean up test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../migrations/auth/00001_create_users.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestAuthRegistrationAndLoginIntegration(t *testing.T) {
	pool := authTestPool(t)
	const secret = "integration-test-signing-secret"
	h := &Handler{service: &Service{repo: &UserRepository{pool: pool}, jwtSecret: secret, jwtTTL: 2 * time.Minute}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", h.Register)
	mux.HandleFunc("POST /auth/login", h.Login)
	request := func(path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: got status %d, want %d; body: %s", path, w.Code, status, w.Body)
		}
		return w
	}

	registered := request("/auth/register", `{"email":"  User@Example.COM  ","password":"password123"}`, http.StatusCreated)
	var user userResponse
	if err := json.Unmarshal(registered.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.ID <= 0 || user.Email != "user@example.com" || user.CreatedAt.IsZero() {
		t.Fatalf("unexpected registered user: %+v", user)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(registered.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["password"] != nil || fields["password_hash"] != nil {
		t.Fatalf("unexpected registration response fields: %v", fields)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var email, hash string
	if err := pool.QueryRow(ctx, "SELECT email, password_hash FROM users WHERE id = $1", user.ID).Scan(&email, &hash); err != nil {
		t.Fatal(err)
	}
	if email != user.Email || hash == "password123" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123")) != nil {
		t.Fatal("stored email or bcrypt password hash is incorrect")
	}
	request("/auth/register", `{"email":"USER@example.com","password":"different123"}`, http.StatusConflict)
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate registration changed user count: %d", count)
	}
	login := request("/auth/login", `{"email":" USER@Example.com ","password":"password123"}`, http.StatusOK)
	var token TokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	claims := &jwt.RegisteredClaims{}
	if _, err := jwt.ParseWithClaims(token.AccessToken, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("auth-service"), jwt.WithAudience("blog-service"), jwt.WithExpirationRequired()); err != nil {
		t.Fatalf("login returned invalid JWT: %v", err)
	}
	if claims.Subject != strconv.FormatInt(user.ID, 10) || claims.IssuedAt == nil || token.TokenType != "Bearer" || token.ExpiresIn != 120 {
		t.Fatal("login returned incorrect token claims or metadata")
	}
	if login.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login response must disable caching")
	}
	wrong := request("/auth/login", `{"email":"user@example.com","password":"wrong"}`, http.StatusUnauthorized)
	unknown := request("/auth/login", `{"email":"unknown@example.com","password":"password123"}`, http.StatusUnauthorized)
	if wrong.Body.String() != unknown.Body.String() {
		t.Fatal("unknown email and wrong password must return the same response")
	}
	request("/auth/login", `{"email":"user@example.com","password":"password123"}`, http.StatusOK)
}
