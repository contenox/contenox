# Bug Inventory — modeld review + CLI/daemon E2E (2026-07-03)

Sources: full modeld code review, then live E2E with `bin/contenox` +
`bin/modeld` (both backends, isolated `HOME=tmp/e2e-chat-20260703-221041/home`).
E2E findings were re-verified against a **baseline modeld built from clean
HEAD** (fixes stashed), so they are pre-existing at HEAD unless marked
otherwise. Continued verification logs also live under
`tmp/e2e-openvino-codex-*`.

## Open — found in E2E (remaining)

### B2. llama decode output intermittently fails the chat parser (MEDIUM-HIGH, PARTIAL)

- Symptom: a turn fails with
  `llamacppshim: common chat parse: The model produced output that does not
  match the expected peg-native format`.
- Where: `llamacppshim.ParseChatResponse` (peg-native chat format), i.e. the
  thinking-parse area last touched by commit 1ed54ba.
- Repro: first turn of session `cold-probe` (qwen3-4b, think default).
  Intermittent — other first turns parsed fine.
- Status: parser errors now include bounded raw-output diagnostics
  (`partial`, `parse_tool_calls`, `reasoning_format`, `raw_len`,
  `raw_preview`) for the next reproduction. The separate empty-assistant
  persistence defect is fixed below. The remaining parser behavior should be
  changed only after inspecting a captured raw sample.

## Open — carried from the code review (not yet fixed)

### B8. OpenVINO full native suite stability risk (MONITOR)

- Carried from note.md item 9: one full `make -f Makefile.openvino test-genai`
  run timed out in `TestSystem_OpenVINOGenAI_ContextCanceledBeforeGenerate`;
  a later full run passed. Not reproduced since.

## Fixed in the working tree (from the modeld review; verified)

1. **B1: llama/OpenVINO volatile segment boundary hard-fails**
   (`modeld/llama/llamasession/llama.go`, `modeld/openvino/session.go`) —
   per-message volatile token ranges are now advisory. When a chat template is
   not token-prefix-additive per message (observed with qwen3 tool/thinking
   templates), enrichment degrades to coarse volatile residency instead of
   failing the turn. Llama verification before the interrupted session: 5-turn
   conversation, perfect recall, 10/10 messages, zero failure records.
   OpenVINO mirror verification after resume: forced close/reopen with
   `--idle-ttl off`, 6/6 persisted messages, no failure records, `BLUE-17`
   recalled (`tmp/e2e-openvino-codex-memory-20260703/`).
2. **OpenVINO GenAI worker-thread construction race**
   (`modeld/openvino/ovsession/genai.cpp`) — `std::thread worker` was declared
   before `mu`/`cv` and started in the member initializer, so the worker could
   enter `loop()` and lock an unconstructed mutex. Reproduced under GDB as an
   uncaught native `std::system_error: Invalid argument` abort in
   `cx_genai_session::loop()` after the first OpenVINO chat turn. Fixed by
   constructing the synchronization fields before the worker. Verified by the
   same OpenVINO forced close/reopen E2E above.
3. **Embed blocked by idle-resident model** (`modeld/slot/service.go`) —
   regression from fd18f6d: `Embed` returned `ErrModelBusy` for up to the idle
   TTL after any chat turn. Fixed: idle implicit residents are evicted for the
   embed. E2E-verified live: embed via `modeldconn` succeeded (2560 dims,
   2.3s) with qwen3-4b idle-resident; chat reloaded transparently after.
4. **Streaming prefill KV desync on mid-chunk failure**
   (`llamasession/llama.go` `prefillStreamLocked`) — orphaned KV cells past the
   resident tape were presented as clean state; now rolled back and classified
   like the non-streaming paths.
5. **Enrichment-failure state desync** — `EnsurePrefix` (both backends) now
   enriches before committing state. Volatile enrichment is treated as
   non-fatal residency metadata and either applies a complete set of ranges or
   leaves coarse residency plus a diagnostic.
6. **OpenVINO fatal errors did not poison the session locally**
   (`openvino/session.go` `backendErrorLocked`) — now poisons via
   `markFatalLocked`.
7. **owner.Join tight retry loop** — 250ms ctx-cancellable backoff added.
8. **Silent config degradation** — malformed `modeld.json` and invalid
   byte-size/headroom env values now log warnings
   (`capacity.LoadPolicy`, `cmd/modeld` idle-TTL reader).
9. **B2 empty assistant persistence after failed parse**
   (`runtime/taskengine/synthesizer.go`) — failed chat-completion steps now drop
   empty assistant shell messages before persisting synthesized history, while
   still appending the failure annotation. Verified by
   `TestUnit_SynthesizeHistory_ErrorDropsEmptyAssistantShell`.
10. **B3 recovery masking bad-model errors**
    (`runtime/contenoxcli/*`, built-in chain JSON) — `model` remains the
    per-turn requested model, while recovery/safety-net tasks use
    `{{var:alt_model|var:default_model}}` and provider equivalent. The CLI now
    exposes configured defaults separately from request overrides. Covered for
    chat/run/ACP/ACPX fixtures.
11. **B4 `--context` ignored**
    (`runtime/taskengine`, `runtime/modelrepo`, `runtime/agentservice`,
    `runtime/contenoxcli`) — the request context is now carried as a resolver
    minimum and as explicit `NumCtx` for llama/OpenVINO modeld-backed providers
    when supplied. Auto-context behavior remains unchanged when no context is
    requested.
12. **B5 double-wrapped manifest mismatch errors**
    (`runtime/contextasm`, `runtime/transport/grpc`) —
    `NewManifestMismatchError` trims repeated sentinel prefixes and gRPC decode
    reconstructs a typed manifest mismatch instead of wrapping the sentinel text
    around itself.
13. **B6 silent modeld per-request failures** (`modeld/slot/service.go`) —
    slot session operations now log non-cancellation failures with operation,
    generation, backend, active model identity, state, and error class. Stale or
    closed-handle cases are debug-level; manifest, decode, overflow, and fatal
    session errors are warnings.
14. **B7 CGO cold-KV shim return handling**
    (`modeld/llama/llamacppshim/direct.go`, `llamasession`) — audited the shim:
    llama.cpp exposes `seq_cp` / `seq_add` as void, unlike `seq_rm`. The Go
    wrappers now return false when the context is invalid, and all copy/add
    call sites route through helpers that fatalize those failures instead of
    silently continuing.
15. **B2 parser failure diagnostics**
    (`modeld/llama/llamasession/llama.go`) — native common-chat parse errors
    now carry bounded raw-output context, so the next intermittent qwen3 parse
    failure has enough evidence to determine whether the PEG parser, the
    reasoning controls, or model output needs the actual behavior change.

## E2E flows verified good

- OpenVINO CPU chat: multi-turn memory, session persistence (6/6), doctor,
  status, clean shutdown with immediate lease release; resumed verification also
  covered repeated open/close with `--idle-ttl off` after the GenAI worker
  construction-order fix.
- llama CUDA chat: full GPU offload (GTX 1660), multi-turn memory
  (GREEN-42 across 4 turns), model switch qwen3-4b ↔ granite-3.2-2b in both
  directions with idle-resident eviction.
- Idle reaper (20s TTL): VRAM released (~723 MiB used after reap) and the next
  chat turn reloaded transparently.
- Backend dormancy reporting: `model list` correctly flags the openvino
  registration as dormant while modeld runs the llama engine.
- `ReclaimableBytes` confirmed not wire-exposed (trust boundary holds).
