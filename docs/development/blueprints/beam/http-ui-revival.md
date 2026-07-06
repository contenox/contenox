# Plan: Revive HTTP API and Beam UI

## Goal

Restore the OSS HTTP layer and Beam UI that currently live in
`/home/naro/src/github.com/contenox/enterprise/oss-backup2`, without regressing
the current runtime CLI, engine, ACP, reasoning, HITL, or model-provider work.

The migration source is `oss-backup2`, not the old pre-trim OSS commit. The
backup is closer to the current architecture: it already has `contenox serve`,
ACP dual mode, Beam SPA serving, auth, file/VFS-style endpoints, HITL approvals,
terminal sessions, provider setup, task events, MCP lifecycle, and chat routes.

## Current divergence snapshot

Checked against current OSS runtime:

- Current module path: `github.com/contenox/runtime`
- Backup module path: `github.com/contenox/contenox`
- Current Go package count: 56
- Backup Go package count: 81, excluding `node_modules`
- Filtered source comparison, excluding `.git`, `node_modules`, `dist`, `bin`,
  and `tmp`:
  - 261 common files
  - 181 current-only files
  - 561 backup-only files
  - 193 common files differ
- Backup UI source count: 371 files under `packages/beam` and `packages/ui`,
  excluding `node_modules` and `dist`
- Backup route registrations: about 99 `net/http` route registrations

Current OSS removed these HTTP/UI pieces:

- `apiframework`
- `runtime/serverapi`
- `runtime/internal/*api`
- `runtime/internal/auth`
- `runtime/internal/vault`
- `runtime/internal/web`
- `runtime/providerservice`
- `runtime/taskchainservice`
- `runtime/terminalservice`
- `runtime/terminalstore`
- `runtime/vfsservice`
- `runtime/vfsstore`
- `packages/beam`
- `packages/ui`
- `apitests`
- OpenAPI generator/docs assets

Current OSS has also moved ahead in core runtime APIs:

- `hitlservice` now uses `PolicySource`, not `vfsservice.Service`
- `enginesvc.Config` has tenant/policy fields and live `SetupStatus`
- `runtime/internal/modelrepo`, `runtime/internal/llmrepo`,
  `runtime/internal/ollamatokenizer`, and `runtime/internal/runtimestate`
  moved to public runtime packages
- `contenoxcli.BuildEngine` no longer takes a VFS argument
- CLI has newer `setup`, `cache`, `acpx`, model capability, reasoning, and
  thinking support that must not be overwritten

## Migration rules

1. Port, do not bulk overwrite current runtime code.
2. Rewrite imports from `github.com/contenox/contenox` to
   `github.com/contenox/runtime`.
3. Restore generated or dependency-heavy assets only when the phase needs them.
   Do not restore `node_modules`.
4. Keep every phase buildable before starting the next phase.
5. Prefer additive packages until the HTTP surface compiles; avoid changing core
   engine behavior unless a restored route cannot work otherwise.
6. Keep `oss-backup2` read-only. It is the reference snapshot.
7. Treat OSS `serve` as a trusted local server / local LLM gateway by default:
   no backup vault, no multi-user account stack, and no VFS abstraction unless a
   later product requirement makes one necessary.

## Execution status

- Phase 0 complete: baseline and restore manifest created.
- Phase 1 complete: `apiframework` plus health/version server foundation
  restored; `go test ./...` passed.
- Phase 2 complete: `contenox serve` skeleton restored with CORS/request-id,
  root health/version, and CLI command registration; `go test ./...` and live
  health/version smoke passed.
- Phase 3 complete: core API route packages restored under `/api`, wired to the
  current SQLite DB, bus, runtime state, tools provider, MCP worker lifecycle,
  setup status, and embedded OpenAPI placeholder.
- Phase 4 complete: local token gating and provider setup API restored without
  backup vault/account dependencies.
- Phase 5 complete: local filesystem, taskchain, and HITL policy APIs restored
  without VFS; `go test ./...` and live local smoke passed.
- Phase 6 complete: HTTP chat, request-scoped task-event SSE, and approval
  response API restored against current agent/chat/HITL services.
- Phase 7 complete: terminal session metadata store, PTY service, websocket
  bridge, route mounting, local `serve` default enablement, and idle reaper
  lifecycle restored for the local HTTP server.
- Phase 8 skipped by product decision: `serve` remains a local HTTP/UI server,
  not a dual HTTP + ACP stdio mode. Existing `contenox acp` and `contenox acpx`
  commands remain the ACP entry points.
- Phase 9 complete: Beam and `@contenox/ui` source restored, adapted to the
  current local HTTP API, built into an embedded SPA, and mounted by
  `contenox serve` at `/`.
- Phase 9.1 complete: setup readiness now uses one shared runtime state between
  engine, doctor-equivalent checks, and HTTP; Beam refresh runs a real backend
  reconciliation; terminal routes and the `local_shell` tool are enabled by
  default on loopback `serve`.
- Phase 10 complete: API smoke tests restored selectively under `apitests`,
  wired to `make test-api`, and run against an isolated temporary
  HOME/workspace/DB with no real provider credentials required by default.

## Phase 0: Baseline and restore map

### Work

- Capture current git status and leave unrelated user changes alone.
- Create a restore manifest listing every backup package/file to be copied or
  adapted.
- Classify each backup package into one of:
  - direct restore with import rewrite
  - restore with current-runtime API adaptation
  - defer until UI phase
  - obsolete because current runtime replaced it
- Decide exact endpoint groups for the first HTTP milestone.

### Validation checkpoints

- `git status --short` shows only expected documentation or migration work.
- `go list ./...` still succeeds before code changes.
- Restore manifest identifies the owner phase for all of these backup paths:
  - `apiframework`
  - `runtime/serverapi`
  - `runtime/internal/*api`
  - `runtime/providerservice`
  - `runtime/taskchainservice`
  - `runtime/terminalservice`
  - `runtime/vfsservice`
  - `runtime/internal/web`
  - `packages/beam`
  - `packages/ui`
- No source files are copied during this phase.

## Phase 1: HTTP foundation, no product routes

### Work

- Restore `apiframework` with the module import rewrite.
- Restore `runtime/serverapi` only enough to expose:
  - `GET /health`
  - `GET /version`
  - a not-found root handler for API muxes
- Use `runtime/version` for version data. Do not resurrect
  `apiframework/version.go` as a second version source.
- Add a small `serverapi.Config` and `LoadConfig` compatible with the backup
  `serve` command, but keep unused fields harmless.
- Add minimal unit tests for health/version handlers.

### Validation checkpoints

- `go test ./apiframework ./runtime/serverapi`
- `go test ./...`
- A local `httptest` verifies:
  - `GET /health` returns success JSON
  - `GET /version` includes runtime version, node id, and tenancy
  - unknown API root returns the framework not-found error shape
- No CLI command is registered yet.

## Phase 2: `contenox serve` skeleton

### Work

- Port `runtime/contenoxcli/serve_cmd.go` as a skeleton.
- Register `serve` in current `rootCmd` without dropping newer commands:
  `setup`, `cache`, `acpx`, model capability, reasoning, and current ACP logic
  must stay intact.
- Add `serve` to `reservedSubcommands`.
- Start only a plain HTTP mux with health/version and CORS/request-id
  middleware.
- Bind `127.0.0.1:32123` by default, with `ADDR` and `PORT` overrides.
- Defer auth, API route groups, ACP dual mode, and UI serving to later phases.

### Validation checkpoints

- `go test ./runtime/contenoxcli ./runtime/serverapi ./apiframework`
- `go test ./...`
- `go run ./cmd/contenox serve` starts and prints the listening URL.
- `curl http://127.0.0.1:32123/health` succeeds.
- `contenox --help` includes `serve`.
- Existing default command injection still works:
  - `contenox --help` shows root help
  - `contenox chat --help` works
  - `contenox run --help` works
  - arbitrary prompt input still maps to `run`

## Phase 3: Core API route groups

### Work

Restore route packages that mostly target current existing services:

- `runtime/internal/backendapi`
- `runtime/internal/modelregistryapi`
- `runtime/internal/setupapi`
- `runtime/internal/toolsapi`
- `runtime/internal/mcpserverapi`
- `runtime/internal/taskeventsapi`
- `runtime/internal/openapidocs`

Adapt imports and service references:

- Use current `runtime/runtimestate`, not backup `runtime/internal/runtimestate`.
- Use current `runtime/modelrepo`, `runtime/llmrepo`, and
  `runtime/ollamatokenizer` package locations where needed.
- Keep current `tools.NewPersistentRepo` signature with tracker support.
- Keep current `stateservice.New(state, db, workspaceID)` signature.

Wire these groups into `serverapi.New`, behind `/api` in `serve`.

### Validation checkpoints

- `go test ./runtime/internal/backendapi ./runtime/internal/modelregistryapi ./runtime/internal/setupapi ./runtime/internal/toolsapi ./runtime/internal/mcpserverapi ./runtime/internal/taskeventsapi`
- `go test ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- `contenox serve` exposes, at minimum:
  - `GET /api/state`
  - `GET /api/models`
  - `GET /api/model-registry`
  - `GET /api/tools/local`
  - `GET /api/mcp-servers`
  - `GET /api/setup-status`
  - `GET /api/openapi.json`
- API responses use current runtime state and do not panic on an empty database.

### Validation results

Completed in Phase 3:

- `go test ./runtime/internal/backendapi ./runtime/internal/modelregistryapi ./runtime/internal/setupapi ./runtime/internal/toolsapi ./runtime/internal/mcpserverapi ./runtime/internal/taskeventsapi`
- `go test ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- Built `/tmp/contenox-phase3-smoke` from `./cmd/contenox`.
- Live smoke on an isolated data directory verified:
  - `GET /health`
  - `GET /version`
  - `GET /api/state`
  - `GET /api/models`
  - `GET /api/model-registry`
  - `GET /api/tools/local`
  - `GET /api/mcp-servers`
  - `GET /api/setup-status`
  - `GET /api/openapi.json`

## Phase 4: Local security and provider setup

### Work

- Treat `contenox serve` as a trusted local process by default. The primary
  safety boundary is loopback binding (`127.0.0.1`) plus explicit user intent
  when changing `ADDR`.
- Do not restore `runtime/internal/vault` in this phase.
- Do not restore the backup multi-user account/JWT setup by default. Revisit
  only if Beam needs browser-session state beyond what local development needs.
- Add an optional local API token gate:
  - no token required when `ADDR` is loopback and `TOKEN` is unset
  - if `TOKEN` is set, require bearer auth for mutating `/api` routes
  - if `ADDR` is non-loopback, require `TOKEN` or fail startup
  - health/version stay unauthenticated
- Restore/adapt provider setup routes only where they improve the local setup
  UX.
- Provider setup must use current runtime concepts:
  - current backend/model schema
  - current CLI config keys
  - `api-key-env` references as the preferred secret path
  - existing backend records for local persistence when the user explicitly
    enters a literal key
- If durable encrypted secret storage becomes necessary later, design a small
  local secret-store phase separately. Do not pull in the backup vault as a
  prerequisite for the local gateway.

### Validation checkpoints

- `go test ./runtime/internal/providerapi ./runtime/providerservice`
- `go test ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- Fresh local DB flow:
  - `GET /api/setup-status`
  - provider configure/status/list endpoints round trip against current backend
    and CLI config records
  - configured provider appears in `GET /api/backends` or equivalent current
    setup surface
- Loopback behavior:
  - no token required by default for local read routes
  - `TOKEN` set causes protected `/api` calls without bearer auth to return
    `401`
  - non-loopback `ADDR` without `TOKEN` fails startup with a clear error
- Health/version remain reachable without auth.
- Provider configure/delete round trips do not corrupt existing CLI backend
  records.

### Validation results

Completed in Phase 4:

- `go test ./runtime/providerservice ./runtime/internal/providerapi ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- Built `/tmp/contenox-phase4-smoke` from `./cmd/contenox`.
- Startup guard verified:
  - `ADDR=0.0.0.0` without `TOKEN` fails before listening.
- Live smoke on an isolated data directory with `TOKEN=secret` verified:
  - unauthenticated `GET /api/providers/openai/status` succeeds
  - unauthenticated `POST /api/providers/ollama/configure` returns `401`
  - authenticated `POST /api/providers/ollama/configure` succeeds
  - `GET /api/providers/configs` shows configured Ollama
  - `GET /api/backends` shows the configured backend
  - `GET /api/openapi.json` includes provider paths

## Phase 5: Local file, taskchain, and HITL policy APIs

### Work

- Add `runtime/localfileservice` as a direct local filesystem service rooted at
  the workspace directory. It must reject absolute paths, traversal, NUL bytes,
  and symlink escapes.
- Add `runtime/taskchainservice` backed by `localfileservice`, rooted at the
  workspace `.contenox` directory.
- Restore/adapt these route packages:
  - `runtime/internal/localfileapi`
  - `runtime/internal/taskchainapi`
  - `runtime/internal/hitlpolicyapi`
- Keep HITL policy core behavior on the current `hitlservice.PolicySource`.
  The HTTP route only reads and writes policy JSON files under `.contenox`.
- Wire `contenox serve` so project file routes point at the workspace root and
  taskchain/policy routes point at `.contenox`.
- Do not restore `runtime/vfsservice`, `runtime/vfsstore`, or
  `runtime/internal/vfsapi` for the local-only OSS server.

### Validation checkpoints

- `go test ./runtime/localfileservice ./runtime/taskchainservice ./runtime/internal/localfileapi ./runtime/internal/taskchainapi ./runtime/internal/hitlpolicyapi ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- File API smoke:
  - create folder
  - upload/create file
  - list files
  - get metadata
  - update file
  - rename/move file
  - download file
  - delete file
- Taskchain API smoke:
  - list taskchains from `.contenox`
  - create a chain
  - update it
  - fetch it
  - delete it
- HITL policy API smoke:
  - list policies
  - create/update/get/delete a policy
- CLI HITL behavior remains unchanged for `contenox run` and `contenox chat`.

### Validation results

- `go test ./runtime/localfileservice ./runtime/taskchainservice ./runtime/internal/localfileapi ./runtime/internal/taskchainapi ./runtime/internal/hitlpolicyapi ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- Built `/tmp/contenox-phase5-smoke` from `./cmd/contenox`.
- Live smoke on an isolated workspace with `TOKEN=secret` verified:
  - create folder, create file, list directory, get metadata, update content,
    move file, download file, delete file, and delete folder
  - unauthenticated mutating file request returns `401`
  - create/update/list/fetch/delete taskchain from `.contenox`
  - create/update/list/fetch/delete HITL policy from `.contenox`
  - `GET /api/openapi.json` includes file, taskchain, and HITL policy paths

## Phase 6: Chat, approvals, and task-event streaming

### Work

- Restore `runtime/internal/internalchatapi`.
- Restore `runtime/internal/approvalapi`.
- Wire HTTP chat through current `agentservice`, `chatservice.Manager`, and
  `enginesvc.Build`.
- Use current `hitlservice.New`/`NewWithDefaultPolicy` with filesystem-backed
  `PolicySource`.
- Ensure HTTP chat can run with:
  - default model/provider from CLI config
  - default chain ref from `.contenox`
  - task events published through the same bus as CLI runs
- Ensure approval events can be answered through `/api/approvals/{approvalId}`.

### Validation checkpoints

- `go test ./runtime/internal/internalchatapi ./runtime/internal/approvalapi ./runtime/chatservice ./runtime/agentservice`
- `go test ./runtime/taskengine ./runtime/hitlservice`
- `go test ./...`
- With a fake/mock provider or existing local backend:
  - create chat session
  - send message
  - read chat history
  - stream task events
  - trigger a HITL approval
  - approve/deny via API
	- observe the task continue or fail as expected
- Existing CLI chat/session tests still pass.

### Validation results

- `go test ./runtime/internal/internalchatapi ./runtime/internal/approvalapi ./runtime/chatservice ./runtime/agentservice ./runtime/taskengine ./runtime/hitlservice ./runtime/serverapi ./runtime/contenoxcli`
- New route tests cover:
  - create/list chat sessions
  - send a chat request through a fake agent and chain service
  - read persisted chat history from SQLite
  - resolve a real pending `hitlservice` approval through
    `POST /approvals/{approvalId}`
- Built `/tmp/contenox-phase6-smoke` from `./cmd/contenox`.
- Live smoke on an isolated workspace with `TOKEN=secret` and a no-op
  `.contenox/default-chain.json` verified:
  - `POST /api/chats` creates a session
  - `GET /api/chats` lists the session
  - `POST /api/chats/{id}/chat` executes the default chain without provider
    credentials
  - `GET /api/chats/{id}` returns persisted history
  - `GET /api/task-events?requestId=...` streams request-scoped chain/step
    events when the chat request uses the same `X-Request-ID`
  - `POST /api/approvals/not-there` returns `404`
  - unauthenticated mutating chat request returns `401`
  - `GET /api/openapi.json` includes chat, approval, and task-event paths

## Phase 7: Terminal service and websocket route

### Work

- Restore `runtime/terminalstore`.
- Restore `runtime/terminalservice`.
- Restore `runtime/internal/terminalapi`.
- Enable terminal routes by default for local `serve`.
- Keep `TERMINAL_ENABLED=false` as the explicit opt-out.
- Require terminal tokens only when `serve` itself is token-protected; non-loopback
  binds still fail startup without `TOKEN`.
- Default `TERMINAL_ALLOWED_ROOT` to the workspace root when terminal is enabled
  from `serve`.
- Preserve cross-platform files:
  - Unix create/attach/resize
  - Windows create/attach/resize
- Re-enable idle reaping loop only when terminal is enabled.

### Validation checkpoints

- `go test ./runtime/terminalstore ./runtime/terminalservice ./runtime/internal/terminalapi`
- `go test ./...`
- Default `contenox serve` on loopback:
  - terminal routes are registered
  - idle reaper loop runs
- `TERMINAL_ENABLED=false contenox serve`:
  - terminal routes are not registered
  - no idle reaper loop runs
- `TERMINAL_ALLOWED_ROOT=$(pwd) contenox serve`:
  - create session
  - attach websocket
  - send input
  - resize
  - delete/close session
- Attempting to start a terminal outside allowed root is rejected.

### Validation results

- `go test ./runtime/terminalstore ./runtime/terminalservice ./runtime/internal/terminalapi ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- Built `/tmp/contenox-phase7-smoke` from `./cmd/contenox`.
- Live smoke on an isolated workspace with `TOKEN=secret` verified:
  - `POST /api/terminal/sessions` creates a PTY-backed shell session.
  - `GET /api/terminal/sessions` lists the active session.
  - `GET /api/terminal/sessions/{id}` returns metadata.
  - `PATCH /api/terminal/sessions/{id}` updates persisted geometry and resizes
    the local PTY.
  - `GET /api/terminal/sessions/{id}/ws` attaches to the shell; sending a
    command over websocket returned the expected marker output.
  - Websocket attach accepts `?token=...`, which browser clients can supply
    when `TOKEN` is configured.
  - `DELETE /api/terminal/sessions/{id}` closes the session.
  - Attempting to create a session with `cwd` outside the workspace root returns
    `400`.
  - Unauthenticated terminal requests return `401` when `TOKEN` is set.
  - Non-loopback `ADDR` without `TOKEN` fails startup before listening.
  - `GET /api/openapi.json` includes terminal paths and version `phase-7`.
- Live smoke with terminal disabled verified:
  - `GET /api/terminal/sessions` returns `404`.

## Phase 8: ACP dual mode in `serve` (skipped)

### Work

- Do not port backup `serve --acp` behavior.
- Keep current ACP command behavior intact; do not copy older ACP code over it.
- Keep `serve` focused on local HTTP API and, later, the local UI.
- Treat Beam/UI as a browser client of the local HTTP server, not as an ACP
  dual-mode desktop launch path.

### Validation checkpoints

- `go test ./runtime/acpsvc ./runtime/contenoxcli ./runtime/serverapi`
- `go test ./...`
- Existing `contenox acp` and `contenox acpx` tests still pass.
- `contenox serve --help` does not advertise `--acp`.
- No new ACP stdio lifecycle is added to `serve`.

## Phase 9: Beam and UI package restore

### Work

- Restore `packages/ui` source and package manifests.
- Restore `packages/beam` source and package manifests.
- Do not restore `node_modules`.
- Do not restore stale `dist` as source of truth.
- Use the HTTP-first strategy: Beam is a same-origin browser client of
  `contenox serve`; no ACP dual mode and no backup account/vault login stack.
- Update frontend API base paths and payloads to match current `/api` routes:
  path-based local files, local identity, optional local token storage, current
  terminal websocket paths, current `execute_config.tools` /
  `tools_policies`, and current task handlers.
- Add the missing `POST /api/tasks` route for UI prompt execution, backed by
  the current `agentservice.Prompt` path.
- Keep current product names and route labels consistent with current CLI
  concepts: chains, sessions, models, providers, tools, MCP, HITL, setup.
- Rebuild Beam with current API contracts.
- Restore `runtime/internal/web` embed/SPA handler after a real `dist` exists
  and mount it after `/api/` in `serve` so API routes keep priority.
- Add Makefile targets for:
  - install UI deps
  - build UI
  - embed/verify UI

### Validation checkpoints

- `npm ci` in `packages/ui`
- `npm ci` in `packages/beam`
- `npm test` where package scripts exist
- `npm run build` in `packages/ui`
- `npm run build` in `packages/beam`
- `go test ./runtime/internal/web ./runtime/contenoxcli ./runtime/serverapi`
- `go test ./...`
- `contenox serve` loads the SPA at `/`.
- Direct SPA navigation works, for example `/chats` or current Beam route
  equivalent.
- `/api/*` requests are never swallowed by the SPA fallback.
- Browser smoke:
  - login/setup screen renders
  - backends/providers screen loads
  - chat screen loads
  - files/taskchains screen loads
  - terminal panel hides or disables itself when terminal is disabled

### Validation results

- `npm ci` in `packages/ui`
- `npm run build` in `packages/ui`
- `npm ci` in `packages/beam`
- `npm run build` in `packages/beam`
- `npm test` in `packages/beam`
- `make verify-ui-embed`
- `go test ./runtime/internal/taskexecapi ./runtime/internal/web ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- `go build -o /tmp/contenox-phase9-smoke ./cmd/contenox`
- Beam production build emitted `runtime/internal/web/beam/dist` with 25 files.
- `runtime/internal/web` unit test verifies `/` and deep-link SPA fallback.
- Focused Go tests verify the restored `/tasks` route and server route wiring.
- Live smoke on an isolated workspace verified:
  - `GET /` returns the embedded Beam HTML shell.
  - `GET /chat/smoke` falls back to the Beam HTML shell.
  - `GET /api/health` returns API JSON, not the SPA fallback.
- Generated dependency directories (`packages/*/node_modules`,
  `packages/ui/dist`) were removed after validation.

## Phase 9.1: Unified setup status and local shell default

### Work

- Remove the second runtime-state instance from `contenox serve`.
- Make `enginesvc.Build` own or accept the runtime state and expose the state it
  actually reconciles.
- Wire HTTP route dependencies to `engine.State`, so `/api/setup-status`,
  `/api/state`, `/api/models`, and setup readiness observe the same backend
  cycle that the engine/doctor path uses.
- Add `POST /api/setup/refresh` to run one backend reconciliation cycle and
  return the updated setup readiness result.
- Update Beam onboarding Refresh to call `/api/setup/refresh` instead of only
  invalidating a stale client cache.
- Update Beam setup copy so it describes explicit refresh, not an automatic
  10-second background scan.
- Enable terminal routes by default for local loopback `serve`; keep
  `TERMINAL_ENABLED=false` as the opt-out.
- Enable the `local_shell` tool by default for `serve`; keep `--shell=false` as
  the opt-out.

### Validation checkpoints

- `go test ./runtime/enginesvc ./runtime/stateservice ./runtime/internal/setupapi ./runtime/serverapi ./runtime/contenoxcli`
- `go test ./...`
- `npm test` in `packages/beam`
- `npm run build` in `packages/beam`
- Live local smoke:
  - `GET /api/setup-status` reads the engine-owned runtime state.
  - `POST /api/setup/refresh` runs a backend cycle and returns setup status.
  - Beam health Refresh updates the setup status cache from the refresh response.
  - Default `contenox serve` exposes `/api/terminal/sessions` on loopback.
  - Default `contenox serve` registers the `local_shell` tool.
  - `TERMINAL_ENABLED=false contenox serve` hides terminal routes.
  - `contenox serve --shell=false` does not register the `local_shell` tool.
  - Non-loopback `ADDR` without `TOKEN` still fails startup.

### Validation results

- `go test ./runtime/enginesvc ./runtime/stateservice ./runtime/internal/setupapi ./runtime/serverapi ./runtime/contenoxcli`
- `npm ci` in `packages/ui`
- `npm run build` in `packages/ui`
- `npm ci` in `packages/beam`
- `npm test` in `packages/beam`
- `npm run build` in `packages/beam`
- `make verify-ui-embed`
- `go test ./...`
- Built `/tmp/contenox-phase9_1-smoke` from `./cmd/contenox`.
- Live smoke on the existing configured local runtime verified:
  - `GET /api/setup-status` returns the same configured defaults and backend
    health shape as `contenox doctor`: 6 registered backends, 5 reachable, and
    the invalid OpenAI key reported as the single backend error.
  - `POST /api/setup/refresh` runs a backend reconciliation and returns the same
    updated setup status.
  - Default loopback `contenox serve` exposes `GET /api/terminal/sessions`
    without requiring `TOKEN`.
  - Default `contenox serve` registers `local_shell` in `GET /api/tools/local`.
  - `TERMINAL_ENABLED=false contenox serve` returns `404` for terminal routes.
  - `contenox serve --shell=false` omits `local_shell` from
    `GET /api/tools/local`.
  - `ADDR=0.0.0.0` without `TOKEN` fails startup before listening.
- Generated dependency directories (`packages/*/node_modules`,
  `packages/ui/dist`) were removed after validation.

## Phase 10: API tests and OpenAPI contract

### Work

- Restore `apitests` selectively. Do not restore obsolete tests for APIs that
  are intentionally deferred or removed.
- Restore `tools/openapi-gen` only if the OpenAPI generation path is still
  useful for the current API. Otherwise, preserve embedded `openapi.json` as a
  generated artifact with an explicit regeneration command.
- Add tests for current behavior that the backup did not know about:
  - reasoning/thinking model fields where exposed
  - tenant/workspace ID behavior
  - current HITL policy source behavior
  - current setup readiness behavior
- Make API tests run against `contenox serve` with an isolated temporary HOME,
  workspace, and DB.

### Validation checkpoints

- `go test ./...`
- `make test-api`
- OpenAPI document includes every registered route group.
- OpenAPI smoke:
  - `/api/openapi.json` is valid JSON
  - `/api/docs` serves documentation UI
  - route count in generated/openapi docs is close to actual registered route
    count, with documented exclusions only
- API tests must not require real OpenAI/Gemini/Ollama credentials.

## Phase 11: End-to-end release readiness

### Work

- Run full Go, API, and UI suites.
- Run manual `serve` smoke on a clean workspace.
- Validate build/release packaging does not accidentally ship `node_modules`.
- Validate binary size impact from embedded Beam.
- Update user docs for `contenox serve`, auth/setup, and local UI.
- Decide whether `serve` is advertised as stable, preview, or hidden.

### Validation checkpoints

- `go test ./...`
- `make test-api`
- `npm run build` for UI packages
- `go build ./cmd/contenox`
- Clean workspace smoke:
  - `contenox init`
  - `contenox serve`
  - open UI
  - configure optional local token if serving beyond loopback
  - configure provider or local backend
  - pass setup readiness
  - send chat
  - observe task events
  - edit/list files
  - edit/list taskchains
- `git status --short` contains only intended source, docs, and generated
  assets.

## Suggested branch strategy

Use small branches that align with the phases:

1. `http-foundation`
2. `serve-skeleton`
3. `api-core-routes`
4. `api-auth-provider`
5. `api-local-files-taskchains`
6. `api-chat-hitl-events`
7. `api-terminal`
8. `serve-acp-dual`
9. `beam-ui-restore`
10. `api-contract-tests`

Each branch should be mergeable on its own and should leave `go test ./...`
green unless the branch is explicitly UI-only and documents why Go is unchanged.

## First implementation target

The first concrete milestone is intentionally small:

```
contenox serve
GET /health
GET /version
GET /api/health or equivalent mux health check
go test ./...
```

After this works, add route groups one at a time. This keeps the migration from
turning into a single unreviewable copy of the backup tree.
