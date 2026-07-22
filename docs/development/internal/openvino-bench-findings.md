# OpenVINO Backend Facts

Scope: text-only `modeld` OpenVINO backend on Windows Intel AI-PC hardware.

Test host:

- CPU: Intel Core Ultra 7 155H
- GPU: integrated Intel Arc GPU
- NPU: Intel AI Boost
- Runtime path: `contenox-windows-amd64.exe -> modeld.cmd -> OpenVINO GenAI`
- Packaged modeld: `v0.32.5`, OpenVINO GenAI `2026.2.0.0`, backends `llama`, `openvino`
- Runtime CLI: `v0.32.8`
- GPU driver in latest rerun: `32.0.101.8132`
- Python token counter runtime in latest rerun: `openvino_genai 2026.2.1.0-3123-7dea0459b2a`

## Validated Constraints

| Area | Fact | Runtime stance |
|---|---|---|
| Text pipeline | The backend uses OpenVINO GenAI `ContinuousBatchingPipeline`. | Certify text models only. |
| NPU | The CB/PagedAttention path is unsupported on NPU. | Explicit NPU opens are rejected; AUTO excludes NPU for this path. |
| Arc iGPU XAttention | The tested Arc/driver stack rejects XAttention. | Automatic sparse attempts retry dense; explicit sparse remains a hard failure. |
| Multimodal repos | `gemma4-e4b-ov` is a VLM repo, not a text-only CB target. | Do not curate it for the text adapter. |
| Scheduler pool | Too-small CB block pools can poison an OpenVINO pipeline after allocator exhaustion. Oversized pools can thrash unified memory. | Default `cache_size` is derived from hot context and capped; known allocator-leak errors mark sessions fatal and close the backend. |
| TinyLlama length cap | The model config carries a total `max_length` that can conflict with `max_new_tokens`. | modeld clears inherited total-length caps for generation and echo-prefill. |
| TinyLlama context advertisement | Runtime/model metadata advertises TinyLlama at its trained ceiling (`max_position_embeddings=2048`). | 11.6k-token runtime requests are rejected unless a certified long-context profile/model exists. |
| Deferred physical prefill | `CONTENOX_OPENVINO_DEFER_PREFILL=1` avoids physical prefill in no-cold-store sessions. | Keep opt-in; it is slower for the single-turn no-tools TinyLlama workload below. |
| Token accounting | Runtime trace rows report zero prompt/completion usage for these OpenVINO runs. | Benchmark completion tokens are counted from `--raw` assistant output with the OpenVINO tokenizer. |
| Windows packaging | The tested host needed a local executable cgo wrapper to rebuild the MSVC/OpenVINO package. | Add a checked-in Windows packaging path before treating this as reproducible release machinery. |

## TinyLlama Results

Model: `tinyllama-1.1b-chat-v1.0-int4-ov`

Workload: `contenox run --chain scripts/contenox-bench-no-tools-chain.json`, no tool schemas,
`--context 4096`, `--max-tokens 64`, device `GPU`.

Prompt files: `prompt-00374.txt`, `prompt-02900.txt`, `prompt-11600.txt`.

### Runtime Path

Rerun date: 2026-07-01.

| prompt label | runtime assembled tokens | mode | device | context flag | result | wall | trace task | completion | e2e rate | trace rate | run directory |
|---:|---:|---|---|---:|---|---:|---:|---:|---:|---:|---|
| 374 | 300 | default | GPU | 4,096 | success | 6.88 s | 5.45 s | 65 tok | 9.45 tok/s | 11.94 tok/s | `C:\Users\builder\contenox-build\bench-runs\codex-20260701-tinyllama-product\default-00374` |
| 2,900 | 1,907 | default | GPU | 4,096 | success | 9.97 s | 8.47 s | 64 tok | 6.42 tok/s | 7.56 tok/s | `C:\Users\builder\contenox-build\bench-runs\codex-20260701-tinyllama-product\default-02900` |
| 11,600 | 7,437 | default | GPU | 16,384 | resolver reject | 1.61 s | 0.21 s | n/a | n/a | n/a | `C:\Users\builder\contenox-build\bench-runs\codex-20260701-tinyllama-product\default-11600` |
| 374 | 300 | deferred prefill | GPU | 4,096 | success | 6.91 s | 5.52 s | 65 tok | 9.41 tok/s | 11.79 tok/s | `C:\Users\builder\contenox-build\bench-runs\codex-20260701-tinyllama-product\defer-00374` |
| 2,900 | 1,907 | deferred prefill | GPU | 4,096 | success | 15.43 s | 14.01 s | 64 tok | 4.15 tok/s | 4.57 tok/s | `C:\Users\builder\contenox-build\bench-runs\codex-20260701-tinyllama-product\defer-02900` |
| 374 | 300 | default | NPU | 4,096 | unsupported | 1.53 s | 0.14 s | n/a | n/a | n/a | `C:\Users\builder\contenox-build\bench-runs\codex-20260701-tinyllama-product\npu-00374` |

NPU error:

```text
OpenVINO NPU cannot run the continuous-batching (effective-context) pipeline;
PagedAttention is unsupported on the NPU; use CONTENOX_OPENVINO_DEVICE=GPU or CPU, or AUTO
```

### Raw OpenVINO Control

Raw rows use Python OpenVINO GenAI `ContinuousBatchingPipeline`, dense attention,
`cache_size=1`, `max_new_tokens=64`. They do not include runtime routing, transport, system
prompt, chain execution, or session bookkeeping.

| prompt label | load | generate | wall | completion | generate rate | wall rate | run directory |
|---:|---:|---:|---:|---:|---:|---:|---|
| 374 | 4.9 s | 1.5 s | 6.4 s | 65 tok | 43.55 tok/s | 10.22 tok/s | `C:\Users\builder\contenox-build\bench-runs\raw-openvino-tinyllama-cb-context-00374-audited` |
| 2,900 | 5.0 s | 4.0 s | 9.0 s | 53 tok | 13.28 tok/s | 5.90 tok/s | `C:\Users\builder\contenox-build\bench-runs\raw-openvino-tinyllama-cb-context-02900` |
| 11,600 | 5.0 s | 30.6 s | 35.7 s | 62 tok | 2.03 tok/s | 1.74 tok/s | `C:\Users\builder\contenox-build\bench-runs\raw-openvino-tinyllama-cb-context-11600` |

## Reads

- The rebuilt default runtime path is usable on this GPU for the tested TinyLlama no-tools
  workload at 374 and 2,900 prompt-token labels.
- Raw OpenVINO generate-only throughput remains higher than the runtime path. At 374 tokens the
  runtime wall is near raw load+generate wall; at 2,900 tokens raw wall is lower.
- Deferred physical prefill is not a default candidate for single-turn no-tools generation.
- The 11.6k raw OpenVINO control is not a certified runtime result because the runtime rejects
  the request before modeld inference.
- Explicit NPU is a supported negative test: modeld rejects it before inference with a
  PagedAttention unsupported-feature error.
- TinyLlama output quality is poor for the repo-summary prompt: it echoes or continues the
  prompt. Throughput and answer quality must be tracked separately.
- The tested Arc iGPU rows are dense fallback rows, not successful XAttention rows.

Requirements implied by these rows are owned by the
[OpenVINO hardening blueprint](openvino-hardening-blueprint.md) (packaging,
fatal classification and upstream tracking, certification matrix), the
[backend parity contract](backend-parity-blueprint.md) (trace token usage),
and the [capability-truth blueprint](modeld-capability-truth-blueprint.md)
(context advertisement).
