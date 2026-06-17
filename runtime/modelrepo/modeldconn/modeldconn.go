// Package modeldconn is the runtime's client seam to the modeld daemon: it
// resolves the current lease leader (via modeldprobe), dials it over the gRPC
// transport, and opens sessions. Local backend providers (llama, openvino) call
// OpenSession instead of constructing an in-process CGO session, so the runtime
// stays pure Go and talks to modeld only through runtime/transport.
package modeldconn

import (
	"context"
	"sync"

	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// dataRoot is the contenox data root the probe inspects. It defaults to the
// standard location and is overridable (tests, non-default installs).
var dataRoot string

// SetDataRoot overrides the data root used to locate the owner lease.
func SetDataRoot(root string) { dataRoot = root }

func detector() *modeldprobe.Detector { return modeldprobe.New(dataRoot) }

// Available is the cheap, offline check (lease inspection, no network): is a
// modeld owner currently holding a fresh lease? Providers use it to gate
// capability advertisement without a round-trip per call.
func Available() bool { return detector().Detect().State == modeldprobe.StateRunning }

// connection caches a single dialed client to the current leader, keyed by
// endpoint+instance so a takeover re-dials.
var (
	mu     sync.Mutex
	client *transportgrpc.Client
	key    string
)

func dial(endpoint, instance string) (*transportgrpc.Client, error) {
	k := endpoint + "|" + instance
	mu.Lock()
	defer mu.Unlock()
	if client != nil && key == k {
		return client, nil
	}
	if client != nil {
		_ = client.Close()
		client = nil
	}
	c, err := transportgrpc.DialLeader(endpoint, instance)
	if err != nil {
		return nil, err
	}
	client, key = c, k
	return c, nil
}

// OpenSession resolves the modeld leader, confirms it actually answers a health
// probe, and opens a session on it. The returned session is resident in modeld;
// the caller drives EnsurePrefix/PrefillSuffix/Decode over the wire. A
// not-running/unreachable owner surfaces the probe's typed error.
func OpenSession(ctx context.Context, modelID string, cfg transport.Config) (transport.Session, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return nil, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return nil, err
	}
	return c.OpenSession(ctx, transport.OpenSessionRequest{
		Fence:   transport.Fence{OwnerInstanceID: st.Instance},
		ModelID: modelID,
		Config:  cfg,
	})
}
