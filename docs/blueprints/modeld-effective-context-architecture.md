# Blueprint: modeld effective-context beyond the model's window

> Status: implementation snapshot plus remaining architecture. Captures the required
> mechanism, the code seams it attaches to, and the gotchas. Scope is **`modeld/` only**: the
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
   `capacity.Resolve` (model + device), never a constant. The hot/cold split now exists;
   automatic retrieval policy is the remaining sizing consumer.

Orthogonal multipliers (not requirements, do **not** gate the architecture):
KV quantization (`KVCacheType` — fit more per VRAM byte) and recompute-from-tokens
(rebuild a block from its token range when cheaper than fetching). Noted; not required.

## Code map (the seams that exist, and what each lacks)

| Seam | File(s) | Provides | Lacks for this |
|---|---|---|---|
| Session contract | `runtime/transport/session.go` | `EnsurePrefix/PrefillSuffix/Decode`, warm reuse, `Snapshot/Restore`, hot/planner context reporting, sparse/SWA metadata, residency capabilities | no runtime-facing "admit this cold block" API; cold retrieval is still adapter-internal/manual |
| llama adapter | `modeld/llama/llamasession/llama.go`, `llamacppshim/direct.go` | `resident []int`, `prefixLen`, `removeKV(seq,p0,p1)`, position shift, `StateSeqGetData/SetData`, append-mode cold KV store/restore, per-segment token ranges, model-native SWA detection via `llama_model_n_swa` | runtime-driven cold-hit matching, in-place reinsertion, cold-block snapshot serialization, arbitrary custom sparse masks over retrieved blocks |
| OpenVINO adapter | `modeld/openvino/session.go`, `ovsession/genai*.go/.cpp` | `resident []int`; backend-resolved segment token ranges; native XAttention + `CacheEvictionConfig`; cold KV export/import through the GenAI bridge; shifted tail import into destination prefix-cache blocks with RoPE key rotation for f16/f32 KV | quantized cold KV import (`u8`/`i8`), in-place reinsertion, cold-block snapshot serialization, runtime-driven cold-hit matching |
| Retention model | `runtime/contextasm/segments.go`, `manifest.go` | `CacheClass` (`task_pinned/repo_map/volatile`), `MoreEvictableThan`, per-segment `TokenStart/TokenEnd`, `Invalidation`; modeld planners consume backend-filled token ranges | producer doesn't populate `CacheClass` reliably yet (out of scope), so modeld still relies on defaults |
| Capacity | `modeld/capacity/capacity.go` | `KVBytesPerToken`, `Resolve`, host-cold budget, `HotContextTokens`, `PlannerEffectiveContext`, dense-compatible `EffectiveContext` | no long-context quality proof; planner still lacks automatic retrieval decisions |
| Single owner/slot | `modeld/owner/owner.go`, `modeld/slot/service.go` | one writer, one active model — all device memory to one context | — |

## Gotchas

- **Offload only works *with* sparse attention.** Against dense attention (read all KV
  per token) host/SSD paging is dead on arrival at batch-1; it is viable *only* because
  sparse attention bounds the hot set. The two are coupled, not alternatives.
- **Sizing is derived, never asserted.** The VRAM KV budget is a function of the model's
  `KVBytesPerToken` and the device's free memory (`capacity.Resolve`). It can rival the
  weights. No GB constants in the design.
- **llama sparse attention is model-native only today.** Stock llama.cpp executes SWA for
  models that declare it (`attention.sliding_window` / `llama_model_n_swa`). modeld now
  reports that capability and window size, but it does not force XAttention or an
  arbitrary retrieved-block sparse mask onto dense llama models.
- **Middle eviction is implemented, but in-place reinsertion remains hard.** llama removes
  ranges and shifts the surviving tail; both llama and OpenVINO can restore cold KV in
  append/tail mode. Reopening a hole at the original position without corrupting RoPE/KV
  layout is still a later pass.
- **OpenVINO is both declarative and KV-capable.** The CB pipeline still owns physical KV
  blocks and runs the policy natively with XAttention plus `CacheEvictionConfig`; the shim
  now also reaches the prefix-cache block store to export/import cold KV. Shifted imports
  compose destination cache blocks and rotate RoPE-positioned keys.
- **OpenVINO cold KV is precision-gated.** The shifted import path is advertised only for
  exact float KV precisions currently exposed by config (`f16`/`f32`). Quantized `u8`/`i8`
  KV is allowed for normal generation but does not claim cold-KV support because rotating
  and re-encoding quantized keys is not implemented.
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

- **Proven seams:** warm reuse, manifest cache key, backend-resolved segment token ranges
  in both adapters, capacity-derived hot/planner split, single-slot/single-owner,
  whole-session snapshot/restore, llama range removal/shift, llama append-mode cold KV,
  OpenVINO cold KV export/import with shifted f16/f32 restore, OpenVINO native XAttention,
  and llama native-SWA reporting.
- **Open / hard:** runtime-driven retrieval against the cold store, in-place reinsertion,
  cold-block snapshot serialization, latency policy for restore-vs-recompute, quantized
  OpenVINO cold KV import, arbitrary sparse masks for dense llama models, and long-context
  quality proof.
