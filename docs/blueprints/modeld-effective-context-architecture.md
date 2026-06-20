# Blueprint: modeld effective-context beyond the model's window

> Status: architecture / direction — not a roadmap. Captures the required mechanism,
> the code seams it attaches to, and the gotchas. Scope is **`modeld/` only**: the
> daemon owns resident KV, the device budget, the session lifecycle, and
> snapshot/restore. Runtime-side context authoring (`runtime/modelrepo`,
> `runtime/contextasm` producers) is out of scope; modeld consumes the
> `transport.ContextManifest` that already crosses the wire. Companion to
> `effective-context-north-star.md` (the higher-level vision→code map).

## The bet

One model, one user, many sessions, on one consumer accelerator. Effective context
**beyond the model's native window** (~200k for real work), worst-case response under
~2–3 min so it stays usable in an agent loop.

## The mechanism

Decode is memory-bandwidth bound: each generated token reads the KV it attends to. The
hardware bandwidth ordering is fixed and orders of magnitude apart — **VRAM ≫
PCIe (host RAM) ≫ SSD** — and at batch-1 (single user) there is no batch to amortize a
transfer against. Two consequences drive everything:

1. **The KV that is *attended* each token must be VRAM-resident.** You cannot stream it
   from host RAM or SSD per token.
2. **So you must not attend to everything.** Sparse attention (attention sinks + a recent
   sliding window + a small set of *retrieved* relevant blocks) bounds the per-token
   working set. That bound is the whole point: it is what makes host-RAM offload viable.

The architecture that follows:

- **Hot set (VRAM):** sinks + recent window + retrieved blocks. **Its size is derived,
  not chosen** — `capacity.Resolve` already computes it from model architecture
  (`KVBytesPerToken`) and device free memory. It can be small or on par with the
  weights; it depends entirely on the model and the hardware. We never assert a figure.
- **Cold store (host RAM):** the full context's KV, keyed by token range. Never streamed
  per-token; relevant blocks are fetched in **bulk on relevance change**, amortized over
  many decode steps.
- **Retrieval/admission policy** decides the hot set from `CacheClass` (pin
  `task_pinned`, prefer `repo_map`, drop `volatile`) + recency + attention scores.
- **Beyond the *trained* window** comes from the sparse attention itself (sinks + window),
  not from storing more KV.

## Requirements (what the vision actually needs in modeld)

1. **A block admission/eviction/retrieval policy.** Pure decision logic: given resident
   block token-ranges + per-block `CacheClass` + recency + the derived VRAM block budget,
   produce the hot set and the evict/offload set. `task_pinned` never evicted before
   `repo_map` before `volatile`; sinks + recent window always retained. (Pure Go;
   independent of any engine; the one piece testable without the CGo toolchain.)
2. **Engine-level KV block manipulation.** Evict a token-range from VRAM KV, offload it,
   reinsert it, and keep positions coherent (sinks + sliding window need position
   handling). This is the load-bearing engine work.
3. **A host-RAM KV cold store** keyed by token range — the full context parked off the
   accelerator, block-addressable for retrieval.
4. **Sparse/streaming attention over the retrieved set** — sinks + sliding window +
   selected blocks, via a custom attention mask or an engine feature.
5. **Derived sizing** — the VRAM/host split and the hot-set budget come from
   `capacity.Resolve` (model + device), never a constant. (Exists today for the simple
   resident case; must extend to the hot-set/cold-store split.)

Orthogonal multipliers (not requirements, do **not** gate the architecture):
KV quantization (`KVCacheType` — fit more per VRAM byte) and recompute-from-tokens
(rebuild a block from its token range when cheaper than fetching). Noted; not required.

## Code map (the seams that exist, and what each lacks)

| Seam | File(s) | Provides | Lacks for this |
|---|---|---|---|
| Session contract | `runtime/transport/session.go` | `EnsurePrefix/PrefillSuffix/Decode`, warm reuse, `Snapshot/Restore` | block-granular ops; today overflow → `ErrContextOverflow` |
| llama adapter | `modeld/llama/llamasession/llama.go` | `resident []int`, `prefixLen`, `removeKV(seq,p0,p1)`, `prefillAt(pos)`, per-segment token ranges (`enrichStable/VolatileSegments`) | offload store, retrieval, sparse mask, position-shift after middle removal |
| OpenVINO adapter | `modeld/openvino/session.go`, `ovsession/genai.go` | `resident []int`; CB pipeline owns physical KV + its own prefix cache | no direct KV block control; no per-segment token ranges yet |
| Retention model | `runtime/contextasm/segments.go`, `manifest.go` | `CacheClass` (`task_pinned/repo_map/volatile`), `MoreEvictableThan`, per-segment `TokenStart/TokenEnd`, `Invalidation` | consumed nowhere in modeld; producer doesn't populate `CacheClass` yet (out of scope) |
| Capacity | `modeld/capacity/capacity.go` | `KVBytesPerToken`, `Resolve` → `EffectiveContext` from device memory | no hot-set/cold-store split |
| Single owner/slot | `modeld/owner/owner.go`, `modeld/slot/service.go` | one writer, one active model — all device memory to one context | — |

## Gotchas

- **Offload only works *with* sparse attention.** Against dense attention (read all KV
  per token) host/SSD paging is dead on arrival at batch-1; it is viable *only* because
  sparse attention bounds the hot set. The two are coupled, not alternatives.
- **Sizing is derived, never asserted.** The VRAM KV budget is a function of the model's
  `KVBytesPerToken` and the device's free memory (`capacity.Resolve`). It can rival the
  weights. No GB constants in the design.
- **Middle eviction leaves positional holes.** Removing a token-range from llama.cpp KV
  (`removeKV`) requires position handling; StreamingLLM keeps sinks + recent and shifts
  positions. Whether the engine exposes enough to do this cleanly is the key unknown.
- **OpenVINO does residency declaratively, not by KV surgery.** The CB pipeline manages
  its own cache, so the runtime cannot do `EvictRange`-style ops — but OpenVINO does the
  policy *natively*: XAttention sparse attention (on by default) plus a `SchedulerConfig`
  `CacheEvictionConfig(start_size, recent_size, max_cache_size, NORM_SUM)` — sink + recent
  + attention-scored evictable middle. Verified end-to-end through the shim
  (`use_cache_eviction`, `cache_eviction_test.go`). So the two backends express the same
  policy differently: llama = imperative range surgery, OpenVINO = declarative config.
- **OpenVINO lacks per-segment token ranges.** It populates only `StableTokenHash`; the
  block policy needs `TokenStart/TokenEnd` per segment (llama already has them).
- **Retrieval is approximate and locality-dependent.** A missed block = a quality hit;
  it only stays fast if the selected set changes slowly (good locality).
- **KV quantization is lossy by precision.** Default 8-bit KV is lossy; f16 round-trips
  cleanly (measured). Precision is a deliberate quality/footprint trade, not a default.
- **`CacheClass` is currently unpopulated** by the (out-of-scope) runtime producer, so
  modeld must apply sensible defaults from the `Stable` flag / segment kind until it is.
- **Local build/test/run is fully available** — the dev box has gcc, CUDA 12 + an
  RTX 3060 (6 GB), prebuilt llama.cpp `cpu` *and* `cuda` runtimes (`.llamacpp-runtime/`),
  the OpenVINO venv, and tiny GGUFs. Build and run the CGo adapters on CPU or GPU via the
  Makefile (`make build-modeld`, `make run-modeld`, `make test-llamacpp-direct`) or
  `go test -tags 'llamanode llamacpp_direct'` with the runtime CGO flags
  (`mk/llama-flags.mk`). Engine execution (eviction, sparse attention) is verified on the
  real backend, including on the GPU — not just the pure policy.

## Status: proven vs. open

- **Proven seams:** warm reuse, manifest cache key, capacity-derived `EffectiveContext`,
  single-slot/single-owner, whole-session snapshot/restore, llama `removeKV`.
- **Open / hard:** engine-level block evict→host→reinsert, sparse/streaming attention
  integration, and whether stock llama.cpp / OpenVINO expose enough — or whether custom
  attention/KV handling is required. This is the riskiest surface and is unproven.
