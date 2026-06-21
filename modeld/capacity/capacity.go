// Package capacity is modeld's hardware capacity planner: it resolves the
// EFFECTIVE context window a model can actually be served at on this device,
// from the model's KV-cache footprint and the device's free memory — not the
// model's trained ceiling alone. modeld owns this calculation because it owns
// the backend process and hardware telemetry; the runtime consumes the resolved
// value and does not inspect model files.
package capacity

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
)

// DefaultHeadroomFrac of free memory is reserved for activations, the compute
// graph, and fragmentation, leaving the rest for model weights + KV cache.
const DefaultHeadroomFrac = 0.1

// DefaultMaxResidentFrac caps modeld's resident footprint at this fraction of
// the device's CURRENTLY free memory when the user did not set an explicit
// ceiling. It is evaluated fresh on every resolution, so the budget tracks the
// device live instead of freezing a launch-time view.
const DefaultMaxResidentFrac = 0.8

// DefaultHostColdFrac is the launch-time cap for the host-RAM KV cold store
// when the user did not set one explicitly.
const DefaultHostColdFrac = 0.25

// kvTypeBytes is the per-element size of one KV cache entry for a precision.
// KV is two tensors (K and V); KVBytesPerToken accounts for both. Quantized KV
// rounds up to a whole byte — KV is tiny next to weights, so over-estimating is
// the safe direction for a no-spill budget.
func kvTypeBytes(kvType string) int64 {
	switch kvType {
	case "", "f16", "fp16":
		return 2
	case "f32", "fp32":
		return 4
	case "q8_0":
		return 1
	case "q4_0", "q4_1":
		return 1
	default:
		return 2
	}
}

// KVBytesPerToken is the memory one token of context costs in the KV cache:
// K and V, across every layer and KV head, at the KV precision.
func KVBytesPerToken(nLayers, nKVHeads, headDim int, kvType string) int64 {
	if nLayers <= 0 || nKVHeads <= 0 || headDim <= 0 {
		return 0
	}
	return 2 * int64(nLayers) * int64(nKVHeads) * int64(headDim) * kvTypeBytes(kvType)
}

// Params are the inputs to a capacity resolution. Zero values mean "unknown":
// an unknown ModelMaxCtx or KVBytesPerToken disables that side of the clamp
// rather than producing a bogus window.
type Params struct {
	ModelMaxCtx         int     // model's trained context ceiling (0 = unknown)
	KVBytesPerToken     int64   // 0 = unknown (cannot budget by memory)
	WeightsBytes        int64   // resident model weight footprint
	OverheadBytes       int64   // fixed runtime buffers (compute graph, staging)
	FreeBytes           int64   // device free memory
	ReservedBytes       int64   // memory already reserved by resident sessions
	UserLimitBytes      int64   // user cap for modeld resident memory (0 = no cap)
	MinFreeBytes        int64   // memory to leave free for the desktop/other workloads
	HostColdBudgetBytes int64   // host-RAM budget for cold KV blocks (0 = none)
	Request             int     // requested window (0 = use the resolved max)
	HeadroomFrac        float64 // <=0 or >=1 falls back to DefaultHeadroomFrac
}

// ModelCapacity is the resolved result reported to the runtime. EffectiveContext
// remains the dense context window modeld will actually serve today and the
// value the cache identity must use. MemoryContextTokens is the raw KV-token
// budget from memory before model/request clamping. HotContextTokens is the
// physical hot KV budget. PlannerEffectiveContext is the logical planner window:
// it equals the dense window when no host cold budget exists, and can grow by the
// cold KV token budget once host offload is configured.
type ModelCapacity struct {
	ModelMaxContext         int
	EffectiveContext        int
	MemoryContextTokens     int
	HotContextTokens        int
	PlannerEffectiveContext int
	KVBytesPerToken         int64
	FreeBytes               int64
	WeightsBytes            int64
	OverheadBytes           int64
	ReservedBytes           int64
	UserLimitBytes          int64
	MinFreeBytes            int64
	HostColdBudgetBytes     int64
	UsableBytes             int64
	RequiredBytes           int64
	Clamped                 bool
	Reason                  string
}

// Resolve computes the dense compatibility window, physical hot context budget,
// and logical planner window:
//
//	usable = min(free - minFree, userLimit - reserved) * (1 - headroom)
//	effective = clamp(request, 0, min(modelMax, (usable - weights - overhead) / kvBytesPerToken))
//
// Unknown inputs degrade gracefully: with no KV cost it falls back to the model
// ceiling (clamped by request); with no ceiling it uses the memory budget.
func Resolve(p Params) ModelCapacity {
	headroom := p.HeadroomFrac
	if headroom <= 0 || headroom >= 1 {
		headroom = DefaultHeadroomFrac
	}

	eff := p.ModelMaxCtx // may be 0 = unknown ceiling
	usable := max(p.FreeBytes-p.MinFreeBytes, 0)
	if p.UserLimitBytes > 0 {
		usable = min(usable, max(p.UserLimitBytes-p.ReservedBytes, 0))
	}
	usable = max(int64(float64(usable)*(1-headroom)), 0)

	memoryTokens := 0
	if p.KVBytesPerToken > 0 {
		budget := max(usable-p.WeightsBytes-p.OverheadBytes, 0)
		memoryTokens = int(budget / p.KVBytesPerToken)
		if eff <= 0 || memoryTokens < eff {
			eff = memoryTokens
		}
	}

	if p.Request > 0 {
		switch {
		case eff > 0 && p.Request < eff:
			eff = p.Request
		case eff <= 0 && p.KVBytesPerToken <= 0 && p.ModelMaxCtx <= 0:
			eff = p.Request
		}
	}
	if eff < 0 {
		eff = 0
	}

	requestForRequired := p.Request
	if requestForRequired <= 0 {
		requestForRequired = eff
	}
	required := p.WeightsBytes + p.OverheadBytes
	if p.KVBytesPerToken > 0 && requestForRequired > 0 {
		required += int64(requestForRequired) * p.KVBytesPerToken
	}
	clamped, reason := false, ""
	switch {
	case p.UserLimitBytes > 0 && required > p.UserLimitBytes:
		clamped, reason = true, "request_exceeds_user_limit"
	case p.MinFreeBytes > 0 && p.FreeBytes <= p.MinFreeBytes:
		clamped, reason = true, "device_free_memory_below_reserve"
	case p.Request > 0 && eff < p.Request:
		clamped, reason = true, "request_exceeds_memory_budget"
	case p.Request <= 0 && p.ModelMaxCtx > 0 && eff < p.ModelMaxCtx:
		clamped, reason = true, "model_context_exceeds_memory_budget"
	}

	hotTokens := eff
	if p.KVBytesPerToken > 0 && memoryTokens > 0 {
		hotTokens = memoryTokens
		if p.ModelMaxCtx > 0 && hotTokens > p.ModelMaxCtx {
			hotTokens = p.ModelMaxCtx
		}
		if p.Request > 0 && hotTokens > p.Request {
			hotTokens = p.Request
		}
	}
	coldTokens := 0
	if p.KVBytesPerToken > 0 && p.HostColdBudgetBytes > 0 {
		coldTokens = int(p.HostColdBudgetBytes / p.KVBytesPerToken)
	}
	planner := hotTokens + coldTokens
	if p.ModelMaxCtx > 0 && planner > p.ModelMaxCtx {
		planner = p.ModelMaxCtx
	}
	if p.Request > 0 && planner > p.Request {
		planner = p.Request
	}
	if planner < eff {
		planner = eff
	}

	return ModelCapacity{
		ModelMaxContext:         p.ModelMaxCtx,
		EffectiveContext:        eff,
		MemoryContextTokens:     memoryTokens,
		HotContextTokens:        hotTokens,
		PlannerEffectiveContext: planner,
		KVBytesPerToken:         p.KVBytesPerToken,
		FreeBytes:               p.FreeBytes,
		WeightsBytes:            p.WeightsBytes,
		OverheadBytes:           p.OverheadBytes,
		ReservedBytes:           p.ReservedBytes,
		UserLimitBytes:          p.UserLimitBytes,
		MinFreeBytes:            p.MinFreeBytes,
		HostColdBudgetBytes:     p.HostColdBudgetBytes,
		UsableBytes:             usable,
		RequiredBytes:           required,
		Clamped:                 clamped,
		Reason:                  reason,
	}
}

// Policy is the user/operator memory policy modeld applies before opening a
// resident session. MaxResidentBytes is a hard ceiling on modeld's resident
// footprint for the served device; MinFreeBytes preserves memory for the desktop
// or other local workloads that may share the same device.
type Policy struct {
	MaxResidentBytes    int64   `json:"max_resident_bytes,omitempty"`
	MinFreeBytes        int64   `json:"min_free_bytes,omitempty"`
	HostColdBudgetBytes int64   `json:"host_cold_budget_bytes,omitempty"`
	HeadroomFrac        float64 `json:"headroom_frac,omitempty"`
}

// WithResidentDefault fills a missing resident-memory cap from the device's
// CURRENT free memory. Services call it with a fresh snapshot on every
// resolution, so the default tracks the device live — it rises when memory frees
// up and falls when other workloads claim it. An explicit MaxResidentBytes (the
// user's hard cap) always wins and is left untouched.
func WithResidentDefault(p Policy, dev DeviceSnapshot) Policy {
	if p.MaxResidentBytes <= 0 && dev.FreeBytes > 0 {
		p.MaxResidentBytes = int64(float64(dev.FreeBytes) * DefaultMaxResidentFrac)
	}
	return p
}

// WithHostColdDefaults fills the host-RAM cold-store budget from a host memory
// snapshot. It is separate from WithResidentDefault because the hot model budget
// may come from VRAM while the cold store always lives in host RAM.
func WithHostColdDefaults(p Policy, host DeviceSnapshot) Policy {
	if p.HostColdBudgetBytes <= 0 && host.FreeBytes > 0 {
		p.HostColdBudgetBytes = int64(float64(host.FreeBytes) * DefaultHostColdFrac)
	}
	return p
}

// LoadPolicy reads <dataRoot>/modeld.json and then applies env overrides. The
// JSON accepts either numeric byte fields or string fields ("8GiB", "512MiB"):
//
//	{"memory":{"max_resident":"8GiB","reserve_free":"2GiB","headroom_frac":0.15}}
func LoadPolicy(dataRoot string) Policy {
	var p Policy
	if dataRoot != "" {
		var raw struct {
			Memory struct {
				MaxResidentBytes    int64   `json:"max_resident_bytes"`
				MinFreeBytes        int64   `json:"min_free_bytes"`
				HostColdBudgetBytes int64   `json:"host_cold_budget_bytes"`
				MaxResident         string  `json:"max_resident"`
				ReserveFree         string  `json:"reserve_free"`
				HostColdBudget      string  `json:"host_cold_budget"`
				HeadroomFrac        float64 `json:"headroom_frac"`
			} `json:"memory"`
		}
		if b, err := os.ReadFile(dataRoot + string(os.PathSeparator) + "modeld.json"); err == nil {
			_ = json.Unmarshal(b, &raw)
			p.MaxResidentBytes = raw.Memory.MaxResidentBytes
			p.MinFreeBytes = raw.Memory.MinFreeBytes
			p.HostColdBudgetBytes = raw.Memory.HostColdBudgetBytes
			p.HeadroomFrac = raw.Memory.HeadroomFrac
			if v, err := ParseBytes(raw.Memory.MaxResident); err == nil && v > 0 {
				p.MaxResidentBytes = v
			}
			if v, err := ParseBytes(raw.Memory.ReserveFree); err == nil && v > 0 {
				p.MinFreeBytes = v
			}
			if v, err := ParseBytes(raw.Memory.HostColdBudget); err == nil && v > 0 {
				p.HostColdBudgetBytes = v
			}
		}
	}
	if v, err := ParseBytes(os.Getenv("CONTENOX_MODELD_MEM_MAX")); err == nil && v > 0 {
		p.MaxResidentBytes = v
	}
	if v, err := ParseBytes(os.Getenv("CONTENOX_MODELD_MEM_RESERVE")); err == nil && v > 0 {
		p.MinFreeBytes = v
	}
	if v, err := ParseBytes(os.Getenv("CONTENOX_MODELD_MEM_COLD")); err == nil && v > 0 {
		p.HostColdBudgetBytes = v
	}
	if v := os.Getenv("CONTENOX_MODELD_MEM_HEADROOM"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			p.HeadroomFrac = f
		}
	}
	return p
}

// ParseBytes parses byte strings used by modeld memory settings.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	lower := strings.ToLower(s)
	mult := int64(1)
	for _, suffix := range []struct {
		s string
		m int64
	}{
		{"gib", 1 << 30}, {"gb", 1000 * 1000 * 1000}, {"gi", 1 << 30}, {"g", 1000 * 1000 * 1000},
		{"mib", 1 << 20}, {"mb", 1000 * 1000}, {"mi", 1 << 20}, {"m", 1000 * 1000},
		{"kib", 1 << 10}, {"kb", 1000}, {"ki", 1 << 10}, {"k", 1000},
		{"b", 1},
	} {
		if strings.HasSuffix(lower, suffix.s) {
			mult = suffix.m
			s = strings.TrimSpace(s[:len(s)-len(suffix.s)])
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	return int64(n * float64(mult)), nil
}

// HeadroomFromEnv reads CONTENOX_MODELD_MEM_HEADROOM (a fraction in (0,1)),
// falling back to DefaultHeadroomFrac.
func HeadroomFromEnv() float64 {
	if v := os.Getenv("CONTENOX_MODELD_MEM_HEADROOM"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			return f
		}
	}
	return DefaultHeadroomFrac
}

// MemorySource reports the free memory of the device a backend serves on. modeld
// picks the source by device: system RAM for CPU; GPU VRAM (ov::Core / ggml) is a
// CGO seam filled per backend when a GPU device is selected.
type MemorySource interface {
	FreeBytes() (int64, error)
}

// DeviceSnapshot describes the memory pool the backend will allocate from.
type DeviceSnapshot struct {
	Kind              string `json:"kind,omitempty"`
	DeviceID          string `json:"device_id,omitempty"`
	TotalBytes        int64  `json:"total_bytes,omitempty"`
	FreeBytes         int64  `json:"free_bytes,omitempty"`
	SharedWithDisplay bool   `json:"shared_with_display,omitempty"`
}

// SystemRAM reports available host RAM via gopsutil — the CPU-device source.
type SystemRAM struct{}

func (SystemRAM) FreeBytes() (int64, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	return int64(v.Available), nil
}

func (SystemRAM) Snapshot() (DeviceSnapshot, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return DeviceSnapshot{}, err
	}
	return DeviceSnapshot{
		Kind:       "system",
		DeviceID:   "ram",
		TotalBytes: int64(v.Total),
		FreeBytes:  int64(v.Available),
	}, nil
}

// Snapshot returns a DeviceSnapshot for either a richer source with Snapshot or
// a legacy FreeBytes-only source.
func Snapshot(src MemorySource) (DeviceSnapshot, error) {
	if src == nil {
		src = SystemRAM{}
	}
	if s, ok := src.(interface {
		Snapshot() (DeviceSnapshot, error)
	}); ok {
		return s.Snapshot()
	}
	free, err := src.FreeBytes()
	if err != nil {
		return DeviceSnapshot{}, err
	}
	return DeviceSnapshot{Kind: "unknown", FreeBytes: free}, nil
}
