package interceptors

import (
	"google.golang.org/grpc"
	"strings"
)

type UnaryInterceptorApplicable func(*grpc.UnaryServerInfo) bool
type StreamInterceptorApplicable func(*grpc.StreamServerInfo) bool

func isUnaryInterceptorApplicable(info *grpc.UnaryServerInfo, fns ...UnaryInterceptorApplicable) bool {
	if len(fns) == 0 {
		return true
	}

	for _, fn := range fns {
		if fn(info) == false {
			return false
		}
	}
	return true
}

func UnaryApplicableForService(svc string) UnaryInterceptorApplicable {
	return func(info *grpc.UnaryServerInfo) bool {
		// FullMethod is the full RPC method string, i.e., /package.service/method.
		if strings.Contains(info.FullMethod, svc) {
			return true
		}
		return false
	}
}
