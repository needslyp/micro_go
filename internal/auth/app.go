package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// Config holds the Auth service settings.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
	JWTSecret   string
}

func loadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	cfg := &Config{
		HTTPAddr:    os.Getenv("AUTH_HTTP_ADDR"),
		DatabaseURL: os.Getenv("AUTH_DATABASE_URL"),
		JWTSecret:   os.Getenv("AUTH_JWT_SECRET"),
	}
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		cfg.HTTPAddr = ":8081"
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, errors.New("AUTH_DATABASE_URL is required")
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return nil, errors.New("AUTH_JWT_SECRET is required")
	}
	return cfg, nil
}

// Run starts Auth service
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Printf("Auth configuration loaded; HTTP address: %s\n", cfg.HTTPAddr)

	pool, err := connectDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	fmt.Println("Auth connected to PostgreSQL; waiting for shutdown signal")

	repo := &UserRepository{pool: pool}
	handler := &Handler{repo: repo}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", handler.Register)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)

	go func() {
		fmt.Printf("Auth HTTP listening on %s\n", cfg.HTTPAddr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil

	case <-ctx.Done():
		fmt.Println("Auth stopping HTTP server")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	return nil
}
