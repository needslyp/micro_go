package blog

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"

	"errors"
	"fmt"
	authv1 "microservices_project/gen/auth/v1"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BLOG_TEST_DATABASE_URL must use a dedicated test instance and a role with CREATEDB permission.
func blogTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BLOG_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BLOG_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	database := fmt.Sprintf("blog_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{database}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("clean up test database: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = database
	cfg.ConnConfig.RuntimeParams["search_path"] = "public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../migrations/blog/00001_create_articles.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestArticleUpdateDeleteIntegration(t *testing.T) {
	repo := &ArticleRepository{pool: blogTestPool(t)}
	ctx := context.Background()
	original, err := repo.Create(ctx, 1, "Original", "Original body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Update(ctx, original.ID, 2, "Forbidden", "Forbidden"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign update: %v", err)
	}
	if err := repo.Delete(ctx, original.ID, 2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign delete: %v", err)
	}
	unchanged, err := repo.GetByID(ctx, original.ID)
	if err != nil || unchanged != original {
		t.Fatalf("foreign operation changed article: %+v, %v", unchanged, err)
	}
	updated, err := repo.Update(ctx, original.ID, 1, "Updated", "Updated body")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Updated" || updated.Body != "Updated body" || updated.AuthorID != original.AuthorID || !updated.CreatedAt.Equal(original.CreatedAt) || !updated.UpdatedAt.After(original.UpdatedAt) {
		t.Fatalf("unexpected update: %+v", updated)
	}
	stored, err := repo.GetByID(ctx, original.ID)
	if err != nil || stored != updated {
		t.Fatalf("update not persisted: %+v, %v", stored, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.Update(canceled, original.ID, 1, "Canceled", "Canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled update: %v", err)
	}
	if err := repo.Delete(canceled, original.ID, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delete: %v", err)
	}
	if err := repo.Delete(ctx, original.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, original.ID); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("get deleted: %v", err)
	}
	if _, err := repo.Update(ctx, original.ID, 1, "Missing", "Missing"); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := repo.Delete(ctx, original.ID, 1); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestProtectedArticleHTTPIntegration(t *testing.T) {
	pool := blogTestPool(t)
	client := authClientFunc(func(_ context.Context, req *authv1.ValidateTokenRequest, _ ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
		switch req.GetToken() {
		case "owner":
			return &authv1.ValidateTokenResponse{UserId: 1}, nil
		case "other":
			return &authv1.ValidateTokenResponse{UserId: 2}, nil
		default:
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
	})
	router := newRouter(&Handler{service: &Service{repo: &ArticleRepository{pool: pool}}}, client)
	request := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s %s: status=%d, want=%d, body=%s", method, path, response.Code, want, response.Body.String())
		}
		return response
	}
	request("POST", "/articles", "", "", 401)
	request("POST", "/articles", "invalid", "", 401)
	created := request("POST", "/articles", "owner", `{"title":"  Title  ","body":"Text"}`, 201)
	var article articleResponse
	if err := json.Unmarshal(created.Body.Bytes(), &article); err != nil {
		t.Fatal(err)
	}
	if article.AuthorID != 1 || article.Title != "Title" {
		t.Fatalf("unexpected article: %+v", article)
	}
	path := "/articles/" + strconv.FormatInt(article.ID, 10)
	if created.Header().Get("Location") != path {
		t.Fatal("incorrect Location")
	}
	request("GET", path, "", "", 200)
	request("GET", "/articles", "", "", 200)
	request("PUT", path, "other", `{"title":"Foreign","body":"Foreign"}`, 403)
	request("DELETE", path, "other", "", 403)
	request("PUT", path, "owner", `{"title":"Updated","body":"Updated"}`, 200)
	deleted := request("DELETE", path, "owner", "", 204)
	if deleted.Body.Len() != 0 {
		t.Fatal("DELETE response has a body")
	}
	request("DELETE", path, "owner", "", 404)
	request("PUT", path, "owner", `{"title":"Updated","body":"Updated"}`, 404)
}

// TestRepositoryDeadlineIntegration checks that waiting for a busy pool is bounded.
func TestRepositoryDeadlineIntegration(t *testing.T) {
	source := blogTestPool(t)
	config := source.Config()
	config.MaxConns = 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	held, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(held.Release)
	repo := &ArticleRepository{pool: pool}
	t.Run("create", func(t *testing.T) {
		t.Parallel()
		_, err := repo.Create(ctx, 1, "Title", "Body")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if ctx.Err() != nil {
			t.Fatal("operation waited for the parent deadline instead of its own")
		}
	})
	t.Run("get", func(t *testing.T) {
		t.Parallel()
		_, err := repo.GetByID(ctx, 1)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if ctx.Err() != nil {
			t.Fatal("operation waited for the parent deadline instead of its own")
		}
	})
	t.Run("list", func(t *testing.T) {
		t.Parallel()
		_, err := repo.GetList(ctx, 20, 0)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if ctx.Err() != nil {
			t.Fatal("operation waited for the parent deadline instead of its own")
		}
	})
	t.Run("update", func(t *testing.T) {
		t.Parallel()
		_, err := repo.Update(ctx, 1, 1, "Title", "Body")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if ctx.Err() != nil {
			t.Fatal("operation waited for the parent deadline instead of its own")
		}
	})
	t.Run("delete", func(t *testing.T) {
		t.Parallel()
		err := repo.Delete(ctx, 1, 1)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if ctx.Err() != nil {
			t.Fatal("operation waited for the parent deadline instead of its own")
		}
	})
}
