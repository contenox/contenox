# Blueprint: TensorRT-LLM and the modeld Boundary

Owner: modeld

Purpose: record the constraints behind the current decision not to embed
TensorRT-LLM in the modeld process, and the conditions any future TensorRT-LLM
cell must satisfy. Facts below were rechecked on 2026-07-03 against public
TensorRT-LLM docs and NGC release metadata; version-sensitive claims must be
re-verified before acting on this blueprint against a newer release.

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

## TensorRT-LLM Facts

Separate verified facts from local integration conclusions. The vendor docs are
an API surface snapshot, not a guarantee that an unlisted integration path does
not exist.

- The supported backend is PyTorch-based, driven by the Python `tensorrt_llm.LLM`
  API; model definitions are PyTorch Python code, which is how day-0 model
  support works.
- TensorRT-LLM also documents a C++ `Executor` API and Python bindings for that
  API. The documented `Executor` construction path takes a TensorRT-LLM engine
  directory or engine buffers plus model JSON. That is a real C++ API, but it
  does not by itself establish a supported C++ entry into the PyTorch-native
  model-definition workflow used by the high-level `LLM` API.
- No documented, stable C or C++ entrypoint has been verified that lets Go
  modeld host the PyTorch-native `LLM` workflow in-process without embedding a
  Python runtime or switching to pre-built engine artifacts. Treat this as the
  current integration assessment, not as a vendor-stated impossibility.
- Pre-built TensorRT-LLM engines are compiled per GPU architecture and
  TensorRT-LLM version, which breaks content-digest artifact identity unless a
  separate engine-build artifact policy is introduced.
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

In-process integration is not an admissible modeld cell today under the
invariants above: the verified PyTorch-native path is Python-facing, while the
verified C++ path is engine-facing and introduces per-architecture artifacts.
NVIDIA GPUs are served by the llama.cpp CUDA backend; closing its feature gaps
is governed by the backend-parity blueprint.

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

- Whether the current TensorRT-LLM release exposes a documented, stable C/C++
  entrypoint for the PyTorch-native model-definition path without embedding
  Python.
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
- [LLM API](https://nvidia.github.io/TensorRT-LLM/llm-api/index.html) ·
  [C++ Executor API](https://nvidia.github.io/TensorRT-LLM/advanced/executor.html)
- Connector precedents: [LMCache](https://docs.lmcache.ai/integrations/tensorrt_llm.html) ·
  [Dynamo KVBM](https://docs.nvidia.com/dynamo/archive/0.5.1/guides/run_kvbm_in_trtllm.html)
- [Triton TRT-LLM backend](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/tensorrtllm_backend/README.html)
- [NGC TensorRT-LLM release container](https://catalog.ngc.nvidia.com/orgs/nvidia/tensorrt-llm/containers/release)
