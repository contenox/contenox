# Blueprint: Benchmark Integrity and Reproducibility

Owner: runtime / modeld

Purpose: prevent backend probes, failed runs, and packaging issues from becoming
product performance claims.

## Core Rule

Headline benchmark numbers come from the product path:

```text
contenox-runtime binary -> modeld launcher -> backend adapter -> selected runtime/device
```

Raw backend scripts are controls. They are never product results.

## Result Classes

| Class | Allowed use | Required label |
|---|---|---|
| Product benchmark | User-facing performance and certification. | `contenox_product_path` |
| Raw substrate control | Backend/runtime sanity check and lower-level comparison. | `raw_backend_control` |
| Microbenchmark | Kernel, tokenizer, loader, or transport isolation. | `microbenchmark` |
| Failed run | Debugging environment, packaging, or runtime failures. | `failed_no_performance_claim` |

Every row records one class. Mixed classes are not comparable unless explicitly
normalized and labeled.

## Facts Encoded

### Raw backend probes

Raw Python backend probes (for example `openvino_genai` scripts) bypass:

- `contenox` CLI behavior.
- modeld startup and launcher behavior.
- backend selection and profile identity.
- transport/session bookkeeping.
- trace token accounting.
- agentic chain/tool execution.

Required behavior:

- Keep raw rows only as substrate controls.
- Do not compare raw rows to product rows unless model, prompt, context, output, device,
  profile, and warm/cold state are matched.
- Never use raw generate-only throughput as user-facing agentic throughput.

### Windows launcher and app-control preflight

Two independent Windows facts:

- `modeld.exe` can fail with `0xC0000135` when launched directly because DLL paths are
  set by `modeld.cmd`.
- Windows CodeIntegrity/WDAC/Smart App Control can block `contenox.exe` depending on
  local app-control state.

Required behavior:

- Bench scripts on Windows use the packaged modeld launcher.
- Preflight verifies `contenox version` and `modeld.cmd version`.
- Preflight records CodeIntegrity/WDAC/SAC status when available.
- App-control blocks are environment failures, not zero-token benchmark rows.

### Same-model rule

Cross-model numbers do not answer whether an implementation is broken.

Required behavior:

- Comparisons require the same model repository, digest, quantization, prompt set,
  context, output-token cap, device, runtime profile, and warm/cold state.
- If any field differs, the report says "not comparable" and explains the difference.

### Warm-state rule

Cold prompt throughput and warm agentic-turn throughput answer different questions.

Required behavior:

- Cold rows state whether they include model load, session open, full stable-prefix prefill,
  suffix prefill, and decode.
- Warm rows must keep the same runtime process and same modeld session alive unless explicitly
  labeled as cross-process.
- Warm rows record `PrefixStatus` and `SuffixStatus`: reused, prefilled, dropped, prefix,
  suffix, resident, and available token counts.
- Long-lived product surfaces (`serve`, editor/ACP/VS Code agents) and repeated one-shot CLI
  calls are separate benchmark classes.
- Reusable context must be identified as stable prefix. A large user-turn payload is volatile
  unless the chain/runtime explicitly maps it to stable context.

## Required Benchmark Artifact

Every run directory contains:

- command line and executable paths.
- `contenox` version.
- `modeld` version, backend list, backend runtime version, and backend commit/digest.
- OS, driver, accelerator model, memory capacity, bandwidth when known, bus, and
  interconnect/topology.
- model repo, local path, digest, quantization, tokenizer identity, and model profile.
- backend profile: device, KV precision, sparse/XAttention, cache size, eviction,
  prefix cache, scheduler options, and env vars.
- prompt file, prompt token count, requested context, accepted context, output cap.
- raw trace rows and parsed metrics.
- stdout/stderr logs.
- quality-smoke result.
- result class.

## Required Metrics

Product rows record:

- load time separately from warm-session time.
- session-open time separately from turn time.
- `EnsurePrefix`, `PrefillSuffix`, and decode timings when the backend exposes them.
- prefix/suffix reuse counters when the backend exposes them.
- wall time.
- time to first token or closest first-output measure.
- time per output token or decode-token measure.
- prompt tokens and completion tokens from runtime trace when available.
- fallback tokenizer counts when trace usage is missing, labeled as fallback.
- p50, p90, and p95 across repeated runs.
- timeout, context overflow, unsupported feature, fallback, and fatal backend errors.
- answer-quality smoke.

## Workflow

1. Run preflight.
2. Resolve model and write immutable run manifest.
3. Start modeld through the packaged launcher.
4. Run `contenox` workloads with trace enabled.
5. Parse metrics from product logs.
6. Run raw backend controls only after product rows are captured or explicitly mark the
   product path as blocked.
7. Compare only rows with matching model and workload identity.
8. Publish failed runs separately from performance rows.

## Preflight

Minimum Windows preflight:

```text
contenox.exe version
modeld.cmd version
PowerShell execution policy
CodeIntegrity/WDAC/SAC state when available
modeld DLL path/launcher verification
OpenVINO device list
selected backend profile
```

Minimum Linux preflight:

```text
contenox version
modeld version
ldd or runtime library path check for packaged modeld
driver/runtime version
device list
selected backend profile
```

## Reporting Rules

- A zero-token run caused by process block, launcher failure, missing DLL, context
  overflow, or unsupported feature is a failed run, not throughput data.
- A raw backend row can be shown next to product rows only when labeled as a control.
- Context claims must include accepted runtime context and certified context tier.
- Throughput claims require quality-smoke status.
- Agentic-workflow claims require the `contenox` chain/tool/chat path, not standalone
  backend generation.
- Warm agentic-workflow claims require a long-lived runtime process or a measured persisted
  snapshot/restore path. Replaying chat history through one-shot CLI calls is a separate result.

## Acceptance

A benchmark report is acceptable only when:

- every row has a result class.
- every product claim has a product-path artifact.
- every comparison satisfies the same-model rule or is labeled not comparable.
- environment blockers are separated from model performance.
- raw controls cannot be mistaken for product results.
