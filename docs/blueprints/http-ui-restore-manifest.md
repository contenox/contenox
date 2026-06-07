# HTTP/UI Restore Manifest

Phase 0 baseline for reviving the HTTP API and Beam UI from:

```
/home/naro/src/github.com/contenox/enterprise/oss-backup2
```

This manifest is intentionally source-neutral: it records what should be
restored, what must be adapted, and what is deferred or obsolete. No source code
is copied in Phase 0.

## Baseline

Checked from the current OSS runtime worktree:

```
/home/naro/src/github.com/contenox/runtime
```

Current status after Phase 0 documentation, before migration code changes:

```
?? docs/blueprints/acp-registry-submission/
?? docs/blueprints/plan-http-ui-revival.md
?? docs/blueprints/http-ui-restore-manifest.md
```

Notes:

- `docs/blueprints/acp-registry-submission/` was already untracked before this
  migration work.
- `docs/blueprints/plan-http-ui-revival.md` is the phase plan created before
  this manifest.
- `docs/blueprints/http-ui-restore-manifest.md` is this restore manifest.
- Current `go list ./...` succeeds and lists 56 Go packages.
- `oss-backup2` is read-only source material.

## Classification

| Classification | Meaning |
|---|---|
| Direct restore | Copy package/file and rewrite module imports. Minimal behavioral adaptation expected. |
| Adapt restore | Copy package/file, rewrite imports, and update for current runtime APIs. |
| Split restore | Restore only part of the backup path in an early phase; defer the rest. |
| Defer | Do not restore until a later phase needs it. |
| Obsolete | Do not restore; current runtime already owns this behavior or the path is stale. |

## Restore map

| Backup path | Phase owner | Classification | Notes |
|---|---:|---|---|
| `apiframework/auth.go` | 1 | Direct restore | Keep package-level auth helpers if route packages still use them. |
| `apiframework/clienterrors.go` | 1 | Direct restore | Needed for consistent API error responses. |
| `apiframework/encode.go` | 1 | Direct restore | JSON response helpers. |
| `apiframework/errorconstants.go` | 1 | Direct restore | Error identifiers used by route packages. |
| `apiframework/errors.go` | 1 | Direct restore | Error response shape. |
| `apiframework/middleware/auth.go` | 1 | Adapt restore | Restore interfaces and middleware for optional local token gating; do not require backup auth service. |
| `apiframework/middleware/cors.go` | 1 | Direct restore | Needed by `serve` skeleton. |
| `apiframework/openapi.go` | 1 | Direct restore | May move to Phase 10 if only OpenAPI docs use it. |
| `apiframework/params.go` | 1 | Direct restore | Path/query helper used by API routes. |
| `apiframework/requestid.go` | 1 | Direct restore | Needed by API middleware. |
| `apiframework/version.go` | none | Obsolete | Current runtime owns version in `runtime/version`. Do not reintroduce a second source. |
| `apiframework/version.txt` | none | Obsolete | Current runtime owns version in `runtime/version/version.txt`. |
| `runtime/serverapi/health_routes.go` | 1 | Direct restore | First HTTP endpoint. |
| `runtime/serverapi/version_routes.go` | 1 | Adapt restore | Use current `runtime/version`. |
| `runtime/serverapi/server.go` | 1, 3-8 | Split restore | Phase 1 restores config and health/version skeleton; later phases wire product routes. |
| `runtime/contenoxcli/serve_cmd.go` | 2, 7, 9, 9.1 | Split restore | Phase 2 skeleton; Phase 7 terminal lifecycle; Phase 9 UI serving; Phase 9.1 unified runtime state plus default terminal and `local_shell` behavior. Backup `serve --acp` dual mode is intentionally skipped. Must not overwrite current CLI logic. |
| `runtime/contenoxcli/auth_cmd.go` | none | Defer | Backup command targets account/vault auth. Defer unless a local session flow is explicitly needed. |
| `runtime/internal/backendapi` | 3 | Adapt restore | Use current `backendservice`, `stateservice`, `runtimestate`, and `runtimetypes`. |
| `runtime/internal/modelregistryapi` | 3 | Adapt restore | Use current model registry service and schema. |
| `runtime/internal/setupapi` | 3, 9.1 | Adapt restore | Use current setup readiness behavior, `stateservice.SetupStatus`, and explicit refresh via the shared runtime state. |
| `runtime/internal/toolsapi` | 3 | Adapt restore | Use current `toolsproviderservice` and current persistent tools repo signature. |
| `runtime/internal/mcpserverapi` | 3 | Adapt restore | Use current `mcpserverservice` and `mcpworker` event behavior. |
| `runtime/internal/taskeventsapi` | 3, 6 | Split restore | Basic stream route in Phase 3; chat/HITL event assertions in Phase 6. |
| `runtime/internal/openapidocs` | 3, 10 | Split restore | Restore handlers in Phase 3 if useful; regenerate/validate contract in Phase 10. |
| `runtime/internal/vault` | none | Defer | Not required for a trusted local server / LLM gateway. Revisit only if durable encrypted secret storage becomes a product requirement. |
| `runtime/internal/auth` | none | Defer | Backup account/JWT flow is multi-user shaped. Prefer optional local token gating from current `apiframework` first. |
| `runtime/internal/authapi` | none | Defer | Backup login/setup endpoints are not needed for default local-loopback operation. Revisit only if Beam needs browser session state. |
| `runtime/providerservice` | 4 | Adapt restore | Provider setup may still be useful, but must use current backend/model schema, CLI config keys, and env-var secret references rather than backup vault. |
| `runtime/internal/providerapi` | 4 | Adapt restore | Provider configure/status/list endpoints for local setup UX, without backup account/vault prerequisites. |
| `runtime/localfileservice` | 5 | Local replacement | New direct filesystem service for the local OSS server; not copied from backup VFS. |
| `runtime/internal/localfileapi` | 5 | Local replacement | File/folder API over `localfileservice`; replaces the useful route behavior from backup `vfsapi` without a VFS abstraction. |
| `runtime/vfsservice` | none | Obsolete | Do not restore for the local-only server. Revisit only if virtual/remote storage becomes a product requirement. |
| `runtime/vfsstore` | none | Obsolete | DB-backed VFS is not needed for local workspace files. |
| `runtime/taskchainservice` | 5 | Adapt restore | Chain CRUD for `.contenox` JSON files, backed directly by `localfileservice`. |
| `runtime/internal/vfsapi` | none | Obsolete | Replaced by `runtime/internal/localfileapi`. |
| `runtime/internal/taskchainapi` | 5 | Adapt restore | Chain CRUD over current `taskchainservice`. |
| `runtime/internal/hitlpolicyapi` | 5 | Adapt restore | Writes/reads policy JSON files under `.contenox`; core HITL still uses `PolicySource`. |
| `runtime/internal/internalchatapi` | 6 | Adapt restore | Use current `agentservice`, `chatservice`, and task events. |
| `runtime/internal/approvalapi` | 6 | Adapt restore | Respond to current `hitlservice` pending approvals. |
| `runtime/terminalstore` | 7 | Adapt restore | Terminal DB schema/store. |
| `runtime/terminalservice` | 7 | Adapt restore | Must preserve Unix/Windows files and current workspace-root safety. |
| `runtime/internal/terminalapi` | 7 | Adapt restore | HTTP + websocket route; terminal-enabled `serve` protects terminal methods when `TOKEN` is configured, including websocket attach. |
| `runtime/internal/web/web.go` | 9 | Adapt restore | Restore after Beam `dist` generation strategy is decided. |
| `runtime/internal/web/beam/dist` | 9 | Defer | Generated asset. Do not restore stale dist as source of truth. |
| `packages/ui` | 9 | Adapt restore | Restore source/package files, not `node_modules` or stale `dist`. |
| `packages/beam` | 9 | Adapt restore | Restore source/package files, not `node_modules` or stale `dist`. |
| `package.json` | 9 | Adapt restore | Needed only once UI workspace is restored. |
| `package-lock.json` | 9 | Adapt restore | Restore/update with UI dependency decisions. |
| `scripts/openapi-rapidoc.html` | 10 | Defer | OpenAPI docs asset; restore if still used. |
| `tools/openapi-gen` | 10 | Defer | Restore only if current API contract generation still uses it. |
| `apitests` | 10 | Restored selectively | Curated Python smoke suite under `apitests`; `make test-api` builds the binary, starts `contenox serve` with isolated HOME/workspace/DB, and avoids real provider credentials by default. |
| `runtime/internal/groupapi` | none | Obsolete | Backup directory is empty; no route package to restore. |
| `runtime/internal/modelrepo` | none | Obsolete | Current runtime uses `runtime/modelrepo`. Do not restore internal copy. |
| `runtime/internal/llmrepo` | none | Obsolete | Current runtime uses `runtime/llmrepo`. Do not restore internal copy. |
| `runtime/internal/ollamatokenizer` | none | Obsolete | Current runtime uses `runtime/ollamatokenizer`. Do not restore internal copy. |
| `runtime/internal/runtimestate` | none | Obsolete | Current runtime uses `runtime/runtimestate`. Do not restore internal copy. |
| `runtime/internal/setupcheck` | none | Obsolete | Current runtime owns setup checks and Ollama probe behavior. |
| `runtime/internal/llmresolver` | none | Obsolete | Current runtime owns resolver behavior. |
| `runtime/internal/tools` | none | Obsolete | Current runtime already has this package and has diverged. Adapt call sites to current package. |
| `runtime/acpsvc` | none | Obsolete | Current ACP service is newer. Do not copy backup ACP code. Keep standalone `contenox acp`/`contenox acpx`; do not add backup dual-mode ACP to `serve`. |
| `runtime/agentservice` | none | Obsolete | Current service has diverged. Adapt HTTP chat to current service. |
| `runtime/backendservice` | none | Obsolete | Current service has diverged. Adapt routes to current service. |
| `runtime/chatservice` | none | Obsolete | Current service has compaction/session changes. Adapt routes to current service. |
| `runtime/enginesvc` | none | Obsolete | Current engine has tenant/policy/setup changes. Do not copy backup engine. |
| `runtime/execservice` | none | Obsolete | Current task execution service is authoritative. |
| `runtime/hitlservice` | none | Obsolete | Current `PolicySource` implementation is authoritative. |
| `runtime/localtools` | none | Obsolete | Current tools include newer file/exec/HITL behavior. |
| `runtime/mcpserverservice` | none | Obsolete | Current service is authoritative. |
| `runtime/mcpworker` | none | Obsolete | Current worker behavior is authoritative. |
| `runtime/messagestore` | none | Obsolete | Current store is authoritative. |
| `runtime/modelregistry` | none | Obsolete | Current registry is authoritative. |
| `runtime/modelregistryservice` | none | Obsolete | Current service is authoritative. |
| `runtime/modelservice` | none | Obsolete | Current service is authoritative. |
| `runtime/runtimetypes` | none | Obsolete | Current schema/types have tenant and capability changes. |
| `runtime/sessionservice` | none | Obsolete | Current service is authoritative. |
| `runtime/stateservice` | none | Obsolete | Current service takes `workspaceID` and has current setup readiness. |
| `runtime/taskengine` | none | Obsolete | Current engine has newer DSL, reasoning, guard, macro, and cancellation changes. |
| `runtime/toolsproviderservice` | none | Obsolete | Current service is authoritative. |
| `runtime/version` | none | Obsolete | Current version package is authoritative. |
| `libacp` | none | Obsolete | Current libacp is newer. |
| `libauth` | none | Obsolete | Current libauth is authoritative. |
| `libbus` | none | Obsolete | Current libbus is authoritative. |
| `libcipher` | none | Obsolete | Current libcipher is authoritative. |
| `libdbexec` | none | Obsolete | Current DB abstraction is authoritative. |
| `libkvstore` | none | Obsolete | Current KV store is authoritative. |
| `libroutine` | none | Obsolete | Current routine group is authoritative. |
| `libtracker` | none | Obsolete | Current tracker is authoritative. |
| `cmd/contenox/main.go` | none | Obsolete | Current entrypoint already points to current `contenoxcli`. |
| `Makefile` | 9-11 | Defer | Restore individual UI/test targets only, not wholesale. |
| `.air.serve.toml` | 11 | Defer | Local dev convenience only. |
| `.github/workflows/*` | 11 | Defer | CI changes after source phases compile. |
| `docs/*` | 11 | Defer | User docs after `serve` behavior is stable. |
| `examples/*` | 11 | Defer | Restore/update only if still useful. |
| `bin/*` | none | Obsolete | Built binaries; never restore. |
| `tmp/*` | none | Obsolete | Temporary artifacts; never restore. |
| `.pytest_cache/*` | none | Obsolete | Test cache; never restore. |
| `.claude/*` | none | Obsolete | Local tool config; never restore. |
| `.zed/*` | none | Obsolete | Current repo owns editor settings. |

## First endpoint groups

The first code milestone should be:

1. `GET /health`
2. `GET /version`
3. API not-found error shape through the restored framework
4. `contenox serve` skeleton with CORS and request ID middleware

Do not include auth, product API routes, ACP dual mode, terminal, or UI assets
in the first code milestone.

## Phase 1 source set

Restore/adapt only:

```
apiframework/auth.go
apiframework/clienterrors.go
apiframework/encode.go
apiframework/errorconstants.go
apiframework/errors.go
apiframework/middleware/auth.go
apiframework/middleware/cors.go
apiframework/openapi.go
apiframework/params.go
apiframework/requestid.go
runtime/serverapi/health_routes.go
runtime/serverapi/version_routes.go
runtime/serverapi/server.go
```

Explicit Phase 1 exclusions:

```
apiframework/version.go
apiframework/version.txt
runtime/contenoxcli/serve_cmd.go
runtime/internal/*
packages/*
apitests/*
```

## Phase 0 validation results

Baseline commands run from current OSS runtime:

```
git status --short
go list ./...
```

Results:

- `git status --short` showed only untracked docs paths.
- `go list ./...` completed successfully.
- No source files were copied from `oss-backup2`.

## Phase 9 update

Completed restore/adaptation for the UI-owned manifest entries:

- `packages/ui` source and package manifests restored without `node_modules`.
- `packages/beam` source and package manifests restored without `node_modules`.
- `runtime/internal/web/web.go` restored as an embedded Beam SPA handler.
- Beam `dist` generated from restored source into
  `runtime/internal/web/beam/dist` and mounted by `contenox serve`.
- Backup account/vault auth and ACP dual-mode launch were not restored; Beam now
  runs HTTP-first against the local `/api` surface with local identity and
  optional token storage.
