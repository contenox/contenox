package llama

import (
	"bytes"
	"encoding/binary"
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

func TestUnit_ServiceDescribeResolvesCapacity(t *testing.T) {
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
		WithHostMemorySource(staticMemory(0)),
		WithCapacityPolicy(capacity.Policy{MaxResidentBytes: 1 << 20, HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 4096, KVCacheType: "f16"},
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
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(10<<20)),
		WithHostMemorySource(staticMemory(16<<20)),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 1024, KVCacheType: "f16"},
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
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
		WithHostMemorySource(staticMemory(16<<20)),
		WithCapacityPolicy(capacity.Policy{MaxResidentBytes: 1 << 20, HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{KVCacheType: "f16"},
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

func TestUnit_ServiceDescribeReportsSparseAttentionForSWA(t *testing.T) {
	path := writeTestGGUFWithSWA(t, 32768, 4096)
	svc := NewService(
		WithMemorySource(staticMemory(64<<20)),
		WithHostMemorySource(staticMemory(0)),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 4096, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if !info.SparseAttention || info.SlidingWindowAttentionTokens != 4096 {
		t.Fatalf("sparse attention metadata = enabled %v window %d, want true/4096", info.SparseAttention, info.SlidingWindowAttentionTokens)
	}
}

func TestUnit_ServiceOpenSessionRejectsOversizedContextBeforeBackend(t *testing.T) {
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
		WithHostMemorySource(staticMemory(0)),
		WithCapacityPolicy(capacity.Policy{MaxResidentBytes: 1 << 20, HeadroomFrac: 0.1}),
	)

	_, err := svc.OpenSession(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 4096, KVCacheType: "f16"},
	})
	if !errors.Is(err, transport.ErrContextOverflow) {
		t.Fatalf("OpenSession = %v, want ErrContextOverflow", err)
	}
}

func TestUnit_ServiceResolveConfigAppliesDaemonEnvOverrides(t *testing.T) {
	t.Setenv("CONTENOX_LLAMA_GPU_LAYERS", "999")
	t.Setenv("CONTENOX_LLAMA_CTX", "16384")
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		// An accelerator snapshot with ample memory: the daemon GPU-layer override
		// only materializes as offload when an accelerator is present (otherwise it
		// resolves to 0 — see TestUnit_ServiceZeroesDaemonGpuLayersWithoutAccelerator),
		// and ample memory keeps the budget from clamping it back to zero.
		WithMemorySource(staticSnapshot{snap: capacity.DeviceSnapshot{
			Kind:       "gpu",
			DeviceID:   "test-gpu",
			TotalBytes: 16 << 30,
			FreeBytes:  16 << 30,
		}}),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	cfg, err := svc.resolveConfig(transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 8192, NumGpuLayers: 0, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if cfg.NumGpuLayers <= 0 {
		t.Fatalf("NumGpuLayers = %d, want positive layer count from daemon env override 999", cfg.NumGpuLayers)
	}
	if cfg.NumCtx != 16384 {
		t.Fatalf("NumCtx = %d, want daemon env override 16384", cfg.NumCtx)
	}
}

func TestUnit_ServiceResolveConfigHonorsRequestedPlannerContext(t *testing.T) {
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(16<<20)),
		WithHostMemorySource(staticMemory(0)),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	cfg, err := svc.resolveConfig(transport.OpenSessionRequest{
		Type: "llama",
		Path: path,
		Config: transport.Config{
			NumCtx:                  256,
			PlannerEffectiveContext: 384,
			KVCacheType:             "f16",
		},
	})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if cfg.NumCtx != 256 || cfg.HotContextTokens != 256 {
		t.Fatalf("hot config = num_ctx %d hot %d, want 256/256", cfg.NumCtx, cfg.HotContextTokens)
	}
	if cfg.PlannerEffectiveContext != 384 {
		t.Fatalf("PlannerEffectiveContext = %d, want requested logical planner 384", cfg.PlannerEffectiveContext)
	}
}

func TestUnit_ServiceAutoOffloadsToDetectedAccelerator(t *testing.T) {
	// No CONTENOX_LLAMA_GPU_LAYERS and no profile request (NumGpuLayers: 0): modeld
	// must still offload because it detected an accelerator with ample memory —
	// the value is derived from the device, not a knob.
	t.Setenv("CONTENOX_LLAMA_GPU_LAYERS", "")
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticSnapshot{snap: capacity.DeviceSnapshot{
			Kind:       "gpu",
			DeviceID:   "test-gpu",
			TotalBytes: 16 << 30,
			FreeBytes:  16 << 30,
		}}),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	cfg, err := svc.resolveConfig(transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 8192, NumGpuLayers: 0, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if cfg.NumGpuLayers <= 0 {
		t.Fatalf("NumGpuLayers = %d, want auto-offload (>0) on a detected accelerator without any request", cfg.NumGpuLayers)
	}
}

func TestUnit_ServiceZeroesDaemonGpuLayersWithoutAccelerator(t *testing.T) {
	t.Setenv("CONTENOX_LLAMA_GPU_LAYERS", "999")
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		// staticMemory yields a non-accelerator (system RAM) snapshot — the
		// universal-binary-on-CPU case.
		WithMemorySource(staticMemory(16<<30)),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	cfg, err := svc.resolveConfig(transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 8192, NumGpuLayers: 0, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if cfg.NumGpuLayers != 0 {
		t.Fatalf("NumGpuLayers = %d, want 0 without an accelerator", cfg.NumGpuLayers)
	}

	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 8192, NumGpuLayers: 999, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.RequestedGpuLayers != 999 || info.ResolvedGpuLayers != 0 {
		t.Fatalf("gpu layer explanation = requested %d resolved %d, want 999/0", info.RequestedGpuLayers, info.ResolvedGpuLayers)
	}
	if !info.Clamped || info.Reason != "no_accelerator_present" {
		t.Fatalf("expected no_accelerator_present clamp explanation: %+v", info)
	}
}

func TestUnit_ServiceResolveConfigClampsDaemonGpuLayersToMemoryBudget(t *testing.T) {
	t.Setenv("CONTENOX_LLAMA_GPU_LAYERS", "999")
	t.Setenv("CONTENOX_LLAMA_GPU_COMPUTE_RESERVE", "8MiB")
	path := writeTestGGUF(t, 32768)
	if err := os.Truncate(path, 90<<20); err != nil {
		t.Fatalf("pad GGUF: %v", err)
	}
	svc := NewService(
		WithMemorySource(staticSnapshot{snap: capacity.DeviceSnapshot{
			Kind:       "gpu",
			DeviceID:   "test-gpu",
			TotalBytes: 256 << 20,
			FreeBytes:  130 << 20,
		}}),
		WithCapacityPolicy(capacity.Policy{HeadroomFrac: 0.1}),
	)

	cfg, err := svc.resolveConfig(transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 8192, NumGpuLayers: 0, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if cfg.NumGpuLayers <= 0 || cfg.NumGpuLayers >= 999 {
		t.Fatalf("NumGpuLayers = %d, want clamped positive layer count", cfg.NumGpuLayers)
	}
	info, err := svc.Describe(t.Context(), transport.OpenSessionRequest{
		Type:   "llama",
		Path:   path,
		Config: transport.Config{NumCtx: 8192, NumGpuLayers: 999, KVCacheType: "f16"},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.RequestedGpuLayers != 999 || info.ResolvedGpuLayers != cfg.NumGpuLayers {
		t.Fatalf("gpu layer explanation = requested %d resolved %d, cfg %d", info.RequestedGpuLayers, info.ResolvedGpuLayers, cfg.NumGpuLayers)
	}
	if !info.Clamped || info.Reason == "" {
		t.Fatalf("expected clamped gpu-layer explanation: %+v", info)
	}
}

func writeTestGGUF(t *testing.T, ctx int) string {
	return writeTestGGUFWithSWA(t, ctx, 0)
}

func writeTestGGUFWithSWA(t *testing.T, ctx, slidingWindow int) string {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("GGUF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(3))
	_ = binary.Write(&b, binary.LittleEndian, uint64(0))
	kvs := []struct {
		key string
		val uint32
	}{
		{"qwen2.context_length", uint32(ctx)},
		{"qwen2.block_count", 2},
		{"qwen2.attention.head_count_kv", 1},
		{"qwen2.attention.head_count", 2},
		{"qwen2.attention.key_length", 128},
	}
	if slidingWindow > 0 {
		kvs = append(kvs, struct {
			key string
			val uint32
		}{"qwen2.attention.sliding_window", uint32(slidingWindow)})
	}
	_ = binary.Write(&b, binary.LittleEndian, uint64(len(kvs)))
	for _, kv := range kvs {
		writeGGUFString(&b, kv.key)
		_ = binary.Write(&b, binary.LittleEndian, ggufUint32)
		_ = binary.Write(&b, binary.LittleEndian, kv.val)
	}
	path := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
