# OpenVINO/modeld Hardening Blueprint

Scope: text-only `modeld` OpenVINO backend and the `contenox-runtime` agentic path.

Target: certify OpenVINO only where the runtime can state exact hardware, model, context, and
throughput expectations.

## Certification Rules

- CPU/GPU text models use OpenVINO GenAI `ContinuousBatchingPipeline`.
- NPU is not certified for the ContinuousBatching/PagedAttention path.
- Multimodal/VLM repositories are not valid targets for the text adapter.
- Explicit sparse/XAttention requests fail if the device rejects them.
- Automatic sparse/XAttention attempts may retry dense.
- Model context advertisement must not exceed the trained model ceiling unless a certified
  long-context model/profile exists.
- Runtime throughput must be measured through `contenox`, not only through raw OpenVINO APIs.
- Answer quality and token throughput are separate certification dimensions.

## Failure Modes and Required Handling

| ID | Failure mode | Required handling |
|---|---|---|
| OV-1 | CB scheduler pool exhaustion can poison an OpenVINO pipeline. | Derive a bounded `cache_size`; classify pool-exhaustion and allocator/block-leak asserts as fatal; close the backend session; evict the slot; track the upstream issue. |
| OV-2 | Large static `cache_size` values can thrash unified-memory iGPUs. | Derived default capped at `4 GiB`; explicit profile override remains operator-controlled. |
| OV-3 | Shared-memory GPU plugins can report total memory but zero free memory. | Fall back to host available RAM capped by plugin total. |
| OV-4 | Arc iGPU XAttention can be unsupported on the selected driver stack. | Automatic sparse attempts retry dense; explicit sparse remains a hard failure. |
| OV-5 | NPU cannot run the CB/PagedAttention path. | Reject explicit NPU opens; AUTO excludes NPU. |
| OV-6 | VLM repos such as `gemma4-e4b-ov` are incompatible with the text CB adapter. | Keep VLM repos out of curated OpenVINO text choices and setup guidance. |
| OV-7 | Model `generation_config.json` `max_length` can cap total length despite `max_new_tokens`. | Clear inherited total-length caps for generation and echo-prefill. |
| OV-8 | Raw OpenVINO benchmark extraction can corrupt token counts if it stringifies Python result objects. | Extract generation IDs and record OpenVINO perf counters in raw scripts. |
| OV-9 | A model can produce high throughput while echoing/continuing the prompt. | Certification includes answer-quality smoke checks. |
| OV-10 | Physical prefill before decode can add overhead in no-cold-store sessions. | Deferred prefill stays opt-in behind `CONTENOX_OPENVINO_DEFER_PREFILL=1` with contract counters; enabling it by default requires a product-path benchmark win. |
| OV-11 | Raw OpenVINO accepts prompts beyond runtime-advertised context. | The runtime rejects oversized requests unless a certified long-context model/profile exists. |
| OV-12 | Windows MSVC/OpenVINO package rebuilds can depend on ad hoc host setup. | Windows packaging must be checked-in and reproducible. |

## Required Runtime Work

### Routing and Catalog

- Keep non-text/VLM OpenVINO repos out of the curated text registry.
- Keep NPU out of the ContinuousBatching/effective-context path.
- Surface context limits from certified model/profile metadata.
- Do not advertise raw OpenVINO long-context behavior as runtime support.

### Scheduler and Failure Handling

- Keep the derived scheduler cache formula:
  `ceil(hot_context_tokens * kv_bytes_per_token * 1.25 / GiB)`, minimum `1`, capped at `4`.
- Keep explicit profile `genai.cache_size` precedence.
- Treat allocator leak, block leak, and pool exhaustion assertions as fatal session errors.
- Close poisoned backend sessions and evict the active slot on fatal errors.
- Add bounded retry only with certified limits and an upstream repro.

### Agentic Hot Path

- Keep physical prefill for cold-KV export/import, snapshot restore, and explicit residency
  operations.
- Keep deferred physical prefill behind `CONTENOX_OPENVINO_DEFER_PREFILL=1`.
- Do not enable deferred prefill by default for single-turn no-tools generation.
- Add trace-visible OpenVINO prompt/completion usage instead of relying on tokenizer post-counts
  in benchmark scripts.

### Windows Packaging

- Provide a checked-in Windows MSVC/OpenVINO package script or Makefile target.
- Ensure the target works without local shell-specific compiler wrappers.
- Verify the rebuilt package reports backends `llama`, `openvino`.
- Preserve OpenVINO and llama runtime DLL bundling in the package.

## Certification Matrix

Every certified OpenVINO target must publish:

- CPU/GPU/NPU model, driver, OpenVINO GenAI version, OS.
- model repo, digest, and model profile.
- runtime context limits and tested prompt sizes.
- raw OpenVINO control throughput.
- `contenox` end-to-end throughput.
- output token accounting method.
- answer-quality smoke result.
- unsupported modes for the target.
