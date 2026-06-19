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
	if !info.Clamped || info.UserLimitBytes != 1<<20 {
		t.Fatalf("capacity explanation missing clamp/user limit: %+v", info)
	}
}

func TestUnit_ServiceDescribeDefaultsResidentCapFromDetectedFreeMemory(t *testing.T) {
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(10<<20)),
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
}

func TestUnit_ServiceOpenSessionRejectsOversizedContextBeforeBackend(t *testing.T) {
	path := writeTestGGUF(t, 32768)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
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
	if cfg.NumGpuLayers != 999 {
		t.Fatalf("NumGpuLayers = %d, want daemon env override 999", cfg.NumGpuLayers)
	}
	if cfg.NumCtx != 16384 {
		t.Fatalf("NumCtx = %d, want daemon env override 16384", cfg.NumCtx)
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
