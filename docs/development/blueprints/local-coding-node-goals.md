# Contenox Local Coding Node — Goals

> **Status:** product-goals blueprint (substrate-neutral). This is the "why" and
> "what" behind the modeld backend implementation tracks (llama.cpp and
> OpenVINO). Backends are *means* to these goals, not the goals.

---

## The one goal (north star)

Unlock useful local LLM coding on a **budget consumer node**:

```text
one developer, one machine, one active repo/workspace
one 7B/8B-class coding model (small is fine; recent small coders are good)
long EFFECTIVE context: 64-128k hot, 200k+ effective
first useful response ~1-2 min, warm
fully local / offline, no cloud, no API keys
buyable node under ~1.5k EUR  (no 24GB+ GPU, no ~4k unified-memory box)
```

This is a single-user product target, not a single-process assumption. The user
can have multiple VS Code windows, Zed ACP threads, and CLI sessions open at the
same time. Runtime ownership and workspace-write coordination are covered in
`modeld/multi-client-coordination.md`.

Everything else serves this single goal: a developer (or we) should not have to
buy frontier hardware or leak code to the cloud to get useful coding help.

## The design goal that makes it possible (workspace-context reuse)

A small model + big context is normally too slow, because cold prefill is brutal
(measured on CPU: ~2m14s for ~47k tokens, ~350 tok/s). But a single-user coding
node has a structural edge a generic inference server lacks: **the same workspace
/ repo / tool / system context repeats all day.** So the design goal is:

> Turn the coding workspace into **reusable model execution state**: keep the
> stable prefix hot, invalidate it precisely, re-prefill only the changed suffix,
> and snapshot/restore KV only where it improves durability, branching, or
> measured warm-start behavior.

Warm live prefix reuse — not raw speed and not snapshot files by themselves — is
what makes "8B + 100k local" usable. Snapshots are an important primitive for
suspend/resume, branch, crash recovery, and reproducible benchmarks, but the hot
coding loop should be: keep stable KV hot, prefill the suffix, decode, measure,
and explain. This is the differentiator Contenox owns *above* any inference
engine.

## Concrete targets (Tiers)

| Tier | Model | Context | Latency goal |
|---|---|---|---|
| **T1** | 7B/8B | 64k hot | first useful response < 60s |
| **T2** | 7B/8B | 128k hot | < 120s when the prefix cache is warm |
| **T3** | 7B/8B | **200k+ effective** | via prefix reuse + pins + retrieval + summaries + optional snapshots — never raw dense 200k/turn |

**T3 is the goal.** "Effective" is a measured product behavior, not a marketing
number: given a repo/task larger than the hot window, the assistant can find,
cite, reason about, and edit the relevant parts without sending all tokens fresh
every turn.

Core primitives required (all backend-independent in design):

- deterministic context-segment assembler — **built**: `AssembleContext`
- model/profile-stable segment manifests with byte hashes, token hashes, token
  ranges, and invalidation rules — **partially built in `llama`**: profile,
  model digest, prompt template digest, runtime digest, BOS policy, stable byte
  hash, rendered segment byte ranges, stable-prefix token hash, volatile suffix
  token hash, and backend-resolved per-segment token ranges/hashes now gate and
  explain llama.cpp warm reuse; this still needs to move into the shared
  assembler so OpenVINO and llama.cpp use the same manifest code
- live warm prefix reuse, suffix-only prefill, and optional KV snapshot/restore
- cache admission and eviction policy driven by coding semantics, not plain LRU

## Cache correctness contract

A cache hit is valid only when the reusable prefix is identical for the actual
runtime profile, not merely when the source text looks similar. The manifest for
each turn must carry enough identity to reject false hits:

```json
{
  "profile_id": "qwen-coder-7b-llama",
  "backend": "llamacpp",
  "backend_version": "...",
  "model_digest": "...",
  "tokenizer_digest": "...",
  "chat_template_digest": "...",
  "context_size": 65536,
  "kv_type": "q8_0",
  "flash_attention": true,
  "cache_block_size": 32,
  "segments": [
    {
      "kind": "repo_map",
      "byte_hash": "...",
      "token_hash": "...",
      "token_start": 5100,
      "token_end": 17100,
      "cache_class": "task_pinned",
      "invalidation": "repo_index_change"
    }
  ]
}
```

Hit compatibility includes at least: model digest, tokenizer digest, chat
template digest, backend/runtime version, context/RoPE settings, KV precision,
segment token hash, token position, and backend block/page alignment. Tokenizer
or template drift is a cache invalidation bug, not a tolerable mismatch.

Implementation note as of 2026-06-16: llama now enforces the first-order
compatibility gate for llama.cpp with `ContextManifest`. The manifest includes
model digest, backend version, prompt format/template digest, runtime digest,
BOS policy, stable byte hash, backend-resolved stable-prefix token hash, volatile
suffix token hash, and true per-segment token ranges/hashes under the active
tokenizer. A tiny opt-in GGUF fixture also proves manifest/prefix/suffix handling
and one-token warm/cold equivalence with a sub-512 MiB model. The next
correctness step is moving this into the shared context assembler and proving
the same manifest path with OpenVINO. Product-facing embedded GGUF inference is
`llama`; the old `local` keyword is only a compatibility alias that canonicalizes
to `llama`, and the separate `local`/`localnode` packages are retired.

## Cache priority policy

Generic inference engines can evict by recency. A coding node knows the meaning
of the prefix and should use that information:

```text
highest: system/developer prompt, tool schemas, repo instructions
high:    repo map, pinned files, active task summary
medium:  current diff, recent failing test output
low:     stale terminal logs, old user turns, exploratory snippets
```

Pinned core segments should survive while the workspace session is active.
Volatile suffix material should be admitted only when it is likely to be reused.

## Architecture boundary

Product code should not speak directly in llama.cpp or OpenVINO terms. The
runtime should keep four layers clear:

```text
1. contextasm
   deterministic segment assembly, tokenization cache, segment manifest,
   invalidation rules, explain-context output

2. llama session core
   backend-neutral session interface, lifecycle, cancellation, cache policy,
   metrics, structured errors

3. backend adapters
   llamacpp/session: sequence ops, KV state, GPU config, GGUF
   openvino/session: CB pipeline, SchedulerConfig, state API, IR
   ortgenai/session: generator append/rewind, provider capabilities, ONNX GenAI

4. coding context planner
   repo map, symbol graph, pins, retrieval, summaries, diff/test/log budgeter
```

The product-level calls should look like:

```go
PlanTurn(workspace, task) -> []Segment
EnsurePrefix(session, segments) -> PrefixStatus
PrefillSuffix(session, suffix)
Decode(session, generationConfig)
ExplainContext(session)
```

## Hardware target + runtime strategy

- **Deployment target:** a budget **Intel** node — Arc / Arc Pro dGPU (16 GB is
  the sweet spot), or NPU / AMX — under ~1.5k EUR. Intel because it gives the
  cheapest usable VRAM/compute per euro and **one runtime (OpenVINO)** across
  CPU(AMX) / iGPU / Arc / NPU. (NVIDIA caps at 16 GB in budget; unified-memory
  boxes start ~4k.)
- **Why llama.cpp plus OpenVINO:** `AssembleContext` is substrate-independent Go,
  so the product layer ports with limited substrate-specific work. We prove the
  design *fast* on llama.cpp + an accessible GPU, then replicate to the
  OpenVINO/Intel target. Owning both de-risks the single-vendor bet and proves
  the workspace-context abstraction is real.
- **Why ORT GenAI / Windows ML is separate:** AI PCs are Windows-heavy, and ORT
  GenAI exposes logical append/rewind primitives that may express the same
  session shape on Qualcomm, AMD, DirectML, and CPU paths. This is a
  certification/probe lane, not a replacement for the main Intel/OpenVINO target
  and not a shortcut to claiming 7B/8B at 64k on NPUs.
- **Why direct SDKs wait:** QNN, Ryzen AI/Vitis, RKLLM, and similar SDKs should
  only become direct adapters after a bridge runtime fails a measured Contenox
  requirement such as append/rewind, memory placement, metrics, or cancellation.
- **Where Jetson fits:** Jetson Orin NX 16GB is a real CUDA edge node, but it is
  not the default T1/T2 target. Treat it only as a standalone edge llama
  profile unless a specific profile proves 64k hot context without swap.
  Pi+Jetson helper splits, STT/vision helper services, and NPU sidecars are out
  of scope for this plan. Unified-memory residency and model eviction remain
  certification gates on that class of hardware.

## Why local-only at all (the bet, stated honestly)

- **Privacy / ownership:** code never leaves the machine.
- **Cost:** no per-token cloud bill; one-time hardware.
- **The honest ceiling:** 8B + 100k is a strong local junior-to-mid coding
  assistant with perfect project memory for the current task — *not* an
  autonomous senior engineer. The bet is that small coding models + reusable
  context + tools/verification are genuinely useful, offline, and cheap.

## Non-goals

- Not a generic multi-tenant inference server (single user, one workspace).
- Not raw dense 200k every turn (effective context via workspace-context reuse).
- Not a frontier-model replacement; no 24GB+ / unified-memory hardware assumption.
- Not a KV snapshot database before live prefix reuse is proven.
- Not a sidecar architecture: no required helper model/device/service for
  embeddings, reranking, summaries, STT, vision, file triage, or remote support.
- Not trusting local OpenAI-compatible tool calls without certification; if a
  backend/model/template cannot reliably emit declared tool calls, prompt-injected
  RAG is an acceptable fallback but does not earn a tool-call capability claim.

## Required benchmark report

Every backend/model/hardware profile should emit one local JSON report with the
same shape so runtime claims stay honest:

```text
cold_full_prefill: tokens, ms, prompt_tps, TTFT, memory_high_water
warm_same_prefix: cached_tokens, new_tokens, ms, TTFT, hit_rate
warm_changed_suffix: cached_tokens, suffix_tokens, ms, TTFT, output_equals_cold
edited_stable_segment: expected_cache_miss, actual_cache_miss, output_equals_cold
snapshot_save: tokens, bytes, ms, throughput_MBps
snapshot_restore: tokens, bytes, ms, output_equals_original
decode: output_tokens, tokens_per_sec
failure_cases: over_context, over_batch, cancel_prefill, cancel_decode,
               missing_model, incompatible_snapshot, profile_mismatch
residency: resident_models, on_demand_models, unload_ms, host_memory_peak,
           swap_events
```

The most important curve is TTFT as the changed suffix grows:

```text
0 tokens changed, 256, 1k, 4k, 8k, 16k
```

Go/no-go for the local coding node:

```text
target budget hardware
7B/8B coder
64k hot prefix
edited suffix 1k-8k tokens
warm first useful response < 60s
output equivalent to cold full prompt
no memory spill path during decode
structured recovery after cancellation
certified tool-call protocol when tools are enabled
```

## Coding-context eval gate

Cache mechanics do not prove T3. Effective 200k context is real only when the
context planner succeeds on repo tasks that require selection, citation, and
editing across more text than the hot window can hold:

```text
cross-file bug localization
edit file A based on behavior in file B and config in file C
trace failing test to implementation
large refactor with usage search
repo architecture question with file/symbol citations
```

These evals should record which segments were pinned, retrieved, summarized,
cached, missed, and cited. A long context window is not a substitute for proving
the planner chose the right context.

## Status (what is and isn't proven)

- **Workspace-context DESIGN proven on OpenVINO (CPU):** KV round-trip (S0), 99.5% warm reuse
  (S2), assembler drives the cache (S2.5).
- **Budget-HARDWARE latency goal NOT yet proven:** everything ran CPU-only, where
  cold prefill is the bottleneck → a GPU is required. Proving the warm/cold GPU
  number, then the Tier targets on the budget node, is what the llama.cpp
  backend track now drives.
- **AI PC / alternative silicon NOT yet proven:** ORT GenAI has relevant
  append/rewind APIs, and Windows ML is a credible distribution/runtime layer,
  but each provider/device/model profile must pass capability and equivalence
  gates. Near-term NPUs are treated as 1.5B-4B standalone small-coder engines or
  out of scope, not sidecars and not the first 7B/8B 64k main-coder target.
