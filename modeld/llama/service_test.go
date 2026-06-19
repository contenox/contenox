//go:build !llamanode

package llama_test

import (
	"context"
	"errors"
	"testing"

	"github.com/contenox/runtime/modeld/llama"
	"github.com/contenox/runtime/runtime/transport"
)

// fakeSession is a no-op transport.Session used to prove the Service wires a
// request to a backend session without compiling the CGO llama.cpp backend.
type fakeSession struct{ closed bool }

func (f *fakeSession) EnsurePrefix(context.Context, transport.PrefixInput) (transport.PrefixStatus, error) {
	return transport.PrefixStatus{}, nil
}

func (f *fakeSession) PrefillSuffix(context.Context, transport.SuffixInput) (transport.SuffixStatus, error) {
	return transport.SuffixStatus{}, nil
}

func (f *fakeSession) Decode(context.Context, transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	ch := make(chan transport.StreamChunk)
	close(ch)
	return ch, nil
}

func (f *fakeSession) ExplainContext() transport.ContextReport { return transport.ContextReport{} }

func (f *fakeSession) Snapshot(context.Context) (transport.SessionSnapshot, error) {
	return transport.SessionSnapshot{}, nil
}

func (f *fakeSession) Restore(context.Context, transport.SessionSnapshot) error { return nil }

func (f *fakeSession) Close() error { f.closed = true; return nil }

// The reshaped modeld/llama exposes exactly one boundary: transport.Service.
var _ transport.Service = (*llama.Service)(nil)

func TestOpenSessionWithoutBackendIsUnavailable(t *testing.T) {
	llama.SetSessionFactory(nil)
	if llama.SessionAvailable() {
		t.Fatal("no backend should be available after clearing the factory")
	}
	_, err := (&llama.Service{}).OpenSession(context.Background(), transport.OpenSessionRequest{Path: "x", Type: "llama"})
	if !errors.Is(err, llama.ErrSessionUnavailable) {
		t.Fatalf("want ErrSessionUnavailable, got %v", err)
	}
}

func TestOpenSessionRejectsForeignBackend(t *testing.T) {
	llama.SetSessionFactory(func(string, transport.Config) (transport.Session, error) {
		t.Fatal("backend must not be reached for a foreign model type")
		return nil, nil
	})
	t.Cleanup(func() { llama.SetSessionFactory(nil) })
	_, err := (&llama.Service{}).OpenSession(context.Background(), transport.OpenSessionRequest{
		Path: "/ir/coder", Type: "openvino",
	})
	if !errors.Is(err, transport.ErrBackendMismatch) {
		t.Fatalf("want ErrBackendMismatch, got %v", err)
	}
}

func TestOpenSessionRoutesModelAndConfigToBackend(t *testing.T) {
	var gotModel string
	var gotCfg transport.Config
	fake := &fakeSession{}
	llama.SetSessionFactory(func(modelPath string, cfg transport.Config) (transport.Session, error) {
		gotModel, gotCfg = modelPath, cfg
		return fake, nil
	})
	t.Cleanup(func() { llama.SetSessionFactory(nil) })

	// NumGpuLayers is intentionally omitted: GPU layers are subject to capacity
	// resolution (zeroed without an accelerator), which is covered by the capacity
	// tests. This test proves the request is routed to the backend, so it uses
	// fields that pass through unchanged.
	cfg := transport.Config{NumCtx: 4096, PromptFormat: "chatml"}
	sess, err := (&llama.Service{}).OpenSession(context.Background(), transport.OpenSessionRequest{
		ModelName: "foo",
		Type:      "llama",
		Path:      "/models/foo/model.gguf",
		Config:    cfg,
	})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if gotModel != "/models/foo/model.gguf" {
		t.Errorf("model id not routed to backend: got %q", gotModel)
	}
	if gotCfg.NumCtx != 4096 || gotCfg.PromptFormat != "chatml" {
		t.Errorf("config not routed to backend: got %+v", gotCfg)
	}
	if sess != transport.Session(fake) {
		t.Fatal("OpenSession did not return the backend session")
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fake.closed {
		t.Error("Close did not reach the backend session")
	}
}
