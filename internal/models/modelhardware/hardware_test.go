package modelhardware

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func fixtureDetector() detector {
	return detector{
		goos: "linux", goarch: "amd64",
		run:      func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("unavailable") },
		readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
		glob:     func(string) ([]string, error) { return nil, nil },
		ram:      func(context.Context) (int64, int64, error) { return 64 << 30, 40 << 30, nil },
	}
}

func TestUnit_NVIDIAUsesOneMeasuredPool(t *testing.T) {
	d := fixtureDetector()
	d.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "nvidia-smi" {
			t.Fatalf("unexpected command %s", name)
		}
		return []byte("GPU A, 8192, 6144\nGPU B, 16384, 12288\n"), nil
	}
	h := d.detect(context.Background())
	if !h.Known || h.PreferredBackend != "llama" || h.AvailableBytes != 12<<30 || h.TotalBytes != 16<<30 || h.MemoryBytes >= 12<<30 {
		t.Fatalf("hardware = %+v", h)
	}
	if h.SystemAvailableBytes != 40<<30 || len(h.Devices) != 2 {
		t.Fatalf("hardware = %+v", h)
	}
}

func TestUnit_LinuxVendorAndAvailableVRAM(t *testing.T) {
	for _, tc := range []struct{ vendor, backend string }{{"0x1002", "llama"}, {"0x8086", "openvino"}} {
		t.Run(tc.vendor, func(t *testing.T) {
			d := fixtureDetector()
			d.glob = func(string) ([]string, error) {
				return []string{"/sys/class/drm/card0", "/sys/class/drm/card0-DP-1"}, nil
			}
			d.readFile = func(path string) ([]byte, error) {
				switch {
				case strings.HasSuffix(path, "/vendor"):
					return []byte(tc.vendor), nil
				case strings.HasSuffix(path, "/mem_info_vram_total"):
					return []byte("8589934592"), nil
				case strings.HasSuffix(path, "/mem_info_vram_used"):
					return []byte("2147483648"), nil
				}
				return nil, os.ErrNotExist
			}
			h := d.detect(context.Background())
			if h.PreferredBackend != tc.backend || h.AvailableBytes != 6<<30 || len(h.Devices) != 1 {
				t.Fatalf("hardware = %+v", h)
			}
		})
	}
}

func TestUnit_UnknownIntelMemoryUsesRAMOnce(t *testing.T) {
	d := fixtureDetector()
	d.goos = "windows"
	d.run = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "powershell.exe" {
			return []byte(`{"Name":"Intel Arc","PNPDeviceID":"PCI\\VEN_8086&DEV_1234"}`), nil
		}
		return nil, errors.New("unavailable")
	}
	h := d.detect(context.Background())
	if h.PreferredBackend != "openvino" || h.Kind != "system" || h.AvailableBytes != 40<<30 || h.Devices[0].MemoryKnown {
		t.Fatalf("hardware = %+v", h)
	}
}

func TestUnit_AppleUnifiedMemoryIsNotAddedToRAM(t *testing.T) {
	d := fixtureDetector()
	d.goos, d.goarch = "darwin", "arm64"
	d.run = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "sysctl" {
			return []byte("Apple M4 Pro\n"), nil
		}
		return nil, errors.New("unavailable")
	}
	h := d.detect(context.Background())
	if h.PreferredBackend != "llama" || h.AvailableBytes != 40<<30 || len(h.Devices) != 1 || h.Devices[0].Kind != "unified" || h.Kind != "unified" {
		t.Fatalf("hardware = %+v", h)
	}
}

func TestUnit_MissingGPUAndInvalidMemoryNeverInventCapacity(t *testing.T) {
	for _, output := range []string{"not,csv,valid,output", "GPU, 8192, -1", "GPU, 8192, 16384", "GPU, 999999999999999, 0"} {
		d := fixtureDetector()
		d.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(output), nil }
		d.ram = func(context.Context) (int64, int64, error) { return 0, 0, errors.New("unavailable") }
		h := d.detect(context.Background())
		if h.Known || h.MemoryBytes != 0 || h.PreferredBackend != "llama" || len(h.Devices) != 0 {
			t.Fatalf("output %q: %+v", output, h)
		}
	}
}

func TestUnit_NoAcceleratorDefaultsToCPU(t *testing.T) {
	h := fixtureDetector().detect(context.Background())
	if h.PreferredBackend != "llama" || h.Kind != "system" || !h.Known || len(h.Devices) != 0 {
		t.Fatalf("hardware = %+v", h)
	}
}

func TestUnit_MemoryTiersRemainAvailableForSelection(t *testing.T) {
	for _, gib := range []int64{8, 16, 24, 32, 48} {
		for _, kind := range []string{"nvidia", "amd", "intel", "cpu", "apple"} {
			t.Run(fmt.Sprintf("%s-%dGiB", kind, gib), func(t *testing.T) {
				total, available := gib<<30, (gib-1)<<30
				d := fixtureDetector()
				d.ram = func(context.Context) (int64, int64, error) { return total, available, nil }
				switch kind {
				case "nvidia":
					d.run = func(context.Context, string, ...string) ([]byte, error) {
						return []byte(fmt.Sprintf("GPU, %d, %d", gib*1024, (gib-1)*1024)), nil
					}
				case "amd", "intel":
					d.glob = func(string) ([]string, error) { return []string{"/sys/class/drm/card0"}, nil }
					d.readFile = func(path string) ([]byte, error) {
						switch {
						case strings.HasSuffix(path, "/vendor"):
							if kind == "amd" {
								return []byte("0x1002"), nil
							}
							return []byte("0x8086"), nil
						case strings.HasSuffix(path, "/mem_info_vram_total"):
							return []byte(fmt.Sprint(total)), nil
						case strings.HasSuffix(path, "/mem_info_vram_used"):
							return []byte(fmt.Sprint(total - available)), nil
						}
						return nil, os.ErrNotExist
					}
				case "apple":
					d.goos, d.goarch = "darwin", "arm64"
					d.run = func(_ context.Context, name string, _ ...string) ([]byte, error) {
						if name == "sysctl" {
							return []byte("Apple M4"), nil
						}
						return nil, os.ErrNotExist
					}
				}
				h := d.detect(context.Background())
				if !h.Known || h.TotalBytes != total || h.AvailableBytes != available {
					t.Fatalf("pool = %+v", h)
				}
				if h.MemoryBytes <= available/2 || h.MemoryBytes >= available {
					t.Fatalf("unexpected selection budget %d for available %d", h.MemoryBytes, available)
				}
				if gib == 8 && h.MemoryBytes < 4<<30 {
					t.Fatalf("8GiB hardware lost room for a 4GiB artifact: %+v", h)
				}
				if kind == "cpu" && (h.Kind != "system" || strings.Contains(h.Summary, "bandwidth")) {
					t.Fatalf("invented memory characteristics: %+v", h)
				}
			})
		}
	}
}

func TestUnit_AMDSharedMemoryDoesNotBecomeExtraVRAM(t *testing.T) {
	d := fixtureDetector()
	d.glob = func(string) ([]string, error) { return []string{"/sys/class/drm/card0"}, nil }
	d.readFile = func(path string) ([]byte, error) {
		values := map[string]string{
			"vendor": "0x1002", "mem_info_vram_total": "8589934592", "mem_info_vram_used": "1073741824",
			"mem_info_gtt_total": "34359738368", "mem_info_gtt_used": "8589934592",
		}
		for key, value := range values {
			if strings.HasSuffix(path, "/"+key) {
				return []byte(value), nil
			}
		}
		return nil, os.ErrNotExist
	}
	h := d.detect(context.Background())
	if h.TotalBytes != 8<<30 || h.AvailableBytes != 7<<30 {
		t.Fatalf("shared pool added to VRAM: %+v", h)
	}
	if len(h.Devices) != 1 || !h.Devices[0].SharedMemoryKnown || h.Devices[0].SharedTotalBytes != 32<<30 || h.Devices[0].SharedAvailableBytes != 24<<30 {
		t.Fatalf("missing shared pool facts: %+v", h)
	}
}

func TestUnit_ExplicitCPUUsesSeparateRAMPool(t *testing.T) {
	for _, ramGiB := range []int64{32, 48} {
		d := fixtureDetector()
		d.getenv = func(key string) string {
			if key == "CONTENOX_LLAMA_GPU_LAYERS" {
				return "0"
			}
			return ""
		}
		d.ram = func(context.Context) (int64, int64, error) { return ramGiB << 30, (ramGiB - 4) << 30, nil }
		d.run = func(context.Context, string, ...string) ([]byte, error) { return []byte("Small GPU, 1024, 768"), nil }
		h := d.detect(context.Background())
		if h.Kind != "system" || h.TotalBytes != ramGiB<<30 || h.AvailableBytes != (ramGiB-4)<<30 || !strings.Contains(h.Summary, "explicit CPU selection") || h.PreferredBackend != "llama" {
			t.Fatalf("hardware = %+v", h)
		}
		if h.Devices[0].AvailableBytes != 768<<20 {
			t.Fatalf("GPU fact changed: %+v", h.Devices)
		}
	}
}
