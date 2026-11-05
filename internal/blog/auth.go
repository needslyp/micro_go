package blog

import (
	"context"
	"net/http"
	"strings"
	"time"

	authv1 "microservices_project/gen/auth/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userIDContextKey struct{}

func userIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDContextKey{}).(int64)
	return id, ok && id > 0
}

type authMiddleware struct {
	client authv1.AuthServiceClient
}

// RequireAuth validates a Bearer token with Auth and attaches the user ID.
func (m *authMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid access token")
			return
		}
		parts := strings.Fields(headers[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "invalid access token")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		response, err := m.client.ValidateToken(ctx, &authv1.ValidateTokenRequest{Token: parts[1]})
		cancel()
		if err != nil {
			if status.Code(err) == codes.Unauthenticated {
				writeError(w, http.StatusUnauthorized, "invalid access token")
				return
			}
			writeError(w, http.StatusServiceUnavailable, "authentication service unavailable")
			return
		}
		if response.GetUserId() <= 0 {
			writeError(w, http.StatusServiceUnavailable, "authentication service unavailable")
			return
		}

		requestCtx := context.WithValue(r.Context(), userIDContextKey{}, response.GetUserId())
		next.ServeHTTP(w, r.WithContext(requestCtx))
	})
}
