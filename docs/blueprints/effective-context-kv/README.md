# Blueprint: Effective-Context KV / Sparse-Attention Layer

Status: proposed
Owner: runtime / modeld
Relates to: north-star effective context (~200k on one consumer accelerator, single-user/
single-model, <2–3 min worst case)

## 1. The bet, and the gap

The core product bet is **large effective context on one consumer accelerator, for arbitrary
models, locally** — not just for the rare architectures that ship native sliding-window
attention. Delivering it has two walls:

- **Memory wall** — fitting the KV cache in VRAM as context grows.
- **Latency wall** — prefilling a long prompt within the worst-case budget. Cold prefill is
  O(N²) in attention and is the dominant cost at long context.

Today contenox clears neither wall generically. Concretely, from the code:

- **The capacity planner over-counts KV and clamps the window.**
  `capacity.KVBytesPerToken` (`modeld/capacity/capacity.go:54`) multiplies by `nLayers`
  flat, and `modeld/llama/service.go:254` passes `params.BlockCount` — *every* layer billed
  as full-context. For a sliding-window model (Gemma 4 E4B: 42 layers, ~5 local : 1 global,
  window 512, 512 head-dim → 168 KiB/tok dense) this over-counts KV ~6×. The sliding window
  *is* read (`modeld/llama/gguf.go:37,116`) and detected (`service.go:285`) but only set as a
  **report field** (`info.SparseAttention`); it never enters the budget or `capacity.Resolve`.
  Result: a constrained 3060 was clamped to 10,608 tokens for a model whose real SWA KV for
  the full 131k window is ~3.6 GiB — i.e. it fits.

- **There is no model-agnostic KV algorithm on the llama path.** The only token reduction is
  whatever the model declares natively (GGUF SWA). The field's own doc admits the boundary
  (`runtime/transport/session.go:99`): *"For llama.cpp this means GGUF-declared SWA; it does
  not mean arbitrary XAttention can be forced on a dense model."* For dense models (Qwen,
  Llama — the majority) there is no eviction, no attention-aware retention, no block-sparse
  prefill: you get full dense KV and hit the VRAM/latency walls.

- **The cold-KV offload is spill, not reduction.** `modeld/llama/llamasession/coldstore.go`
  parks evicted blocks to host RAM (`planner context > hot`, e.g. 15,682 > 10,608). It
  stretches the *logical* window but is bandwidth/latency-bound and does not make a dense
  long window cheap.

For comparison, OpenVINO GenAI already has this layer and contenox merely flips it on
(`modeld/openvino/ovsession/genai.go:264`: `use_sparse_attention`, `xattention_threshold`,
`num_last_dense_tokens_in_prefill`). That capability is Intel-only. The llama path — the
primary, multi-vendor one — has no equivalent. Building it is the subject of this blueprint.

## 2. What already exists (the substrate we build on)

This is **not greenfield.** The residency layer was scaffolded for exactly this and the
intent is in the code (`modeld/residency/residency.go:50`): *"Sinks and recent-window blocks
are protected because sparse/streaming attention requires them hot."*

- **Block flags** (`residency.go:55`): `FlagPinned`, `FlagSink`, `FlagRecent`, `FlagRetrieved`.
  `Block.protected()` (`:76`) already keeps sinks + recent + pinned.
- **Retention classes** (`runtime/contextasm/segments.go:32`): `ClassTaskPinned`, `ClassRepoMap`,
  `ClassVolatile`, with `MoreEvictableThan` ordering.
- **Plan/Drive eviction** (`residency.go:223` `Plan{KeepHot,EvictCold}`, `:235` `PlanHotSet`;
  `residency/drive.go:16` `EvictColdRanges` tail-first; `Drive` executes eviction).
- **KV primitives already wrapped in the shim** (`modeld/llama/llamacppshim/direct.go`):
  `MemorySeqRm` (`:664`), `MemorySeqCp` (`:672`), `MemorySeqAdd` (`:676`), `Decode` (`:703`)
  over `llama_memory_seq_*` (`include/llama.h:718–775`, `llama_memory_can_shift:775`).

What is missing is the **policy** (which tokens to keep/evict, by what signal) and the
**prefill-time sparse kernel** — plus the capacity fix so the planner stops clamping below
what the hardware holds.

## 3. The three tiers, and how they combine

The real system is not one technique; it is a layered KV/attention subsystem combining all
three, each attacking a different wall. They are independent enough to ship in order.

### Tier 0 (prerequisite, small): SWA-aware capacity budget
Make `KVBytesPerToken`/`Resolve` split **global** layers (grow with context) from
**sliding-window** layers (capped at the window). Feed `params.SlidingWindow` and the
per-layer pattern into the budget instead of the report field. Unblocks Gemma-class models
immediately (~6× more offered context) with no kernel work. Touch points:
`modeld/capacity/capacity.go:54,113`, `modeld/llama/service.go:254,285`,
`modeld/llama/gguf.go` (read `sliding_window_pattern`). Validate: Gemma 4 E4B effective
context on the 3060 moves from 10,608 toward the memory-fit ceiling.

### Tier 1: StreamingLLM — attention-sink + recency eviction (memory + decode wall, any model)
Keep the first few **sink** tokens + a recent **window**, discard the middle, re-base
positions. Needs **no attention scores**, works on dense models, and the substrate is mostly
there:
- Policy: extend `PlanHotSet` to emit a sink+recency eviction (not just cold-offload),
  honoring `FlagSink`/`FlagRecent`/`protected()`.
- Mechanism: `MemorySeqRm` to drop the middle range; `MemorySeqAdd` to shift the kept recent
  window's positions (`can_shift` gating); native sink support via
  `ggml_flash_attn_ext_add_sinks` (`ggml.h:2426`) so dropping the prefix stays numerically
  correct.
- Property: lossy (forgets the middle) but decouples resident tokens from context length.
  This is the first **real, model-agnostic** KV algorithm on the llama path.

### Tier 2: attention-aware eviction (H2O / SnapKV) — keep the *important* tokens
Replace "evict by recency/class" with "evict by accumulated attention mass," so the kept set
is the heavy hitters, not just the recent window. This is the quality upgrade over Tier 1 and
plugs into the same `Plan`/`CacheClass`/eviction path — `CacheClass` ranking becomes
attention-driven.
- **The hard dependency:** it needs per-token attention scores, and flash attention (the
  `fattn` kernels) deliberately never materializes the attention matrix. Options, in
  increasing cost: (a) a cheap periodic scoring pass over a subset of heads/layers with
  flash-attn disabled; (b) instrument the ggml fattn kernel to emit per-block attention
  sums as a side output; (c) approximate heavy-hitters from a proxy (e.g. recent-window
  attention to older blocks). (b) is the principled path and overlaps with Tier 3's kernel
  work.

### Tier 3: block-sparse prefill (XAttention-class) — the latency wall
Cut prefill from O(N²) by skipping low-importance (query-block, key-block) pairs. This is the
deepest and highest-value tier; it does **not** shrink KV (you keep the full cache) — it makes
prefilling a long context fast enough to meet the budget.
- Algorithm (XAttention): tile the attention matrix; score each block cheaply via the
  **antidiagonal sum** of Q·K; threshold-select blocks (`xattention_threshold`), force-keeping
  diagonal + sink blocks; run attention only over selected blocks.
- Integration seam: `ggml_flash_attn_ext(q,k,v,mask,…)` (`ggml.h:2409`). The `mask` is
  `src[3]` in the CUDA dispatcher (`ggml-cuda/fattn.cu`). Two routes:
  1. **Mask-only (cheap, no speedup):** set unselected blocks to −inf in the mask. Correct,
     but the dense kernel still iterates all blocks — proves the math, not the latency.
  2. **Block-index kernel (the real win):** add a new op (`ggml_flash_attn_sparse`) or a
     block-index input, and fork the fattn inner loop (`fattn-common.cuh`, `fattn-tile.cu`,
     `fattn-mma-f16.cuh`) to iterate only selected key-blocks. Thread it through llama's
     `build_attn` for long prefill; branch dense for decode.
- This is net-new CUDA in the most performance-critical, numerically-delicate kernel, with
  data-dependent sparsity (warp divergence, irregular gather, load-balancing). The MMA
  (tensor-core) variant is hardest. It is a **maintained fork of the hottest file in
  llama.cpp**, carried across pin bumps.

### How they compose
- Tier 0 stops the planner under-selling. Tier 1 makes the offered window real on dense
  models by bounding resident KV. Tier 2 makes the kept set *good* instead of merely recent.
  Tier 3 makes prefilling that window fast.
- Shared spine: the `residency.Plan` decides the retained block set (Tiers 1–2 set
  `FlagSink/FlagRecent` + attention-driven `CacheClass`); the same retained set defines the
  block-selection input for Tier 3's prefill. One retention model drives both eviction and
  sparse prefill.

## 4. Multi-vendor strategy

ggml has no write-once-run-everywhere GPU layer; kernels are per-backend — **except CUDA and
HIP share source** (the HIP backend compiles `ggml-cuda/*.cu` via hipcc; we already build
both). So:
- Tier 3 kernel lands in `ggml-cuda` → **NVIDIA + AMD** from one source. Caveat: `tile`/`vec`
  variants port to HIP cleanly; the tensor-core **MMA** path needs AMD-specific divergence
  (warp size 64 vs 32, matrix-core intrinsics).
- Backends without the sparse path (Metal/Vulkan/SYCL/CPU) **fall back to dense** — correct
  output, no speedup. "Works everywhere, accelerated where ported."
- Order: CUDA/HIP first (two vendors, the stack we build), Vulkan as the single most-portable
  follow-up, Metal for Apple Silicon as a separate kernel if that segment is first-class.

## 5. Phasing (each milestone independently shippable + verifiable)

- **M0 — SWA budget (Tier 0).** Planner splits global/SWA layers. *Proof:* Gemma 4 E4B offered
  context rises ~6× on the 3060; no regression for dense models (Qwen unchanged).
- **M1 — StreamingLLM eviction (Tier 1).** Sink+recency discard policy on the residency Plan +
  `MemorySeqRm/Add` + native sinks. *Proof:* a dense model (Qwen) serves a context far beyond
  its dense VRAM fit, resident KV stays bounded, quality holds on a needle-in-haystack at the
  sink+recent regions.
- **M2 — attention-score side-output (Tier 2 dependency).** Instrument fattn to emit per-block
  attention sums (shared with M3). *Proof:* scores match a dense reference within tolerance.
- **M3 — attention-aware eviction (Tier 2).** Drive `CacheClass`/eviction by M2 scores.
  *Proof:* beats Tier 1 on long-context retrieval at equal resident budget.
- **M4 — block-sparse prefill, mask route (Tier 3a).** Correctness-only. *Proof:* output
  matches dense within tolerance at a given threshold.
- **M5 — block-sparse prefill, kernel route (Tier 3b).** Forked fattn iterating selected
  blocks, CUDA then HIP. *Proof:* measured prefill speedup at 64k/131k on the 3060 with
  bounded quality loss; worst-case prefill under the 2–3 min budget.

## 6. Risks

- **Fork maintenance.** Tier 3 forks the hottest llama.cpp kernel; every pin bump risks
  conflicts (cf. the `libcommon.a`→`libllama-common.so` break from a single 8-month jump).
  Prefer an upstreamable shape (new op behind a flag) over an invasive patch.
- **Numerical correctness.** Online-softmax renormalization across a sparse block set, and
  position re-basing after eviction, are easy to get subtly wrong. Gate every tier behind a
  dense-reference tolerance test on real backends.
- **Quality loss.** Tiers 1–3 are approximations; thresholds/window sizes are per-model.
  Needs a retrieval/quality harness, not just "it ran."
- **Score exposure cost.** Materializing attention stats fights flash attention's reason for
  existing; the side-output must stay cheap or it eats the latency it's meant to save.

## 7. Non-goals

- Datacenter batching / multi-tenant serving (that is vLLM/SGLang territory and contradicts
  the single-user, embeddable, ship-a-binary deployment bet).
- Replacing the OpenVINO backend, which already provides this on Intel; this blueprint brings
  parity to the llama (CUDA/HIP/…) path.
