# Blueprint: modeld Local Inference Cross-Compare

Status: design note

Scope: landscape-derived gaps for `modeld`, focused on how adjacent local-inference systems determine effective context windows for available VRAM. This is not a dependency review.

Snapshot: public repositories cloned into the local temporary comparison workspace on 2026-07-02, plus the existing local `llama.cpp` reference clone under `tmp/ref`.

## Reading Lens

The useful comparison is not "who has the most features." For `modeld`, the relevant question is:

1. How does the project decide which context length can actually run on the current hardware?
2. Does it separate model training context, requested context, resident KV capacity, and product-level prompt budget?
3. Does it expose the reason for the resolved context in a way a higher-level runtime can trust?
4. Does it model hot/warm/cold KV residency, prefix reuse, or offload as first-class behavior?

The closest cross-compare set for `modeld` is:

- Ollama, because it is the closest local daemon and model lifecycle reference.
- vLLM, because it has the strongest explicit KV-capacity profiling, block cache, and prefix-cache model.
- SGLang, because it cleanly converts memory budgets into token pool capacity across attention variants.
- llm-d / llm-d-router, because it defines useful cache metrics, events, and routing scores over backend capacity.
- Tabby, because it shows how a coding product consumes context budgets.
- Cortex.cpp, because it is a similar local UX surface around GGUF, GPU layers, and context length.

The secondary set is still useful, but more for UX, packaging, and operational boundaries than core context math: LocalAI, KoboldCpp, llamafile, text-generation-webui, Jan, LM Studio `lms`, Text Generation Inference, and OpenVINO Model Server.

## modeld Baseline

`modeld` already treats effective context as a runtime capacity result rather than a model metadata field. The current path reads GGUF metadata, derives KV bytes per token from layers, KV heads, head dimension, KV dtype, and sliding-window profile, then resolves usable context against live device memory, model weights, runtime overhead, policy headroom, minimum free memory, and host cold-store budget.

The important distinction is that `modeld` can report multiple context ceilings:

- `modelMax`: the model-declared training or configured maximum context.
- `requested`: the user's requested context.
- `memoryContextTokens`: the token count that fits the active memory budget.
- `hotContextTokens`: the resident context that can stay hot on the accelerator.
- `plannerEffectiveContext`: the larger planning budget when cold host residency is available.

The high-value work is making that distinction impossible to miss in user-facing reports and APIs. A user should be able to ask: "Why did this model get this context window on this VRAM?" and receive a deterministic answer with bytes, tokens, policy inputs, and the limiting reason.

## Ollama

### Effective-context / VRAM mechanism

Ollama is the closest local daemon reference. Its startup path chooses a default context from total VRAM tiers, then the scheduler predicts whether a requested model load fits. The prediction combines model weights, KV cache size, graph/workspace estimates, requested `num_ctx`, parallelism, KV cache dtype, GPU layers, and GPU free memory. Requested context is clamped to the model training context, and automatic context can be lowered after OOM-like failures.

Ollama's useful pattern is the pragmatic launch loop: pick a hardware-aware default, predict VRAM, place or evict models, and reduce automatic context when the prediction was still too optimistic.

### modeld difference

`modeld` is already more explicit about effective context as a capacity contract. Ollama mostly answers "can I load this runner with these options?" while `modeld` needs to answer "which part of this long-lived session is hot, which part can be cold, and what context can the planner safely assume?"

### Blueprint actions

- Add or harden an `explain context` capability report that shows VRAM tier, free bytes, minimum free bytes, headroom, weights estimate, runtime overhead, KV bytes per token, requested context, model max context, hot context, planner context, resolved GPU layers, and limiting reason.
- Add an auto-context retry policy for automatic settings only. If a backend OOMs after a predicted fit, retry with the next lower context or GPU-layer plan and persist the reason in the report.
- Keep Ollama's hardware-aware defaults as a UX reference, but keep `modeld`'s capacity math as the source of truth.

## vLLM

### Effective-context / VRAM mechanism

vLLM is the strongest reference for server-side KV capacity. It profiles real non-KV memory by running the model, accounts for weights, activations, CUDA graph memory, and other non-KV allocations, then treats the remaining requested GPU memory as KV cache capacity. It can also accept an explicit `kv_cache_memory_bytes`, bypassing utilization heuristics.

For auto-fitting, vLLM estimates whether the KV cache can hold the configured maximum model length and can reduce `max_model_len` when auto-fit is enabled. Its block cache model uses fixed-size KV blocks, parent-linked block hashes, full-block caching, LRU eviction, prefix cache reuse, and optional KV offload connectors for CPU or secondary tiers.

### modeld difference

`modeld` is local-session oriented rather than throughput-server oriented. That makes durable session continuity, planner-visible cold context, and snapshot semantics more important than maximizing concurrent requests. vLLM is still the best reference for how to make KV blocks auditable and movable.

### Blueprint actions

- Introduce stable KV block identity: model digest, tokenizer/template digest, backend profile, KV dtype, block size, parent hash, token IDs, adapter IDs, multimodal hashes when relevant, and cache salt or trust scope.
- Report `available_kv_cache_bytes`, `needed_kv_cache_bytes`, and `max_context_that_fits` explicitly.
- Add cache events compatible with block lifecycle thinking: stored, removed, restored, offloaded, prefetched, and cleared.
- Treat prefix cache reuse as a first-class metric even for single-user local sessions.

## SGLang

### Effective-context / VRAM mechanism

SGLang exposes the memory equation clearly: total GPU memory is split across model weights, KV cache pool, CUDA graph buffers, and activations. Its `mem_fraction_static` controls how much of GPU memory is reserved for model weights plus KV cache. The runtime then converts available bytes into token capacity using a per-token cell size derived from layers, KV heads, head dimensions, dtype, attention form, and page size.

SGLang is especially useful for hybrid attention and sliding-window models. It has separate logic for full-attention and SWA pools, adjusts token capacity for hybrid profiles, and documents quantized KV cache tradeoffs such as FP8/FP4 capacity gains versus quality and fused-kernel requirements. Its HiCache design also treats host cache sizing and page granularity as meaningful tuning controls.

### modeld difference

`modeld` already has a `LayerKVProfile`, which is the right local abstraction for full layers, windowed layers, and sliding-window behavior. SGLang's gap-closing value is in making the pool math visible, page-sized, and tunable across KV dtype and attention variants.

### Blueprint actions

- Add page or block size to the effective-context report, not only per-token KV bytes.
- Show full-attention and windowed-layer contributions separately for hybrid models.
- Add backend-gated quantized KV policy: supported dtypes, K/V split, capacity change, expected quality risk, and whether fused attention is available.
- Add a calibration path that compares predicted KV capacity to backend-observed allocation.

## llm-d and llm-d-router

### Effective-context / VRAM mechanism

llm-d is not primarily a context calculator. It builds around backend capacity metrics and routing decisions. The useful ideas are capacity observability, KV event streams, prefix-aware routing, cache indexing, and saturation math. Its autoscaling model distinguishes KV-memory-bound saturation from compute-bound saturation, using cache configuration, block size, GPU blocks, usage percentage, and observed request behavior.

The router scores pods by prefix match, KV cache utilization, and context-length-aware labels. The indexer tracks block stored, removed, and cleared events and scores longest consecutive prefix availability across tiers.

### modeld difference

`modeld` does not need Kubernetes routing to benefit from the same vocabulary. A single-machine runtime still needs to know resident prefix length, cache pressure, hot/cold tier usage, and whether a request is about to evict valuable context.

### Blueprint actions

- Emit `kv_cache_usage_percent`, `kv_cache_tokens_total`, `kv_cache_tokens_used`, `kv_cache_bytes_by_tier`, and `prefix_hit_rate`.
- Track longest resident prefix for the active session.
- Use block lifecycle events internally before adding any distributed feature.
- Keep cluster routing and autoscaling as non-goals unless `modeld` becomes a multi-node serving component.

## Tabby

### Effective-context / VRAM mechanism

Tabby is useful as a coding-product consumer of context, not as a VRAM calculator. Its defaults cap completion input length conservatively and its repository-context features select snippets, documents, issues, commits, or pull-request context to fit the serving model's available prompt budget. The serving/runtime layer is largely delegated to configured model backends.

This makes Tabby a good reference for the product layer above `modeld`: context selection must be budget-aware and latency-aware, even when the model can theoretically accept more tokens.

### modeld difference

`modeld` should provide truthful token and residency budgets to a code-context assembler instead of forcing that layer to guess from character limits or model names.

### Blueprint actions

- Expose prompt-budget classes for coding workflows: pinned system prompt, repository map, current file, neighboring files, retrieved snippets, conversation tail, and volatile completion suffix.
- Return a selection report showing which context classes fit in hot context, which fit only in planner context, and which were dropped.
- Support code-completion envelopes such as FIM separately from chat envelopes.

## Cortex.cpp

### Effective-context / VRAM mechanism

Cortex.cpp is a local model lifecycle reference around GGUF-style settings. It exposes `ctx_len`, GPU-layer count, batch sizes, and KV cache type, and includes hardware estimation for RAM/VRAM split across offloaded layers. It can fall back to CPU when available VRAM is too low for the requested configuration.

The project is archived, but the UX shape remains useful: model start, model list, process list, hardware estimation, and fallback messages.

### modeld difference

`modeld` should avoid making users manually search the space of `ctx_len`, GPU layers, batch, and KV dtype. The runtime can derive those values, then expose the derived plan.

### Blueprint actions

- Add a lifecycle-oriented status surface: loaded models, resolved GPU layers, hot context, planner context, memory pressure, and last fallback reason.
- Preserve expert overrides, but make automatic plans explain themselves.
- Treat CPU fallback as an explicit plan state, not a silent degradation.

## LocalAI

### Effective-context / VRAM mechanism

LocalAI is a broad local AI gateway. Its hardware logic detects GPU vendor and VRAM and can make coarse choices such as warning on very low VRAM or defaulting to CPU. Context size and GPU-layer behavior are mostly backend configuration concerns rather than a central effective-context solver.

It does contain useful distributed-prefix plumbing, such as prompt prefix hashes passed through request context for routing or backend hooks.

### modeld difference

`modeld` is narrower and should keep its advantage: precise local capacity resolution instead of broad backend dispatch.

### Blueprint actions

- Borrow the idea of carrying prefix identity through request context.
- Defer broad modality routing, model galleries, and gateway-scale API compatibility unless they directly serve `modeld` session continuity.

## KoboldCpp

### Effective-context / VRAM mechanism

KoboldCpp is a power-user GGUF runner. It exposes knobs for context size, GPU layers, flash attention, KV quantization, smart context behavior, and many backend options. Context fitting is largely user-driven, with backend support for reporting fitted parameters and current context state.

### modeld difference

`modeld` should not become a knob pile. Its value is converting hardware, model metadata, and policy into a defensible plan. Expert controls are useful, but the default surface should be a resolved capacity report.

### Blueprint actions

- Keep manual overrides behind an expert surface.
- For each override, show what part of the effective-context equation changed.
- Include smart-context-like behavior only if it is expressed through residency policy and visible eviction decisions.

## llamafile

### Effective-context / VRAM mechanism

llamafile is primarily a packaging and distribution reference. It makes local model execution easy by bundling the runner into a portable executable and largely delegates context length, GPU offload, KV cache, and backend behavior to llama.cpp-style flags and runtime support.

### modeld difference

`modeld`'s core difference is not packaging. It is runtime context capacity, session state, and memory residency.

### Blueprint actions

- Consider llamafile only for future distribution or offline install strategy.
- Do not treat it as a source for effective-context policy beyond inherited llama.cpp behavior.

## text-generation-webui

### Effective-context / VRAM mechanism

text-generation-webui is a loader and UI aggregation layer. It exposes backend-specific controls such as truncation length, GPU memory, automatic device placement, GPU layers, loader choice, and quantization settings. Effective context is mostly the result of selected loader behavior plus manual UI configuration.

### modeld difference

`modeld` should be less loader-centric. The runtime should normalize backend capabilities into one capacity model so higher layers do not need to understand every loader's memory knobs.

### Blueprint actions

- Keep a backend capability adapter boundary.
- Normalize context, KV dtype, offload, and layer-placement fields across backends.
- Avoid making the primary UX depend on backend-specific names.

## Jan

### Effective-context / VRAM mechanism

Jan is primarily a desktop product reference: local chat UX, local API, model management, privacy posture, and integrations. Its context and VRAM decisions are largely delegated to the underlying runtime and model settings rather than implemented as a standalone capacity model in the app layer.

### modeld difference

`modeld` can serve as the lower-level truth source that desktop products usually lack: explainable local capacity and session residency.

### Blueprint actions

- Prioritize clear setup, diagnostics, and local API ergonomics.
- Expose enough structured state for a desktop or CLI shell to show trustworthy model readiness and context capacity.

## LM Studio lms

### Effective-context / VRAM mechanism

The `lms` CLI is most useful as an ergonomics reference. It exposes status, server start/stop, downloaded model listing, loaded model listing, load/unload commands, logs, and JSON output. The deeper context and VRAM decisions live in the LM Studio application/runtime rather than this CLI repository.

### modeld difference

`modeld` should make its capacity model scriptable. CLI and API users should be able to inspect the same resolved plan without reading logs.

### Blueprint actions

- Provide JSON output for model status, context capacity, residency state, and last planning decision.
- Add log streaming or recent decision history for capacity changes.
- Keep command names boring and operational: status, inspect, load, unload, explain, doctor.

## Text Generation Inference

### Effective-context / VRAM mechanism

Text Generation Inference is a production serving reference. It works with admission limits such as max input tokens, max total tokens, max batch token budgets, continuous batching, paged attention, quantization, and metrics. Its value is less about local workstation auto-sizing and more about explicit serving limits.

The project is in maintenance mode, so it is a lower-priority source for new `modeld` design.

### modeld difference

`modeld` should not inherit a production-batch-first mental model. Local sessions need truthful single-session capacity, resumability, and context residency before throughput serving policy.

### Blueprint actions

- Borrow explicit admission-limit naming where useful.
- Keep throughput batching and multi-tenant admission control out of the first `modeld` effective-context scope.

## OpenVINO Model Server

### Effective-context / VRAM mechanism

OpenVINO Model Server is a vendor serving reference for Intel-optimized deployment, OpenAI-compatible APIs, model repository management, metrics, and serving operations. Context-window and KV behavior are tied to the OpenVINO GenAI backend and model configuration rather than a general local capacity solver in the server layer.

### modeld difference

`modeld` should treat OpenVINO as a backend profile with explicit capability reporting. The same effective-context API should work whether the backend is llama.cpp-like, OpenVINO, or another runtime.

### Blueprint actions

- Keep backend-specific capability probes isolated.
- Require each backend profile to report model max context, KV capacity inputs, offload capabilities, and unsupported features.
- Align OpenVINO validation docs with the same report fields used by other backends.

## Consolidated Blueprint

### B0. Capacity Explanation

Add a single capacity explanation payload for every loaded or loadable model:

- model max context
- requested context
- effective hot context
- planner effective context
- KV bytes per token
- layer split for full and windowed attention
- KV dtype and quantized KV support
- block or page size
- model weights estimate
- runtime overhead estimate
- free device memory
- usable device memory after min-free and headroom policy
- host cold-store budget
- resolved GPU layers
- fallback state
- limiting reason

This is the core answer to: "What context window can I actually use on this VRAM?"

### B1. Stable KV Block Identity

Define a block key that can survive backend boundaries and snapshots:

- model digest
- tokenizer digest
- prompt-template digest
- backend profile
- KV dtype
- block size
- parent block hash
- token IDs
- adapter IDs
- multimodal hashes when applicable
- cache salt or trust scope

This enables prefix reuse, safe snapshot restore, cold storage, and future routing.

### B2. Cache Telemetry

Emit local cache telemetry before attempting distributed behavior:

- block stored
- block removed
- block restored
- block offloaded
- block prefetched
- all blocks cleared
- prefix hit and miss
- longest resident prefix
- KV bytes and tokens by tier
- KV usage percentage

The first user-facing result can be a local `status` or `explain` report rather than a metrics server.

### B3. Tiered Residency Policy

Make hot, warm, and cold KV tiers explicit:

- hot accelerator KV budget
- warm host KV budget
- cold snapshot budget
- page or block size
- eviction policy
- prefetch policy
- writeback policy
- snapshot compatibility
- expected latency class

This is where `modeld` should differ most clearly from launch-only local runners.

### B4. Quantized KV Policy

Quantized KV should be a capability-gated policy, not just a memory knob:

- supported K/V dtypes
- separate K and V dtype support when backends allow it
- capacity multiplier
- backend fused-kernel support
- expected quality risk
- model-family allowlist or denylist
- validation result

If the backend cannot run quantized KV efficiently or safely, the report should say so.

### B5. OOM and Replanning

Add conservative replanning for automatic settings:

- retry lower context only when context was automatically chosen
- retry fewer GPU layers when layer placement was automatic
- preserve explicit user choices unless the user allowed fallback
- record the failed estimate and observed backend error
- surface the new limiting reason

This closes the gap between static estimates and real allocator behavior.

### B6. Coding Context Contract

Expose context budgets in a way `contextasm` and coding tools can consume:

- pinned system and tool prompt budget
- repository map budget
- current file budget
- neighbor file budget
- retrieved snippet budget
- conversation tail budget
- completion suffix or FIM budget
- hot versus planner-only classification
- dropped-context report

This prevents coding layers from falling back to character caps or unstructured truncation.

### B7. Operational UX

Provide a small operational surface:

- `status`: what is loaded and healthy
- `inspect`: what a model can support
- `explain`: why the current capacity was chosen
- `doctor`: why acceleration or long context is unavailable
- `logs`: recent planning and backend decisions

The important part is not the exact command names. It is that the capacity model is inspectable without reading debug logs.

### B8. Explicit Non-Goals

Keep these outside the effective-context blueprint unless a later product goal requires them:

- broad multimodal gateway behavior
- desktop chat application scope
- model marketplace scope
- multi-tenant auth and quota systems
- Kubernetes routing and autoscaling
- full production serving admission control

Those systems are useful references, but they are not the differentiator for `modeld`.
