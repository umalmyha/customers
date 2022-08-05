package interceptors

import (
	"context"

	"github.com/umalmyha/customers/internal/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthUnaryInterceptor verifies that jwt is provided in metadata and valid
func AuthUnaryInterceptor(validator *auth.JwtValidator, applicables ...UnaryInterceptorApplicable) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if !isUnaryInterceptorApplicable(info, applicables...) {
			return h(ctx, req)
		}

		headers, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "no auth info provided")
		}

		tokenHdr := headers.Get("accessToken")
		if len(tokenHdr) == 0 {
			return nil, status.Error(codes.Unauthenticated, "accessToken header is missing")
		}

		if _, err := validator.Verify(tokenHdr[0]); err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid access token provided - %v", err)
		}

		return h(ctx, req)
	}
}
