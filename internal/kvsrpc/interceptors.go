package kvsrpc

import (
	"context"
	"crypto/subtle"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryLoggingInterceptor logs each unary call with its status code and duration.
func UnaryLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.Printf("grpc unary method=%s code=%s duration=%s", info.FullMethod, status.Code(err), time.Since(start))
		return resp, err
	}
}

// StreamLoggingInterceptor logs stream open/close with the final status code.
func StreamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		log.Printf("grpc stream method=%s opened", info.FullMethod)
		err := handler(srv, ss)
		log.Printf("grpc stream method=%s closed code=%s duration=%s", info.FullMethod, status.Code(err), time.Since(start))
		return err
	}
}

// authorize checks the "authorization: Bearer <token>" metadata entry.
// grpc-gateway forwards the HTTP Authorization header with a
// "grpcgateway-" prefix, so that key is accepted too.
func authorize(ctx context.Context, token string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		values = md.Get("grpcgateway-authorization")
	}
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization token")
	}

	got, found := strings.CutPrefix(values[0], "Bearer ")
	if !found {
		return status.Error(codes.Unauthenticated, "authorization must be a Bearer token")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	return nil
}

// UnaryAuthInterceptor rejects unary calls lacking a valid bearer token.
func UnaryAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, token); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor rejects streaming calls lacking a valid bearer token.
func StreamAuthInterceptor(token string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := authorize(ss.Context(), token); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}
