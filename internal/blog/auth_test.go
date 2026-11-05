package blog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authv1 "microservices_project/gen/auth/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authClientFunc func(context.Context, *authv1.ValidateTokenRequest, ...grpc.CallOption) (*authv1.ValidateTokenResponse, error)

func (f authClientFunc) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
	return f(ctx, req, opts...)
}

func TestRequireAuth(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header string
		userID int64
		err    error
		want   int
		calls  int
	}{
		{name: "missing", want: 401},
		{name: "wrong scheme", header: "Basic token", want: 401},
		{name: "empty token", header: "Bearer ", want: 401},
		{name: "extra token", header: "Bearer one two", want: 401},
		{name: "invalid token", header: "Bearer token", err: status.Error(codes.Unauthenticated, "invalid"), want: 401, calls: 1},
		{name: "unavailable", header: "Bearer token", err: status.Error(codes.Unavailable, "offline"), want: 503, calls: 1},
		{name: "deadline", header: "Bearer token", err: status.Error(codes.DeadlineExceeded, "timeout"), want: 503, calls: 1},
		{name: "internal", header: "Bearer token", err: status.Error(codes.Internal, "failed"), want: 503, calls: 1},
		{name: "invalid user", header: "Bearer token", want: 503, calls: 1},
		{name: "valid", header: "bEaReR token", userID: 42, want: 204, calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var validationCtx context.Context
			client := authClientFunc(func(ctx context.Context, req *authv1.ValidateTokenRequest, _ ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
				calls++
				validationCtx = ctx
				if req.GetToken() != "token" {
					t.Fatalf("raw token = %q", req.GetToken())
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 2*time.Second {
					t.Fatal("missing Auth deadline")
				}
				return &authv1.ValidateTokenResponse{UserId: tt.userID}, tt.err
			})
			middleware := &authMiddleware{client: client}
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				id, ok := userIDFromContext(r.Context())
				if !ok || id != 42 {
					t.Fatalf("user ID = %d, valid = %v", id, ok)
				}
				if r.Context().Err() != nil {
					t.Fatal("handler context canceled after Auth call")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, "/articles", nil)
			if tt.header != "" {
				request.Header.Set("Authorization", tt.header)
			}
			response := httptest.NewRecorder()
			middleware.RequireAuth(next).ServeHTTP(response, request)
			if response.Code != tt.want || calls != tt.calls || called != (tt.want == 204) {
				t.Fatalf("status=%d calls=%d handler=%v", response.Code, calls, called)
			}
			if validationCtx != nil && validationCtx.Err() == nil {
				t.Fatal("Auth context not released")
			}
		})
	}
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	client := authClientFunc(func(context.Context, *authv1.ValidateTokenRequest, ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
		t.Fatal("Auth called without token")
		return nil, nil
	})
	router := newRouter(&Handler{}, client)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/articles"},
		{http.MethodPut, "/articles/1"},
		{http.MethodDelete, "/articles/1"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != 401 {
			t.Fatalf("%s %s: %d", route.method, route.path, response.Code)
		}
	}
}

func TestPublicRoutesDoNotCallAuth(t *testing.T) {
	client := authClientFunc(func(context.Context, *authv1.ValidateTokenRequest, ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
		t.Fatal("public route called Auth")
		return nil, nil
	})
	router := newRouter(&Handler{service: &Service{}}, client)
	for _, path := range []string{"/articles?limit=0", "/articles/-1"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer invalid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", path, response.Code)
		}
	}
}

func TestRequireAuthPropagatesCancellation(t *testing.T) {
	client := authClientFunc(func(ctx context.Context, _ *authv1.ValidateTokenRequest, _ ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	})
	middleware := &authMiddleware{client: client}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/articles", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called after cancellation")
	})
	middleware.RequireAuth(next).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", response.Code)
	}
}
