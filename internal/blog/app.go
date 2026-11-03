package blog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
)

// Run connects Blog to PostgreSQL and waits for SIGINT or SIGTERM.
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	databaseURL := os.Getenv("BLOG_DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		return errors.New("BLOG_DATABASE_URL is required")
	}
	pool, err := connectDatabase(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := &ArticleRepository{pool: pool}
	service := &Service{repo: repo}
	handler := &Handler{service: service}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /articles", handler.GetList)
	mux.HandleFunc("GET /articles/{id}", handler.GetByID)

	<-ctx.Done()

	return nil
}
