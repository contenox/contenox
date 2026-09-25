package modelregistry

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrNoAutoModel means no eligible model fits the requested origin, backend, and memory.
var ErrNoAutoModel = errors.New("no eligible automatic model")

// MinAgenticHotContext is the usable context automatic selection requires. An
// agent spends it on the tool catalogue before it spends it on the task: a
// large tool surface costs thousands of tokens of schema on every call, and the
// turns before this one have to fit beside it. A model that cannot hold this
// much can call a tool once and then runs out, so a bigger window is not a
// reason to pick it — the window has to be usable.
const MinAgenticHotContext = 32 * 1024

// AutoHotContext estimates genuinely resident context using the curated KV profile.
// Budget is usable device memory after hardware reservations. Weights and a
// conservative runtime allowance are deducted; cold or planner capacity is excluded.
// The opened modeld session reports the actual hot allocation after download.
func (d ModelDescriptor) AutoHotContext(budget int64) int {
	return d.AutoHotContextType(budget, "f16")
}

// AutoHotContextType is AutoHotContext at a KV cache quantization. It has to be
// the type the worker will run — modeld takes it from
// CONTENOX_LLAMA_KV_CACHE_TYPE — because a quantized cache is what buys a small
// card its window: at f16 a 32K window costs more than the whole budget on the
// models that fit a 6 GB device, and half that at q8_0.
func (d ModelDescriptor) AutoHotContextType(budget int64, kvType string) int {
	if d.SizeBytes <= 0 || d.AutoContextLimit <= 0 || !d.AutoKVProfile.Valid() {
		return 0
	}
	available := budget - d.residentBytes()
	if available <= 0 {
		return 0
	}
	return d.AutoKVProfile.ContextForBudgetType(available, min(d.AutoContextLimit, 280*1024), kvType)
}

// residentBytes is what this model costs before any context: its weights, any
// vision projector, and the runtime allowance.
func (d ModelDescriptor) residentBytes() int64 {
	weights := d.SizeBytes + d.MMProjSizeBytes
	return weights + max(weights/4, int64(256<<20))
}

// SelectAuto picks the model an unattended setup should run: one curated as able
// to sustain a multi-step tool loop, holding at least MinAgenticHotContext of
// usable context within the memory budget, for the requested developer origin
// and backend.
//
// Capability gates the choice and context only qualifies it. The order that used
// to stand here preferred whichever model could reach a 128K hot window, which
// is how a 0.8B model came to be installed on machines that could run a 9B one:
// a large window on a model that cannot use it is not a capability. Among models
// that clear the gate the larger one wins — parameter count is a weak signal
// across families and a fair one inside a tier — and release date is not a
// tiebreak at all, so a new release does not displace a model that already does
// the job. [SelectAuto] is only reached for a machine that has no working pick
// yet; the caller keeps the previous model while it still qualifies, so a model
// is never swapped for a marginal gain.
func SelectAuto(origin, backend string, budget int64) (ModelDescriptor, error) {
	return SelectAutoType(origin, backend, budget, "f16")
}

// SelectAutoType is SelectAuto for a machine whose worker runs the given KV
// cache type. The caller must pass what modeld will actually allocate: the
// estimate is only as honest as the type it is given.
func SelectAutoType(origin, backend string, budget int64, kvType string) (ModelDescriptor, error) {
	origin = strings.ToLower(strings.TrimSpace(origin))
	backend = strings.ToLower(strings.TrimSpace(backend))
	switch origin {
	case "", "auto", "us", "eu", "china", "gus":
	default:
		return ModelDescriptor{}, fmt.Errorf("unsupported model origin %q", origin)
	}
	if backend != "llama" && backend != "openvino" {
		return ModelDescriptor{}, fmt.Errorf("unsupported automatic backend %q", backend)
	}
	if budget <= 0 {
		return ModelDescriptor{}, fmt.Errorf("automatic model selection requires a positive memory budget")
	}
	var eligible []ModelDescriptor
	for _, d := range curatedModels {
		if !d.Curated || !d.AutoEligible || !d.SupportsAgenticTools() || d.BackendType() != backend {
			continue
		}
		if origin != "" && origin != "auto" && d.Origin != origin {
			continue
		}
		if d.License != "apache-2.0" && d.License != "mit" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d.ReleasedAt); err != nil {
			continue
		}
		if d.AutoHotContextType(budget, kvType) < MinAgenticHotContext {
			continue
		}
		eligible = append(eligible, d)
	}
	if len(eligible) == 0 {
		return ModelDescriptor{}, noAgenticModelError(origin, backend, budget, kvType)
	}
	sort.Slice(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if a.SizeBytes != b.SizeBytes {
			return a.SizeBytes > b.SizeBytes
		}
		ac, bc := a.AutoHotContextType(budget, kvType), b.AutoHotContextType(budget, kvType)
		if ac != bc {
			return ac > bc
		}
		return a.Name < b.Name
	})
	return eligible[0], nil
}

// noAgenticModelError explains the refusal in the terms the operator has to
// decide in. A budget number alone reads as "get more memory", which is often
// wrong: the same model can fit once the KV cache is quantized, and a model the
// catalogue will not call agentic can fit today and be judged only by the setup
// probe. Both are named when they are the real reason.
func noAgenticModelError(origin, backend string, budget int64, kvType string) error {
	var reason strings.Builder
	fmt.Fprintf(&reason, "origin=%s backend=%s budget=%s kv=%s; nothing curated as agentic holds %d tokens of usable context in it",
		origin, backend, humanGiB(budget), kvLabel(kvType), MinAgenticHotContext)
	if cheapest, need, cost := cheapestAgenticModel(origin, backend, kvType); cheapest != "" {
		fmt.Fprintf(&reason, ". The smallest that would is %s at about %s: %s of weights, %s of runtime allowance, %s of KV for that context",
			cheapest, humanGiB(need), humanGiB(cost.weights), humanGiB(cost.overhead), humanGiB(cost.kv))
	} else {
		reason.WriteString(". No agentic model is curated for this origin and backend at any budget")
	}
	if fits, tier := bestBelowTier(origin, backend, budget, kvType); fits != "" {
		fmt.Fprintf(&reason, ". %s fits this machine and is curated %q — no measured multi-turn evidence — so the automatic pick will not take it; install it by name to accept that risk",
			fits, tier)
	}
	reason.WriteString(". Otherwise raise the memory budget, choose another origin, or use a hosted backend")
	return fmt.Errorf("%w: %s", ErrNoAutoModel, reason.String())
}

// modelCost is what one model's floor costs, broken down the way an operator
// can act on: weights are the download, the allowance is runtime slack, and the
// KV cache is what the context window costs in device memory.
type modelCost struct {
	weights  int64
	overhead int64
	kv       int64
}

func (c modelCost) total() int64 { return c.weights + c.overhead + c.kv }

func costForHotContext(d ModelDescriptor, tokens int, kvType string) modelCost {
	if d.SizeBytes <= 0 || tokens <= 0 || !d.AutoKVProfile.Valid() {
		return modelCost{}
	}
	weights := d.SizeBytes + d.MMProjSizeBytes
	return modelCost{
		weights:  weights,
		overhead: max(weights/4, int64(256<<20)),
		kv:       d.AutoKVProfile.KVBytesForContextType(tokens, kvType),
	}
}

// bestBelowTier names the model a machine could run today that the agentic gate
// is holding back, which is the one case where the refusal is a curation
// judgement rather than a memory limit.
func bestBelowTier(origin, backend string, budget int64, kvType string) (string, string) {
	var name, tier string
	var best int64
	for _, d := range curatedModels {
		if !d.Curated || !d.AutoEligible || d.ToolProtocol == "" || d.BackendType() != backend {
			continue
		}
		if origin != "" && origin != "auto" && d.Origin != origin {
			continue
		}
		if d.SupportsAgenticTools() || d.AgenticTier == "" || d.AgenticTier == AgenticTierNone {
			continue
		}
		cost := costForHotContext(d, MinAgenticHotContext, kvType)
		if cost.total() == 0 || cost.total() > budget {
			continue
		}
		if best == 0 || cost.total() > best {
			name, tier, best = d.Name, d.AgenticTier, cost.total()
		}
	}
	return name, tier
}

// cheapestAgenticModel is the least memory any curated agentic model of this
// origin and backend would need to hold the minimum usable context.
func cheapestAgenticModel(origin, backend string, kvType string) (string, int64, modelCost) {
	var name string
	var need int64
	var cheapest modelCost
	for _, d := range curatedModels {
		if !d.Curated || !d.AutoEligible || !d.SupportsAgenticTools() || d.BackendType() != backend {
			continue
		}
		if origin != "" && origin != "auto" && d.Origin != origin {
			continue
		}
		if d.AutoContextLimit < MinAgenticHotContext {
			continue
		}
		cost := costForHotContext(d, MinAgenticHotContext, kvType)
		if cost.total() <= 0 {
			continue
		}
		if need == 0 || cost.total() < need {
			name, need, cheapest = d.Name, cost.total(), cost
		}
	}
	return name, need, cheapest
}

// BytesForHotContext is the memory budget that would give this model the given
// hot context at f16, the inverse of [ModelDescriptor.AutoHotContext].
func (d ModelDescriptor) BytesForHotContext(tokens int) int64 {
	return costForHotContext(d, tokens, "f16").total()
}

// kvLabel names the cache a machine will run, so a refusal says which type it
// was judged under: the same machine can fit a model at q8_0 and not at f16.
func kvLabel(kvType string) string {
	if strings.TrimSpace(kvType) == "" {
		return "f16"
	}
	return kvType
}

func humanGiB(bytes int64) string {
	if bytes <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30))
}
