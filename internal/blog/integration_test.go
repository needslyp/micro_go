package blog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

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
