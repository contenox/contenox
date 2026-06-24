package runtimestate

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/statetype"
)

// fakeModeldLease points modeldconn at a temp data root holding a fresh lease that
// advertises the given engine, so the real reconcile path observes "modeld is
// running <engine>" without a live daemon. Returns a cleanup the caller defers.
func fakeModeldLease(t *testing.T, engine string) {
	t.Helper()
	dir := t.TempDir()
	modeldconn.SetDataRoot(dir)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
	lease, err := liblease.Acquire(
		filepath.Join(dir, "modeld.lease"), 30*time.Second,
		liblease.WithMeta(map[string]string{"backend": engine, "endpoint": "127.0.0.1:5000"}),
	)
	if err != nil {
		t.Fatalf("acquire fake modeld lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })
}

func loadState(t *testing.T, s *State, id string) *statetype.BackendRuntimeState {
	t.Helper()
	v, ok := s.state.Load(id)
	if !ok {
		t.Fatalf("no runtime state stored for backend %q", id)
	}
	return v.(*statetype.BackendRuntimeState)
}

// modeld is single-slot and autodetects its engine. With modeld live in the llama
// engine, the openvino local backend must reconcile to an honest dormant error —
// exercised through the real processLocalBackend against a real (faked) lease, not
// a hand-built state entry. (The dormant branch returns before any gRPC/DB call,
// so a zero-value State is sufficient.)
func TestUnit_ProcessLocalBackend_DormantWhenEngineMismatch(t *testing.T) {
	fakeModeldLease(t, "llama")

	s := &State{}
	ov := &runtimetypes.Backend{ID: "ov", Name: "openvino", Type: "openvino", BaseURL: t.TempDir()}
	s.processLocalBackend(context.Background(), ov, nil)

	st := loadState(t, s, "ov")
	if st.Error == "" {
		t.Fatalf("openvino not marked dormant while modeld runs llama: %+v", st)
	}
	if !strings.Contains(st.Error, "llama") || !strings.Contains(st.Error, "dormant") {
		t.Fatalf("dormant error not descriptive: %q", st.Error)
	}
}

// The backend whose format matches the live engine is NOT dormant, even when it
// currently has no pulled models — that is an empty-but-healthy state, not an
// error. Guards against false-positive dormancy on the live engine.
func TestUnit_ProcessLocalBackend_LiveEngineNotDormant(t *testing.T) {
	fakeModeldLease(t, "llama")

	s := &State{}
	llama := &runtimetypes.Backend{ID: "llama", Name: "llama", Type: "llama", BaseURL: t.TempDir()}
	s.processLocalBackend(context.Background(), llama, nil)

	st := loadState(t, s, "llama")
	if st.Error != "" {
		t.Fatalf("live llama backend marked errored with no models: %q", st.Error)
	}
}
