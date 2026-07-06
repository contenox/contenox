# Blueprint: Specialization Cells and Multi-GPU Runtime Shapes

Owner: runtime / modeld

Purpose: define how Contenox can use narrow hardware/model/runtime assumptions without
settling on one stack.

## Core Rule

`modeld` owns the session contract and certification. Implementations are cells.

A cell is a tuple:

```text
backend/runtime + hardware topology + model family + quant/KV profile + context tier + workload
```

Cells can be vendor-runtime adapters, modeld-native kernels, compatibility backends,
or specialist appliance adapters. A cell is kept only if it produces a step-function
result for the target workload.

## Shared Contract

Every cell must map to the same product behavior:

- `EnsurePrefix`
- `PrefillSuffix`
- `Decode`
- `Snapshot`/`Restore` when supported or explicitly reported unsupported.
- capability report with exact context, memory, runtime, and unsupported modes.
- trace-visible prompt/completion token accounting.
- error classification for clamp, overflow, fallback, timeout, and fatal backend state.

## Multi-GPU Shapes

### Tensor Parallel

Each transformer layer is split across accelerators.

Use when:

- model weights do not fit on one accelerator.
- aggregate memory bandwidth improves latency.
- interconnect is fast enough for per-layer reductions.

Costs:

- communication every layer.
- smaller per-GPU kernels.
- synchronization overhead.

Certify only per topology. NVLink/NVSwitch and PCIe/eGPU are different cells.

### Pipeline Parallel

Layer ranges are split across accelerators.

Use when:

- weights do not fit on one accelerator.
- latency budget tolerates pipeline bubbles.

This is usually less attractive for a single interactive agent turn than tensor
parallel or replicated-weights/sharded-KV, unless model fit requires it.

### Data Parallel

Each accelerator serves independent requests with a full model copy.

Use for throughput across many users or workers. Do not count it as a single-agent
long-context optimization.

### Replicated Weights + Sharded KV

Each accelerator holds full weights and a different shard of the KV cache.

Use when:

- weights fit on each accelerator.
- KV/cache memory is the context limiter.
- the target is one long-context request or agent session.

The exact-attention decode shape is:

```text
1. broadcast or duplicate hidden state.
2. each accelerator computes Q and attends over local KV blocks.
3. reduce softmax statistics and partial attention outputs.
4. continue with identical hidden state on each accelerator.
```

This can unlock larger usable context without sharding weights. It is a candidate
modeld-native narrow-kernel cell for fixed two-GPU/eGPU setups and fixed model families.

Risks:

- per-layer communication can dominate on weak interconnects.
- prefill needs a separate plan; decode-only success is not enough.
- exact softmax reduction must be numerically tested against dense single-device output.

## Narrow Kernel Candidate Cells

### Block-Sparse Prefill

Target: reduce long-prompt prefill wall time for repo/agent prompts.

Valid only if:

- it delivers a large prefill or end-to-end speedup at 32k+ context.
- quality smoke passes on retrieval/code tasks.
- dense fallback remains available.

### Fused Quantized KV Attention

Target: increase hot context and reduce decode bandwidth by storing K/V lower precision
and fusing dequantization into attention.

Valid only if:

- usable context increases materially.
- decode latency improves or stays within budget.
- quality smoke passes per model family.

### Replicated-Weights/Sharded-KV Attention

Target: use multiple accelerators as a larger KV memory system for one agent session.

Valid only if:

- the target context fails or is too slow on one accelerator.
- the multi-accelerator cell passes latency and quality gates.
- interconnect overhead stays below the step-function benefit.

### Model-Family Prefix/KV Reuse

Target: remove repeated prefill across agent turns and branches.

Valid only if:

- stable-prefix reuse is exact.
- cache identity includes model, backend, profile, adapter, tokenizer, template, and manifest.
- trace data reports hit/miss, bytes/tokens reused, and restore cost.

## Step-Function Gates

Keep a candidate cell only if it delivers at least one:

- context tier that otherwise cannot run.
- at least 2x usable context at the same latency budget.
- at least 3x prefill/first-output improvement at 32k+ context.
- materially better end-to-end agent turn time at the same quality.

Do not keep a custom cell for small decode-token throughput gains alone.

## Certification Matrix

Every cell publishes:

- hardware topology and interconnect.
- backend/runtime and version.
- model family, model digest, quantization, KV precision.
- context tiers tested.
- prefill/first-output p50/p90/p95.
- decode p50/p90/p95.
- end-to-end `contenox-runtime` p50/p90/p95.
- quality smoke result.
- dense or baseline comparison.
- unsupported modes.

## Contenox Implications

- `modeld` can support multiple optimized cells without picking one stack.
- The runtime chooses cells by certified capability and workload fit.
- The same model may have different certified cells for local laptop, single GPU,
  dual GPU, pro workstation, and HBM server.
- Hardware-market conclusions feed certification priority, not global product strategy.
- A narrow modeld-native cell is valid when generic runtime abstractions leave a real
  context/latency unlock on the table.
