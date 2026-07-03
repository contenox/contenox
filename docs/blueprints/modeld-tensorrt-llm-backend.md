# Blueprint: TensorRT-LLM and the modeld Boundary

Owner: modeld

Purpose: record the standing constraints that keep TensorRT-LLM out of the
modeld process, and the conditions any future TensorRT-LLM cell must satisfy.
Facts are pinned to TensorRT-LLM release 1.2; re-verify them before acting on
this blueprint against a newer release.

## Invariants

- modeld never delegates its slot to a child process. A backend either lives
  in-process behind the compiled-in registry or is a whole separate daemon
  implementing the gRPC transport contract. Driver-subprocess architectures
  (two half-owners of one slot's state) are not admissible.
- Raw backend benchmark wins are not certification. Only end-to-end `contenox`
  measurements per the benchmark-integrity blueprint justify a new cell.
- A new runtime cell must clear the step-function gates in the
  specialization-cells blueprint; small decode-throughput gains alone do not
  justify a cell.

## TensorRT-LLM Facts (release 1.2)

- The supported backend is PyTorch-based, driven by the Python `tensorrt_llm.LLM`
  API; model definitions are PyTorch Python code, which is how day-0 model
  support works. There is no C API and no supported C/C++ entry into this
  backend; Triton itself reaches it through the Python LLM API.
- The C++ `executor` API serves only the pre-compiled-engine lane. Engines are
  compiled per GPU architecture and per TensorRT-LLM version, which breaks
  content-digest artifact identity.
- KV cache: engine-global radix-tree block reuse (`enable_block_reuse`,
  `enable_partial_reuse`), priority retention (`KvCacheRetentionConfig`),
  request isolation via `cache_salt`, host offload (`host_cache_size`), an
  events API, and a pluggable KV Cache Connector API (custom scheduler/worker
  classes; used by LMCache and Dynamo KVBM).
- Guided decoding (XGrammar: JSON schema / regex / EBNF) and per-request LoRA
  are supported in the PyTorch backend.
- Platform: Linux x86_64 / SBSA only. Distribution is a multi-GB pip wheel set
  under NVIDIA's EULA.

## Consequence

In-process integration is impossible without violating an invariant: the
supported path requires a Python interpreter, and the C++ path rides a
deprecating lane with per-architecture artifacts. NVIDIA GPUs are served by
the llama.cpp CUDA backend; closing its feature gaps is governed by the
backend-parity blueprint.

## Admissible Future Shape

A TensorRT-LLM cell, if ever justified, is a separate binary implementing the
modeld gRPC transport contract on Linux/NVIDIA, coexisting with the Go modeld
for other platforms. Entry conditions:

- a benchmark on target hardware (certified model, agentic loop, product path)
  showing a step-function win over the llama.cpp CUDA cell, including a
  llama.cpp-with-speculation baseline;
- the contract-mapping table below validated against the then-current
  TensorRT-LLM release.

## Contract Mapping Reference

How the transport contract maps onto TensorRT-LLM mechanisms, for whenever the
cell is evaluated:

| Contract | TensorRT-LLM mechanism |
|---|---|
| Manifest-keyed reuse validity | `cache_salt` = manifest digest ⊕ adapter digests; engine-level guarantee that KV never crosses manifests. |
| `EnsurePrefix` | Render via HF `apply_chat_template`, tokenize, warm-up `generate_async(max_tokens=1)` with `KvCacheRetentionConfig` pinning the prefix range. Reuse counters from per-request perf metrics (unverified; see Open Unknowns). Divergent-tail drop is bookkeeping; stale blocks age out of the radix tree. |
| `PrefillSuffix` | Deferred: bookkeeping only, riding the next decode's prefill; the contract's `DeferredPrefill*` counters report it. |
| `Decode` | `generate_async(streaming=True)`; `SamplingParams` for sampling; `GuidedDecodingParams` for `StructuredOutput`; shared Go-side thinking/tool-call parsing. |
| `ExplainContext` | Adapter bookkeeping + KV events/perf metrics. Capabilities: `RemoveTail=false`, `RemoveMiddle=false`, `PositionShift=false`, `RecomputeRange=true`. |
| `Snapshot`/`Restore` | Text-level snapshot; restore by re-render + warm-up prefill. Engine KV blobs are not exportable; `ColdKVBlock` parity requires a KV Cache Connector worker bridging to the contenox cold store. |
| `Describe` | HF `config.json` for KV geometry and model ceiling, NVML for memory; feed `capacity.Policy`; enforce via `max_seq_len` + `KvCacheConfig.max_tokens` (never the default memory fraction). |
| `Embed` | `ErrUnsupportedFeature`. |
| LoRA | `LoraConfig` + per-request adapters; digests fold into `cache_salt`. |
| Slot switch | Tear down / recreate the `LLM` instance. |
| `ErrContextOverflow` | Enforced adapter-side from token counts vs `NumCtx` before dispatch. |

## Risks

| Risk | Position |
|---|---|
| Warm-up `max_tokens=1` may not materialize/retain all prefix blocks under pressure. | Measure reuse ratio on the agentic loop before trusting `EnsurePrefix` semantics. |
| Priority retention may only bias LRU rather than pin. | Same measurement; degrade honestly in `PrefixStatus` if so. |
| Per-request perf metrics may not expose reused-block counts. | Fall back to KV events; if neither, reuse counters are estimates and must be labeled. |
| Version churn across TensorRT-LLM releases. | Pin exactly; a version bump re-runs certification. |
| `free_gpu_memory_fraction` conflicts with byte-based capacity policy. | Always drive KV budget in tokens/bytes from the policy. |

## Open Unknowns

- Exact LLM-API surface and granularity of per-request KV reuse metrics.
- Whether priority-100 retention survives sustained pressure.
- Whether the Connector API can export blocks with positions usable for
  shifted import, or only whole-prefix onboard.

## Sources

- [TensorRT-LLM repo](https://github.com/NVIDIA/TensorRT-LLM) ·
  [release notes](https://nvidia.github.io/TensorRT-LLM/release-notes.html) ·
  [PyTorch backend arch](https://github.com/NVIDIA/TensorRT-LLM/blob/main/docs/source/torch/arch_overview.md)
- [KV cache system](https://nvidia.github.io/TensorRT-LLM/latest/features/kvcache.html) ·
  [KV cache offloading example](https://nvidia.github.io/TensorRT-LLM/examples/llm_kv_cache_offloading.html)
- [LLM API](https://nvidia.github.io/TensorRT-LLM/llm-api/index.html)
- Connector precedents: [LMCache](https://docs.lmcache.ai/integrations/tensorrt_llm.html) ·
  [Dynamo KVBM](https://docs.nvidia.com/dynamo/archive/0.5.1/guides/run_kvbm_in_trtllm.html)
- [Triton TRT-LLM backend](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/tensorrtllm_backend/README.html)
