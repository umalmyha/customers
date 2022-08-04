package interceptors

import (
	"context"
	"errors"
	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
)

type errorLogger interface {
	Errorf(format string, args ...any)
}

func httpToGrpcCode(s int) codes.Code {
	switch s {
	case http.StatusBadRequest:
		return codes.FailedPrecondition
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	default:
		return codes.Internal
	}
}

func ErrorUnaryInterceptor(logger errorLogger, applicables ...UnaryInterceptorApplicable) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if !isUnaryInterceptorApplicable(info, applicables...) {
			return h(ctx, req)
		}

		res, err := h(ctx, req)
		if err == nil {
			return res, err
		}
		logger.Errorf("error occurred on grpc request processing - %v", err)

		if _, ok := status.FromError(err); ok { // it is already grpc status error
			return nil, err
		}

		code := codes.Internal

		var echoErr *echo.HTTPError
		if errors.As(err, &echoErr) {
			code = httpToGrpcCode(echoErr.Code)
		}

		if code == codes.Internal {
			return nil, status.Error(code, "Internal server error")
		}
		return nil, status.Error(code, err.Error())
	}
}
