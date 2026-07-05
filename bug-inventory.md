# Contenox v1 Bug / Coverage Inventory

Companion to `testing-plan.md`. Populated from actual test runs executed
against a fresh build (`make build-modeld`, `make build-contenox`,
2026-07-05, Linux x86_64, RTX 3060 Laptop 6GB, backends: llama + openvino).

Status legend: **Open** (needs a decision/fix before v1) · **Fixed** (landed
this session) · **Tracked** (pre-existing, known, not newly found) ·
**By-design** (confirmed correct, documented here to close the loop).

---

## BUG-001 — `make -f Makefile.llamacpp-direct test` is a false green

**Status:** Open
**Severity:** High (testing-infrastructure integrity)
**Component:** `Makefile.llamacpp-direct` (`test-session` target)

**Description:** The `test-session` target runs
`go test -tags 'llamanode llamacpp_direct' ./modeld/llama/llamasession`
without setting `CONTENOX_LLAMA_TINY_GGUF`. Every real system test in that
package (`driver_system_test.go`, `eviction_test.go`, `gpu_test.go`,
`tiny_test.go`, `structured_system_test.go`, `bench_test.go`, and more — 20
tests total) gates on that env var via `requireTinyGGUF()`
(`modeld/llama/llamasession/tiny_test.go:220`) and silently `t.Skip()`s when
it's unset. Running `make -f Makefile.llamacpp-direct test` reports:

```
ok  	github.com/contenox/runtime/modeld/llama/llamasession	0.085s
```

0.085s for a package whose real suite takes ~45s when actually exercised.
The command exits 0 and looks identical to a real pass — nothing in the
Makefile output signals that zero real inference happened. A `.gguf` model
IS already in-tree at `.llamacpp-runtime/models/fastthink-0.5b-tiny-q2_k.gguf`
(338MB), so there is no reason this can't be wired by default.

**Verified fix path (ran manually this session, all green):**
```sh
CONTENOX_LLAMA_TINY_GGUF="$(pwd)/.llamacpp-runtime/models/fastthink-0.5b-tiny-q2_k.gguf" \
  make -f Makefile.llamacpp-direct test-session
```
→ 20 real tests PASS in ~45s (PrefillSuffixOverflowParksToCold, EvictRange,
EvictAdmitTail, DecodeSlidesPastNumCtx, GPU_Throughput,
StructuredToolCalls, StructuredJSONSchema, Tiny_SnapshotRestoreOneToken,
Tiny_WarmSuffixEqualsColdOneToken, Tiny_ToolsRenderThroughSession, bench
report warm-reuse, etc.) plus 4 legitimate skips for models genuinely not
in-tree (LoRA adapter, phi-4-mini, qwen3 — see BY-DESIGN-001).

**Recommendation:** default `CONTENOX_LLAMA_TINY_GGUF` in the Makefile to
the in-tree tiny model path (with a clear message if it's missing), same
pattern the OpenVINO `Makefile.openvino` already uses for
`CONTENOX_OPENVINO_TEST_MODEL`/`OPENVINO_MODEL`. Until fixed, anyone running
`make test-llamacpp-direct` and trusting the green checkmark is not
actually validating CUDA/llama.cpp inference at all.

---

## BUG-002 — ~40% of the HTTP API has zero apitest coverage

**Status:** Open
**Severity:** High
**Component:** `apitests/` (pytest suite) vs. `runtime/internal/openapidocs/openapi.json`

**Description:** Cross-referencing the 61 documented routes against the 18
existing pytest functions (`apitests/test_*.py`) leaves these entirely
untested at the API layer: `/chats*`, `/files*`, `/providers/*`,
`/setup-status`, `/setup/refresh`, **`/approvals/{approvalId}`**,
`/state`, `/task-events`, `/cli-config`, `/modeld/status`, all
Ollama-compat routes (`/api/chat`, `/api/generate`, `/api/ps`, `/api/show`,
`/api/tags`), and all OpenAI-compat routes (`/v1/*`, `/openai/*`,
`/openai/{chainID}/*`).

`/approvals/{approvalId}` is the HITL approval-gate endpoint — the
mechanism the product's README calls out by name ("human approval gates",
"'the model decided' is not an acceptable control boundary"). It has no
test today.

**Recommendation:** prioritize `/approvals` and one task-chain-execution
test (see BUG-003) above the rest; the compat shims and `/files`/`/chats`
can follow.

---

## BUG-003 — No test executes a task chain end-to-end

**Status:** Open
**Severity:** High
**Component:** `apitests/test_taskchains.py`, `runtime/taskengine`

**Description:** `test_taskchains.py` covers CRUD (create/get/update/delete)
and a validation check (path required) only. Nothing in the repo's
apitests or Go system tests actually **runs** a chain through
`POST /tasks` and asserts on branch conditions, retries, budgets,
tool-calling (`execute_tool_calls`), or a HITL approval gate firing
mid-chain. Chains are the product's core unit — "the Chain is the
contract" per the README — and the contract itself is untested end-to-end.

---

## BUG-004 — No *automated* live add+chat test for the two zero-cost local providers

**Status:** Open (manually verified this session; still needs to land as an automated test)
**Severity:** Medium
**Component:** CLI (`backend add`), API (`/backends`)

**Description:** `backend add`/CRUD is tested for all 8 provider types, but
per `docs/blueprints/product-surface-truth-blueprint.md` ("every provider
type... passes... a live add-plus-chat test"), none of them has one as an
automated test. Manually driving both this session (isolated
`CONTENOX_DATA_ROOT`/`--db`, in-tree tiny models, real `modeld serve`)
proved both paths *can* work end-to-end — but surfaced two real bugs along
the way (BUG-011 missing default session, BUG-013 OpenVINO GPU-probe
silently empties the catalog). See those entries; this one tracks turning
the manual repro into a permanent automated test now that the path is
proven.

---

## BUG-005 — ACP/ACPX policy enforcement is untested

**Status:** Open
**Severity:** Medium-High (safety boundary)
**Component:** `contenox acp` / `contenox acpx`, `hitl-policy-acp.json` / `hitl-policy-acpx.json`

**Description:** No automated test drives either ACP profile's JSON-RPC
session lifecycle or confirms the documented policy differences actually
hold at runtime — e.g., that `acpx` (untrusted/headless, OpenClaw profile)
really denies `local_shell` and gates web mutations as its docstring
claims. `runtime/contenoxcli` has unit tests asserting the *policy JSON
files themselves* contain the right invariants
(`TestUnit_SeededACPXPolicy_SecretInvariantAndHeavyDeltas`,
`TestUnit_InteractivePoliciesRequireApprovalForPlainShellFallback` — both
verified passing this session), but nothing drives an actual `acpx`
process and attempts a denied action to confirm enforcement, not just
configuration.

---

## BUG-006 — OpenVINO embed/reasoning system tests skip by default

**Status:** Tracked (confirmed this session, was already a known gap in `modeld-backend-parity-embeddings`)
**Severity:** Low-Medium
**Component:** `modeld/openvino/ovsession/embed_test.go`, `reasoning_test.go`

**Description:** Running the full `make -f Makefile.openvino test-genai`
suite this session (201.9s, otherwise all green):
```
--- SKIP: TestSystem_OpenVINOEmbed_GeneratesVectors (0.00s)
    embed_test.go:17: set CONTENOX_OPENVINO_EMBED_MODEL to run embedding tests
--- SKIP: TestSystem_OpenVINOGenAI_ReasoningStreaming (0.00s)
    reasoning_test.go:17: set CONTENOX_OPENVINO_REASONING_MODEL to run reasoning parser tests
```
Both need a model path env var not set by the Makefile target and not
pointing at anything in-tree. Confirms the same "capability-fields-that-lie"
gap already tracked in memory (embedding capability advertised but native
inference not re-exercised on this hardware).

---

## BUG-007 — LoRA CGo E2E still deferred

**Status:** Tracked (pre-existing, confirmed this session)
**Severity:** Medium
**Component:** `modeld/llama/llamasession` (LoRA), `modeld/openvino` (LoRA)

**Description:** This session's real-GGUF run confirmed
`TestSystem_LlamaSessionLoRA_AdapterChangesContinuation` and
`TestSystem_LlamaService_AdapterRequestChangesGeneration` both still
**SKIP** — `CONTENOX_LLAMA_LORA_GGUF`/`CONTENOX_LLAMA_LORA_ADAPTER` aren't
in-tree. Matches `lora-identity-attach-slice` memory: identity+native
attach wiring landed, but no real adapter has ever been exercised against
either backend on this box. Needs a small real LoRA adapter added to the
in-tree model set to close.

---

## BUG-008 — Capacity planner over-counts KV for sliding-window models

**Status:** Tracked (pre-existing, not re-verified this session)
**Severity:** Medium
**Component:** `modeld/capacity`

**Description:** Per `planner-ignores-swa-kv` memory: the planner charges
dense (full-context) KV cost for SWA/sliding-window models (Gemma-style),
over-counting by ~6x and clamping effective context far below what the
hardware can actually hold. Not re-verified in this session's runs (no
Gemma model in-tree); carried forward as-is.

---

## BUG-009 — OpenVINO cold-KV store unreachable via `OpenSession`

**Status:** Tracked (pre-existing, not re-verified this session)
**Severity:** Low-Medium
**Component:** `modeld/openvino`

**Description:** Per `effective-context-residency-loop` memory, the cold
store path exists but the `OpenSession` entrypoint can't reach it. Not
re-verified this session.

---

## BUG-010 — Snapshot design: fixed this session

**Status:** Fixed
**Severity:** was High (silent data loss + false-green test)
**Component:** `modeld/slot`, `runtime/transport`, `runtime/transport/grpc`

**Description:** Earlier in this working session: a change moved Llama
snapshot `State` off the gRPC wire (`json:"-"`) in favor of a
`StateID`-keyed blob cache on the daemon's local disk
(`modeld-blobs/<backend>/<id>.bin`, not the originally-claimed
`modeld/blobs/<backend>/`). The change broke a pre-existing wire test
(`TestLargeSnapshotRoundTripOverWire`) that still asserted raw `State`
crossed the wire — a real regression that the original report claimed
"tests pass cleanly" without having actually run.

**Fixed:** `runtime/transport/grpc/grpc_test.go` rewritten as
`TestSnapshotStateIDRoundTripOverWire`, asserting the new contract
(`StateID` + metadata cross the wire, raw `State` does not). Verified
green: `runtime/transport`, `runtime/transport/grpc`, `modeld/slot` all
pass.

**Still open as a sub-item:** the disk blob write/read round-trip itself
(`modeld/slot/service.go` `Snapshot`/`Restore`) has no test — the slot
package's fake session (`modeld/slot/service_test.go`) always returns an
empty snapshot, so `len(out.State) > 0` never triggers the blob-write path
in any test. Silent-failure risk noted in the earlier fact-check
(MkdirAll/WriteFile/Rename errors are logged but not surfaced — a failed
write leaves `State` populated but stripped by the JSON codec anyway,
producing an empty snapshot with no error).

---

## BUG-011 — Plain `contenox "<prompt>"` never creates the documented "default" session

**Status:** Open
**Severity:** High (contradicts documented core behavior, silent data loss)
**Component:** `runtime/contenoxcli` (chat path), `runtime/sessionservice`, `runtime/messagestore`

**Description:** `contenox --help` and `contenox chat --help` both state:
"Sessions persist conversation history across invocations (stored in
SQLite)... The first run auto-creates a 'default' session." This is false.

**Clean repro** (fresh isolated DB, no prior session, live llama backend +
in-tree tiny GGUF):
```sh
$ ./bin/contenox --db local2.db session list --all
No matching sessions.
$ ./bin/contenox --db local2.db "reply with exactly one word: apple"
Thinking...
fruit
$ ./bin/contenox --db local2.db session list --all
No matching sessions.
$ sqlite3 local2.db "SELECT count(*) FROM messages;"
0
```
The chat call succeeds (exit 0, real model output), yet zero session or
message rows exist afterward — not "default", not any name. First
reproduced non-cleanly (see BUG-012's repro) with a `work` session already
active, where `chat` *did* persist correctly once a session already
existed — isolating the bug specifically to the "no session exists yet"
path, i.e. the documented auto-create-on-first-run never fires.

**Impact:** any user who does exactly what the CLI's own quickstart tells
them to do (`contenox "say hello world in python"`) gets no persisted
history at all, contradicting "sessions persist conversation history
across invocations" — the CLI's headline stateful-chat feature is silently
a no-op for the most common first-touch invocation.

---

## BUG-012 — HITL denial has no backoff/cap; identical tool call retried ~10x until timeout

**Status:** Open
**Severity:** Medium-High
**Component:** `runtime/approvalflow`, `runtime/taskengine` (tool-call retry), CLI HITL prompt

**Description:** Running a prompt non-interactively (no stdin attached to
answer `Approve? [y/N]:`) against the tiny llama model with `local_fs`
tools available produced ~10 near-identical `find_files` HITL approval
prompts in rapid succession, each presumably read as an implicit denial
(EOF on stdin), before the surrounding `--timeout` finally killed the
chain with `context canceled`. The tool call was not meaningfully
different between rounds (same `pattern`, same `recursive`/`max_depth`,
minor arg-key reordering only).

**Repro:**
```sh
./bin/contenox --db local.db "the secret code is banana42"
# → 10 rounds of "HITL approval required / Tools: local_fs / Tool: find_files"
#   each unanswerable (no stdin), until: Error: chain execution failed:
#   ... task recovery_run: context canceled
```

**Impact:** in any environment where HITL prompts can't be answered
interactively (CI, scripted invocation, a piped/backgrounded chain), a
single denied tool call turns into a multi-round retry storm that only
ever terminates via the outer wall-clock timeout, not a bounded retry
count or a fail-fast "approval denied, aborting" path. This also means no
partial conversation state was persisted for the failed run (see BUG-011
context — messages stayed at 0 even for this longer, tool-using attempt).

---

## BUG-013 — OpenVINO GPU-device telemetry probe failure silently empties the entire local model catalog

**Status:** Open (workaround identified, verified this session)
**Severity:** High (breaks the OpenVINO local-inference feature by default on any non-Intel-GPU host)
**Component:** `runtime/modelrepo/openvino/catalog.go`, `modeld/openvino` capacity probe

**Description:** On this box (NVIDIA RTX 3060, no Intel GPU), starting
modeld in OpenVINO mode with default device selection and running
`contenox model list` against a directory with 4 valid, complete OpenVINO
IR models (`openvino_model.xml` + `config.json` present in each) reports:
```
BACKEND  MODEL        CHAT  EMBED  PROMPT  THINK  CTX
ov       (no models)
No loadable models found on any live backend.
```
`doctor` shows the backend as "reachable" with "0 chat model(s)" — no
error surfaced anywhere in the CLI output.

**Root cause (found via a temporary debug build — reverted, `git diff` is
clean):** `catalog.go`'s `ListModels` calls `modeldconn.Describe()` per
model to get live capability data; when that RPC errors *and* modeld is
otherwise live, the code intentionally omits the model
("modeld is live but cannot describe THIS model — it is genuinely
unusable, so omit it rather than advertise a broken model" —
`catalog.go:77-80`). The `Describe()` error, for all 4 models, was:
```
rpc error: code = Internal desc = internal: openvino capacity memory probe:
OpenVINO device "GPU" reported no memory telemetry; set
CONTENOX_OPENVINO_DEVICE=CPU or use a plugin exposing device memory
```
OpenVINO's default device selection targets "GPU" (Intel iGPU/dGPU in
OpenVINO's terminology), which doesn't exist on this NVIDIA-only host. The
capacity/memory probe fails, `Describe()` returns an `Internal` error with
an accurate, actionable message and fix — **and that message is discarded
entirely** by the catalog's omit-and-continue path. Setting
`CONTENOX_OPENVINO_DEVICE=CPU` on modeld immediately fixed it: all 4
models appeared with correct capabilities, and live chat through
`qwen-coder-0.5b-int4` worked end to end.

**Impact:** any user whose machine has no Intel GPU (the overwhelming
majority of non-Intel laptops/desktops, e.g. any NVIDIA/AMD discrete-GPU
or Apple Silicon-hosted-via-VM setup) gets a completely silent, totally
opaque "no models found" for the entire OpenVINO backend, with the actual
diagnosis and one-line fix sitting in a discarded gRPC error. This
directly violates the product-surface-truth invariant ("setup/health
output never presents a dormant-but-healthy... backend as an error state")
in the opposite direction: a *fixable* backend is presented as having no
models at all, with zero guidance.

**Recommendation:** surface `Describe()`'s error (or at least its message)
through `doctor`/`model list` diagnostics instead of silently omitting the
model, and/or have modeld's OpenVINO backend auto-fall-back to CPU when
the GPU device reports no memory telemetry rather than requiring a manual
env var.

---

## OBSERVATION-001 — Background `classify_request` task reported an anomalous context-window error

**Status:** Needs investigation (not root-caused; flagging for follow-up, not blocking)
**Component:** `runtime/taskengine` route/classify handler, capacity planning

**Description:** `contenox state show <reqID>` for a successful OpenVINO
chat call (model advertised 32768 ctx, declared via `model list`) showed:
```
TASK              HANDLER  STATUS
contenox_chat     chat_completion  OK
classify_request  route            ERROR: exceeded the session context window:
  model "qwen-coder-0.5b-int4" serves only 122 context tokens on the system
  device with 6.1 GiB free after model weights
```
122 tokens is wildly inconsistent with the model's declared 32768-token
context. The overall chat request still succeeded (the user got a
response), so this looks like a background/parallel classification step
failing independently without surfacing to the user — but the "122
context tokens" figure and "6.1 GiB free" wording are suspicious enough
(similar free-memory figures appeared in this session's separate llama/CUDA
run) to warrant checking whether the OpenVINO capacity planner is reading
a stale or wrong-backend capacity snapshot for this secondary task path.
Not chased further this session — flagging with the exact repro (same
scratch DB/data-root, request ID pattern `cli-*`) for whoever picks this
up.

---

## BY-DESIGN-001 — Model-gated system test skips are correct behavior

**Status:** By-design
**Component:** `modeld/llama/llamasession`, `modeld/openvino/ovsession`

**Description:** Distinguishing from BUG-001: once `CONTENOX_LLAMA_TINY_GGUF`
is set, 4 tests still correctly `t.Skip()` because they need models that
are genuinely not in-tree and shouldn't be auto-downloaded by a test run:
`TestSystem_LlamaSessionLoRA_AdapterChangesContinuation`,
`TestSystem_LlamaSession_Phi4Mini_PrefixStable`,
`TestSystem_LlamaChatParser_QwenThinkingStreamTolerated`,
`TestSystem_LlamaService_AdapterRequestChangesGeneration`. This is correct
— the bug was the Makefile not wiring the one model that IS in-tree, not
these skips.

---

## BY-DESIGN-002 — Raw `bin/modeld serve` needs `CONTENOX_LLAMA_BACKEND_DIR` set manually; this is intentional

**Status:** By-design (documented in code, cost real debugging time this session — worth a sharper error message)
**Component:** `modeld/llama/llamacppshim/direct.go`, `Makefile` (`run-modeld`, packaged wrapper)

**Description:** Launching `bin/modeld serve` directly (not via
`make run-modeld` or the packaged/installed wrapper) with the llama
backend fails every model load with:
```
llama_model_load_from_file_impl: no backends are loaded. hint: use
ggml_backend_load() or ggml_backend_load_all() to load a backend before
calling this function
```
This is because `GGML_BACKEND_DL` plugins (`libggml-cpu-*.so`,
`libggml-cuda.so`, `libggml-hip.so`) are `dlopen`'d at runtime, not linked,
and ggml only searches the executable's own directory and cwd by default
— neither of which is `.llamacpp-runtime/local/lib`. `make run-modeld` and
the packaged wrapper both set `CONTENOX_LLAMA_BACKEND_DIR` correctly
(`modeld/llama/llamacppshim/direct.go:82-87` documents this explicitly).
Confirmed **not a bug** — real end users go through one of those two
paths, never raw `bin/modeld serve`. Noting it here only because the
failure mode (silent "no backends loaded", not "set this env var") cost
real time to diagnose and would do the same to any contributor who runs
the binary directly during dev, same category as BUG-001's
discoverability problem. A one-line startup check in `modeld serve` itself
("llama backend selected but CONTENOX_LLAMA_BACKEND_DIR is unset and no
plugin was found in the executable directory — see `make run-modeld`")
would close this without changing any behavior.

---

## Verified this session — hands-on CLI/API sweep (`contenox chat`/`run`/`session`/`config`/`state`/`cache`/`mcp`, real modeld)

Executed against an isolated `CONTENOX_DATA_ROOT` + scratch `--db` (never
touched the real `~/.contenox`), real `modeld serve` for both backends, the
in-tree tiny GGUF and all 4 in-tree OpenVINO IR models.

| Surface | Result |
|---|---|
| `doctor` / `doctor --json` / `doctor --skip-cycle` | clean, consistent, correct error/warning reporting at every state (0 backends, 1 backend, reachable/unreachable) |
| `backend add llama` + `model list` + live `chat` | works end-to-end once `CONTENOX_LLAMA_BACKEND_DIR` is set correctly (BY-DESIGN-002) |
| `backend add openvino` + `model list` + live `chat` | works end-to-end once `CONTENOX_OPENVINO_DEVICE=CPU` is set (BUG-013 — silent without this) |
| `run --input-type string/chat` (bare + explicit) | executes; tiny-model output quality is poor (expected, Q2_K 0.5B) but no crashes |
| `run --input-type int/json` | correctly errors (no default chain accepts non-string/chat input) — expected, not a bug |
| `session new/list/show/workspaces` (once a session exists) | all correct |
| session auto-create on first bare/`chat` invocation | **broken, see BUG-011** |
| HITL approval loop under non-interactive stdin | **unbounded retries until outer timeout, see BUG-012** |
| `config set/get` (valid + invalid keys + invalid enum values) | clean validation, good error messages, no crashes |
| `state list` / `state show <reqID>` | works; surfaced OBSERVATION-001 |
| `cache clear` | clean, no crash |
| `mcp add` (stdio) / `mcp list` / `mcp show` | clean CRUD, correct JSON output |
| `config set hitl-policy-name` | clean, correctly scoped as workspace-level |

## Verified-green this session — automated suites (for the record — not bugs, closes out testing-plan items)

| Suite | Result |
|---|---|
| `go test -short -run '^TestUnit_' ./...` | all green, zero failures |
| `make test-contenox-help` | 13/13 subcommands present, version matches |
| `go test -v ./runtime/contenoxcli/...` | all green incl. HITL policy invariant tests |
| `make -f Makefile.llamacpp-direct test` (shim only; session gated, see BUG-001) | shim green, session false-green |
| `CONTENOX_LLAMA_TINY_GGUF=... test-session` (manual) | 20/20 real tests green, 4 correctly skipped |
| `make -f Makefile.openvino test-genai` | `modeld/openvino` (20.8s) + `modeld/openvino/ovsession` (201.9s), all green, 2 correctly skipped (BUG-006) |
| Real prefix-cache warm-reuse measurement | cold 2m31s → warm 614ms on a 166KB prefix (99.6% speedup) — reconfirms `openvino-kv-snapshot-finding` |
| `modeld version` on fresh build | `backends: llama, openvino` — both compiled in correctly |
