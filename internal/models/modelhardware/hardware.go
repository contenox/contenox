// Package modelhardware provides read-only hardware facts before a model is downloaded.
package modelhardware

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/modeld/capacity"
	"github.com/shirou/gopsutil/v4/mem"
)

// Device records observed hardware; detection does not establish driver or model compatibility.
// Shared memory is backed by system RAM and is not added to the selected device budget.
type Device struct {
	Vendor               string
	Name                 string
	Kind                 string
	TotalBytes           int64
	AvailableBytes       int64
	MemoryKnown          bool
	Source               string
	SharedTotalBytes     int64
	SharedAvailableBytes int64
	SharedMemoryKnown    bool
}

// Hardware describes one conservative memory pool; RAM and device memory are never added.
// MemoryBytes excludes modeld's default reserve and headroom. Live modeld capacity remains authoritative.
type Hardware struct {
	MemoryBytes          int64
	PreferredBackend     string
	Kind                 string
	Label                string
	TotalBytes           int64
	AvailableBytes       int64
	Known                bool
	SystemTotalBytes     int64
	SystemAvailableBytes int64
	Devices              []Device
	Summary              string
}

type detector struct {
	goos     string
	goarch   string
	getenv   func(string) string
	run      func(context.Context, string, ...string) ([]byte, error)
	readFile func(string) ([]byte, error)
	glob     func(string) ([]string, error)
	ram      func(context.Context) (int64, int64, error)
}

// Detect probes installed platform tools and OS memory telemetry without installing or downloading anything.
func Detect(ctx context.Context) Hardware {
	d := detector{
		goos: runtime.GOOS, goarch: runtime.GOARCH, getenv: os.Getenv,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		readFile: os.ReadFile, glob: filepath.Glob,
		ram: func(ctx context.Context) (int64, int64, error) {
			v, err := mem.VirtualMemoryWithContext(ctx)
			if err != nil {
				return 0, 0, err
			}
			return int64(v.Total), int64(v.Available), nil
		},
	}
	return d.detect(ctx)
}

func (d detector) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return d.run(probeCtx, name, args...)
}

func (d detector) detect(ctx context.Context) Hardware {
	h := Hardware{PreferredBackend: "llama", Kind: "system", Label: "system RAM"}
	if total, available, err := d.ram(ctx); err == nil && validMemory(total, available) {
		h.SystemTotalBytes, h.SystemAvailableBytes = total, available
		h.TotalBytes, h.AvailableBytes, h.Known = total, available, true
	}
	if ctx.Err() == nil {
		out, err := d.command(ctx, "nvidia-smi", "--query-gpu=name,memory.total,memory.free", "--format=csv,noheader,nounits")
		if err == nil {
			h.Devices = nvidiaDevices(out)
		}
	}
	switch d.goos {
	case "linux":
		h.Devices = append(h.Devices, d.linuxDevices()...)
	case "windows":
		out, err := d.command(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID | ConvertTo-Json -Compress")
		if err == nil {
			h.Devices = append(h.Devices, windowsDevices(out)...)
		}
	case "darwin":
		if d.goarch == "arm64" {
			out, err := d.command(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
			name := strings.TrimSpace(string(out))
			if err == nil && strings.HasPrefix(name, "Apple ") {
				h.Devices = append(h.Devices, Device{Vendor: "apple", Name: name, Kind: "unified", TotalBytes: h.SystemTotalBytes, AvailableBytes: h.SystemAvailableBytes, MemoryKnown: h.Known, Source: "sysctl"})
			}
		}
	}
	best := int64(-1)
	for _, dev := range h.Devices {
		if (dev.Kind != "gpu" && dev.Kind != "unified") || !dev.MemoryKnown {
			continue
		}
		budget := memoryBudget(dev.Kind, dev.TotalBytes, dev.AvailableBytes)
		if budget <= best {
			continue
		}
		best = budget
		h.Kind, h.Label, h.TotalBytes, h.AvailableBytes, h.Known = dev.Kind, dev.Name, dev.TotalBytes, dev.AvailableBytes, true
		h.PreferredBackend = backend(dev.Vendor)
	}
	if best < 0 {
		for _, dev := range h.Devices {
			if dev.Vendor == "intel" {
				h.PreferredBackend = "openvino"
			}
		}
	}
	if d.getenv != nil && (strings.TrimSpace(d.getenv("CONTENOX_LLAMA_GPU_LAYERS")) == "0" || strings.EqualFold(strings.TrimSpace(d.getenv("CONTENOX_OPENVINO_DEVICE")), "CPU")) {
		h.Kind, h.Label = "system", "system RAM (explicit CPU selection)"
		h.TotalBytes, h.AvailableBytes = h.SystemTotalBytes, h.SystemAvailableBytes
		h.Known = validMemory(h.TotalBytes, h.AvailableBytes)
		h.PreferredBackend = "llama"
		if strings.EqualFold(strings.TrimSpace(d.getenv("CONTENOX_OPENVINO_DEVICE")), "CPU") {
			h.PreferredBackend = "openvino"
		}
	}
	if h.Known {
		h.MemoryBytes = memoryBudget(h.Kind, h.TotalBytes, h.AvailableBytes)
		h.Summary = fmt.Sprintf("%s: %.1f GiB available, %.1f GiB selection budget", h.Label, float64(h.AvailableBytes)/(1<<30), float64(h.MemoryBytes)/(1<<30))
	} else {
		h.Summary = "memory capacity unavailable"
	}
	return h
}

func validMemory(total, available int64) bool {
	return total > 0 && available >= 0 && available <= total
}

func memoryBudget(kind string, total, available int64) int64 {
	p := capacity.WithResidentDefault(capacity.Policy{}, capacity.DeviceSnapshot{Kind: kind, TotalBytes: total, FreeBytes: available})
	return capacity.Resolve(capacity.Params{FreeBytes: available, UserLimitBytes: p.MaxResidentBytes, MinFreeBytes: p.MinFreeBytes}).UsableBytes
}

func backend(vendor string) string {
	if vendor == "intel" {
		return "openvino"
	}
	return "llama"
}

func nvidiaDevices(out []byte) []Device {
	rows, err := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if err != nil {
		return nil
	}
	var devices []Device
	for _, row := range rows {
		if len(row) != 3 {
			continue
		}
		total, e1 := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
		available, e2 := strconv.ParseInt(strings.TrimSpace(row[2]), 10, 64)
		if e1 != nil || e2 != nil || total > (1<<63-1)/(1<<20) || !validMemory(total, available) {
			continue
		}
		devices = append(devices, Device{Vendor: "nvidia", Name: strings.TrimSpace(row[0]), Kind: "gpu", TotalBytes: total << 20, AvailableBytes: available << 20, MemoryKnown: true, Source: "nvidia-smi"})
	}
	return devices
}

func (d detector) linuxDevices() []Device {
	paths, _ := d.glob("/sys/class/drm/card[0-9]*")
	var devices []Device
	for _, path := range paths {
		name := filepath.Base(path)
		if _, err := strconv.Atoi(strings.TrimPrefix(name, "card")); err != nil {
			continue
		}
		vendorData, err := d.readFile(filepath.Join(path, "device/vendor"))
		if err != nil {
			continue
		}
		var vendor string
		switch strings.TrimSpace(string(vendorData)) {
		case "0x1002":
			vendor = "amd"
		case "0x8086":
			vendor = "intel"
		case "0x10de":
			vendor = "nvidia"
		default:
			continue
		}
		dev := Device{Vendor: vendor, Name: vendor + " " + name, Kind: "gpu", Source: "linux-drm"}
		totalData, e1 := d.readFile(filepath.Join(path, "device/mem_info_vram_total"))
		usedData, e2 := d.readFile(filepath.Join(path, "device/mem_info_vram_used"))
		total, e3 := strconv.ParseInt(strings.TrimSpace(string(totalData)), 10, 64)
		used, e4 := strconv.ParseInt(strings.TrimSpace(string(usedData)), 10, 64)
		if e1 == nil && e2 == nil && e3 == nil && e4 == nil && used >= 0 && validMemory(total, total-used) {
			dev.TotalBytes, dev.AvailableBytes, dev.MemoryKnown = total, total-used, true
		}
		sharedTotalData, e1 := d.readFile(filepath.Join(path, "device/mem_info_gtt_total"))
		sharedUsedData, e2 := d.readFile(filepath.Join(path, "device/mem_info_gtt_used"))
		sharedTotal, e3 := strconv.ParseInt(strings.TrimSpace(string(sharedTotalData)), 10, 64)
		sharedUsed, e4 := strconv.ParseInt(strings.TrimSpace(string(sharedUsedData)), 10, 64)
		if e1 == nil && e2 == nil && e3 == nil && e4 == nil && sharedUsed >= 0 && validMemory(sharedTotal, sharedTotal-sharedUsed) {
			dev.SharedTotalBytes, dev.SharedAvailableBytes, dev.SharedMemoryKnown = sharedTotal, sharedTotal-sharedUsed, true
		}
		devices = append(devices, dev)
	}
	return devices
}

func windowsDevices(out []byte) []Device {
	var rows []struct {
		Name        string
		PNPDeviceID string
	}
	data := strings.TrimSpace(string(out))
	if strings.HasPrefix(data, "{") {
		data = "[" + data + "]"
	}
	if json.Unmarshal([]byte(data), &rows) != nil {
		return nil
	}
	var devices []Device
	for _, row := range rows {
		id := strings.ToUpper(row.PNPDeviceID)
		var vendor string
		switch {
		case strings.Contains(id, "VEN_10DE"):
			vendor = "nvidia"
		case strings.Contains(id, "VEN_1002"):
			vendor = "amd"
		case strings.Contains(id, "VEN_8086"):
			vendor = "intel"
		default:
			continue
		}
		devices = append(devices, Device{Vendor: vendor, Name: row.Name, Kind: "gpu", Source: "windows-cim"})
	}
	return devices
}
