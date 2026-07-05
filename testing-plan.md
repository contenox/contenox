# Contenox v1 End-to-End Testing Plan

Status: draft, built from a repo feature screen on 2026-07-05; execution
started same day (see "Verified-green this session" and BUG-001..010 in
`bug-inventory.md`).
Companion doc: `bug-inventory.md` (findings go there as testing executes).

## Methodology

Per `docs/blueprints/product-surface-truth-blueprint.md`: a user-visible
surface (CLI command, flag, curated model, provider type, setup step, UI
panel, documented syntax) is either backed by an E2E test that exercises it
on the real product path, or it is a removal candidate — not a backlog item.
This plan enumerates every such surface found in the repo and marks it
`[tested]` / `[gap]` / `[known-issue]` / `[not-locally-testable]`.

## Environment / build matrix

| Platform | Backends buildable here | Status |
|---|---|---|
| Linux x86_64 (this box) | llama.cpp (CPU+CUDA+HIP), OpenVINO GenAI | built 2026-07-05, `backends: llama, openvino` confirmed via `modeld version` |
| Windows (Intel Core Ultra 7 155H) | llama.cpp CPU, OpenVINO (NPU/iGPU) | remote box only, not driven from here — see build/test host notes |
| macOS | llama.cpp + Metal (no OpenVINO) | not built/tested anywhere in this session |

Local GPU: RTX 3060 Laptop, 6 GB VRAM. System RAM: 15 GB, frequently under
heavy desktop load — **native builds must use the plain `make build-modeld` /
`make build-contenox` targets and should not run concurrently with other
heavy compiles**; an overridden `-j` build alongside a loaded desktop OOM'd
and hard-rebooted this machine once already during this project (see
bug-inventory.md).

Curated model catalog spans 6 GB → 32 GB VRAM classes (41 entries). Only the
6 GB and some 8 GB tier is realistically pullable/loadable on this box. The
16/24/32 GB tiers are `[not-locally-testable]` here — flag explicitly rather
than skip silently.

## 1. modeld daemon

| Surface | Status | Notes |
|---|---|---|
| `modeld serve` (lease acquire/renew, gRPC listen) | [tested] | covered by `modeld/slot` + `runtime/transport/grpc` system tests |
| `modeld status` / `modeld status --json` | [gap] | no test found exercising the CLI subcommand output itself |
| `modeld version` / `--json` | [tested] | manually verified this session: reports `backends: llama, openvino` correctly |
| gRPC transport contract (OpenSession/Decode/Snapshot/Restore/ExplainContext) | [tested] | `runtime/transport/grpc/grpc_test.go` |
| Snapshot → blob-cache indirection (`StateID`, `modeld-blobs/<backend>/<id>.bin`) | [tested] | fixed + covered this session (`TestSnapshotStateIDRoundTripOverWire`); slot-level disk round-trip still `[gap]` (fake session in `modeld/slot/service_test.go` never returns real State) |
| Idle-TTL reaper (`--idle-ttl`) | [tested] | `modeld-idle-ttl-resource-mgmt` — Phase 1 landed |
| Capacity planner / resident cap (dynamic VRAM-based) | [tested] | `modeld-resident-cap-dynamic` fix validated |
| Capacity planner SWA-awareness (Gemma-style sliding window) | [known-issue] | `planner-ignores-swa-kv` — over-counts KV ~6x for SWA models, clamps context far below actual capacity |
| llama backend: session lifecycle, eviction, LoRA, structured output, Qwen3 chat parsing | [tested] | `modeld/llama/llamasession/*_system_test.go` — **re-verified today with the tiny in-tree GGUF: 20/20 real tests green (~45s), 4 correctly skipped (no LoRA/phi4/qwen3 model in-tree)**. Note: `make -f Makefile.llamacpp-direct test` alone does NOT run these — it's a false-green without `CONTENOX_LLAMA_TINY_GGUF` set (BUG-001) |
| llama backend: even-`n_ctx` requirement | [tested] | `llama-even-n-ctx-cuda-crash` — planner now rounds down |
| llama backend: embeddings | [tested] | `modeld-backend-parity-embeddings` — daemon-arm landed; native embed inference itself `[gap]` (not re-exercised against a real embed model on this GPU) |
| OpenVINO backend: session lifecycle, eviction, LoRA, prefix-cache reuse, reasoning, scheduler controls | [tested] | `modeld/openvino/ovsession/*_test.go` — **re-run today via `make -f Makefile.openvino test-genai`: `modeld/openvino` 20.8s + `modeld/openvino/ovsession` 201.9s, all green.** Real prefix-reuse measurement: 166KB prefix, cold 2m31s → warm 614ms (99.6% speedup), reconfirming `openvino-kv-snapshot-finding`. Embed/reasoning subtests skip by default (BUG-006) |
| OpenVINO backend: cold-KV round-trip / precision guard | [tested] | `openvino-kv-snapshot-finding` — only lossless at `KV_CACHE_PRECISION=f16`; default 8-bit is lossy, must confirm daemon default matches expectation before v1 |
| OpenVINO cold store reachability via `OpenSession` | [known-issue] | `effective-context-residency-loop` — unreachable from that entrypoint |
| Effective-context: warm prefix reuse (both backends) | [tested] | driven live, default-on |
| Effective-context: PrefillSuffix-overflow cold-park (both backends) | [tested] | `modeld-effective-context-not-driven-e2e` |
| Effective-context: EnsurePrefix-overflow, admit-on-reuse, durable snapshot | [gap] | implemented at a lower layer but **not yet user-facing / not driven E2E** |
| LoRA identity + native attach (both backends) | [gap] | `lora-identity-attach-slice` — pure-Go verified, CGo E2E on real adapters still deferred; confirmed still skipping today, no in-tree adapter (BUG-007) |
| NPU backend selection | [known-issue] | `npu-no-paged-attention` — Intel NPU compiler rejects PagedAttention; effective-context path can never target NPU, only iGPU/CPU. Confirm this is surfaced correctly (not silently offered) in backend/model selection |
| Capability-fields-that-lie (advertised model capabilities vs actual) | [gap] | last item outstanding from `modeld-backend-parity-embeddings` audit |
| Windows/Intel AI-PC build+bench (NPU/iGPU) | [not-locally-testable] | requires the remote Windows box; not re-verified this session |
| Release packaging (`bundle-modeld-deps`, `package-modeld-release`, S3 store dedupe) | [gap] | no E2E test found; `modeld-release-runbook.md` describes manual flow only |

## 2. contenox CLI

Every subcommand from `contenox --help` / `contenox <cmd> --help`, enumerated this session:

| Command | Status | Notes |
|---|---|---|
| `setup` (interactive wizard) | [gap] | no automated E2E for the wizard flow itself |
| `init` | [tested] | exercised by `make test-api` harness (`contenox init --force`) |
| `doctor` / `doctor --json` / `doctor --skip-cycle` | [tested] | manually driven this session against 0-backend, 1-backend, reachable/unreachable states — clean and consistent in all three output modes |
| `backend add/list/show/remove` (8 provider types) | [partial] | CRUD covered by `apitests/test_backends.py`; **llama + openvino live add+chat manually verified this session** (see §4, BUG-004/011/013); the 6 remaining hosted providers still untested locally |
| `model list/local/registry-list` | [partial] | registry CRUD covered by API tests; `model list` live-backend path manually verified working for both local backends this session (once BUG-013's env var workaround is applied for openvino); `model local` disk-inventory path still has no dedicated test |
| `model pull` (curated download) | [partial] | `test_model_registry_download_curated_model` exists but gated behind `APITEST_RUN_DOWNLOAD=1` (real network) — confirm it's actually run before v1, not just present |
| `mcp add/list/show/remove/update` (stdio/sse/http) | [partial] | CRUD + stdio covered (`test_mcp_servers.py`); stdio add/list/show manually re-verified this session; sse/http transport and OAuth handshake `[gap]` |
| `tools add/list/show/remove/update` (remote OpenAPI providers) | [partial] | basic list covered (`test_tools.py`); add/CRUD, spec-fetch, auth-handshake retry-on-401 `[gap]` |
| `session new/list/switch/delete/show/fork/workspaces` | [partial] | manually verified this session: new/list/show/workspaces all correct once a session exists — **but auto-create-on-first-run is broken (BUG-011)**, a High-severity finding |
| `config set/get` (12 keys) | [tested] | manually verified this session: valid keys, invalid key, invalid enum value all produce correct behavior/errors; workspace vs. global scoping confirmed correct |
| `state list/show` | [tested] | manually verified this session; `state show` surfaced OBSERVATION-001 (anomalous context-window error on a background task) |
| `chat` (stateful, default cmd, `-e` editor mode, `--shell`) | [partial] | bare-positional and explicit `chat` both manually exercised — **bare form doesn't persist a session at all (BUG-011)**; `-e`/`--shell` still untested |
| `run` (stateless, `--input-type` string/chat/json/int) | [partial] | `string` verified working; `chat` implied working via the chat path; `int`/`json` manually confirmed they correctly error without a matching custom chain (expected, not a bug) — no automated test for any of the 4 exists yet |
| `acp` / `acpx` (ACP server over stdio, HITL policies, `--auto`) | [gap] | no automated test found for either profile |
| `vscode-agent` (stdio bridge) | [gap] | no automated test found |
| `cache clear` | [tested] | manually verified this session, clean output, no crash |
| `update` | [gap] | no test found (self-update path — verify it's safe to leave gapped for v1, or explicitly scope out) |
| `--chain`, `--input`, `--raw`, `--steps`, `--trace`, `--think`, `--timeout`, `--local-exec-allowed-dir` global flags | [gap] | no flag-by-flag test found |
| CLI `--help` text accuracy | [tested] | `make test-contenox-help` (`scripts/verify_cli_help.sh`) — ran this session, 13/13 subcommands present, version matches |

## 3. HTTP API surface (61 routes, `runtime/internal/openapidocs/openapi.json`)

| Group | Routes | Status |
|---|---|---|
| Health/version | `/health`, `/version` | [tested] |
| Backends | `/backends`, `/backends/{id}` | [tested] |
| MCP servers | `/mcp-servers*`, `/mcp/oauth/callback` | [partial] — CRUD tested, OAuth flow `[gap]` |
| Model registry | `/model-registry*` | [tested] |
| Task chains | `/taskchains*` | [tested] |
| HITL policies | `/hitl-policies*` | [tested] |
| Remote tools | `/tools/remote*`, `/tools/local`, `/tools/schemas` | [partial] — local/schemas untested |
| Chats | `/chats*` | [gap] |
| Files | `/files*` (content/download/move/stat) | [gap] |
| Providers | `/providers/*` (configs/supported/config/configure/status) | [gap] |
| Setup | `/setup-status`, `/setup/refresh` | [gap] |
| Approvals | `/approvals/{approvalId}` | [gap] — this is the HITL gate; high priority given the product's core "human approval gates" pitch |
| State | `/state`, `/task-events` | [gap] |
| CLI config | `/cli-config` | [gap] |
| modeld status | `/modeld/status` | [gap] |
| Ollama-compat | `/api/chat`, `/api/generate`, `/api/ps`, `/api/show`, `/api/tags` | [gap] — no pytest coverage found; this is an advertised compatibility surface |
| OpenAI-compat (global + per-chain) | `/v1/*`, `/openai/v1/*`, `/openai/{chainID}/v1/*` | [gap] — same |
| Root/misc | `/`, `/tasks` | n/a |

**~40% of the HTTP surface has zero apitest coverage**, including the
approval-gate endpoint that is central to the product's pitch, and both
compatibility shims (Ollama, OpenAI). This is the single largest coverage
gap found in this screen.

## 4. Provider types (product-surface-truth: "every advertised provider type
passes service validation and a live add-plus-chat test")

| Provider | CRUD tested | Live add+chat tested | Notes |
|---|---|---|---|
| `llama` (via modeld) | yes | [gap] — no CLI/API test drives an actual chat through this path | in-tree tiny models exist (`fastthink-0.5b-tiny-q2_k.gguf`, `Qwen3-0.6B-Q8_0.gguf`) — cheap to wire |
| `openvino` (via modeld) | yes | [gap] | in-tree tiny OV models exist (`qwen-coder-0.5b-int4`, `deepseek-r1-distill-qwen-1.5b-int4-ov`, etc.) |
| `ollama` | yes | [gap] | requires local `ollama serve`; Makefile has `OLLAMA_HOST`/`TASK_MODEL` vars wired for this already |
| `openai` | yes (validation only) | [not-locally-testable without a key] | flag as intentionally out of local scope, don't silently skip |
| `openrouter` | yes (validation only) | [not-locally-testable without a key] | |
| `gemini` | yes (validation only) | [not-locally-testable without a key] | |
| `vllm` | yes (validation only) | `make test-vllm` exists (`CONTENOX_RUN_VLLM_TESTS=1`) | confirm it's actually run, not just present |
| `vertex-google` | yes (validation only) | [not-locally-testable without gcloud auth] | |

Per the blueprint, a provider type that's advertised but never live-tested
end-to-end is a defect in either the advertisement or the test suite — this
table is the punch list to close that gap for the two zero-cost local
backends (llama, openvino) at minimum before v1.

## 5. Model registry / curated catalog

- 41 curated entries (`contenox model registry-list`), spanning granite,
  phi-4, qwen3, qwen2.5-coder, gemma4, starcoder2, deepseek(-coder),
  gpt-oss, codestral, devstral, qwen3-coder — each in llama (GGUF) and/or
  OpenVINO (IR) variants.
- Per blueprint: "a curated model entry is loader-checked against the
  pinned runtime before it ships" and "quality-smoked for its curated
  workload."
- **Tier 1 (smoke, this box can run today):** qwen2.5-coder-0.5b(-ov),
  tinyllama-1.1b-ov — marked "tiny coding/smoke test" in the catalog itself.
  `[gap]`: no automated test pulls+loads+chats any curated entry by name.
- **Tier 2 (6-8GB, fits this GPU "yes"/"maybe"):** ~15 entries. Spot-check
  a handful per family before v1, don't attempt all.
- **Tier 3 (16-32GB, "no" fit locally):** `[not-locally-testable]` — either
  get CI/cloud runners with bigger GPUs, or explicitly document these as
  "advertised, not locally verified" rather than claim full coverage.
- Alias resolution (fuzzy/substring routing) mentioned in the blueprint as
  a specific risk: `[gap]`, no test found.

## 6. ACP (Agent Client Protocol) surface

- `acp` (trusted/editor profile, e.g. Zed) and `acpx` (untrusted/headless,
  e.g. OpenClaw) — two distinct HITL policy profiles
  (`hitl-policy-acp.json` / `hitl-policy-acpx.json`), separate chain files,
  separate containment models.
- `[gap]`: no automated test found for either ACP profile's JSON-RPC
  session lifecycle, permission-request flow, or policy enforcement
  (e.g. verifying `acpx` actually denies `local_shell` and gates web
  mutations as documented).
- This is a HITL/safety-boundary surface — given the product's stated
  purpose ("where 'the model decided' is not an acceptable control
  boundary"), this gap is higher priority than its size suggests.

## 7. Beam UI (web chat, `packages/beam`)

Admin pages found: backends, chains, chats, control, hitl-policies, models,
prompt, remotehooks, settings. Public pages: login, bye.

- `make test-ui` (`npm test` / vitest) exists — status of what it actually
  covers vs. these pages: `[gap]`, needs a pass to map vitest specs to
  pages/panels.
- No browser-driven (Playwright) E2E found for the SPA — memory notes
  ("Beam UI revival") confirm the SPA + REST API were manually verified
  end-to-end once, but that was ad hoc, not a repeatable test.
- `[gap]`: dist-rebuild trap noted in memory (`beam-ui-dist-rebuild-gotcha`)
  — editing `@contenox/ui` src requires `npm run build` in `packages/ui`
  before changes show in Beam; verify this isn't silently stale for v1.

## 8. VS Code extension (`packages/vscode`)

36 registered commands, spanning: chat panel, walkthrough, agent sessions
(open/diagnose), selection actions (ask/fix/add-to-chat), diagnostics
(fix/explain), git (review changes/draft commit message), session
management, status/runtime info/restart, setup wizard, provider/model/
autocomplete/HITL-policy/think-level pickers, autocomplete triggers
(trigger/test/enable/disable/toggle), output/telemetry logs, language-model
provider test, MCP servers (show/refresh), tool-diff viewer.

- `[gap]`: no test-plan visibility into what of these 36 commands has any
  automated coverage — needs its own pass through `packages/vscode/src`
  test directories before v1 (out of scope for this screen's depth, flagged
  here as a follow-up).
- Packaging (`package:proposed`, VSIX secret-scan) has make targets but
  no evidence of a recent successful run in this session.

## 9. Task chain engine (`runtime/taskengine`)

Handler types: `raise_error`, `route`, `chat_completion`,
`execute_tool_calls`, `noop`, `tools`. Chain features per README/docs:
model routing, tool allowlists, command policy, retries, branch
conditions, budgets, human approval gates.

- `apitests/test_taskchains.py` covers CRUD + path-required validation only
  — **no test actually runs a chain** (branch conditions, retries, budgets,
  tool-calling, HITL approval gate mid-chain) end-to-end via the API.
  `[gap]`, and a significant one: chains are the product's core unit.
- Built-in local tools (`runtime/localtools`): shell, local exec (with
  `--local-exec-allowed-dir` confinement), fileio/fs, ssh, webhook, mcp
  pool, hitl, echo, print. `[gap]`: no enumeration found of which of these
  have dedicated system tests vs. only unit tests with mocks — needs a
  pass distinguishing "unit-tested with a fake" from "proven against a
  real shell/filesystem/ssh target."

## 10. Product-surface-truth invariants (blueprint-specific checks)

Direct checklist from `docs/blueprints/product-surface-truth-blueprint.md`,
not yet swept:

- [ ] Setup/health output never presents a dormant-but-healthy backend as
      an error (e.g. OpenVINO compiled in but no OV model loaded yet).
- [ ] A chain with no toolchain bound strips/replaces tool-assuming
      instructions rather than narrating fake tool use.
- [ ] Failure-summarization paths bound their input (can't overflow and
      mask the original error).
- [ ] Advertised context (`model list` CTX, catalog, UI) is *reachable*
      context, not a theoretical ceiling — directly relevant given the
      known SWA-planner over-counting bug (§1).

## Execution sequencing for v1

1. **Fast, zero-cost first:** `make test-unit`, `make test-contenox-help`,
   `make test-contenox-verbose`, `go test ./...` (pure Go, already fast on
   this box) — establish a clean baseline before anything native.
   **[DONE 2026-07-05]** — all green, see bug-inventory.md.
2. **Native system tests, already backend-proven:** `make test-llamacpp-direct`,
   the `TestSystem_*` suites in `modeld/llama` and `modeld/openvino`.
   **[DONE 2026-07-05, with a caveat]** — OpenVINO suite (`make -f
   Makefile.openvino test-genai`) ran clean end to end. The llama suite
   required manually setting `CONTENOX_LLAMA_TINY_GGUF`: the bare Makefile
   target silently skips all 20 real tests and reports a false "ok" —
   fix this before trusting CI green on this target (BUG-001).
3. **API surface:** `make test-api` as-is, then close the biggest gaps from
   §3 in priority order: approvals (HITL gate) → task-chain execution →
   ollama/openai compat shims → chats/files/providers.
4. **Live provider smoke:** llama + openvino live add-plus-chat using the
   in-tree tiny models (zero download cost, matches §4's highest-priority
   gap). **[DONE 2026-07-05, manually]** — both work end-to-end, but this
   pass surfaced three new bugs that now block calling it "tested" in the
   automated sense: BUG-011 (no default session auto-created — high
   severity, contradicts documented behavior), BUG-012 (HITL-denial retry
   storm, no backoff/cap), BUG-013 (OpenVINO silently shows zero models on
   any non-Intel-GPU host — high severity, default-broken for most
   hardware). See bug-inventory.md. Fix these before turning the manual
   repro into a permanent automated test.
5. **Curated catalog Tier 1 smoke:** pull + load + chat one curated entry
   per backend (llama, openvino) via `model pull` — real network, run
   deliberately and once, not in every CI loop.
6. **ACP + HITL boundary tests:** acp/acpx session lifecycle and policy
   enforcement — flagged as high priority despite being untested today.
7. **UI passes (manual, this session cannot drive a browser reliably):**
   Beam UI smoke via `/run` skill or Playwright once available; VS Code
   extension manual walkthrough.
8. **Explicitly out of scope for this box, document rather than skip:**
   Tier 3 curated models (16-32GB), Windows/macOS builds, hosted-provider
   live tests (openai/gemini/openrouter/vertex) without credentials.

## Exit criteria for "v1 e2e tested"

Per the blueprint's acceptance bar: every row above is either `[tested]`
with a named test, or has an explicit removal/rescope decision recorded —
not left as a silent `[gap]`. This plan's job is done when every `[gap]`
in this document has become one of `[tested]`, `[known-issue]` (tracked in
bug-inventory.md with a fix decision), or `[out-of-scope-for-v1]` (with a
one-line reason).
