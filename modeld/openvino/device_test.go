package openvino

import (
	"reflect"
	"testing"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
)

func di(name, typ string) ovsession.DeviceInfo {
	return ovsession.DeviceInfo{Name: name, Type: typ}
}

func TestOrderDeviceCandidates(t *testing.T) {
	tests := []struct {
		name     string
		devices  []ovsession.DeviceInfo
		priority []string
		want     []string
	}{
		{
			// NPU ("accel") is excluded from the default CB-path priority: it cannot
			// compile PagedAttention, so it is dropped, not selected.
			name:    "default order: discrete gpu, igpu, cpu; npu dropped",
			devices: []ovsession.DeviceInfo{di("CPU", "cpu"), di("GPU.1", "igpu"), di("NPU", "accel"), di("GPU.0", "gpu")},
			want:    []string{"GPU.0", "GPU.1", "CPU"},
		},
		{
			name:    "multiple discrete gpus preserve enumeration order",
			devices: []ovsession.DeviceInfo{di("GPU.0", "gpu"), di("GPU.1", "gpu")},
			want:    []string{"GPU.0", "GPU.1", "CPU"},
		},
		{
			name:    "cpu appended as fallback when not enumerated",
			devices: []ovsession.DeviceInfo{di("GPU.0", "igpu")},
			want:    []string{"GPU.0", "CPU"},
		},
		{
			name:     "custom priority order is honored",
			devices:  []ovsession.DeviceInfo{di("GPU.0", "gpu"), di("NPU", "accel"), di("CPU", "cpu")},
			priority: []string{"accel", "cpu", "gpu"},
			want:     []string{"NPU", "CPU", "GPU.0"},
		},
		{
			name:    "no devices falls back to cpu",
			devices: nil,
			want:    []string{"CPU"},
		},
		{
			name:    "unknown device types are dropped, cpu still guaranteed",
			devices: []ovsession.DeviceInfo{di("FOO", "weird")},
			want:    []string{"CPU"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orderDeviceCandidates(tt.devices, tt.priority)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("orderDeviceCandidates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenSessionDevices_ExplicitPinSkipsAutodetect(t *testing.T) {
	for _, dev := range []string{"GPU.0", "CPU", "NPU"} {
		got := openSessionDevices(dev, nil)
		if !reflect.DeepEqual(got, []string{dev}) {
			t.Fatalf("openSessionDevices(%q) = %v, want single pin %v", dev, got, []string{dev})
		}
	}
}

func TestOpenSessionDevices_AutoIncludesCPUFallback(t *testing.T) {
	// In the default non-CGO build ovsession.Runtime reports that GenAI is not
	// compiled in, so AUTO is exactly CPU. In the tagged native build it may
	// enumerate accelerators first, but CPU must remain the universal fallback.
	got := openSessionDevices("AUTO", nil)
	if len(got) == 0 || got[len(got)-1] != "CPU" {
		t.Fatalf("openSessionDevices(AUTO) = %v, want CPU fallback as final candidate", got)
	}
	if _, err := ovsession.Runtime(); err != nil && !reflect.DeepEqual(got, []string{"CPU"}) {
		t.Fatalf("openSessionDevices(AUTO) = %v, want [CPU] when runtime probe fails", got)
	}
}

func TestDevicePriority(t *testing.T) {
	t.Setenv("CONTENOX_OPENVINO_DEVICE_PRIORITY", "")
	if got := devicePriority(); !reflect.DeepEqual(got, defaultDevicePriority) {
		t.Fatalf("devicePriority() default = %v, want %v", got, defaultDevicePriority)
	}
	t.Setenv("CONTENOX_OPENVINO_DEVICE_PRIORITY", " npu , gpu ,cpu")
	if got := devicePriority(); !reflect.DeepEqual(got, []string{"accel", "gpu", "cpu"}) {
		t.Fatalf("devicePriority() override = %v, want [accel gpu cpu]", got)
	}
}
