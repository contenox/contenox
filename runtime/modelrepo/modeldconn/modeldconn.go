// Package modeldconn is the runtime's client seam to the modeld daemon: it
// resolves the current lease leader (via modeldprobe), dials it over the gRPC
// transport, and opens sessions. Local backend providers (llama, openvino) call
// OpenSession instead of constructing an in-process CGO session, so the runtime
// stays pure Go and talks to modeld only through runtime/transport.
package modeldconn

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// dataRoot is the contenox data root the probe inspects. It defaults to the
// standard location and is overridable (tests, non-default installs).
var dataRoot string

// SetDataRoot overrides the data root used to locate the owner lease. It also
// invalidates the serveable-backend grace cache: a different data root is a
// different modeld context, so a backend observed under the old root must not
// keep being advertised under the new one.
func SetDataRoot(root string) {
	dataRoot = root
	serveableMu.Lock()
	serveableBackend, serveableSeenAt = "", time.Time{}
	serveableMu.Unlock()
}

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

// serveableGraceWindow is how long a previously-observed backend stays advertised
// after the modeld lease stops reading fresh. The modeld owner renews its lease
// well within a 30s TTL, so the only time Backend() reads "" while modeld is in
// fact coming back is a daemon restart / lease-renewal gap (a few seconds). A
// grace of 2x the TTL covers a restart without keeping a genuinely-down daemon's
// models selectable for long.
const serveableGraceWindow = 60 * time.Second

var (
	serveableMu      sync.Mutex
	serveableBackend string
	serveableSeenAt  time.Time
)

// ServeableBackend reports the inference backend modeld can serve, smoothed over
// brief lease gaps so local-model capability does not flap during a daemon
// restart. While a fresh lease is held it returns the live backend and refreshes
// the cache; during a gap it returns the last-observed backend for up to
// serveableGraceWindow; after that it returns "". Capability advertisement
// (provider SessionAvailable / catalog listing) uses this; live-decode paths keep
// using the strict Backend()/Available() so they fail honestly when modeld is
// actually gone.
func ServeableBackend() string { return serveableFrom(Backend(), time.Now()) }

// serveableFrom is the pure core of ServeableBackend, taking the live backend and
// clock so the grace behavior is unit-testable without a lease file.
func serveableFrom(live string, now time.Time) string {
	serveableMu.Lock()
	defer serveableMu.Unlock()
	if live != "" {
		serveableBackend, serveableSeenAt = live, now
		return live
	}
	if serveableBackend != "" && now.Sub(serveableSeenAt) <= serveableGraceWindow {
		return serveableBackend
	}
	serveableBackend = ""
	return ""
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
	req := openRequest(st.Instance, ref, cfg)
	sess, err := c.OpenSession(ctx, req)
	if errors.Is(err, transport.ErrModelSwitchRequired) || errors.Is(err, transport.ErrModelNotActive) {
		if _, loadErr := c.LoadModel(ctx, loadRequest(st.Instance, ref, cfg)); loadErr != nil {
			return nil, loadErr
		}
		return c.OpenSession(ctx, req)
	}
	return sess, err
}

// Status returns the live modeld slot status. It is a runtime-state check, not
// an installed-model catalog scan.
func Status(ctx context.Context) (transport.DaemonStatus, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return transport.DaemonStatus{}, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return transport.DaemonStatus{}, err
	}
	return c.Status(ctx)
}

// LoadModel explicitly activates the single modeld slot, switching away from an
// idle active model when needed.
func LoadModel(ctx context.Context, ref ModelRef, cfg transport.Config) (transport.ActiveModel, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return transport.ActiveModel{}, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return transport.ActiveModel{}, err
	}
	return c.LoadModel(ctx, loadRequest(st.Instance, ref, cfg))
}

// UnloadModel releases the active modeld slot.
func UnloadModel(ctx context.Context, expectedGeneration uint64) error {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return err
	}
	return c.UnloadModel(ctx, transport.UnloadModelRequest{
		Fence:              transport.Fence{OwnerInstanceID: st.Instance},
		ExpectedGeneration: expectedGeneration,
	})
}

func openRequest(owner string, ref ModelRef, cfg transport.Config) transport.OpenSessionRequest {
	return transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: owner},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
	}
}

func loadRequest(owner string, ref ModelRef, cfg transport.Config) transport.LoadModelRequest {
	return transport.LoadModelRequest{
		Fence:     transport.Fence{OwnerInstanceID: owner},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
	}
}

// Describe asks the running modeld owner for a model's capabilities, read from
// the model metadata by the backend that serves it. This is the modeld→runtime
// info-flow for model facts (e.g. the trained context window): the runtime is
// the consumer and never parses model files itself. A not-running/unreachable
// owner surfaces the probe's typed error.
func Describe(ctx context.Context, ref ModelRef, cfg transport.Config) (transport.ModelInfo, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return transport.ModelInfo{}, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return transport.ModelInfo{}, err
	}
	return c.Describe(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: st.Instance},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
	})
}

// Embed asks the running modeld owner to compute a one-shot embedding for text.
// It uses the same typed model handle and owner fence as Describe, but does not
// open a persistent session.
func Embed(ctx context.Context, ref ModelRef, cfg transport.Config, text string) (transport.EmbedResult, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return transport.EmbedResult{}, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return transport.EmbedResult{}, err
	}
	return c.Embed(ctx, transport.EmbedRequest{
		Fence:     transport.Fence{OwnerInstanceID: st.Instance},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
		Text:      text,
	})
}
