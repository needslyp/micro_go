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
	"time"

	authv1 "microservices_project/gen/auth/v1"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Run starts the Blog HTTP server and shuts it down on SIGINT or SIGTERM.
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

	authAddr := strings.TrimSpace(os.Getenv("BLOG_AUTH_GRPC_ADDR"))
	if authAddr == "" {
		authAddr = "localhost:9091"
	}
	conn, err := grpc.NewClient(authAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("create Auth gRPC client: %w", err)
	}
	defer conn.Close()

	mux := newRouter(handler, authv1.NewAuthServiceClient(conn))

	address := strings.TrimSpace(os.Getenv("BLOG_HTTP_ADDR"))
	if address == "" {
		address = ":8080"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("Blog HTTP listening on %s\n", address)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve Blog HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown Blog HTTP: %w", err)
	}

	return nil
}

// newRouter connects public reads and authenticated writes to the HTTP handlers.
func newRouter(handler *Handler, client authv1.AuthServiceClient) *http.ServeMux {
	mux := http.NewServeMux()
	auth := &authMiddleware{client: client}
	mux.HandleFunc("GET /articles", handler.GetList)
	mux.HandleFunc("GET /articles/{id}", handler.GetByID)
	mux.Handle("POST /articles", auth.RequireAuth(http.HandlerFunc(handler.Create)))
	mux.Handle("PUT /articles/{id}", auth.RequireAuth(http.HandlerFunc(handler.Update)))
	mux.Handle("DELETE /articles/{id}", auth.RequireAuth(http.HandlerFunc(handler.Delete)))
	mux.HandleFunc("/articles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "GET, HEAD, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})
	mux.HandleFunc("/articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "DELETE, GET, HEAD, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return mux
}
