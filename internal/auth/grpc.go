package auth

import (
	"context"
	"errors"

	authv1 "microservices_project/gen/auth/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type tokenValidator interface {
	ValidateToken(string) (int64, error)
}

type grpcHandler struct {
	authv1.UnimplementedAuthServiceServer
	validator tokenValidator
}

func newGRPCServer(validator tokenValidator) *grpc.Server {
	server := grpc.NewServer(grpc.MaxRecvMsgSize(1 << 20))
	authv1.RegisterAuthServiceServer(server, &grpcHandler{validator: validator})
	return server
}

func (h *grpcHandler) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	userID, err := h.validator.ValidateToken(req.GetToken())
	if errors.Is(err, ErrInvalidToken) {
		return nil, status.Error(codes.Unauthenticated, "invalid access token")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "token validation failed")
	}
	return &authv1.ValidateTokenResponse{UserId: userID}, nil
}
