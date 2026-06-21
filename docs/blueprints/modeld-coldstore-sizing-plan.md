# Plan: lossless long context — derived hot/cold sizing (#5) + host-RAM KV cold store (#3)

> Execution plan for requirements #3 and #5 in
> `modeld-effective-context-architecture.md`. Scope: **`modeld/` only**. The sizing
> split and append-mode cold KV plumbing now exist for both adapters. The remaining
> work is making retrieval automatic and proving when restore beats recompute and when
> longer logical context still helps model quality.

## Why #3 and #5 are one piece
- **#5 (sizing)** splits the budget into a physical hot VRAM budget
  (`HotContextTokens`) and a larger logical window (`PlannerEffectiveContext`).
- **#3 (cold store)** is what makes that split **lossless**: tokens evicted to fit
  `HotContextTokens` are parked in host RAM, not discarded, and paged back when
  relevant. Without #3 the split is lossy (today's slide); with #3 it is lossless.
- Build order: #5's derivation → thread the hot budget → #3's store → retrieval.

## #5 — Derived hot/cold sizing
Landed status: `capacity.Resolve` (`modeld/capacity/capacity.go`) now
accepts `HostColdBudgetBytes`. With no host-cold budget it preserves the old dense
behavior; with a budget it keeps `EffectiveContext` as the dense compatibility
window, keeps `HotContextTokens` as the physical KV budget, and lets
`PlannerEffectiveContext` grow from `hot + hostColdBudget/KVBytesPerToken` (capped by
the request/model ceiling for now). Current shape:

1. **`capacity.Resolve`**:
   - `HotContextTokens` = physical VRAM KV budget = `memoryTokens` (already computed:
     `(usable − weights − overhead) / KVBytesPerToken`) — the eviction `max_cache_size`.
   - `PlannerEffectiveContext` = `HotContextTokens + coldStoreTokens`, where
     `coldStoreTokens = HostColdBudgetBytes / KVBytesPerToken`. Only clamp by
     `ModelMaxContext` when the model truly cannot attend further (sparse attention
     may exceed the dense ceiling).
   - Keep `EffectiveContext` (dense window served today) for back-compat;
     `PlannerEffectiveContext` becomes what the runtime serves once #3 exists.
2. **Capacity policy input**: `HostColdBudgetBytes` is in `capacity.Policy` /
   `modeld.json` (and a `MemorySource` for host RAM), defaulting to a fraction of free
   system RAM. Derived, not asserted.
3. **Thread the hot budget**: sessions evict to `HotContextTokens` and serve
   `PlannerEffectiveContext`. `transport.ModelInfo` carries both, service code passes them
   into session config, and `DeriveEvictionBudget` uses the hot budget.

### Plumbing landed
- `capacity.Policy.HostColdBudgetBytes` via `modeld.json` (`host_cold_budget` /
  `host_cold_budget_bytes`), `CONTENOX_MODELD_MEM_COLD`, and `modeld serve --mem-cold`.
- Host-cold defaults derive from host RAM (`DefaultHostColdFrac`) separately from the
  hot device memory source.
- `transport.ModelInfo` reports `HostColdBudgetBytes`; `transport.Config` carries
  `HotContextTokens` and `PlannerEffectiveContext` into sessions.
- llama/OpenVINO services thread the split into session config; OpenVINO native
  `CacheEvictionConfig` is derived from the hot budget.
- Sessions surface hot/planner context in `ExplainContext`.

## #3 — Host-RAM KV cold store + retrieval
The store holds evicted KV so it can be paged back losslessly. The implementation is
backend-specific.

### llama — KV-byte offload (append-mode landed)
The shim already exposes per-sequence KV export: `StateSeqGetData(seq)` /
`StateSeqSetData` (`llama_state_seq_get_data/set_data`) plus `MemorySeqCopy`,
`MemorySeqAdd`, `MemorySeqRemove`. So:
- **Evict→cold** (in `EvictRange`): `MemorySeqCopy(0→scratch, a, b)`,
  `StateSeqGetData(scratch)` → bytes, store in a host-RAM cold map keyed by the range's
  token-hash + position + `CacheClass`; then drop the scratch seq and do the existing
  remove+slide on seq 0.
- **Cold store**: `map[tokenHash]coldBlock{tokens []int; kv []byte; class CacheClass}`
  bounded by `coldStoreTokens`; when full, drop by `MoreEvictableThan` + LRU (lossy only
  at the cold tier, and only for the most-evictable classes).
- **Retrieve→hot** (`AdmitRange`, real impl):
  - *Append mode (first)*: `StateSeqSetData` into a scratch seq, `MemorySeqCopy` to the
    tail of seq 0, fix positions with `MemorySeqAdd`. The block returns as recent
    context — sidesteps mid-sequence insertion; good for "bring a relevant file back."
  - *In-place mode (later)*: shift seq-0 positions up to open a hole, copy the block in
    at its original position. Preserves structure; harder.
- **Why bytes, not recompute**: bulk PCIe restore (~25 GB/s) beats re-prefilling a large
  block on the GPU, so the KV-byte store is a real win for large stable context (repo
  map / files) scrolling out and back. Token-recompute is the fallback when no bytes are
  stored.

Current llama status: append-mode cold store is implemented behind
`PlannerEffectiveContext > NumCtx`. `EvictRange` exports a block through scratch seq 1,
stores KV bytes plus token/hash/range/cache-class metadata, and then removes/slides as
before. `AdmitRange` imports that block back through scratch seq 1, shifts it to the
current tail, copies it into seq 0, and appends its tokens. The store is bounded by
`PlannerEffectiveContext - NumCtx` and evicts cold blocks by cache class plus LRU. It does
not yet do runtime-driven hash matching, in-place insertion, or snapshot serialization of
cold blocks.

### llama — sparse attention parity boundary
llama.cpp already implements model-native SWA for GGUFs that declare
`*.attention.sliding_window`. modeld now:
- parses the GGUF sliding-window metadata in `ggufModelParams`;
- reports `SparseAttention` and `SlidingWindowAttentionTokens` from `Describe`;
- carries those fields over the gRPC wire;
- reads the loaded model's `llama_model_n_swa()` through the direct shim and surfaces it in
  live residency capabilities.

This is true parity with what stock llama.cpp can execute. It is not OpenVINO XAttention
for dense llama models, and it is not a custom sparse mask over arbitrary retrieved cold
blocks.

### OpenVINO — KV-byte offload (append-mode landed)
OpenVINO GenAI holds KV as block-structured per-layer `ov::Tensor`s inside the continuous
batching prefix cache. The local bridge now reaches those internals (`#define protected
public` around the GenAI CB headers) and exposes `SupportsColdKV`, `ExportColdKV`, and
`ImportColdKV` through `modeld/openvino/ovsession`.

Current OpenVINO status:
- `EvictRange` exports the evicted logical token range into a host cold block, then rebuilds
  the remaining resident prefix through `PrefillTokens`.
- `AdmitRange` restores the saved block at the hot tail by importing into destination
  prefix-cache blocks. Shifted imports copy values, copy keys, and rotate RoPE-positioned
  keys by the destination/source position delta.
- The bridge advertises cold KV only for exact float KV precisions currently wired through
  config (`f16`/`f32`). `u8`/`i8` generation remains valid, but cold KV import/export is not
  advertised there.
- System coverage exists for cold-KV capability and shifted import
  (`TestSystem_OpenVINOGenAI_ColdKVCapability`,
  `TestSystem_OpenVINOGenAI_ShiftedColdKVImport`).

Remaining OpenVINO gaps: runtime-driven cold-hit matching, in-place reinsertion, cold-block
snapshot serialization, quantized KV restore, and restore-vs-recompute latency policy.

So the backends are **not asymmetric in capability** for append-mode KV-byte restore:
llama uses public llama.cpp state APIs; OpenVINO uses the local GenAI bridge over prefix
cache blocks. They still differ in sparse attention: OpenVINO has XAttention/cache
eviction, while llama only has model-native SWA unless we build a custom llama attention
path.

### Who triggers retrieval
**Runtime-driven (still open)**: the agent runtime assembles context and knows relevance. When it
re-includes an evicted segment in `EnsurePrefix`/`PrefillSuffix`, modeld matches it
against the cold store by token-hash and restores instead of a cold prefill — extending
the existing `commonPrefixLen` warm reuse to a cold tier. A modeld-internal attention-score
retriever is a later, optional autonomous mode.

## Phased execution (each phase has a local proof point on the 3060 / IR)
1. **#5 sizing (landed)** — diverge `HotContextTokens` vs `PlannerEffectiveContext` + the
   host-cold-budget policy input. *Proof: capacity unit tests; `ModelInfo` reports
   `PlannerEffectiveContext > HotContextTokens` on a small-VRAM profile.*
2. **Thread hot budget (landed)** — sessions evict to `HotContextTokens`, serve
   `PlannerEffectiveContext`. *Proof: llama physical resident bounded at
   `HotContextTokens` while generating a longer logical context.*
3. **llama cold store (append-mode landed)** — `EvictRange` offloads KV bytes; `AdmitRange`
   restores. *Proof: direct llama tag build passes; tail evict→admit has a system test
   comparing the restored one-token continuation to the pre-eviction reference when
   `CONTENOX_LLAMA_TINY_GGUF` is set. Latency benchmark still pending.*
4. **OpenVINO cold store (append-mode landed for f16/f32)** — `EvictRange` exports KV
   bytes; `AdmitRange` imports into shifted destination prefix-cache blocks with RoPE key
   rotation. *Proof: `make -f Makefile.openvino test-genai`, including
   `TestSystem_OpenVINOGenAI_ShiftedColdKVImport`.*
5. **llama native-SWA observability (landed)** — report GGUF/llama.cpp SWA support and
   window size through `Describe`, gRPC, and residency capabilities. *Proof: GGUF parsing,
   service describe, gRPC, and direct llama tag tests.*
6. **Runtime-driven retrieval** — match re-sent segments to the cold store; restore
   instead of re-prefill. *Proof: re-sent evicted segment is a cold hit.*
7. *(optional)* in-place retrieval + autonomous attention-score retriever.

## Code map
- `modeld/capacity/capacity.go` — hot/cold sizing split; `Policy` gains `HostColdBudgetBytes`.
- `modeld/residency/residency.go` — cold-store types + a `Plan` that marks hot/cold/retrieve;
  reuse `EvictionBudget`, `CacheClass.MoreEvictableThan`.
- `modeld/llama/llamacppshim/direct.go` — `StateSeqGetData/SetData`, `MemorySeqCopy`,
  `MemorySeqAdd`, plus `SlidingWindowAttention()` via `llama_model_n_swa`.
- `modeld/llama/llamasession/llama.go` — real `EvictRange`→cold / `AdmitRange`←cold + cold-store field.
- `modeld/openvino/ovsession` (`genai.h/.cpp` + shim) — `SupportsColdKV`,
  `ExportColdKV`, `ImportColdKV`; destination block composition and RoPE key rotation for
  shifted imports.
- `runtime/transport/session.go`, `runtime/transport/grpc/*` — sparse attention and
  sliding-window metadata through local and gRPC `Describe`, plus live residency
  capabilities.
- `modeld/*/service.go` — thread `HotContextTokens` / `PlannerEffectiveContext`.

## Verification (all local)
- Pure: capacity sizing + residency cold-store policy unit tests (`go test ./modeld/...`).
- llama (CPU + 3060): evict→admit round-trip is **lossless** (restored continuation ==
  reference); restore latency < re-prefill for a large block.
- OpenVINO (IR/GenAI bridge): evict→restore via `ExportColdKV`/`ImportColdKV` works for
  f16/f32 KV; native eviction still bounds VRAM. Quantized cold restore remains open.
- No regressions: full `llamasession` + default sweep.

## Risks / honest unknowns
- **Restore value is real but bounded**: PCIe bulk restore beats re-prefill for *large*
  blocks; for small blocks recompute wins. The cold store should prefer big stable
  segments (`task_pinned`/`repo_map`), not churny `volatile` — the `CacheClass` plan gates
  what earns a KV-byte slot.
- **In-place retrieval (positional)** is the hard part; append-mode is the pragmatic first
  cut and may suffice for agent workloads.
- **OpenVINO byte-offload depends on GenAI internals** — the local bridge uses protected
  continuous-batching internals, so version bumps can break it. That is now a maintenance
  risk, not an unproven capability.
- **llama sparse attention is bounded by stock llama.cpp** — native SWA is wired and
  reported; arbitrary XAttention/retrieved-block sparse masks for dense llama models are
  not implemented.
- **Quality at long effective context** stays unproven — even lossless KV doesn't
  guarantee the model attends well at ~200k. A long-context quality benchmark (separate
  lab task) is the final arbiter.
