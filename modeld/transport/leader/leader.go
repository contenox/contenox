// Package leader routes follower calls to the current modeld owner advertised
// in the lease file.
package leader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/modeld/owner"
	"github.com/contenox/runtime/modeld/transport"
	modeldgrpc "github.com/contenox/runtime/modeld/transport/grpc"
	"google.golang.org/grpc"
)

var (
	ErrNoLiveLeader = errors.New("modeld/leader: no live leader")
	ErrNoEndpoint   = errors.New("modeld/leader: leader has no endpoint")
)

// Client is a fenced remote Service connection to the current lease holder.
type Client struct {
	transport.Service

	Record     liblease.Record
	InstanceID string
	Endpoint   string

	close func() error
}

func (c *Client) Close() error {
	if c.close == nil {
		return nil
	}
	return c.close()
}

// Dial reads leasePath, extracts the advertised endpoint, and returns a remote
// Service client that fences every RPC with the lease instance id.
func Dial(ctx context.Context, leasePath string, opts ...grpc.DialOption) (*Client, error) {
	rec, err := liblease.Inspect(leasePath)
	if err != nil {
		return nil, err
	}
	if time.Now().After(rec.ExpiresAt()) {
		return nil, fmt.Errorf("%w: expired at %s", ErrNoLiveLeader, rec.ExpiresAt().Format(time.RFC3339))
	}
	endpoint := rec.Meta[owner.EndpointMetaKey]
	if endpoint == "" {
		return nil, ErrNoEndpoint
	}

	svc, err := modeldgrpc.DialLeader(ctx, endpoint, rec.InstanceID, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		Service:    svc,
		Record:     rec,
		InstanceID: rec.InstanceID,
		Endpoint:   endpoint,
		close:      svc.Close,
	}, nil
}
