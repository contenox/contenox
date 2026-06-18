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

// Backend is the cheap, offline check for the inference backend the running
// modeld owner serves ("llama"/"openvino"/"none"), read from the lease. Empty
// when no owner holds a fresh lease. Providers gate on this so the backend modeld
// is NOT in advertises no local capability instead of failing deep in the engine.
func Backend() string {
	st := detector().Detect()
	if st.State != modeldprobe.StateRunning {
		return ""
	}
	return st.Backend
}

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

// ModelRef is the typed model handle the runtime passes to modeld: a logical
// Name + backend Type + content Digest form the cache identity, and Path is the
// runtime-resolved on-disk location (GGUF file or IR directory) the daemon loads
// from. Type lets the daemon reject a model it does not serve.
type ModelRef struct {
	Name   string
	Type   string
	Digest string
	Path   string
}

// OpenSession resolves the modeld leader, confirms it actually answers a health
// probe, and opens a session on it. The returned session is resident in modeld;
// the caller drives EnsurePrefix/PrefillSuffix/Decode over the wire. A
// not-running/unreachable owner surfaces the probe's typed error; a model typed
// for a backend the daemon does not serve surfaces transport.ErrBackendMismatch.
func OpenSession(ctx context.Context, ref ModelRef, cfg transport.Config) (transport.Session, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return nil, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return nil, err
	}
	return c.OpenSession(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: st.Instance},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
	})
}
