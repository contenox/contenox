# Plan: lossless long context — derived hot/cold sizing (#5) + host-RAM KV cold store (#3)

> Execution plan for the two open requirements in
> `modeld-effective-context-architecture.md`. Scope: **`modeld/` only**. Today both
> backends bound physical KV and generate past the window by **lossy** eviction
> (llama slide / OpenVINO native `CacheEvictionConfig`). This plan makes the window
> **lossless**: a small derived hot VRAM budget, the rest of the KV parked in host
> RAM, paged back on demand — so the served (logical) window greatly exceeds VRAM
> without dropping context.

## Why #3 and #5 are one piece
- **#5 (sizing)** splits the budget into a physical hot VRAM budget
  (`HotContextTokens`) and a larger logical window (`PlannerEffectiveContext`).
- **#3 (cold store)** is what makes that split **lossless**: tokens evicted to fit
  `HotContextTokens` are parked in host RAM, not discarded, and paged back when
  relevant. Without #3 the split is lossy (today's slide); with #3 it is lossless.
- Build order: #5's derivation → thread the hot budget → #3's store → retrieval.

## #5 — Derived hot/cold sizing
First plumbing pass status: `capacity.Resolve` (`modeld/capacity/capacity.go`) now
accepts `HostColdBudgetBytes`. With no host-cold budget it preserves the old dense
behavior; with a budget it keeps `EffectiveContext` as the dense compatibility
window, keeps `HotContextTokens` as the physical KV budget, and lets
`PlannerEffectiveContext` grow from `hot + hostColdBudget/KVBytesPerToken` (capped by
the request/model ceiling for now). Target shape:

1. **`capacity.Resolve`**:
   - `HotContextTokens` = physical VRAM KV budget = `memoryTokens` (already computed:
     `(usable − weights − overhead) / KVBytesPerToken`) — the eviction `max_cache_size`.
   - `PlannerEffectiveContext` = `HotContextTokens + coldStoreTokens`, where
     `coldStoreTokens = HostColdBudgetBytes / KVBytesPerToken`. Only clamp by
     `ModelMaxContext` when the model truly cannot attend further (sparse attention
     may exceed the dense ceiling).
   - Keep `EffectiveContext` (dense window served today) for back-compat;
     `PlannerEffectiveContext` becomes what the runtime serves once #3 exists.
2. **Capacity policy input**: add `HostColdBudgetBytes` to `capacity.Policy` /
   `modeld.json` (and a `MemorySource` for host RAM), defaulting to a fraction of free
   system RAM. Derived, not asserted.
3. **Thread the hot budget**: sessions evict to `HotContextTokens` and serve
   `PlannerEffectiveContext`. `transport.ModelInfo` already carries both — pass them
   from `service.go` into the session; `DeriveEvictionBudget` takes `HotContextTokens`
   (not `numCtx`).

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
The store holds evicted KV so it can be paged back losslessly. Backend-asymmetric.

### llama — KV-byte offload (buildable now)
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

First llama pass status: append-mode cold store is implemented behind
`PlannerEffectiveContext > NumCtx`. `EvictRange` exports a block through scratch seq 1,
stores KV bytes plus token/hash/range/cache-class metadata, and then removes/slides as
before. `AdmitRange` imports that block back through scratch seq 1, shifts it to the
current tail, copies it into seq 0, and appends its tokens. The store is bounded by
`PlannerEffectiveContext - NumCtx` and evicts cold blocks by cache class plus LRU. It does
not yet do runtime-driven hash matching, in-place insertion, or snapshot serialization of
cold blocks.

### OpenVINO — KV-byte offload via a patched GenAI (proven feasible)
OpenVINO GenAI *does* hold the KV as accessible C++ data — it just isn't in the public
API. Confirmed by reading the source (`openvino.genai/src/cpp/src/continuous_batching/`):
the cache is per-layer `ov::Tensor` (`KVCacheManager::m_key_cache/m_value_cache`,
block-structured); `Scheduler::get_block_tables(seq)` → `BlockManager` gives a sequence's
logical→physical block map (`CacheBlock::get_index()`); and the manager already ROI-copies
blocks internally (`kv_cache_manager.hpp:184`). So byte-offload is real — it requires
**patching `ContinuousBatchingPipeline` to expose `export_kv`/`import_kv` and rebuilding
OpenVINO GenAI from source** (a fork). Verified buildable locally end-to-end: submodules
fetched, venv `OpenVINOConfig.cmake` found, cmake configure passes, and the unpatched
`openvino_genai` lib **builds clean (exit 0)** — so the fork-build is real, not theoretical.
- **Patch surface**: getters on `KVCacheManager`; an export/import helper on
  `CacheOrchestrator` (walk block tables → ROI-slice the tensors → bytes, and the inverse);
  `export_kv`/`import_kv` on `ContinuousBatchingImpl` (`pipeline_impl.hpp`) via `m_scheduler`;
  public methods on the pipeline. The shim then calls them, exactly like llama.
- **Cost**: this forks OpenVINO GenAI — ship a patched `libopenvino_genai.so` and re-apply
  the patch on version bumps.
- **No-fork fallback**: recompute through the pipeline's own prefix cache (re-send the
  evicted segment; prefix caching skips the still-resident part). Lossless, no fork, slower
  than byte restore.

So the backends are **not asymmetric in capability** — both can byte-offload KV. They
differ only in *delivery*: llama via the existing shim KV-state APIs, OpenVINO via a forked
GenAI build (or the recompute fallback).

### Who triggers retrieval
**Runtime-driven**: the agent runtime assembles context and knows relevance. When it
re-includes an evicted segment in `EnsurePrefix`/`PrefillSuffix`, modeld matches it
against the cold store by token-hash and **restores (llama) / recomputes (OpenVINO)**
instead of a cold prefill — extending the existing `commonPrefixLen` warm reuse to a cold
tier. A modeld-internal attention-score retriever is a later, optional autonomous mode.

## Phased execution (each phase has a local proof point on the 3060 / IR)
1. **#5 sizing** — diverge `HotContextTokens` vs `PlannerEffectiveContext` + the
   host-cold-budget policy input. *Proof: capacity unit tests; `ModelInfo` reports
   `PlannerEffectiveContext > HotContextTokens` on a small-VRAM profile.*
2. **Thread hot budget** — sessions evict to `HotContextTokens`, serve
   `PlannerEffectiveContext`. *Proof: llama physical resident bounded at
   `HotContextTokens` while generating a longer logical context.*
3. **llama cold store (append-mode)** — `EvictRange` offloads KV bytes; `AdmitRange`
   restores. *Proof: direct llama tag build passes; tail evict→admit has a system test
   comparing the restored one-token continuation to the pre-eviction reference when
   `CONTENOX_LLAMA_TINY_GGUF` is set. Latency benchmark still pending.*
4. **Runtime-driven retrieval** — match re-sent segments to the cold store; restore
   instead of re-prefill. *Proof: re-sent evicted segment is a cold hit.*
5. **OpenVINO byte-offload** — patch + rebuild GenAI with `export_kv`/`import_kv`, wire the
   shim cold store. *Proof: evict→restore on the IR model is lossless.* (No-fork fallback:
   prefix-cache recompute.)
6. *(optional)* in-place retrieval + autonomous attention-score retriever.

## Code map
- `modeld/capacity/capacity.go` — hot/cold sizing split; `Policy` gains `HostColdBudgetBytes`.
- `modeld/residency/residency.go` — cold-store types + a `Plan` that marks hot/cold/retrieve;
  reuse `EvictionBudget`, `CacheClass.MoreEvictableThan`.
- `modeld/llama/llamacppshim/direct.go` — confirm/add `StateSeqSetData`; offload uses
  `MemorySeqCopy` + `StateSeqGetData/SetData` + `MemorySeqAdd`.
- `modeld/llama/llamasession/llama.go` — real `EvictRange`→cold / `AdmitRange`←cold + cold-store field.
- `modeld/openvino/ovsession` (`genai.h/.cpp` + shim) — call the new `export_kv`/`import_kv`
  pipeline methods; cold store of the bytes.
- OpenVINO GenAI **fork patch** (`openvino.genai/src/cpp/src/continuous_batching/`):
  `kv_cache_manager.hpp` getters, `cache_orchestrator.hpp` export/import helper,
  `pipeline_impl.hpp/.cpp` `export_kv`/`import_kv`, `continuous_batching_pipeline.hpp`
  public methods. Pin as a patch/branch against the GenAI version for reproducible rebuilds.
- `modeld/*/service.go` — thread `HotContextTokens` / `PlannerEffectiveContext`.

## Verification (all local)
- Pure: capacity sizing + residency cold-store policy unit tests (`go test ./modeld/...`).
- llama (CPU + 3060): evict→admit round-trip is **lossless** (restored continuation ==
  reference); restore latency < re-prefill for a large block.
- OpenVINO (IR, patched GenAI): evict→restore via `export_kv`/`import_kv` is lossless;
  native eviction still bounds VRAM. (No-fork fallback: re-sent segment re-decodes.)
- No regressions: full `llamasession` + default sweep.

## Risks / honest unknowns
- **Restore value is real but bounded**: PCIe bulk restore beats re-prefill for *large*
  blocks; for small blocks recompute wins. The cold store should prefer big stable
  segments (`task_pinned`/`repo_map`), not churny `volatile` — the `CacheClass` plan gates
  what earns a KV-byte slot.
- **In-place retrieval (positional)** is the hard part; append-mode is the pragmatic first
  cut and may suffice for agent workloads.
- **OpenVINO byte-offload costs a GenAI fork** — the capability is real (verified in the
  source + a passing configure/build), but exposing it means shipping a patched
  `libopenvino_genai.so` and re-applying the patch on version bumps. The no-fork fallback
  (prefix-cache recompute) is lossless but slower; choosing fork vs fallback is the real
  decision, not capability.
- **Quality at long effective context** stays unproven — even lossless KV doesn't
  guarantee the model attends well at ~200k. A long-context quality benchmark (separate
  lab task) is the final arbiter.
