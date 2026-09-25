// Package modeldconn is the runtime's client seam to the modeld / embedded daemon:
// it resolves the current lease leader (via modeldprobe), dials it over the gRPC
// transport, and opens sessions.
package modeldconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/contenox/contenox/internal/modeldinstall"
	"github.com/contenox/contenox/internal/models/modeldprobe"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
)

// dataRoot is the contenox data root the probe inspects. It defaults to the
// standard location and is overridable (tests, non-default installs).
var dataRoot string

// SetDataRoot overrides the data root used to locate the owner lease.
func SetDataRoot(root string) {
	dataRoot = root
	serveableMu.Lock()
	serveableBackend, serveableSeenAt = "", time.Time{}
	serveableMu.Unlock()
}

// DataRoot returns the configured local daemon data directory.
func DataRoot() string {
	if dataRoot != "" {
		return dataRoot
	}
	return modeldprobe.DefaultDataRoot()
}

func detector() *modeldprobe.Detector { return modeldprobe.New(dataRoot) }

const (
	snapshotDirEnv     = "CONTENOX_WARM_SNAPSHOT_DIR"
	snapshotDisableEnv = "CONTENOX_WARM_SNAPSHOT_DISABLE"
	snapshotSubdir     = "modeld-snapshots"
)

// SnapshotDir resolves the on-disk directory a backend's warm cache should
// persist durable session snapshots to.
func SnapshotDir(backend string) string {
	if os.Getenv(snapshotDisableEnv) != "" {
		return ""
	}
	root := os.Getenv(snapshotDirEnv)
	if root == "" {
		d := dataRoot
		if d == "" {
			d = modeldprobe.DefaultDataRoot()
		}
		root = filepath.Join(d, snapshotSubdir)
	}
	return filepath.Join(root, backend)
}

// Available is the cheap, offline check: is a model daemon currently holding a fresh lease?
func Available() bool { return detector().Detect().State == modeldprobe.StateRunning }

// Backend is the cheap, offline check for the inference backend the running owner serves.
func Backend() string {
	st := detector().Detect()
	if st.State != modeldprobe.StateRunning {
		return ""
	}
	return st.Backend
}

const serveableGraceWindow = 60 * time.Second

var (
	serveableMu      sync.Mutex
	serveableBackend string
	serveableSeenAt  time.Time
)

// ServeableBackend reports the inference backend modeld can serve, smoothed over
// brief lease gaps so local-model capability does not flap during a daemon restart.
func ServeableBackend() string { return serveableFrom(Backend(), time.Now()) }

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

var (
	autoMu       sync.Mutex
	autoDisabled bool
	autoSpawned  *exec.Cmd
	autoDone     chan struct{}
	autoStarting chan struct{}
)

// SetDisabled turns automatic daemon startup on or off for this process.
func SetDisabled(disabled bool) {
	autoMu.Lock()
	autoDisabled = disabled
	autoMu.Unlock()
}

func autostartDisabled() bool {
	autoMu.Lock()
	defer autoMu.Unlock()
	return autoDisabled
}

// StopAutoStarted stops the daemon this process auto-spawned, if any. Safe to
// call more than once and when nothing was spawned. An auto-started modeld
// lives as long as its hosting process: call this when that process exits so
// no daemon outlives the surface that brought it up.
func StopAutoStarted() {
	autoMu.Lock()
	cmd := autoSpawned
	done := autoDone
	autoSpawned = nil
	autoDone = nil
	autoMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		if err := cmd.Process.Signal(os.Interrupt); err == nil && done != nil {
			select {
			case <-done:
				return
			case <-time.After(2 * time.Second):
			}
		}
		_ = cmd.Process.Kill()
		if done != nil {
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}
}

// ensureReadyWindow bounds how long EnsureDaemon waits for a freshly spawned
// daemon to answer its health probe.
const ensureReadyWindow = 5 * time.Second

// EnsureDaemon ensures the local engine daemon is running, auto-spawning it in
// the background if it is not currently active. The spawned daemon is tracked
// and torn down by StopAutoStarted when its hosting process exits. Auto-start
// is a no-op while SetDisabled(true) is in force.
func EnsureDaemon(ctx context.Context) error {
	if autostartDisabled() {
		return nil
	}
	return ensureDaemon(ctx, detector())
}

// daemonProbe is the slice of the Detector EnsureDaemon depends on, so tests
// can stub the daemon state without touching the real lease.
type daemonProbe interface {
	Probe(context.Context) modeldprobe.Status
	Detect() modeldprobe.Status
	LauncherArgs(binary string) []string
}

func ensureDaemon(ctx context.Context, p daemonProbe) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		autoMu.Lock()
		if autoStarting == nil {
			autoStarting = make(chan struct{})
			autoMu.Unlock()
			break
		}
		pending := autoStarting
		autoMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pending:
		}
	}
	defer func() { autoMu.Lock(); close(autoStarting); autoStarting = nil; autoMu.Unlock() }()

	st := p.Probe(ctx)
	if st.State == modeldprobe.StateRunning {
		return nil
	}

	stDetect := p.Detect()
	if stDetect.Binary == "" {
		return modeldprobe.ErrNotInstalled
	}

	StopAutoStarted()
	args := p.LauncherArgs(stDetect.Binary)
	cmd := modeldinstall.CommandContext(context.Background(), stDetect.Binary, args...)
	cmd.Env = append(cmd.Environ(), "CONTENOX_DATA_ROOT="+DataRoot())
	if os.Getenv("CONTENOX_MODELD_BACKEND") == "" {
		backend, err := modeldprobe.PreferredBackend(DataRoot())
		if err != nil {
			return fmt.Errorf("read managed modeld backend: %w", err)
		}
		if backend != "" {
			cmd.Env = append(cmd.Env, "CONTENOX_MODELD_BACKEND="+backend)
		}
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("autostart model daemon: %w", err)
	}
	done := make(chan struct{})
	autoMu.Lock()
	autoSpawned, autoDone = cmd, done
	autoMu.Unlock()

	ready := false
	defer func() {
		if !ready {
			autoMu.Lock()
			if autoSpawned == cmd {
				autoSpawned = nil
			}
			autoMu.Unlock()
			_ = cmd.Process.Kill()
		}
	}()
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		close(done)
		autoMu.Lock()
		if autoSpawned == cmd {
			autoSpawned = nil
		}
		autoMu.Unlock()
		exited <- err
	}()

	deadline := time.Now().Add(ensureReadyWindow)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exited:
			if err == nil {
				return fmt.Errorf("autostart model daemon: exited before readiness")
			}
			return fmt.Errorf("autostart model daemon: exited during startup: %w", err)
		case <-time.After(100 * time.Millisecond):
		}
		if p.Probe(ctx).State == modeldprobe.StateRunning {
			ready = true
			return nil
		}
	}
	return p.Probe(ctx).Err()
}

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

// ModeldTarget describes a specific modeld to talk to (local lease or remote).
type ModeldTarget struct {
	BackendID string
	Endpoint  string
	Instance  string
}

// ModelRef is the typed model handle the runtime passes to modeld.
type ModelRef struct {
	Name     string
	Type     string
	Digest   string
	Path     string
	Adapters []transport.AdapterSpec
}

// OpenSessionTarget opens using the target.
func OpenSessionTarget(ctx context.Context, target ModeldTarget, ref ModelRef, cfg transport.Config) (transport.Session, error) {
	if target.Endpoint == "" {
		return OpenSession(ctx, ref, cfg)
	}
	ec, err := Endpoint(ctx, target.BackendID, target.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("modeld target %s: %w", target.BackendID, err)
	}
	req := openRequest(ec.InstanceID, ref, cfg)
	sess, err := ec.OpenSession(ctx, req)
	if errors.Is(err, transport.ErrModelSwitchRequired) || errors.Is(err, transport.ErrModelNotActive) {
		if _, loadErr := ec.LoadModel(ctx, loadRequest(ec.InstanceID, ref, cfg)); loadErr != nil {
			return nil, loadErr
		}
		return ec.OpenSession(ctx, req)
	}
	return sess, err
}

// DescribeTarget is the targeted equivalent of Describe.
func DescribeTarget(ctx context.Context, target ModeldTarget, ref ModelRef, cfg transport.Config) (transport.ModelInfo, error) {
	if target.Endpoint == "" {
		return Describe(ctx, ref, cfg)
	}
	ec, err := Endpoint(ctx, target.BackendID, target.Endpoint)
	if err != nil {
		return transport.ModelInfo{}, fmt.Errorf("modeld target %s: %w", target.BackendID, err)
	}
	return ec.Describe(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: ec.InstanceID},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
		Adapters:  ref.Adapters,
	})
}

// EmbedTarget is the targeted equivalent of Embed.
func EmbedTarget(ctx context.Context, target ModeldTarget, ref ModelRef, cfg transport.Config, text string) (transport.EmbedResult, error) {
	if target.Endpoint == "" {
		return Embed(ctx, ref, cfg, text)
	}
	ec, err := Endpoint(ctx, target.BackendID, target.Endpoint)
	if err != nil {
		return transport.EmbedResult{}, fmt.Errorf("modeld target %s: %w", target.BackendID, err)
	}
	return ec.Embed(ctx, transport.EmbedRequest{
		Fence:     transport.Fence{OwnerInstanceID: ec.InstanceID},
		ModelName: ref.Name,
		Type:      ref.Type,
		Digest:    ref.Digest,
		Path:      ref.Path,
		Config:    cfg,
		Text:      text,
	})
}

// ListModelsTarget lists models on the target.
func ListModelsTarget(ctx context.Context, target ModeldTarget) ([]transport.NodeModel, error) {
	if target.Endpoint == "" {
		_ = EnsureDaemon(ctx)
		st := detector().Probe(ctx)
		if st.State != modeldprobe.StateRunning {
			return nil, st.Err()
		}
		ec, err := Endpoint(ctx, "local", st.Endpoint)
		if err != nil {
			return nil, err
		}
		return ec.ListModels(ctx)
	}
	ec, err := Endpoint(ctx, target.BackendID, target.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("modeld target %s: %w", target.BackendID, err)
	}
	return ec.ListModels(ctx)
}

// PushModelTarget pushes a model stream to the target.
func PushModelTarget(ctx context.Context, target ModeldTarget, manifest transport.PushManifest, r io.Reader) (transport.PushResult, error) {
	if target.Endpoint == "" {
		_ = EnsureDaemon(ctx)
		st := detector().Probe(ctx)
		if st.State != modeldprobe.StateRunning {
			return transport.PushResult{}, st.Err()
		}
		ec, err := Endpoint(ctx, "local", st.Endpoint)
		if err != nil {
			return transport.PushResult{}, err
		}
		return ec.PushModel(ctx, manifest, r)
	}
	ec, err := Endpoint(ctx, target.BackendID, target.Endpoint)
	if err != nil {
		return transport.PushResult{}, fmt.Errorf("modeld target %s: %w", target.BackendID, err)
	}
	return ec.PushModel(ctx, manifest, r)
}

// OpenSession opens a session, auto-starting the local daemon if necessary.
func OpenSession(ctx context.Context, ref ModelRef, cfg transport.Config) (transport.Session, error) {
	_ = EnsureDaemon(ctx)
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

// Status returns the live modeld status.
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

// LoadModel explicitly activates the single modeld slot.
func LoadModel(ctx context.Context, ref ModelRef, cfg transport.Config, expectedGeneration ...uint64) (transport.ActiveModel, error) {
	_ = EnsureDaemon(ctx)
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return transport.ActiveModel{}, st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return transport.ActiveModel{}, err
	}
	req := loadRequest(st.Instance, ref, cfg)
	if len(expectedGeneration) > 0 {
		req.ExpectedGeneration = expectedGeneration[0]
	}
	return c.LoadModel(ctx, req)
}

// StopModel unloads only the named resident model, fenced to the observed owner and slot generation.
func StopModel(ctx context.Context, name string) error {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		return st.Err()
	}
	c, err := dial(st.Endpoint, st.Instance)
	if err != nil {
		return err
	}
	status, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if status.Active == nil || status.Active.ModelName != name {
		return fmt.Errorf("model %q is not resident", name)
	}
	return c.UnloadModel(ctx, transport.UnloadModelRequest{
		Fence:              transport.Fence{OwnerInstanceID: st.Instance},
		ExpectedGeneration: status.Active.Generation,
	})
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
		Adapters:  ref.Adapters,
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
		Adapters:  ref.Adapters,
	}
}

// Describe returns model capabilities from metadata.
func Describe(ctx context.Context, ref ModelRef, cfg transport.Config) (transport.ModelInfo, error) {
	_ = EnsureDaemon(ctx)
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
		Adapters:  ref.Adapters,
	})
}

// Embed computes embeddings for text.
func Embed(ctx context.Context, ref ModelRef, cfg transport.Config, text string) (transport.EmbedResult, error) {
	_ = EnsureDaemon(ctx)
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
