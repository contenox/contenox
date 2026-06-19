package openvino

import (
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
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticMemory(2<<20)),
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
	if !info.Clamped || info.UserLimitBytes != 1<<20 {
		t.Fatalf("capacity explanation missing clamp/user limit: %+v", info)
	}
}

func TestUnit_ServiceDescribeDefaultsResidentCapFromDetectedFreeMemory(t *testing.T) {
	dir := writeTestIR(t)
	svc := NewService(
		WithMemorySource(staticMemory(10<<20)),
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
