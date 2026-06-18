# OpenVINO Provider Plumbing — Handover

Date: 2026-06-16

## What this is

Close the OpenVINO provider's **plumbing** gaps so the proven warm-prefix-reuse
substrate becomes a usable runtime backend. Hardware proof (S3) is explicitly out
of scope. Full step detail lives in `~/.claude/plans/cozy-wondering-matsumoto.md`;
product/why context is `local-coding-node-goals.md` and `plan-openvino.md`.

## Locked decisions (do not relitigate)

- **Segment input → classify in the provider.** No `LLMChatClient` interface
  change; map `[]Message` + `cfg.Tools` → `[]Segment` inside `modelrepo/openvino`.
- **Manifest → shared `contextasm` extracted from `llama`.** `localnode` and
  `local` packages are being deleted; `llama` is canonical
  (`runtime/modelrepo/llama/manifest.go` is the mature contract).
- **Native teardown is already safe.** `cx_genai_session` builds *and* destroys
  the pipeline on its own worker thread (`ovsession/genai.cpp:90-136`).

## Build / test facts

- Native path is gated `//go:build openvino && openvino_genai`. Default build →
  `ovsession.GenAIAvailable == false`, provider advertises `CanChat=false`, catalog
  lists nothing. Pure-Go logic (pool, segments, classifier) must compile + test
  untagged via the `ovsession` stub.
- Tagged suites need OpenVINO libs + a model:
  `make -f Makefile.openvino test-s1-5 | test-s2 | test-s2-5`.
- Default build must stay green: `go build ./...`, `go test ./runtime/...`.

## Status

### DONE — Step 1: session pool lifecycle & eviction

- `runtime/modelrepo/openvino/session_pool.go`: idle sessions (refs==0) stay
  resident for warm reuse but are reaped by idle-TTL (`genAISessionIdleTTL`, 5m)
  and a resident cap (`genAISessionMaxResid`, 2, LRU); in-use sessions never
  evicted. Reaping closes sessions outside the pool lock.
- Generic shutdown-hook registry added (mirrors `RegisterCatalogProvider`):
  `runtime/modelrepo/shutdown.go` (`RegisterShutdownHook` / `Shutdown`). OpenVINO
  self-registers `ShutdownGenAISessions` in its `init()` (`catalog.go`). CLI long-
  lived entrypoints (`contenoxcli/acp_cmd.go`, `vscodeagent_cmd.go`) call the
  generic `modelrepo.Shutdown()` on exit — **CLI never imports a concrete backend.**
- Untagged tests: `session_pool_test.go` (TTL reap, LRU cap, in-use protection,
  shutdown). All pass. NOTE: final full verification (`go build ./...` +
  `go test ./runtime/...`) was interrupted before completion — re-run it first.

### DONE — Step 2: cache signal + digest-keyed sessions

- `runtime/modelrepo/openvino/client.go`: `Chat` and `Prompt` now route GenAI
  calls through a telemetry wrapper that reports `cache_usage` with
  `PipelineMetrics` (`requests`, `scheduled_requests`, cache usage/max/avg,
  cache size bytes, inference duration) plus a stable-prefix hash. `ChatResult`
  is unchanged.
- Stable-prefix telemetry is intentionally only a signal at this step: system
  messages and declared tool JSON are hashed with the existing `Segment`
  assembler; volatile user turns do not change the hash. The real value path
  still belongs to Step 3.
- `runtime/modelrepo/openvino/model_digest.go`: session identity now hashes the
  OpenVINO IR, tokenizer, detokenizer, tokenizer config, generation/config JSON,
  and optional `chat_template.jinja`.
- `runtime/modelrepo/openvino/session_pool.go`: `genAISessionKey` includes that
  model-dir digest, so replacing an IR/tokenizer/template in place cannot reuse
  stale pooled KV state.
- `catalog.go` / `provider.go`: OpenVINO now carries the catalog tracker into
  GenAI clients, matching the other providers.
- Untagged focused verification:
  `go test -count=1 ./runtime/modelrepo/openvino`.

### DONE — Step 3: wire `AssembleContext` into chat

- `runtime/modelrepo/openvino/classify.go`: added the in-provider classifier
  from `[]Message` + declared tools JSON to `[]Segment`, stable-prefix hash, and
  render-order messages. It is deliberately conservative: leading `system`
  messages and declared tools are stable; once conversation turns begin, later
  messages stay volatile and in order.
- `client.go`: `Chat`, `Prompt`, and `Stream` now classify before rendering.
  The prompt still goes through `ApplyChatTemplate`, so OpenVINO/model-native
  chat templates remain the value path; segment wrappers are used only for cache
  identity/hash.
- `runtime/modelrepo/openvino/chat_context_integration_test.go`: added the
  tagged S2.5 chat-path proof. It compares a warm classified chat turn against a
  cold full prompt under deterministic decode settings and asserts the stable
  hash stays equal while the warm path is faster.
- `Makefile.openvino`: `test-s2-5` now runs both the raw segment proof and the
  provider chat-path proof.
- Untagged focused verification:
  `go test -count=1 ./runtime/modelrepo/openvino`.
- Tagged verification not run in this turn; requires OpenVINO native libs and
  `CONTENOX_OPENVINO_TEST_MODEL`.

### DONE — Step 4: shared `contextasm`; OpenVINO manifest adoption

- New pure-Go shared package:
  `runtime/modelrepo/contextasm`.
  It owns the deterministic segment assembler, `ContextManifest`,
  `ManifestSegment`, manifest compatibility checks, token hash helpers, and
  manifest mismatch errors.
- `runtime/modelrepo/llama`: manifest types/errors are now aliases to
  `contextasm`; llama keeps only llama-specific runtime identity helpers. Existing
  llama callers and tests still use `llama.ContextManifest` etc.
- `runtime/modelrepo/openvino`: segment types are compatibility aliases to
  `contextasm`, and chat classification now builds a `ContextManifest` per turn.
  The GenAI client gates reuse observability through `CompatibleRuntime`, reports
  manifest digest/stable hash, and tracks the last successful manifest.
- OpenVINO native tokenizer ABI added:
  `cx_genai_tokenize` in `ovsession/genai.h`, `genai.cpp`, and Go/stub wrappers.
  Successful native tokenization adds rendered prompt token count/hash to
  telemetry.
- Important caveat: OpenVINO still renders through model-native Jinja via
  `ApplyChatTemplate`; the manifest describes deterministic logical segments.
  Exact stable segment token ranges through opaque templates are not claimed yet.
- Untagged focused verification:
  `go test -count=1 ./runtime/modelrepo/contextasm ./runtime/modelrepo/llama/... ./runtime/modelrepo/openvino ./runtime/modelrepo/openvino/ovsession`.
- Tagged native verification not run in this turn; it requires OpenVINO native
  libs and `CONTENOX_OPENVINO_TEST_MODEL`.

### DONE — Step 7: Go-native IR model pull (2026-06-18)

- `runtime/modelregistry`: `ModelDescriptor` gained `Backend` + `Repo`; curated
  OpenVINO IR entries added (`qwen2.5-coder-{0.5b,1.5b}-ov`, repo
  `OpenVINO/Qwen2.5-Coder-*-int4-ov`). `BackendType()` defaults empty → `"llama"`.
- `runtime/contenoxcli/model_pull_cmd.go`: `model pull` branches by descriptor
  backend. GGUF stays a single-file download; OpenVINO IR is fetched as a
  **multi-file repo over the HF Hub HTTP API** (`/api/models/<repo>` → siblings,
  `/resolve/main/<file>`, no Python), verifying `openvino_model.xml`.
- **Layout changed to per-backend subdirs** (supersedes the original
  `~/.contenox/models/<name>/`): GGUF → `~/.contenox/models/llama/<name>/`,
  IR → `~/.contenox/models/openvino/<name>/`. `contenox init` now registers BOTH
  local backends (`ensureLocalBackends`), each `BaseURL` at its subdir.
- **OpenVINO is now a first-class backend type:** accepted by
  `backendservice` validation and routed in `runtimestate.processBackend`
  (reuses the generic catalog path). Catalog/adapter were already type-generic.
- Untagged verification: `go test ./runtime/modelregistry ./runtime/contenoxcli`.

### Enabling plumbing landed alongside (2026-06-18)

These cross-cutting changes make the single-backend daemon coherent (detail in
`remaining-work.md` status note):
- **modeld mode awareness:** the owner lease + gRPC health advertise the served
  backend; `modeldprobe.Status.Backend` / `modeldconn.Backend()` expose it; each
  provider's `SessionAvailable()` gates on the daemon's mode.
- **Typed transport handle:** `transport.OpenSessionRequest` carries
  `{ModelName, Type, Digest, Path}` (was a raw `ModelID` path); modeld rejects a
  foreign model `Type` with `transport.ErrBackendMismatch` before loading.

### TODO — Steps 5, 6, 8

5. **Stream/tool-history parity.** `client.go`: carry
   `ToolCalls`/`ToolCallID`/`Thinking` into `ChatMessage`→`ApplyChatTemplate`;
   give `Stream` the same parser handling as `Chat` (or OpenVINO incremental
   parsers).
6. **Embeddings.** ABI + provider embed path; implement
   `GetEmbedConnection`/`Embed`; flip `CanEmbed()` when an embed model/profile
   exists. (No-sidecar rule: must come from OpenVINO.)
8. **Distribution.** Vendor `.so` + RPATH (or static link); drop the venv +
   checked-out `openvino.genai` header-worktree dependency; reproducible tagged CI.
   `make build-modeld` is now wired (CGO flags via `mk/openvino-flags.mk` +
   `mk/llama-flags.mk`, deps via `make deps-modeld`), but still RPATHs into the
   `.openvino` venv — this step makes it self-contained.

## Deferred

S3 hardware benchmark; `ChatResult.Meta` field; `contenox node explain-context`
CLI; chain-level structured-segment contract (Step 3's in-provider classifier is
the agreed first step).
