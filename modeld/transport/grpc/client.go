package grpc

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/transport/grpc/modeldpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const OwnerInstanceMetadataKey = "x-modeld-owner-instance"

// Client adapts a gRPC ModelRepo connection to transport.Service.
type Client struct {
	conn          *grpc.ClientConn
	pb            modeldpb.ModelRepoClient
	expectedOwner string
}

// NewClient wraps an existing gRPC connection.
func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{pb: modeldpb.NewModelRepoClient(conn)}
}

// Dial connects to a modeld leader endpoint and returns a Service client.
func Dial(ctx context.Context, target string, opts ...grpc.DialOption) (*Client, error) {
	base := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	base = append(base, opts...)
	conn, err := grpc.DialContext(ctx, target, base...)
	if err != nil {
		return nil, fmt.Errorf("modeld/grpc: dial %q: %w", target, err)
	}
	return &Client{conn: conn, pb: modeldpb.NewModelRepoClient(conn)}, nil
}

// DialLeader connects to a modeld leader endpoint and fences every call with the
// expected owner instance id from the lease record.
func DialLeader(ctx context.Context, target, expectedOwner string, opts ...grpc.DialOption) (*Client, error) {
	c, err := Dial(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	c.expectedOwner = expectedOwner
	return c, nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) context(ctx context.Context) context.Context {
	if c.expectedOwner == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, OwnerInstanceMetadataKey, c.expectedOwner)
}

func (c *Client) RegisterBackend(ctx context.Context, backendID string, spec modeld.BackendSpec) error {
	_, err := c.pb.RegisterBackend(c.context(ctx), &modeldpb.RegisterBackendRequest{
		BackendId: backendID,
		Spec:      backendSpecToPB(spec),
	})
	return err
}

func (c *Client) RemoveBackend(ctx context.Context, backendID string) error {
	_, err := c.pb.RemoveBackend(c.context(ctx), &modeldpb.RemoveBackendRequest{BackendId: backendID})
	return err
}

func (c *Client) ListBackends(ctx context.Context) ([]string, error) {
	resp, err := c.pb.ListBackends(c.context(ctx), &modeldpb.ListBackendsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetBackendIds(), nil
}

func (c *Client) ListModels(ctx context.Context, backendID string) ([]modeld.ObservedModel, error) {
	resp, err := c.pb.ListModels(c.context(ctx), &modeldpb.ListModelsRequest{BackendId: backendID})
	if err != nil {
		return nil, err
	}
	models := resp.GetModels()
	out := make([]modeld.ObservedModel, 0, len(models))
	for _, m := range models {
		out = append(out, observedModelFromPB(m))
	}
	return out, nil
}
