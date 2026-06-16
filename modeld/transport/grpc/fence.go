package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// WithOwnerToken requires every incoming RPC to carry the lease instance id of
// the owner serving this endpoint. This fences followers against stale leaders:
// after takeover, calls carrying the old token are rejected instead of reaching
// resident model state.
func WithOwnerToken(instanceID string) grpc.ServerOption {
	if instanceID == "" {
		return grpc.EmptyServerOption{}
	}
	return grpc.UnaryInterceptor(ownerTokenInterceptor(instanceID))
}

func ownerTokenInterceptor(instanceID string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "modeld: missing owner fencing token")
		}
		values := md.Get(OwnerInstanceMetadataKey)
		if len(values) != 1 || values[0] != instanceID {
			return nil, status.Error(codes.FailedPrecondition, "modeld: stale owner fencing token")
		}
		return handler(ctx, req)
	}
}
