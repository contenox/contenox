# Blueprint: Effective-Context Runtime Strategy

Status: design; certified specialization portfolio
Owner: runtime / modeld
Target: large effective context on one local accelerator, single-user/single-model,
with bounded prefill latency, usable decode throughput, and explicit quality gates.

Related OpenVINO validation:
- [OpenVINO/modeld hardening blueprint](openvino-hardening-blueprint.md)
- [OpenVINO benchmark findings](openvino-bench-findings.md)
- [Local NVIDIA llama benchmark findings](local-nvidia-llama-bench-findings.md)

Related hardware/runtime strategy:
- [Local inference cross-compare](modeld-local-inference-cross-compare-blueprint.md)
- [Latency-budgeted effective context](hardware-effective-context-blueprint.md)
- [Specialization cells and multi-GPU runtime shapes](specialization-cells-blueprint.md)

Session-derived guardrails:
- [Session aac21f41 learning map](session-aac21f41-learning-map.md)
- [modeld capability-truth boundary](modeld-capability-truth-blueprint.md)
- [Benchmark integrity and reproducibility](benchmark-integrity-blueprint.md)

## 1. Backend Stance

`modeld`'s durable product boundary is the backend-neutral session contract:
`EnsurePrefix -> PrefillSuffix -> Decode`, plus `Snapshot`/`Restore` and capability reports.

The acceleration strategy is a portfolio of certified runtime cells, not one stack:

- chip-vendor runtime adapters when they expose the right primitives and pass `contenox` gates.
- modeld-native kernels when a narrow model/hardware/workload cell can beat generic abstractions.
- compatibility backends when they are useful for GGUF coverage, development, fallback, or regression tests.

`llama.cpp` is a compatibility, bootstrap, GGUF, and test backend. It is not the long-term
multi-vendor acceleration strategy by itself. A modeld-owned narrow kernel path remains valid
when it is explicitly scoped and proves a step-function result.

## 2. Performance Walls

- **Memory wall:** resident KV grows with context and limits physical hot context.
- **Latency wall:** long-prompt prefill is attention-heavy and dominates time to first token.
- **Decode wall:** generated tokens are serial and often KV-bandwidth-bound.
- **Quality wall:** lossy eviction, sparse attention, quantized KV, and long-context extension require
  retrieval/answer-quality gates, not only throughput numbers.

## 3. Existing Substrate

- **Session contract:** `runtime/transport/session.go` keeps runtime callers independent from backend
  internals.
- **SWA-aware capacity:** `capacity.LayerKVProfile` and `modeld/llama/service.go` feed global/windowed
  layer splits into capacity resolution.
- **Residency policy:** `modeld/residency` has block classes, sink/recent flags, hot/cold planning,
  and an optional attention-score seam.
- **Prefix reuse:** llama and OpenVINO sessions already implement stable-prefix reuse through
  `EnsurePrefix`.
- **Snapshot/restore:** the transport supports persisted session state for branch/reuse workflows.
- **OpenVINO controls:** OpenVINO GenAI exposes prefix caching, cache eviction, sparse prefill, KV
  precision, and scheduler cache controls through model profiles.
- **llama compatibility controls:** the llama adapter exposes KV precision, flash attention, middle
  removal, position shift, cold KV, and decode sliding where llama.cpp supports them.

## 4. Work That Survives Backend Replacement

- **Certification matrix:** publish hardware, driver/runtime version, model digest, context limits,
  prompt sizes, `contenox` end-to-end throughput, raw backend control throughput, token accounting,
  quality smoke, and unsupported modes.
- **Runtime selection:** select backend adapters by certified profile and measured `contenox`
  behavior, not by raw backend microbenchmarks.
- **Prefix snapshot cache:** cache stable-prefix snapshots keyed by model, backend, profile, adapters,
  prompt template, tokenizer policy, and manifest digest.
- **Split K/V cache configuration:** carry `kv_cache_type_k` and `kv_cache_type_v` through profiles,
  transport identity, capacity math, and backend adapters that support it.
- **Residency telemetry:** expose sink/recent sizes, hot tokens, cold tokens, evicted ranges, and
  restore/recompute events in traces.
- **Agentic benchmark harness:** keep workload definitions backend-neutral and runnable through
  `contenox-runtime` on every certified platform.

## 5. Specialization Cells

### Intel / OpenVINO

- Keep ContinuousBatching/PagedAttention text models as the certified path.
- Keep NPU out of the effective-context path unless Intel provides a supported text pipeline.
- Keep sparse prefill and cache eviction profile-gated and benchmarked per model/device.
- Fix trace-visible token accounting for OpenVINO runs.

### NVIDIA and AMD Vendor Runtime Cells

- Add adapters when the runtime can implement the session contract or a strict equivalent.
- Require prefix reuse, explicit context limits, token accounting, and reproducible packaging.
- Use vendor-provided KV/cache/sparse/speculative mechanisms when they produce the best certified cell.
- Certify per runtime version, driver, accelerator, model format, and context profile.

### modeld-Native Narrow Kernel Cells

- Scope each cell to a named model family, hardware topology, backend, context tier, and quality gate.
- Valid targets include replicated-weights/sharded-KV, block-sparse prefill, fused quantized KV
  attention, and model-family-specific prefix/KV reuse.
- Keep the implementation only if it unlocks a step-function: larger usable context, much lower
  prefill wall time, or materially better end-to-end agent turn time.
- Do not generalize the kernel path until a certified narrow cell proves the value.

### llama.cpp Compatibility Cell

- Keep GGUF compatibility, local development, fallback, and regression tests.
- Use llama.cpp primitives when they are already exposed and stable.
- Avoid broad product-critical CUDA/HIP fork work; allow narrow modeld-owned kernels under the
  same certification gates as every other cell.
- Keep llama-specific behavior behind backend capabilities and profile identity.

## 6. Do Not Do Blindly

- Picking one backend stack before the benchmark matrix says it wins.
- Broad CUDA/HIP/ggml fork work without a named model/hardware/workload cell.
- Advertising raw backend context behavior as supported runtime context.
- Backend-specific benchmark results without the `contenox-runtime` agentic path.
- Throughput claims without answer-quality smoke results.

## 7. Implementation Order

- **M0 - Backend stance cleanup.** Catalog/profile docs and setup output describe supported cells
  by certified backend/runtime/hardware/model/workload facts.
- **M1 - Certification schema.** Add a profile schema for backend/runtime/device/model/context
  certification and report it through model capability output.
- **M2 - Prefix snapshot cache.** Add a bounded persistent cache for stable-prefix snapshots.
- **M3 - Split K/V cache config.** Add separate K/V cache precision fields and capacity accounting.
- **M4 - Cell contract.** Define the minimal adapter/kernel requirements against the existing
  `transport.Session` behavior.
- **M5 - Candidate cells.** Benchmark vendor-runtime cells and modeld-native narrow-kernel cells
  against the same `contenox-runtime` workloads and quality smoke.
- **M6 - Keep/drop decisions.** Keep only cells that deliver a step-function result over the
  baseline for the target workload.
