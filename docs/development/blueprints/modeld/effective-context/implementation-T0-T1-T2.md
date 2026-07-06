# Implementation Handover: T0 / T1 / T2 (backend-agnostic)

Read [`README.md`](./README.md) (the blueprint) first for the why. This document is a
**handover prompt**: enough context and a concrete codemap to implement Tiers 0–2 cold, with
interfaces that apply to **both** the llama and OpenVINO backends — not a llama-only patch.

> Scope: T0 (SWA-aware capacity), T1 (StreamingLLM sink+recency eviction), T2 (attention-aware
> eviction). T3 (block-sparse prefill kernel) is **out of scope** here — it is CUDA/HIP kernel
> work tracked in the blueprint and should be gated on usage data first.

---

## 0. The core principle: policy is shared, mechanism is per-backend

The single most important rule for keeping this agnostic:

> **All retention/eviction POLICY lives in `modeld/residency/` (pure Go, no CGo, backend-free).
> Each backend only implements the MECHANISM behind a small interface, and advertises what it
> can do via `residency.Capabilities`.**

This already exists and works for the cold-offload path — do not reinvent it, extend it.

The agnostic seam, with both backends already conforming:

| Concept | Definition | llama impl | OpenVINO impl |
|---|---|---|---|
| `residency.Controller` | `residency.go:409` — `Capabilities()` | `llamasession` | `genaiSession` |
| `residency.Executor` | `residency.go:414` — `EvictRange`/`AdmitRange` | `llama.go:84` | `coldstore.go:32` |
| `residency.Capabilities` | `residency.go:397` — feature flags | advertised | advertised |
| shared planner | `residency.PlanHotSet` (`residency.go:235`) | consumes Plan | consumes Plan |
| retained-set flags | `FlagPinned/FlagSink/FlagRecent` (`residency.go:55`) | honored in `protected()` | honored in `protected()` |
| retention classes | `contextasm.CacheClass` (`runtime/contextasm/segments.go:32`) | shared | shared |

The same `EvictRange(ctx, Range)` already abstracts two very different mechanisms — proof the
seam is real:

- **llama** (`modeld/llama/llamasession/llama.go:107` `evictRangeLocked`): native KV surgery —
  `removeKV(a,b)` then `MemorySeqAdd(0, b, -1, -(b-a))` to slide the tail. Free, in-place.
- **OpenVINO** (`modeld/openvino/coldstore.go:46` `evictRangeLocked`): KV is not
  range-addressable, so it exports the block to cold store, rebuilds the resident slice, and
  **re-prefills** the survivors (`backend.PrefillTokens`). Same interface, drop-and-recompute
  mechanism, gated on `coldEnabledLocked()`.

`residency.Capabilities` (`residency.go:397`) is how a backend tells the planner which
mechanism it has, so the shared policy can pick a strategy or degrade:

```go
type Capabilities struct {
    RemoveTail   bool   // can drop a tail range
    RemoveMiddle bool   // can drop an interior range (StreamingLLM needs this)
    PositionShift bool  // can re-base positions after a drop (can_shift)
    SparseAttention bool
    SlidingWindowAttentionTokens int
    ColdStore    bool   // can park evicted KV to a cold store
    RecomputeRange bool // can rebuild KV by re-prefilling (OV's eviction path)
}
```

**Implication for every tier below:** put the decision in the planner, express it against
`Capabilities`, and let each backend's existing `EvictRange`/`AdmitRange` carry it out. If you
find yourself writing tier logic inside `modeld/llama/` or `modeld/openvino/`, stop — it
belongs in `residency/`.

---

## T0 — SWA-aware capacity budget

**Goal:** stop billing sliding-window layers as full-context. Today the budget is a flat
per-token constant across all layers; make it a context-dependent curve so SWA models
(Gemma 2/3/4, SWA Mistral) are offered the window the hardware can actually hold (~6× more).

**Why it's agnostic:** `modeld/capacity/` is already consumed by both backends
(`modeld/llama/service.go` and `modeld/openvino/service.go` both call `capacity.Resolve`). Fix
the math once; both benefit. Only the *population* of the new inputs is backend-specific.

### Codemap

- `modeld/capacity/capacity.go`
  - **Current:** `KVBytesPerToken(nLayers, nKVHeads, headDim, kvType)` (`:54`) — flat, all
    layers. `Resolve` (`:113`) divides budget by this constant.
  - **Change:** introduce a per-layer KV profile and a context-dependent budget.

    ```go
    // LayerKVProfile describes how KV grows with context for one model.
    // GlobalLayers grow linearly; WindowedLayers cap at Window tokens.
    type LayerKVProfile struct {
        GlobalLayers   int   // full-context attention layers
        WindowedLayers int   // sliding-window layers
        Window         int   // SWA window in tokens (0 => treat all as global)
        PerLayerKVBytes int64 // KV bytes/token for ONE layer (2*nKVHeads*headDim*kvTypeBytes)
    }

    // KVBytesForContext returns total KV bytes to hold `ctx` tokens.
    //   global*perLayer*ctx + windowed*perLayer*min(ctx, window)
    func (p LayerKVProfile) KVBytesForContext(ctx int) int64

    // KVBytesPerTokenAtCeiling is the marginal (growth) cost used when inverting a
    // memory budget into a token count: global layers only.
    func (p LayerKVProfile) MarginalKVBytesPerToken() int64
    ```
  - `capacity.Params` gains `LayerKV LayerKVProfile` (keep the old `KVBytesPerToken` field as a
    fallback when `Window == 0`, so dense models are unchanged).
  - `Resolve` inverts the budget using `MarginalKVBytesPerToken()` plus the fixed windowed
    floor (`WindowedLayers*PerLayerKVBytes*Window`), instead of `budget / KVBytesPerToken`.
- `modeld/llama/gguf.go`
  - Already reads `sliding_window` (`:37,:116`). **Add:** read `sliding_window_pattern` (the
    per-layer array, GGUF type 9) and `block_count` to compute `GlobalLayers`/`WindowedLayers`.
    If the pattern is a scalar stride `n` (older Gemma), `GlobalLayers = ceil(blocks/n)`.
- `modeld/llama/service.go:254`
  - Build a `LayerKVProfile` from `params` and pass it in `capacity.Params.LayerKV`. The
    `info.SparseAttention` block at `:285` stays for reporting but is no longer the only place
    the window is used.
- `modeld/openvino/service.go`
  - Populate `LayerKVProfile` from OV's model introspection (OV exposes per-layer attention
    config). If OV cannot enumerate the pattern, set `Window=0` (dense fallback) — correct, just
    not optimized.

### Acceptance

- Unit test in `modeld/capacity/capacity_test.go`: `KVBytesForContext` for a 42-layer /
  7-global / 512-window / 168 KiB-dense profile at 131k ≈ 3.6 GiB (vs 21 GiB dense); a dense
  profile (`Window=0`) is byte-identical to the old path.
- System: Gemma 4 E4B `Effective context` on the 3060 moves from 10,608 toward the
  memory-fit ceiling; Qwen2.5 (dense) is unchanged. Re-render the capacity panel to confirm.

---

## T1 — StreamingLLM: attention-sink + recency eviction

**Goal:** keep the first few **sink** tokens + a recent **window**, evict the middle, re-base
positions. First real model-agnostic KV algorithm: decouples resident tokens from context
length on **dense** models. Lossy (forgets the middle).

**Why it's agnostic:** the policy is a new partition emitted by the shared `PlanHotSet`; both
backends already execute `EvictRange`/`AdmitRange`. The flags (`FlagSink`, `FlagRecent`) and
`protected()` already exist — they were added "because sparse/streaming attention requires
them hot" (`residency.go:50`). You are completing scaffolding, not starting fresh.

### Codemap

- `modeld/residency/` (the bulk of the work, backend-free)
  - Add a **streaming retention policy**. Extend `PlanInput` (`residency.go:211`) with
    `StreamPolicy { SinkTokens int; RecentTokens int; Enabled bool }`.
  - In `PlanHotSet` (`:235`): when `StreamPolicy.Enabled`, tag the first `SinkTokens` as
    `FlagSink` and the last `RecentTokens` as `FlagRecent`, then partition everything between
    them into `EvictCold` (subject to `protected()` and `CacheClass` order). Result is a `Plan`
    whose `EvictCold` is the "middle."
  - **Capability gating:** the planner takes `Capabilities` and gates on
    `RemoveMiddle && PositionShift`:
    - llama (`llama.go:67`) advertises both unconditionally → native discard + shift.
    - OV (`session.go:142`) advertises both **when a lossless cold backend is configured**;
      the same gate then holds, carried out by OV's export-and-recompute `EvictRange`. Same
      retained set, different cost.
    - gate not satisfied (e.g. OV with no cold backend) → policy degrades to a no-op (behaves
      like today). This single gate is why the policy is genuinely backend-neutral.
  - Keep this **unit-tested without any backend** — the planner takes blocks + capabilities +
    policy and returns a Plan; assert the partition.
- `modeld/llama/llamasession/`
  - `Capabilities()` already returns `RemoveMiddle/PositionShift = true` (verify). 
  - **Native sink correctness:** when the prefix is dropped, attention must still see sink
    tokens. Wire `ggml_flash_attn_ext_add_sinks` (`ggml.h:2426`) through the shim. Add
    `Context.SetAttentionSinks(...)` in `modeld/llama/llamacppshim/direct.go` (alongside the
    existing `MemorySeqRm:664`/`MemorySeqAdd:676`) and call it from the session when sinks are
    flagged. **This is the one llama-specific correctness detail** — but it sits behind the
    shared policy, which simply flags sinks.
- `modeld/openvino/`
  - **No new mechanism.** OV's `Capabilities()` (`session.go:142`) already returns
    `RemoveMiddle/PositionShift=true` when a lossless cold backend is configured, and its
    `EvictRange` (`coldstore.go:32`) already drops+recomputes (`backend.PrefillTokens`). So OV
    gets the same bounded resident set the shared policy asks for, paying a re-prefill
    (acceptable; T3/snapshot reduce that later). Because the recompute keeps the survivors —
    including sink tokens — resident, sink correctness holds without a native-sinks call. The
    only OV-side task is confirming the policy threads through `updateResidencyPlanLocked`
    (`session.go:534`) like the existing cold path.

### Acceptance

- `residency` unit tests: given N blocks + a `StreamPolicy{Sink:4, Recent:W}` + capabilities,
  the Plan keeps sinks+recent, evicts the middle; with `RemoveMiddle=false` the same retained
  set is produced (mechanism differs, partition identical).
- System (llama): a dense model (Qwen2.5) sustains a context well beyond its dense VRAM fit;
  resident KV stays ≤ `numCtx`; a needle placed in the sink or recent region is recalled.
- System (OV): same retained-set behavior via recompute; assert no `ErrUnsupportedFeature`
  when a cold backend is configured, graceful degrade otherwise.

---

## T2 — attention-aware eviction (H2O / SnapKV)

**Goal:** replace "evict by recency/class" with "evict by accumulated attention mass," so the
kept set is the *important* tokens, not merely the recent ones. Quality upgrade over T1 on the
same eviction path.

**Why it's agnostic:** the *policy* (rank blocks by importance, evict the weakest within a
`CacheClass`) stays in `residency/`. The *score source* is backend-specific and hidden behind a
new tiny interface so OV and llama satisfy it differently — or report "unsupported," in which
case the planner falls back to T1.

### Codemap

- `modeld/residency/` (policy + interface)
  - New optional capability interface:

    ```go
    // AttentionScorer is the optional seam a backend implements to expose
    // per-block attention importance. Backends that cannot fall back to T1.
    type AttentionScorer interface {
        // BlockAttentionScores returns accumulated attention mass per input range,
        // higher = more important. len(out) == len(ranges).
        BlockAttentionScores(ctx context.Context, ranges []Range) ([]float32, error)
    }
    ```
  - Add `Capabilities.AttentionScores bool`. `PlanHotSet` (or a new `PlanHotSetScored`) takes
    optional scores: within each `CacheClass`, evict lowest-score blocks first instead of
    oldest. Sinks/recent/pinned remain `protected()`. **No score → identical to T1.**
  - Keep the scoring policy unit-tested with synthetic scores (backend-free).
- `modeld/llama/`
  - The hard dependency: flash attention does not materialize scores. Two implementation paths
    (pick per measurement, the second overlaps T3):
    1. **Periodic cheap pass:** run a non-flash attention over a subset of heads/layers every K
       turns to estimate per-block mass. Lives in the shim/session; expose via `AttentionScorer`.
    2. **Kernel side-output:** instrument the ggml `fattn` kernel to emit per-block attention
       sums (shared with T3's kernel work). Higher cost, principled.
  - If neither is in yet, llama returns `Capabilities.AttentionScores=false` → planner uses T1.
    Ship T1 first; T2 is additive.
- `modeld/openvino/`
  - OV GenAI already does importance-based cache eviction internally (its eviction config). Two
    options: (a) expose OV's internal token scores through `AttentionScorer`; or (b) let OV keep
    using its native eviction and set `AttentionScores=false`, so the shared planner defers to
    OV for the within-class ordering. Either keeps the interface honest and agnostic.

### Acceptance

- `residency` unit test: with synthetic scores, eviction drops lowest-importance non-protected
  blocks first; with no scorer, behavior == T1.
- System: on a long-context retrieval task at a fixed resident budget, T2 (scored) beats T1
  (recency) recall. Compare on whichever backend exposes scores first.

---

## Cross-cutting guidance for the implementer

- **Where code goes:** policy/planning → `modeld/residency/` (no CGo, fully unit-testable).
  Mechanism → behind `Executor`/`Controller`/`AttentionScorer` in `modeld/llama/llamasession/`
  and `modeld/openvino/`. Capacity math → `modeld/capacity/` (shared). If a change needs the
  GPU to test, it's probably in the wrong layer.
- **Capability-first, degrade-gracefully:** every tier must produce correct behavior when a
  backend advertises fewer capabilities — never assume llama's KV surgery. OV is the forcing
  function that keeps you honest; if your design only works on llama, it's wrong.
- **Conformance, not duplication:** add a backend-agnostic conformance test that runs the same
  retention scenarios against any `residency.Executor` (table-driven), so llama and OV are
  validated against one spec.
- **Numerical correctness gates:** position re-basing after eviction and sink handling are easy
  to get subtly wrong. Every tier ships behind a dense-reference tolerance test on a real
  backend before it's "done" — the RTX 3060 + built libs make this runnable locally, so run the
  real-backend proof rather than deferring it.

### Build / test recipe (so this runs cold)

- Build the llama runtime + CGo shim (CUDA auto-on; note libs are now `libllama-common.so`,
  not `libcommon.a`, after the pin bump to `86b94708`):
  ```
  make -f Makefile.llamacpp-direct runtime LLAMA_CPP_BUILD_JOBS=6
  CONTENOX_LLAMA_TINY_GGUF=/path/model.gguf make -f Makefile.llamacpp-direct test-shim
  ```
- Pure-Go planner tests need no backend: `go test ./modeld/residency/...`.
- Real-backend residency tests are tag-gated (`llamanode && llamacpp_direct`); point
  `CONTENOX_LLAMA_TINY_GGUF` at a small gguf.
- Gotchas to respect: `n_ctx` must be even (odd aborts the CUDA daemon); single-slot modeld
  evicts-before-open; OV cold-KV round-trip is precision-sensitive (f16).

### Suggested order

1. **T0** (days, no kernel, no fork) — fixes the visible under-count; ship and verify the panel.
2. **T1** (planner + native sinks) — first real cross-backend KV algorithm; verify on Qwen + OV.
3. **Instrument & measure** real context lengths and warm/cold ratios.
4. **T2** only where a score source exists; otherwise it stays a no-op behind the interface.

Do not start T3 (the forked `fattn` kernel) until the data from step 3 justifies it.
