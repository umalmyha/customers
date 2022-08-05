package interceptors

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

// ErrorUnaryInterceptor converts error retrieved from handler to gRPC error with corresponding code
func ErrorUnaryInterceptor(applicables ...UnaryInterceptorApplicable) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if !isUnaryInterceptorApplicable(info, applicables...) {
			return h(ctx, req)
		}

		res, err := h(ctx, req)
		if err == nil {
			return res, nil
		}
		logrus.Errorf("error occurred on grpc request processing - %v", err)

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
