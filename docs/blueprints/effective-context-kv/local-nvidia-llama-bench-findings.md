# Local NVIDIA Llama Backend Facts

Scope: `contenox` product-path sanity benchmark on the local Linux NVIDIA laptop GPU.

Test host:

- GPU: NVIDIA GeForce RTX 3060 Laptop GPU, 6,144 MiB VRAM
- Driver: `580.167.08`
- Runtime path: `contenox -> modeld -> llama.cpp direct runtime -> CUDA`
- `contenox`: `v0.32.8`
- Packaged `modeld`: `v0.32.5`
- llama.cpp commit: `ee3a5a10adf9e83722d1914dddc56a0623ececaf`
- Model: `qwen2.5-1.5b` GGUF, `model.gguf` size `1,117,320,736` bytes
- Original benchmark artifact root: `.bench/codex-20260701-nvidia-qwen25-product-141259`
- Fixed auto-context artifact root: `.bench/codex-20260701-nvidia-qwen25-fixed-autocontext-142239`
- Warm-reuse artifact root: `.bench/codex-20260701-nvidia-qwen25-warm-reuse-143147`

## Validated Facts

| Area | Fact | Runtime stance |
|---|---|---|
| CUDA path | `modeld` loaded `libggml-cuda.so`, detected the RTX 3060 Laptop GPU, and llama.cpp offloaded `29/29` layers to `CUDA0`. | These rows are GPU rows, not accidental CPU rows. |
| Model directory | Pointing a llama backend at the full local model directory caused unrelated GGUF catalog work before inference. | Benchmark rows use an isolated model directory containing only `qwen2.5-1.5b`. |
| Token accounting | llama product-path output reports `inputTokens:0`, `outputTokens:0`, and trace rows omit `tokens=A+B=C`. | Completion counts below are derived from `publish_step_chunk` start events. Fix llama token accounting before using trace tokens as canonical. |
| Context autodetect | The source-fixed CLI opens omitted llama profile context from modeld's detected capacity. The 11.6k-class prompt opened `num_ctx=32704` with no `CONTENOX_LLAMA_CTX`. | This is the expected product behavior. |
| Original regression | The packaged `v0.32.8` CLI opened the same omitted-profile model at `num_ctx=8192` while model list advertised `CTX=32768`. | Fixed in `runtime/modelrepo/llama`: omitted profile context stays unset until modeld `Describe` resolves capacity. |
| Model list | Model list advertised `CTX=32768`; the fixed runtime session opened `num_ctx=32704` after modeld's 64-token safety margin. | Benchmark rows must record session-open physical context, not only model-list context. |
| Session reuse | The modeld llama session reuses resident stable-prefix KV by token LCP. In a same-session check, turn B reused all `9,350` stable-prefix tokens and prefilled `0` stable tokens. | Warm agentic-turn viability must be measured separately from cold `contenox run` wall time. |
| CLI process boundary | `contenox run` is stateless. `contenox chat` persists history but builds/stops an engine per CLI process; the llama warm cache is process-local. | Repeated one-shot CLI calls do not prove warm-session behavior. Benchmark long-lived surfaces separately. |

## Results

Workload: `contenox run --chain scripts/contenox-bench-no-tools-chain.json`,
no tool schemas, `--max-tokens 64`, isolated llama model directory.

Prompt labels reuse the OpenVINO benchmark tiers, but the model is not identical:
`qwen2.5-1.5b` GGUF is a comparable small-model local CUDA cell, not the
`tinyllama-1.1b-chat-v1.0-int4-ov` OpenVINO model.

| prompt label | assembled tokens | CLI context | physical `num_ctx` | result | wall | trace task | completion | e2e rate | trace rate | GPU layers | run directory |
|---:|---:|---:|---:|---|---:|---:|---:|---:|---:|---:|---|
| 374 | 287 | 4,096 | 8,192 | success | 3.76 s | 1.70 s | 64 tok | 17.02 tok/s | 37.73 tok/s | 29/29 | `.bench/codex-20260701-nvidia-qwen25-product-141259/default-00374` |
| 2,900 | 1,795 | 4,096 | 8,192 | success | 3.07 s | 1.29 s | 64 tok | 20.84 tok/s | 49.60 tok/s | 29/29 | `.bench/codex-20260701-nvidia-qwen25-product-141259/default-02900` |
| 11,600 packaged CLI | 6,988 | 16,384 | 8,192 | context reject | 2.43 s | 0.64 s | n/a | n/a | n/a | 29/29 | `.bench/codex-20260701-nvidia-qwen25-product-141259/default-11600` |
| 11,600 source-fixed CLI | 6,988 | 16,384 | 32,704 | success | 5.29 s | 3.22 s | 64 tok | 12.11 tok/s | 19.91 tok/s | 29/29 | `.bench/codex-20260701-nvidia-qwen25-fixed-autocontext-142239` |

## Warm Session Reuse Check

Workload: direct modeld session contract on the same packaged llama backend and
same Qwen2.5 GGUF. The stable prefix was the 11,600-label benchmark prompt file,
held constant across turns; only a small suffix changed. This isolates the
backend reuse primitive from CLI process startup and task-chain overhead.

| row | stable prefix tokens | reused prefix tokens | prefilled prefix tokens | dropped tokens | suffix tokens | open | ensure prefix | prefill suffix | decode 16 tok | turn wall |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| cold same-session turn A | 9,350 | 0 | 9,350 | 0 | 17 | 0.76 s | 1.77 s | 0.06 s | 0.13 s | 1.97 s |
| warm same-session turn B | 9,350 | 9,350 | 0 | 33 | 16 | n/a | 0.03 s | 0.01 s | 0.13 s | 0.18 s |
| cold new-session turn B | 9,350 | 0 | 9,350 | 0 | 16 | 0.56 s | 1.73 s | 0.04 s | 0.15 s | 1.91 s |

Interpretation:

- The backend primitive works: same-session turn B avoided recomputing the full
  stable prefix and only replaced the volatile suffix/generated tail.
- The previous `contenox run` rows measured a necessary cold/stateless case, not
  the agentic hot loop.
- A useful agentic benchmark must report prefix reuse counters, suffix prefill,
  TTFT, decode rate, and whether the caller stayed in one runtime process.
- Long-context input is useful only if the chain/runtime places reusable context
  in the stable prefix. Large user-turn payloads remain volatile by design.

## Comparison To Windows OpenVINO Rows

| tier | Windows Intel Arc OpenVINO TinyLlama | Linux RTX 3060 llama/Qwen2.5 | Read |
|---:|---:|---:|---|
| 374 | 300 assembled, 6.88 s wall, 9.45 tok/s e2e | 287 assembled, 3.76 s wall, 17.02 tok/s e2e | Local NVIDIA CUDA path is faster for this small tier. |
| 2,900 | 1,907 assembled, 9.97 s wall, 6.42 tok/s e2e | 1,795 assembled, 3.07 s wall, 20.84 tok/s e2e | Local NVIDIA CUDA path is much faster in this comparable tier. |
| 11,600 | Runtime rejects TinyLlama because the certified model ceiling is 2,048. | Source-fixed CLI autodetects physical 32,704 and succeeds at 5.29 s wall. | NVIDIA row proves this model/hardware can handle this prompt when the llama provider honors modeld capacity. |

## Required Fixes

- Add llama token usage reporting to trace rows and raw `inputTokens` / `outputTokens`.
- Add product-path telemetry for `PrefixStatus` and `SuffixStatus` so traces
  show `reused`, `prefilled`, `dropped`, and `suffix` token counts.
- Record session-open physical context in benchmark rows; model-list `CTX` is insufficient.
- Split benchmark suites into cold `run`, warm same-process/session, and repeated
  one-shot CLI process rows.
- Keep isolated model directories or cached catalog snapshots for benchmark cells so unrelated local
  model inventory does not pollute wall-clock latency.
- Add repeat rounds before publishing p50/p90/p95; these rows are single-run sanity checks.
