package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	GRPCAddr    string
	HTTPAddr    string
	DatabaseURL string
	JWTSecret   string
	JWTTokenTTL time.Duration
}

func loadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	cfg := &Config{
		HTTPAddr:    os.Getenv("AUTH_HTTP_ADDR"),
		GRPCAddr:    os.Getenv("AUTH_GRPC_ADDR"),
		DatabaseURL: os.Getenv("AUTH_DATABASE_URL"),
		JWTSecret:   os.Getenv("AUTH_JWT_SECRET"),
	}
	if strings.TrimSpace(cfg.GRPCAddr) == "" {
		cfg.GRPCAddr = ":9091"
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
	ttl := strings.TrimSpace(os.Getenv("AUTH_JWT_TTL"))
	cfg.JWTTokenTTL = defaultAccessTokenTTL
	if ttl != "" {
		parsed, err := time.ParseDuration(ttl)
		if err != nil || parsed < time.Second || parsed%time.Second != 0 {
			return nil, errors.New("AUTH_JWT_TTL must be a positive duration in whole seconds (for example 15m)")
		}
		cfg.JWTTokenTTL = parsed
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
	service := &Service{repo: repo, jwtSecret: cfg.JWTSecret, jwtTTL: cfg.JWTTokenTTL}
	handler := &Handler{service: service}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", handler.Register)
	mux.HandleFunc("POST /auth/login", handler.Login)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Bind both ports before serving so a startup failure closes all listeners.
	httpListener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}
	defer httpListener.Close()
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}
	defer grpcListener.Close()
	grpcServer := newGRPCServer(service)
	serverErr := make(chan error, 2)
	go func() {
		fmt.Printf("Auth HTTP listening on %s\n", httpListener.Addr())
		if err := server.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("serve HTTP: %w", err)
		}
	}()
	go func() {
		fmt.Printf("Auth gRPC listening on %s\n", grpcListener.Addr())
		if err := grpcServer.Serve(grpcListener); err != nil {
			serverErr <- fmt.Errorf("serve gRPC: %w", err)
		}
	}()

	var serveErr error
	select {
	case serveErr = <-serverErr:
	case <-ctx.Done():
	}
	fmt.Println("Auth stopping HTTP and gRPC servers")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
		<-grpcStopped
	}
	if shutdownErr != nil {
		return errors.Join(serveErr, fmt.Errorf("shutdown HTTP server: %w", shutdownErr))
	}
	return serveErr
}
