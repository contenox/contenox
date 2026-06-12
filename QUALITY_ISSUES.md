# Contenox Runtime Quality Issues

This backlog captures the runtime quality issues found from local telemetry,
ACP wire logs, and code inspection. It focuses on problems that make Contenox
feel slower, noisier, or less predictable than comparable agentic runtimes.

## P0: Unbounded State And Telemetry Payloads

Status: first slice implemented

Evidence:
- `~/.contenox/telemetry.log` had a small number of very large `State changed`
  lines accounting for most of the log size.
- Local KV state rows under `state:*` can persist full task inputs and outputs,
  including large chat histories and tool results.
- ACP/task event buses also serialize captured step state, which can duplicate
  the same large payloads in memory, SQLite, logs, and client streams.

Impact:
- Large synchronous JSON/log writes add avoidable I/O to every prompt.
- Debug state and support bundles can retain raw prompts, tool output, and
  credentials-adjacent data longer than expected.
- State inspection remains useful, but full-payload persistence should be an
  explicit debug behavior, not the default.

First slice:
- Done: Keep in-memory execution history unchanged for chain semantics.
- Done: Sanitize/cap captured step payloads before writing to KV or publishing
  over the state bus.
- Done: Cap generic tracker `change_data` values before logging.
- Done: Preserve metadata: type, size, truncation marker, task id, provider,
  model, timing, transition, error, tool names, and token usage.

Later slices:
- Add configurable debug mode for full state persistence.
- Add retention controls by byte budget, not only list length.
- Add support-bundle redaction for KV and telemetry exports.

## P1: MCP Provider Health And Retry Noise

Status: first slice implemented

Evidence:
- Stale OAuth/session failures for MCP providers dominate historical errors.
- A disconnected or unhealthy MCP provider can be retried repeatedly across
  chain runs and appear as framework instability.
- ACP-provided MCP lifecycle leakage has been fixed separately; durable MCP
  providers still need health/cooldown behavior.

Fix slices:
- Done: Add in-process cooldown for repeated worker `list-tools` failures so
  chain tool preload does not reconnect/log the same broken provider on every
  immediate retry.
- Track provider health and last failure class.
- Add durable cooldown/backoff for repeated `connect` and `list_tools`
  failures.
- Show a compact unavailable-tools prelude once per provider/failure window.
- Add `doctor` output for stale auth, missing env vars, and unreachable MCPs.

## P1: ACP Payload And Replay Bloat

Status: open

Evidence:
- ACP wire logs contained multi-megabyte `session/prompt` and message chunks.
- Session load can replay large tool outputs and old agent chunks back to the
  client.

Fix slices:
- Cap prompt block size before chain execution.
- Compact session replay for large histories.
- Represent large tool outputs as artifacts or summaries in ACP updates.

## P1: HITL And Terminal Wait Classification

Status: open

Evidence:
- Longest observed chain spans were dominated by local shell / approval waits,
  not model latency.

Fix slices:
- Separate telemetry durations into model time, tool time, and waiting-for-user.
- Add visible wait status and cancellation path for HITL/terminal work.
- Add idle timeout behavior for terminal commands and permission prompts.

## P1: OpenAI Tool Schema Compatibility

Status: open

Evidence:
- Logs showed OpenAI tool schema errors around missing names and invalid
  parameters.
- GPT-5 Responses API routing exists in current work, but schema normalization
  still needs a focused compatibility check.

Fix slices:
- Ensure all OpenAI function tool names are non-empty and valid.
- Normalize object schemas for OpenAI strict tool calling.
- Add tests for blank names, nested schemas, and `additionalProperties`.

## P2: Model And Provider UX

Status: open

Evidence:
- `no model matched requirements` and quota/provider errors are hard to act on.
- IDE-requested model names should not override Contenox controls silently.

Fix slices:
- Improve model listing with observed models, configured defaults, provider
  health, and fallback status.
- Explain resolver decisions in setup/doctor output.
- Keep Contenox default/alt model controls visible and authoritative.

## P2: Support Bundle Redaction

Status: open

Evidence:
- The local DB can contain provider keys, OAuth tokens, prompts, tool outputs,
  and captured state.

Fix slices:
- Redact known credential keys and OAuth token rows from exports.
- Redact raw prompt/tool output by default.
- Include byte counts and hashes so debugging remains possible.
