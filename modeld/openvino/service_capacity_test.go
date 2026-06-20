package openvino

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/modeld/capacity"
	"github.com/contenox/runtime/runtime/transport"
)

type staticMemory int64

func (m staticMemory) FreeBytes() (int64, error) { return int64(m), nil }

type staticSnapshot struct {
	snap capacity.DeviceSnapshot
}

func (s staticSnapshot) FreeBytes() (int64, error) { return s.snap.FreeBytes, nil }
func (s staticSnapshot) Snapshot() (capacity.DeviceSnapshot, error) {
	return s.snap, nil
}

type fakeEmbedBackend struct {
	prompt string
	closed bool
	vec    []float32
}

func (b *fakeEmbedBackend) Embed(_ context.Context, prompt string) ([]float32, error) {
	b.prompt = prompt
	return b.vec, nil
}

func (b *fakeEmbedBackend) Close() error {
	b.closed = true
	return nil
}

func TestUnit_ServiceDescribeResolvesCapacity(t *testing.T) {
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
		WithHostMemorySource(staticMemory(0)),
		WithCapacityPolicy(capacity.Policy{MaxResidentBytes: 1 << 20, HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "openvino",
		Path:   dir,
		Config: transport.Config{NumCtx: 4096},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.ModelMaxContext != 32768 {
		t.Fatalf("ModelMaxContext = %d, want 32768", info.ModelMaxContext)
	}
	if info.EffectiveContext <= 0 || info.EffectiveContext >= 4096 {
		t.Fatalf("EffectiveContext = %d, want clamp below request", info.EffectiveContext)
	}
	if info.HotContextTokens != info.EffectiveContext || info.PlannerEffectiveContext != info.EffectiveContext {
		t.Fatalf("context split = hot %d planner %d effective %d", info.HotContextTokens, info.PlannerEffectiveContext, info.EffectiveContext)
	}
	if info.MemoryContextTokens < info.EffectiveContext {
		t.Fatalf("MemoryContextTokens = %d, want >= effective %d", info.MemoryContextTokens, info.EffectiveContext)
	}
	if !info.Clamped || info.UserLimitBytes != 1<<20 {
		t.Fatalf("capacity explanation missing clamp/user limit: %+v", info)
	}
}

func TestUnit_ServiceDescribeDefaultsResidentCapFromDetectedFreeMemory(t *testing.T) {
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticMemory(10<<20)),
		WithHostMemorySource(staticMemory(16<<20)),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "openvino",
		Path:   dir,
		Config: transport.Config{NumCtx: 1024},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.UserLimitBytes != 8<<20 {
		t.Fatalf("UserLimitBytes = %d, want 80%% of detected launch free", info.UserLimitBytes)
	}
	if info.HostColdBudgetBytes != 4<<20 {
		t.Fatalf("HostColdBudgetBytes = %d, want 25%% of host free", info.HostColdBudgetBytes)
	}
}

func TestUnit_ServiceDescribeReportsPlannerAboveHotWithHostColdBudget(t *testing.T) {
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
		WithHostMemorySource(staticMemory(16<<20)),
		WithCapacityPolicy(capacity.Policy{MaxResidentBytes: 1 << 20, HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type: "openvino",
		Path: dir,
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.HotContextTokens != info.EffectiveContext {
		t.Fatalf("hot/effective = %d/%d, want equal dense hot window", info.HotContextTokens, info.EffectiveContext)
	}
	if info.PlannerEffectiveContext <= info.HotContextTokens {
		t.Fatalf("PlannerEffectiveContext = %d, want above hot %d", info.PlannerEffectiveContext, info.HotContextTokens)
	}
	if info.HostColdBudgetBytes != 4<<20 {
		t.Fatalf("HostColdBudgetBytes = %d, want default 4MiB", info.HostColdBudgetBytes)
	}
}

func TestUnit_ServiceDescribeReportsRuntimeAndDeviceFields(t *testing.T) {
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticSnapshot{snap: capacity.DeviceSnapshot{
			Kind:              "gpu",
			DeviceID:          "GPU.0",
			TotalBytes:        16 << 20,
			FreeBytes:         10 << 20,
			SharedWithDisplay: true,
		}}),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type: "openvino",
		Path: dir,
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.RuntimeName != "OpenVINO GenAI" {
		t.Fatalf("RuntimeName = %q, want OpenVINO GenAI", info.RuntimeName)
	}
	if info.DeviceKind != "gpu" || info.DeviceID != "GPU.0" || info.DeviceTotalBytes != 16<<20 || !info.SharedWithDisplay {
		t.Fatalf("device fields not reported: %+v", info)
	}
	if info.OverheadBytes != 0 {
		t.Fatalf("OverheadBytes = %d, want 0 until OpenVINO exposes pre-open overhead", info.OverheadBytes)
	}
}

func TestUnit_ServiceOpenSessionRejectsOversizedContextBeforeBackend(t *testing.T) {
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
		WithHostMemorySource(staticMemory(0)),
		WithCapacityPolicy(capacity.Policy{MaxResidentBytes: 1 << 20, HeadroomFrac: 0.1}),
	)

	_, err := svc.OpenSession(t.Context(), transport.OpenSessionRequest{
		Type:   "openvino",
		Path:   dir,
		Config: transport.Config{NumCtx: 4096},
	})
	if !errors.Is(err, transport.ErrContextOverflow) {
		t.Fatalf("OpenSession = %v, want ErrContextOverflow", err)
	}
}

func TestUnit_ServiceEmbedUsesNativeEmbeddingSession(t *testing.T) {
	old := newEmbedSession
	t.Cleanup(func() { newEmbedSession = old })
	t.Setenv("CONTENOX_OPENVINO_DEVICE", "CPU")

	var gotPath, gotDevice string
	backend := &fakeEmbedBackend{vec: []float32{0.25, 0.5, 0.75}}
	newEmbedSession = func(modelPath, device string) (EmbedSessionBackend, error) {
		gotPath, gotDevice = modelPath, device
		return backend, nil
	}

	res, err := NewService().Embed(t.Context(), transport.EmbedRequest{
		Type: "openvino",
		Path: "/models/embedder",
		Text: "search query",
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotPath != "/models/embedder" || gotDevice != "CPU" {
		t.Fatalf("newEmbedSession path/device = %q/%q", gotPath, gotDevice)
	}
	if backend.prompt != "search query" {
		t.Fatalf("prompt = %q", backend.prompt)
	}
	if !backend.closed {
		t.Fatal("embedding backend was not closed")
	}
	if len(res.Vector) != 3 || res.Vector[0] != 0.25 || res.Vector[2] != 0.75 {
		t.Fatalf("embedding vector = %+v", res.Vector)
	}
}

func TestUnit_ServiceEmbedRejectsBackendMismatch(t *testing.T) {
	_, err := NewService().Embed(t.Context(), transport.EmbedRequest{
		Type: "llama",
		Path: "/models/not-openvino",
		Text: "query",
	})
	if !errors.Is(err, transport.ErrBackendMismatch) {
		t.Fatalf("Embed = %v, want ErrBackendMismatch", err)
	}
}

func writeTestIR(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := []byte(`{
		"max_position_embeddings": 32768,
		"num_hidden_layers": 2,
		"num_key_value_heads": 1,
		"num_attention_heads": 2,
		"hidden_size": 256
	}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openvino_model.bin"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
