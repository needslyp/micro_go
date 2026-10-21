package auth

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	authv1 "microservices_project/gen/auth/v1"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestGRPCValidateToken(t *testing.T) {
	const secret = "grpc-test-secret"
	listener := bufconn.Listen(1 << 20)
	server := newGRPCServer(&Service{jwtSecret: secret})
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		listener.Close()
		if err := <-serveDone; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	})
	conn, err := grpc.NewClient("passthrough:///auth", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	client := authv1.NewAuthServiceClient(conn)
	valid, err := issueToken(42, secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := issueToken(42, "another-secret", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer: "auth-service", Audience: jwt.ClaimStrings{"blog-service"}, Subject: "42",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, token string
		code        codes.Code
	}{
		{"valid", valid.AccessToken, codes.OK},
		{"empty", "", codes.Unauthenticated},
		{"malformed", "broken", codes.Unauthenticated},
		{"wrong signature", wrongKey.AccessToken, codes.Unauthenticated},
		{"expired", expired, codes.Unauthenticated},
		{"bearer prefix", "Bearer " + valid.AccessToken, codes.Unauthenticated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			response, err := client.ValidateToken(ctx, &authv1.ValidateTokenRequest{Token: tt.token})
			if status.Code(err) != tt.code {
				t.Fatalf("got %v, want %v", err, tt.code)
			}
			if tt.code == codes.OK && response.GetUserId() != 42 {
				t.Fatalf("got user ID %d", response.GetUserId())
			}
		})
	}
}

func TestGRPCInternalError(t *testing.T) {
	handler := &grpcHandler{validator: &Service{}}
	_, err := handler.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{Token: "token"})
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "token validation failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGRPCCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler := &grpcHandler{validator: &Service{jwtSecret: "secret"}}
	_, err := handler.ValidateToken(ctx, &authv1.ValidateTokenRequest{})
	if status.Code(err) != codes.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}
}
