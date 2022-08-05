package interceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type validator interface {
	Validate() error
	ValidateAll() error
}

// ValidatorUnaryInterceptor runs validation on payload if it implements validator interface
func ValidatorUnaryInterceptor(all bool, applicables ...UnaryInterceptorApplicable) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if !isUnaryInterceptorApplicable(info, applicables...) {
			return h(ctx, req)
		}

		v, ok := req.(validator)
		if ok {
			var err error
			if all {
				err = v.ValidateAll()
			} else {
				err = v.Validate()
			}

			if err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}

		return h(ctx, req)
	}
}
