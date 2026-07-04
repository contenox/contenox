# Bug Inventory — modeld review + CLI/daemon E2E (2026-07-03)

Sources: full modeld code review, then live E2E with `bin/contenox` +
`bin/modeld` (both backends, isolated `HOME=tmp/e2e-chat-20260703-221041/home`).
E2E findings were re-verified against a **baseline modeld built from clean

---

## Session 2026-07-04 — runtime layer + UI E2E (added)

Scope this session: the runtime/serverapi/services layer and the embedded Beam
UI, which the 2026-07-03 pass (modeld-focused) did not cover. Baseline at clean
HEAD `19da023`. Artifacts under `tmp/e2e-20260704/` (apitests temp, UI
screenshots + JSON reports, review notes).

### B9. Wildcard CORS + no CSRF/token on loopback lets any website drive the local API (MEDIUM-HIGH, CONFIRMED)

- Symptom: `contenox serve` on its default `127.0.0.1` bind (no TOKEN) answers
  cross-origin requests from arbitrary websites with
  `Access-Control-Allow-Origin: *` and no CSRF defense. Any page the user has
  open can create chats, start chat/chain runs (`POST /api/chats`,
  `POST /api/chats/{id}/chat`, `POST /api/tasks`), and read the JSON responses
  (chat history, backends, models) while the daemon runs.
- Where: `apiframework/middleware/cors.go` (`DefaultAllowedAPIOrigins = "*"`,
  set unconditionally in `runtime/contenoxcli/serve_cmd.go` `EnableCORS`) and
  `runtime/serverapi/local_security.go` (`ProtectMutatingAPI` returns `next`
  unwrapped when TOKEN is empty; TOKEN is only *required* for non-loopback
  binds via `ValidateLocalServeSecurity`, so the default loopback serve has no
  token and no Origin/Host allowlisting).
- Repro (live, this session, isolated HOME on port 32130):
  - Preflight `OPTIONS /api/chats` with `Origin: https://evil.com` →
    `200`, `Access-Control-Allow-Origin: *`, `Allow-Methods: …,POST,…`.
  - `POST /api/chats` with `Origin: https://evil.com` → `201 Created`,
    body `{"id":"cb7a0f28-…","name":"drive-by",…}`, `Allow-Origin: *`.
  - Because the response is `Allow-Origin: *` and the flow needs no
    credentials, the attacker page can also read the response body.
- Impact scales with what the active chain can do: for a chain with shell/FS
  tools enabled (`--shell`), a drive-by POST becomes local command execution.
- Note: this is a design-level gap, not a regression — the explicit model is
  "mutating requests need the bearer token; token mandatory only off-loopback."
  The gap is that CORS stays `*` and no CSRF/Origin check applies, so the
  loopback-is-safe assumption does not hold against a browser the user drives.
- Suggested direction: restrict CORS to the known UI origin(s), and/or reject
  cross-site requests by validating `Origin`/`Sec-Fetch-Site` (or requiring the
  token) even on loopback. Confirm against the transport contract before
  changing default behavior.

### Info — GET endpoints are unauthenticated even when TOKEN is set

- `isMutatingMethod` (`runtime/serverapi/local_security.go:54`) treats
  GET/HEAD/OPTIONS as non-mutating, so `ProtectMutatingAPI` never guards reads.
  When a TOKEN *is* configured (e.g. a non-loopback bind), chat history, tool
  outputs, backends, and model lists remain readable without the token.
  Combined with wildcard CORS (B9) this widens read exposure. Design decision —
  recorded here so it is a conscious one, not a latent surprise.
- Cross-check via UI: the `drive-by` chat created by the B9 cross-origin
  `POST /api/chats` later appeared in the Beam chat sidebar during the
  Playwright walk — end-to-end confirmation the injected write is real and
  persisted, not just an echoed response.

### B10. Raw SQLite/libdb constraint error leaks through the backend-create API (LOW, CONFIRMED)

- Symptom: `POST /api/backends` with a `(type, base_url)` pair that already
  exists returns `409` with the internal storage error verbatim:
  `{"error":{"message":"libdb: unique constraint violation: constraint failed:
  UNIQUE constraint failed: llm_backends.type, llm_backends.base_url (2067)",
  "type":"invalid_request_error","code":"conflict"}}`. Table/column names and
  the SQLite errno (2067) are exposed to the client and rendered in the UI.
- Where: backend-create path (`runtime/backendservice` → `libdbexec`), which
  maps the driver error to `code:conflict` but passes the raw message through
  instead of a domain-level string.
- Repro (live): create a backend `name=ui-created-backend`,
  `type=Ollama (local)`, `url=http://localhost:19999`; create a second with the
  same type+URL (any name) → `409` with the message above. Reproduced through
  the Beam "Create New Backend" form (surfaced as a console error + error text).
- Contrast: `setup-status` returns well-structured domain issues
  (`{"code":"no_chat_models","severity":"error","message":…,"fixPath":…}`), so
  this endpoint is inconsistent with the rest of the API's error surface.
- Note: the unique constraint is on `(type, base_url)`, not name — so two
  backends may not share a type+URL even under different names. That is a
  reasonable rule; only the leaked internal message is the defect.

## Session 2026-07-04 (cont.) — B2 + B8 modeld hardening

Both carried modeld items were worked this round. Neither original failure
reproduced on the available hardware/models; each got a source-grounded,
zero-downside change plus a regression guard. Full detail per entry below.
Baseline HEAD `19da023`; changes in the working tree, not committed.

### B2. llama decode output intermittently fails the chat parser (MEDIUM-HIGH, DEFENSIVE FIX APPLIED; ORIGINAL TRIGGER NOT REPRODUCED)

- Symptom: a turn fails with
  `llamacppshim: common chat parse: The model produced output that does not
  match the expected peg-native format`.
- Root cause (from the llama.cpp source, not just inferred): the streaming chat
  parser turned **any** parse error into a fatal stream abort, including
  mid-stream (`partial=true`) parses. llama.cpp's peg parser only recovers a
  partial parse when it made some progress —
  `common/chat.cpp`: `if (is_partial && result.end > 0) { return msg; }` — so a
  streamed fragment making **zero** progress (`result.end == 0`) throws "does
  not match the expected <format> format" even with `is_partial=1`. The final
  (`partial=false`) parse is the authoritative one.
- Fix (working tree, `modeld/llama/llamasession/llama.go`): in
  `chatOutputParser.Push`, a parse error while `partial==true` is now swallowed
  (no delta, state preserved, `slog.Debug` diagnostic); the accumulated output
  is re-parsed on the next piece / the final parse, which emits the full
  cumulative delta via `stringDelta`. A failure on the final `partial=false`
  parse stays fatal. No text loss.
- Tests (`chat_parser_partial_test.go`): a seam
  (`var parseChatResponse = llamacppshim.ParseChatResponse`) drives a
  deterministic GPU-free unit test that a partial failure is tolerated and the
  final parse delivers full content; a complement asserts a final-parse failure
  is still fatal; a third streams a **real captured qwen3 completion**
  rune-by-rune through the CGo parser and asserts the turn never aborts and
  content/thinking reconstruct exactly (fixtures under
  `modeld/llama/llamasession/testdata/`).
- Live GPU E2E (`TestSystem_LlamaChatParser_QwenThinkingStreamTolerated`,
  GTX 1660, qwen3-4b): 5 turns (3 reasoning + 2 tool-calling) stream cleanly,
  no parse abort, content/thinking/tool-calls intact.
- Honest caveat: the **original intermittent peg-native throw was not
  reproduced**. Across the live capture and exhaustive GPU-free probes (prefix
  truncations at every rune, leading-mismatch fragments, empty/special-token
  inputs, tool-call fragments), qwen3-4b's peg-native grammar at the current
  llama.cpp pin is fully lenient — it always makes progress, so `result.end`
  never hit 0 and the parser never hard-failed on partial. The fix closes the
  documented `result.end==0` throw path (which stricter grammars/models can hit)
  and is correct and zero-downside, but qwen3-4b does not currently exercise it.
  The `raw_preview` diagnostics remain for the next occurrence.

### B8. OpenVINO full native suite stability risk (MONITOR — stress guard added; not reproduced)

- Carried from note.md item 9: one full `make -f Makefile.openvino test-genai`
  run timed out in `TestSystem_OpenVINOGenAI_ContextCanceledBeforeGenerate`;
  a later full run passed.
- Reproduction attempt (per "reproduce-first"): added
  `TestSystem_OpenVINOGenAI_CancelRaceStress` — 200 iterations each of a
  pre-canceled `Generate` (must return `context.Canceled` promptly) and a
  `Stream` canceled right at dispatch (must stop well under budget), each under
  a 20s per-iteration watchdog that fails on a hang/full-budget run. It **passed
  clean** on the CPU int4 model, so B8 did not reproduce at HEAD. The flaky
  test's pre-cancel path is already guarded (`genai.go` checks `ctx.Err()` at
  entry) and the earlier worker-thread construction race (a likely original
  cause) was already fixed (item 2 below).
- Latent race identified in review (documented, not the confirmed cause):
  `cx_genai_generate` resets `cancel_requested.store(false)` at generation start
  (`genai.cpp`), which can clobber a concurrent `cx_genai_session_cancel` set by
  the Go cancel goroutine, running the full budget. Applied the **safe Go-side
  belt only** — `genai.go` re-checks `ctx.Err()` after arming the cancel watcher
  and skips the native call when already canceled — tightening the pre-cancel
  window. The riskier native reset-race change is **intentionally deferred**
  until the flake actually reproduces (unverified native concurrency edits are
  not worth the risk); the stress test is the guard that will catch it.

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

### Added 2026-07-04 (runtime + UI)

- Go unit baseline: `make test-unit` green at HEAD `19da023` (56 packages `ok`,
  no failures).
- API E2E: `make test-api` / `scripts/run_apitests.sh` — 17 passed, 1 skipped
  (backends, health, HITL policies, MCP servers, model registry, task chains,
  tools) against a real `contenox serve` on an isolated HOME.
- UI E2E (Playwright driving the bundled Chrome, isolated HOME, port 32130).
  The Beam UI uses a **HashRouter** (`App.tsx`), so real routes are `/#/…`; an
  early path-based pass (`/backends`, …) only hit the SPA index fallback and
  must not be trusted. Onboarding is gated client-side by
  `localStorage['beam_onboarding_dismissed']` (`components/Layout.tsx`), so the
  wizard overrides every route until dismissed. Re-walked correctly (bypass
  onboarding via the localStorage flag, then visit hash routes):
  - Onboarding wizard: all 4 steps render, both local backends (llama,
    openvino) report reachable, Back clamps at step 1, Skip/apps-menu work.
  - `/#/chat` + new-chat creation + chat view render; `/#/backends` shows the
    backend cards (Edit/Delete) with Manage/Cloud Providers/modeld/Runtime-state
    tabs and a Create form; `/#/settings` shows runtime + workspace defaults;
    `/#/control` renders; `/#/nonexistent` → "Page not found" (proper SPA 404).
  - Create Backend: empty submit is blocked client-side (no API call);
    a valid create persists across reload; `cli-config` PUT persists
    (`default-model` reflected in `setup-status`).
  - No default model → composer correctly disables Send ("Send is disabled
    until the selected default provider has a chat-capable model"); adversarial
    composer input (`<img onerror>` + long text) stays inert while disabled.
  - Zero console errors, zero warnings, zero unexpected network failures, zero
    i18n key leaks across all real routes.
  - Minor: navigating to an unknown chat id (`/#/chat/<random-uuid>`) renders an
    empty "ready to execute" chat shell rather than a not-found state; harmless
    (the history GET returns empty, no error), noted for polish.
- Manual review of `runtime/serverapi` + `runtime/internal/internalchatapi`
  security/transport surface found the token comparison constant-time
  (`subtle.ConstantTimeCompare`), error responses routed through
  `apiframework.Error`, and path/query params validated before use. The one
  gap surfaced is B9 / the GET-exposure note above.
