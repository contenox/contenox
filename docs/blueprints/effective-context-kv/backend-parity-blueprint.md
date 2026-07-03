# Blueprint: Backend Parity Contract

Owner: modeld

Purpose: define what "parity" means between compiled-in modeld backends and the
invariants every backend must satisfy. Parity is honoring the same contract
surface with truthful capability reporting and the same certification
discipline — not implementing identical mechanisms. Where the underlying API
forbids a behavior, the backend reports the limitation; it never pretends.

## Invariants

Every compiled-in backend must satisfy all of these.

### I1. Fatal-error discipline

Unrecoverable native errors (allocator exhaustion, backend decode/prefill
failure, state-restore failure, device loss) are classified, mark the session
fatal, surface `ContextReport.FatalError`, return `ErrSessionFatal`, and cause
slot eviction. A poisoned native context is never reused. Each classified
error class has test coverage.

### I2. Capability truth under CI

The full `ResidencyCapabilities` struct a backend reports is asserted verbatim
in tests, so capability drift fails CI. A capability is reported if and only
if the backend can execute it:

- `RecomputeRange` is true for every backend that can re-prefill a range
  (all current backends can).
- `ColdStore` is true exactly when the cold store is configured and the
  export/import path is wired.
- Position/range surgery (`RemoveTail`, `RemoveMiddle`, `PositionShift`) is
  reported only where the engine physically supports it.

### I3. Actionable residency plans

A residency plan produced by the shared planner must be executable on every
backend through the capabilities that backend reports: KV surgery where
supported, recompute-based evict-and-refill otherwise. The same system test
drives the plan on all backends.

### I4. Product-path telemetry

`PrefixStatus`/`SuffixStatus` counters (reused, prefilled, dropped, suffix
tokens) and prompt/completion token usage appear in trace rows through the
`contenox` product path for every backend. Benchmark rows never depend on
fallback tokenizer counts without labeling them as fallback.

### I5. Precision certification

Every KV precision/quantization knob a backend exposes (`KVCacheType`,
flash-attention modes, backend KV dtypes) has a certification matrix: quality
smoke plus capacity-delta assertions on a certified model. Capacity math
(`KVBytesPerToken`, `ModelInfo`) reflects the quantized sizes. A knob without
certification rows is not advertised.

### I6. Symmetric optimization patterns

Cross-cutting execution patterns (for example deferred physical prefill) exist
behind per-backend opt-in flags with the same contract counters
(`DeferredPrefill*`), and stay opt-in until a product-path benchmark shows a
win for the target cell.

### I7. Gated speculation

Speculative decoding (or any decode-restructuring optimization) enters a
backend only through the specialization-cells step-function gates: materially
better end-to-end agent turn time at equal quality smoke on a certified cell.
It changes decode internals only — never the transport contract. Draft models
are explicit registry artifacts, off by default.

## Permitted Asymmetries

These are engine limits, reported truthfully, not parity failures:

| Behavior | Where supported | Elsewhere |
|---|---|---|
| KV range surgery + position shift | llama.cpp (sequence KV ops, RoPE re-shift) | Recompute-based execution (I3) |
| Engine-state snapshot (`Snapshot.State`) | llama.cpp (`StateGetData`) | Text-level snapshot + cold blocks; restore cost visible in `ExplainContext` |
| Device-level sparse prefill (XAttention) | OpenVINO | Model-native SWA only; no emulation |
| Continuous-batching scheduler | OpenVINO GenAI | Not required; single-slot product does not need it |

## Dependency Order

Safety and truth (I1, I2) precede everything: plans and benchmarks built on
untruthful capabilities are invalid. Telemetry (I4) precedes ports and
certification (I3, I5, I6) so changes are measured, not assumed. Speculation
(I7) requires the telemetry and benchmark rows to exist, because its gate is
an end-to-end measurement.

## Acceptance

The parity contract holds when:

- both backends' capability structs are pinned by tests and truthful (I2);
- a forced fatal native error on any backend evicts the slot in a system test
  (I1);
- one planner-driven residency system test passes on all backends (I3);
- trace rows show reuse counters and token usage end-to-end on all backends
  (I4);
- every advertised precision knob has certification rows (I5);
- any enabled optimization pattern has a product-path benchmark row justifying
  it (I6, I7).
