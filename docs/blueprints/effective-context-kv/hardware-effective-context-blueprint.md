# Blueprint: Latency-Budgeted Effective Context

Owner: runtime / modeld

Purpose: define how Contenox decides whether a hardware/backend/model cell supports
real agentic context, not only whether the model and KV cache fit in memory.

## Core Rule

Do not certify context by fit alone.

Certified context is the largest context tier that passes:

- prompt/prefill latency budget.
- decode latency budget.
- end-to-end `contenox-runtime` turn budget.
- answer-quality smoke.
- repeated-run stability.
- warm-state correctness: stable context is reused, volatile context is honestly
  measured as recomputed.

VRAM capacity is a fit gate. Memory bandwidth and interconnect determine whether the
fit context remains usable.

## Required Hardware Facts

Every certified cell records:

- accelerator model and count.
- memory capacity per accelerator.
- memory bandwidth per accelerator.
- memory bus width when published.
- memory type: GDDR6, GDDR7, HBM2e, HBM3, HBM3e, LPDDR unified, SRAM appliance.
- host interconnect: PCIe generation/lane width, Thunderbolt/eGPU, NVLink/NVSwitch,
  Infinity Fabric, vendor fabric, or appliance fabric.
- driver/runtime version.
- model family, quantization, KV precision, and model digest.
- context tier and output-token tier.

## Required Runtime Metrics

Every benchmark row records:

- `contenox` wall time.
- prompt tokens and completion tokens.
- stable prefix tokens, reused tokens, prefilled tokens, dropped tokens, and suffix tokens
  when the backend exposes them.
- time to first token or closest available prefill/first-output measure.
- time per output token or closest decode-token measure.
- p50/p90/p95 across repeated runs.
- model load time separately from warm-session time.
- context overflow, backend fallback, unsupported-feature, and timeout errors.
- answer-quality smoke result.

Headline numbers come from `contenox`, not raw backend scripts.

## Derived Checks

### Fit Check

Use the backend's measured/resolved numbers when available:

```text
required_memory ~= weights + hot_KV(context) + runtime_overhead
```

Fit only means the cell can attempt the workload.

### Decode Lower Bound

Use hardware bandwidth as a sanity lower bound:

```text
ideal_decode_tokens_per_second <= memory_bandwidth / bytes_read_per_generated_token
```

`bytes_read_per_generated_token` includes the model-weight traffic and KV traffic
needed by the backend for the tested context. This bound is optimistic; real results
must be lower after kernel overhead, synchronization, scheduling, and framework costs.

If the ideal bound is below the desired latency target, the cell cannot be certified
for that context tier.

### Context Growth Slope

Run the same workload at increasing context tiers:

```text
4k -> 8k -> 16k -> 32k -> 64k -> 128k
```

For each tier, report:

- prefill/first-token growth.
- decode latency growth.
- end-to-end turn growth.
- quality smoke.

Stop certifying at the first tier where latency or quality fails.

### Stable-Reuse Slope

Run the same stable repo/system context with small changing suffixes:

```text
cold turn -> warm turn same stable prefix -> cold new session same prompt
```

For each tier, report:

- stable prefix tokens.
- reused versus prefilled prefix tokens.
- suffix tokens.
- `EnsurePrefix`, `PrefillSuffix`, decode, and end-to-end turn time.
- whether the caller was a long-lived runtime process or a one-shot CLI process.

This is the agentic hot-loop measurement. A backend can pass cold context fit and still fail
agentic viability if the product path recomputes growing volatile history every turn.

## Hardware Class Reads

### 16 GB GDDR Cards

Use for small models and moderate context. Do not certify as long-context agentic
hardware unless the exact model/profile passes the latency and quality gates.

### 24-32 GB Mid-Bandwidth Cards

Capacity can unlock larger models than 16 GB cards. Bandwidth can still cap usable
context. Treat these as capacity-first cells unless benchmarks prove interactive
latency at the target context.

### 32 GB High-Bandwidth Cards

High memory bandwidth can make 16k-32k contexts materially more usable. Capacity still
limits larger model/context combinations.

### 48-96 GB Pro GDDR Cards

These expand fit range substantially. They do not automatically expand interactive
context by the same factor because bandwidth does not scale linearly with capacity or
price.

### Apple Unified Memory

Treat as capacity-first local hardware. Unified memory can fit large workloads, but
published bandwidth is below high-end GDDR and far below HBM accelerators. Certify by
latency, not by unified-memory size.

### HBM Datacenter Accelerators

Treat as the raw long-context serving tier. HBM raises both capacity and bandwidth, so
it changes the usable-context slope rather than only the fit ceiling.

### Specialist Appliances

Compare as systems, not as GPU cards. SRAM/fabric appliances use different memory
hierarchies, so certify by complete-system latency, context, and quality.

## Contenox Implications

- The model picker must distinguish `fits` from `certified_context`.
- Certification must distinguish cold context from warm reusable context.
- Context advertisement must include latency tier and benchmark provenance.
- Hardware certification must include memory bandwidth and interconnect, not only VRAM.
- Long-context claims require `contenox-runtime` end-to-end data.
- Chains need an explicit stable-context channel for reusable repo/system state. Large user-turn
  payloads are volatile unless the planner marks them stable.
- A cell with large memory and low bandwidth is a capacity cell, not automatically a
  long-context interactive cell.
- A cell with smaller memory and high bandwidth can be better for real work when the
  model/context fits.

## Acceptance

A hardware/backend/model cell is certified for a context tier only when:

- the model opens without fallback.
- the requested context is not clamped below the tier.
- p95 end-to-end turn latency is within the product budget for the workload.
- first-output latency is within the product budget for the workload.
- decode latency is within the product budget for the workload.
- answer-quality smoke passes.
- repeated runs do not show allocator poisoning, fallback, or runaway latency.
