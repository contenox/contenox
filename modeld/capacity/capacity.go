// Package capacity is modeld's hardware capacity planner: it resolves the
// EFFECTIVE context window a model can actually be served at on this device,
// from the model's KV-cache footprint and the device's free memory — not the
// model's trained ceiling alone. modeld owns this because it owns the hardware
// (see docs/blueprints/modeld-interface-boundary.md and plan-llamacpp.md:16);
// the runtime consumes the resolved number and never computes it.
package capacity

import (
	"os"
	"strconv"

	"github.com/shirou/gopsutil/v4/mem"
)

// DefaultHeadroomFrac of free memory is reserved for activations, the compute
// graph, and fragmentation, leaving the rest for model weights + KV cache.
const DefaultHeadroomFrac = 0.1

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
	ModelMaxCtx     int     // model's trained context ceiling (0 = unknown)
	KVBytesPerToken int64   // 0 = unknown (cannot budget by memory)
	WeightsBytes    int64   // resident model weight footprint
	FreeBytes       int64   // device free memory
	Request         int     // requested window (0 = use the resolved max)
	HeadroomFrac    float64 // <=0 or >=1 falls back to DefaultHeadroomFrac
}

// ModelCapacity is the resolved result reported to the runtime. EffectiveContext
// is the window modeld will actually serve and the value the cache identity
// (manifest context_size) must use; the rest explain how it was derived.
type ModelCapacity struct {
	ModelMaxContext  int
	EffectiveContext int
	KVBytesPerToken  int64
	FreeBytes        int64
	WeightsBytes     int64
}

// Resolve computes the effective context window:
//
//	effective = clamp(request, 0,
//	    min(modelMax, (free*(1-headroom) - weights) / kvBytesPerToken))
//
// Unknown inputs degrade gracefully: with no KV cost it falls back to the model
// ceiling (clamped by request); with no ceiling it uses the memory budget.
func Resolve(p Params) ModelCapacity {
	headroom := p.HeadroomFrac
	if headroom <= 0 || headroom >= 1 {
		headroom = DefaultHeadroomFrac
	}

	eff := p.ModelMaxCtx // may be 0 = unknown ceiling

	if p.KVBytesPerToken > 0 {
		budget := max(int64(float64(p.FreeBytes)*(1-headroom))-p.WeightsBytes, 0)
		memTokens := int(budget / p.KVBytesPerToken)
		if eff <= 0 || memTokens < eff {
			eff = memTokens
		}
	}

	if p.Request > 0 && (eff <= 0 || p.Request < eff) {
		eff = p.Request
	}
	if eff < 0 {
		eff = 0
	}

	return ModelCapacity{
		ModelMaxContext:  p.ModelMaxCtx,
		EffectiveContext: eff,
		KVBytesPerToken:  p.KVBytesPerToken,
		FreeBytes:        p.FreeBytes,
		WeightsBytes:     p.WeightsBytes,
	}
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

// SystemRAM reports available host RAM via gopsutil — the CPU-device source.
type SystemRAM struct{}

func (SystemRAM) FreeBytes() (int64, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	return int64(v.Available), nil
}
