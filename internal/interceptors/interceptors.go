package interceptors

import (
	"strings"

	"google.golang.org/grpc"
)

// UnaryInterceptorApplicable is function which verify that unary interceptor should be executed
type UnaryInterceptorApplicable func(*grpc.UnaryServerInfo) bool

// StreamInterceptorApplicable is function which verify that stream interceptor should be executed
type StreamInterceptorApplicable func(*grpc.StreamServerInfo) bool

func isUnaryInterceptorApplicable(info *grpc.UnaryServerInfo, fns ...UnaryInterceptorApplicable) bool {
	if len(fns) == 0 {
		return true
	}

	for _, fn := range fns {
		if !fn(info) {
			return false
		}
	}
	return true
}

// UnaryApplicableForService adds verification that interceptor is executed only for specific service
func UnaryApplicableForService(svc string) UnaryInterceptorApplicable {
	return func(info *grpc.UnaryServerInfo) bool {
		// FullMethod is the full RPC method string, i.e., /package.service/method.
		return strings.Contains(info.FullMethod, svc)
	}
}
