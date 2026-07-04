# Bug Inventory

Generated during local modeld stress testing on 2026-07-04.

Scope:
- Stateful multi-turn stress with idle active/idle alternation.
- Multiple llama model families, using installed and curated-pulled models.
- OpenVINO CPU-only stress using curated-pulled models where available.

Planned model coverage:
- llama: `qwen3-4b`, `granite-3.2-2b`, `qwen2.5-coder-0.5b`, `phi-4-mini`.
- OpenVINO CPU: `qwen2.5-coder-0.5b-ov`, `tinyllama-1.1b-chat-v1.0-int4-ov`.

Environment notes:
- Curated registry pulls completed for `qwen2.5-coder-0.5b`, `phi-4-mini`, `qwen2.5-coder-0.5b-ov`, and `tinyllama-1.1b-chat-v1.0-int4-ov`.
- Local model inventory now has four llama models and two OpenVINO IR models installed.
- Stress runs use isolated data roots and explicit `contenox chat` subcommands to exercise persisted sessions.

## Run Log

- Interrupted: first llama-family stress attempt stopped after harness preflight exposed B-001.
- Completed: llama-family multi-turn stress with temp `default-model` configured.
  - `qwen3-4b`: 5/5 turns exited 0; active recall, idle recall, tool echo, and post-tool idle recall all contained the expected marker.
  - `granite-3.2-2b`: 5/5 turns exited 0; recall turns contained the expected marker. Tool-pressure turn exited 0 but did not execute/echo the requested marker.
  - `qwen2.5-coder-0.5b`: 5/5 turns exited 0; active recall, idle recall, tool echo, and post-tool idle recall all contained the expected marker.
  - `phi-4-mini`: first two turns independently failed with `contextasm: context manifest mismatch: model template is not prefix-stable across the suffix`; later turns were contaminated by manual interruption/shutdown and are not counted as independent evidence.
- Completed: OpenVINO CPU-only stress with curated-pulled IR models.
  - `qwen2.5-coder-0.5b-ov`: 4/4 CLI turns exited 0 and recall markers were present, but the tool-pressure turn persisted failed internal steps from B-003.
  - `tinyllama-1.1b-chat-v1.0-int4-ov`: seed and active recall exited 0 with marker present; idle recall exited 0 but produced the wrong marker; tool-pressure turn failed because tool schemas pushed required context above the model's 2048-token limit, then hit B-003 during failure summarization.
  - CPU-only confirmed by modeld log: `CONTENOX_OPENVINO_DEVICE="CPU"`; GPU memory stayed at baseline after the run.

## Open

- B-002: `phi-4-mini` via llama/modeld cannot complete even the first chat turn in this stress path. `classify_request`, fallback chat, recovery, and failure-summary attempts all fail with `contextasm: context manifest mismatch: model template is not prefix-stable across the suffix`; modeld logs `class=manifest_mismatch` for `prefill_suffix`.

## Watching

- W-001: Previous interrupted stress accidentally invoked the default stateless `run` path instead of explicit `chat`, so the model did not receive prior turns. Not a product bug unless reproduced through `contenox chat`.
- W-002: qwen-family models may choose tool calls even when the prompt asks for a direct marker. This is model behavior unless it causes parser/session failures.
- W-003: `granite-3.2-2b` completed session recall across idle gaps, but on the shell echo pressure turn it refused/garbled the tool request instead of calling `local_shell`. Runtime exited cleanly, so this is currently model compliance rather than infrastructure failure.
- W-004: `tinyllama-1.1b-chat-v1.0-int4-ov` lost the exact marker on idle recall even though the session history contained prior turns. Runtime exited 0; this is currently model quality unless a history assembly issue is found.
- W-005: OpenVINO CPU context overflow messages say "free VRAM" even when `CONTENOX_OPENVINO_DEVICE=CPU`; the condition is real, but the remediation text is misleading for CPU-only runs.

## Closed / Already Fixed In HEAD

- B-004: Tool-enabled chat that selects a small-context model and then overflows once tool schemas are added no longer fails with the opaque `no model matched the requirements`. Decision (see plan): surface a clear, actionable error rather than silently trimming/disabling tools or force-switching models. Fix: `llmresolver.filterCandidates` now detects the context-only shortfall (capable, name-matched models exist but every one advertises less context than the request needs) and returns a focused message naming the required tokens vs the largest available model's context, plus a remedy ("use a larger-context model or reduce the request size (fewer tools or shorter history)"). The error still wraps `ErrNoSatisfactoryModel` so `errors.Is` checks in `llmrepo` are unchanged; non-context no-matches keep the existing generic diagnostic listing. The pre-existing `isRecoverableToolSurfaceError` no-tool retry path was deliberately left untouched (the shortfall is reported, not silently recovered).
- B-003: OpenVINO effective-context sizing no longer produces a cache size that is not divisible by the GenAI block size. `residency.DeriveEvictionBudget` left `MaxTokens = windowTokens` unaligned on its main path while aligning sink/recent; a non-32-aligned served window (e.g. a trimmed/effective context like 4940) therefore reached OpenVINO's `CacheEvictionConfig.max_cache_size` unaligned and was rejected (`must be a multiple of block size 32`), marking the session fatal. Fix: align `MaxTokens` down to `blockSize` (new `alignDown` helper), keeping the hot budget within the served window while satisfying the block constraint; the existing sink+recent floor guard still holds. The early-return path is unaffected (`Valid()` is false there, so it never reaches OpenVINO's eviction config). Both `DeriveEvictionBudget` call sites use `openvinoEvictionBlock = 32`.
- B-001: `contenox chat/run --model <name>` no longer fails setup preflight in a fresh DB when `default-model` is unset. An explicit `--model`/`--provider` flag is now credited as an effective default for readiness evaluation even when it was never persisted to KV config, so preflight only blocks when the default is genuinely absent. Fix: `setupcheck.OverlayEffectiveDefaults` (pure overlay that fills empty defaults and clears the matching `missing_default_*` issue) wired into `enginesvc.Build` (both the snapshot and the live `SetupStatus` recompute) via `Config.ReadinessDefaultModel/Provider`, populated by `contenoxcli.readinessDefaults` from the flag-vs-config precedence. The hardcoded model fallback (`qwen2.5:7b`) is not credited, so a no-flag fresh DB still blocks. Model/provider availability is still validated at resolution time.
- C-001: qwen `<tool_call>...</tool_call>` output from llama common-chat parsing now falls back to the shared qwen tag parser.
- C-002: modeld accelerator residency is guarded across data roots by a physical device lease.
- C-003: `--min-hot-context 0` now disables the hot-context floor instead of being treated as unset.
- C-004: Open/Describe capacity flow no longer pins auto-resolved runtime context/GPU layers back into future runtime requests.
