# Blueprint: Long Effective Context on a Consumer Accelerator

> Status: north-star / direction. Some seams exist today (warm reuse, the manifest
> cache key, capacity planning, the single-slot invariant); some are scaffolded but
> not wired (coding-aware eviction); some are open research (effective context
> beyond the model's trained window). This document maps the goal onto the code so
> the gaps are explicit.

## The bet

A local AI coding agent on **one consumer accelerator** that serves **one model**
to **one user across many sessions**, with an **effective context far beyond the
model's native window** (goal: ~200k tokens of working context) and a **worst-case
response under ~2–3 minutes** so it stays usable in an agent loop.

Three numbers, three constraints — and they only reconcile because of the single-user/
single-model/agent-integrated shape. The rest of this doc explains why.

---

## 1. The KV-cache theory, mapped to this codebase

The KV cache stores the **K and V tensors** each token produces at every attention
layer — not tokens, not words, not transition probabilities. That single fact drives
everything below: KV is large, exact, and expensive to recompute, so the whole game
is *keep the right KV resident, recompute or refetch the rest, and never redo work
the math says is identical.*

| KV-cache concept | What it is | Where it lives in modeld |
|---|---|---|
| KV cache (K/V tensors per layer/head/token) | The model's "brain state" for past tokens | Owned by the engine: llama.cpp KV cells / OpenVINO `ContinuousBatchingPipeline`. The adapter tracks only *logical* residency: `resident []int`, `prefixLen` (`llamasession/llama.go`, `openvino/session.go`) |
| Prefill (compute-bound) | Build K/V for many prompt tokens at once | `EnsurePrefix` (stable prefix) + `PrefillSuffix` (changed suffix) — `transport.Session` |
| Decode (memory-bound) | Generate one token at a time, streaming K/V over bandwidth | `Decode` — `transport.Session` |
| Prefix caching | Skip prefill for an already-seen prefix | `EnsurePrefix` reuses the longest common token prefix (`sessionkit.CommonPrefixLen`) of resident vs new; OpenVINO additionally reuses the pipeline's internal prefix cache. Re-prefill is only the divergent tail. |
| PagedAttention / fragmentation | Non-contiguous KV blocks, ~0 waste | **Delegated to the engines.** modeld does not reimplement paging — it offloads physical KV management to llama.cpp / OpenVINO and owns the *semantics* above it. |
| Recompute-vs-store | Drop cold KV, recompute on demand | `Snapshot`/`Restore` carry `ResidentTokenIDs` + opaque `State`, so a dropped span can be **restored** (llama: real KV bytes) or **recomputed from its token range** (OpenVINO: re-tokenize + re-prefill). |
| KV quantization | FP16 → INT8/INT4 to shrink the cache | `transport.Config.KVCacheType` (`q8_0`/`q4_0`), `ovsession.GenAIConfig.KVCachePrecision`. Capacity budgets at the chosen precision (`capacity.kvTypeBytes`). |
| GQA (fewer K/V heads) | Share K/V across query heads, ~75% smaller cache | Capacity sizes KV from **kv_heads, not attention_heads** (`openvinoParams.kvHeads()`, llama `params.kvHeads()` → `capacity.KVBytesPerToken`). |
| Cache correctness | A warm hit must be the *same* tokens/model/template | `contextasm.ContextManifest` is the cache key: `CompatibleRuntime` gates profile/template/runtime identity; `StableTokenHash`/`TokenHash` gate the token prefix. Byte equality alone is never enough. |

The throughline: **modeld offloads the tensor mechanics to llama.cpp/OpenVINO and
keeps the policy** — what to keep resident, when reuse is valid, what to re-prefill.

---

## 2. The seams that already exist

- **`runtime/transport.Session`** — the warm-reuse contract: `EnsurePrefix →
  PrefillSuffix → Decode`, plus `Snapshot`/`Restore`/`ExplainContext`. The hot loop
  keeps the stable prefix's KV hot and re-prefills only the changed suffix.
- **`runtime/contextasm`** — the manifest cache key **and the coding-aware retention
  model**: `SegmentKind` (System, Tools, RepoRules, RepoMap, Pinned, Diff, Terminal,
  UserTurn — ordered stable→volatile, rendered stable-first so prefix caches reuse
  KV) collapsing to `CacheClass` (`task_pinned` / `repo_map` / `volatile`) with
  `MoreEvictableThan`. Plus per-segment `Invalidation` hints (`on_edit`/`on_turn`).
- **`modeld/capacity`** — turns model architecture + device free memory into the
  `EffectiveContext` modeld will actually serve: `effective = min(modelMax,
  (usable − weights − overhead) / KVBytesPerToken)`.
- **`modeld/slot`** — enforces the single active model invariant (generation fencing;
  switch/unload refused while a session is held → `ErrModelBusy`).
- **`modeld/owner`** — the single-owner lease: one writer of resident state, self-fencing
  on lease loss, so resident KV can be treated as authoritative.
- **`modeld/llama/llamasession` + `modeld/openvino`** — the two engine adapters. Same
  contract, deliberately different mechanics (llama owns physical KV; OpenVINO's
  pipeline owns it). The boundary rationale is documented in the
  `runtime/transport` package doc (`session.go`).

---

## 3. Why single-user + single-model is the unlock (the memory argument)

KV cost is fixed per token by the architecture (`capacity.KVBytesPerToken =
2 · layers · kv_heads · head_dim · bytes`). Plug in a mid-size coding model
(~36 layers, GQA kv_heads 8, head_dim 128):

- **f16:** ~144 KiB/token → **200k tokens ≈ 29 GB** of KV.
- **q8/q4 (capacity rounds quantized KV to 1 byte/elem):** ~72 KiB/token → **~14.7 GB**.

A 200k context **does not fit in 8–16 GB of consumer VRAM** alongside weights at any
precision — which is exactly the point:

- A **multi-tenant** server (vLLM-class) must split the KV budget across concurrent
  users, so each user gets a *small* window. You cannot give one user 200k while
  serving others.
- The **single-user, single-model** constraint converts "the whole device" into
  "**one** context's budget." All VRAM that isn't weights is KV for the one
  conversation that matters, and CPU RAM + SSD become the offload tiers for *that*
  context instead of being contended.

So the constraint isn't a limitation we tolerate — it's the thing that makes a
200k-class budget conceivable on hardware that could never multiplex it. It also makes
the **working set predictable**: a coding agent's context is largely append-only (a
stable system/tools/repo-map prefix, then turns appended), so prefix-reuse hit rates
approach 1.0 (the S-series prefix-cache benchmark measured ~99.5% warm reuse). A
multi-tenant server sees churn instead.

---

## 4. Why integrating inference with the agent-runtime is the unlock (the information argument)

A standalone inference server sees an **opaque prompt string**: it can hash a prefix,
but it cannot know *which* bytes are the durable task core vs. throwaway tool output.
Contenox's agent runtime **owns that structure** and ships it as the manifest:

- **Exact stable/volatile boundaries** → exact prefix reuse and minimal re-prefill,
  not best-effort string hashing. The runtime declares `StableBytes`; modeld reuses
  the stable KV and re-prefills only the suffix.
- **Coding-aware `CacheClass`** → when context exceeds the window, drop `volatile`
  (diff, terminal, last turn) before `repo_map`, and pin `task_pinned` (system,
  tools, conventions) hardest. The eviction policy can be *right* because the runtime
  labeled the segments.
- **`Invalidation` hints** (`on_edit`, `on_turn`) → drop exactly the KV that a file
  edit or a new turn invalidates, instead of dropping everything and recomputing.
- **Snapshot/restore scheduled around the agent loop** → persist a stable phase, restore
  on resume, branch a session — because the runtime knows the task's phase boundaries.

This is the division of labor: **offload the tensor mechanics to the engine; own the
semantics in the runtime.** The manifest is the wire between them, and it is the thing
a black-box server structurally cannot have.

---

## 5. The latency budget (the ~2–3 min worst case)

Worst case = a **cold start that must prefill the full effective context** once.

- **Prefill is compute-bound and dominated by throughput.** Measured cold CPU prefill
  (~350 tok/s in the S-series runs) would take 200k tokens ≈ **9.5 min — fails the
  budget.** On an accelerator (thousands–tens-of-thousands tok/s), the same prefill is
  **~10–60 s — passes.** This is why the accelerator is non-negotiable for the worst
  case, and why `modeld` autodetects it and derives offload at runtime.
- **Steady state is warm reuse, not prefill.** The stable prefix is prefilled once;
  each subsequent turn re-prefills only the changed suffix (a new turn + tool output —
  hundreds to a few thousand tokens), which is sub-second to seconds. The ~99.5%
  warm-reuse measurement is the evidence the worst case is *rare*, not typical.
- **Decode is memory-bound.** Streaming at long context needs KV bandwidth; quantized
  KV + accelerator bandwidth keep tok/s usable.
- **Levers already plumbed:** chunked prefill (`NumBatch`), FlashAttention
  (`Config.FlashAttn`), GQA-aware budgeting, and — above all — **not re-prefilling**
  via warm reuse.

Budget holds **iff** (a) the accelerator absorbs cold prefill, (b) warm reuse keeps
per-turn prefill bounded, and (c) cold starts stay rare (snapshot/restore).

---

## 6. What's technically required (the gap list)

Mapped to the seams above; ordered by leverage.

1. **KV residency tiering: VRAM → CPU RAM → SSD.** Today `Snapshot`/`Restore` are
   whole-session. The vision needs **segment-granular live offload** (LMCache-style):
   keep the hot working set in VRAM, stream cold `repo_map`/`volatile` spans to host
   RAM/SSD, fetch on demand. The manifest's per-segment token ranges are the unit.
2. **Budget-aware admission/eviction.** Wire the `CacheClass` drop policy (the seam
   `segments.go` calls "gated on the T3 context planner, #7"): a producer of richly
   classed segments + an eviction loop that drops `MoreEvictableThan` first to fit the
   `EffectiveContext` budget.
3. **Selective recomputation.** Use a segment's `TokenStart/TokenEnd` to recompute
   *exactly* an evicted cold span on access, instead of a full re-prefill — the
   "burn compute to save memory" trade, applied surgically.
4. **Effective context beyond the trained window** *(the hard, partly-open part)*.
   A model trained to 32k cannot natively attend to 200k. Options: position scaling
   (RoPE/YaRN), streaming-LLM (attention sinks + sliding window), or retrieval over
   resident KV. The manifest + `CacheClass` make whichever technique *informed* (it
   knows what's task-pinned vs droppable) rather than blind. `ModelInfo.EffectiveContext`
   already anticipates a "planner-level effective context that may exceed the model's
   dense trained window."
5. **Deliberate KV quantization.** The KV-snapshot finding (default 8-bit KV is lossy;
   f16 round-trips cleanly) means precision is a measured quality/footprint trade, not
   a default — `q8_0`/`q4_0` to stretch the window with eyes open.
6. **Prefill throughput.** Accelerator autodetect + offload derivation, FlashAttention,
   chunked prefill, and warm reuse to avoid prefill entirely on the hot path.

---

## 7. Honest unknowns

- **200k on 8–16 GB** is only reachable with aggressive tiering + quantization **and**
  a model that actually attends usefully at that length. The number is a target that
  forces the architecture, not a measured guarantee.
- **Gap #4 (effective > trained window)** is the riskiest: it is unsolved in general
  and depends on the model and technique. Everything else (tiering, eviction,
  recomputation, warm reuse) is engineering on seams that already exist.
- **Cold-prefill worst case** is the latency risk; it is bounded by the accelerator
  and kept rare by snapshot/restore — both of which depend on the single-owner,
  single-slot invariant holding.
