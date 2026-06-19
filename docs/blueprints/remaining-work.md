# Remaining Work — local-node roadmap (post Phase 5)

> Status note (this session): the codebase was replaced with the completed `modeld`
> refactor and the following landed + verified: daemon wiring (cmd/modeld now imports
> `modeld/llama/llamasession`), manifest token-range population, the common benchmark
> harness (`runtime/benchreport` + real llama driver), the OpenVINO in-flight cancel
> test, and **model-native llama tool calls** (owned minja engine in
> `modeld/llama/chattmpl`, tools across the transport, output parser).
>
> Build/test recipe for the tagged llama path:
> `make deps-llama-headers build-modeld-llama test-llamacpp-direct-cpu`, then
> `CGO_ENABLED=1 CGO_CPPFLAGS="-I$PWD/.llamacpp-vendor -I$PWD/.llamacpp-runtime/cpu/include" CGO_LDFLAGS="-L$PWD/.llamacpp-runtime/cpu/lib -Wl,--disable-new-dtags -Wl,-rpath,$PWD/.llamacpp-runtime/cpu/lib -Wl,-rpath-link,$PWD/.llamacpp-runtime/cpu/lib -l:libllama.so -l:libggml.so -l:libggml-base.so -l:libggml-cpu.so -lstdc++ -lm -ldl -lpthread" go test -tags 'llamanode llamacpp_direct' ./modeld/llama/...`
> OpenVINO: model at `.openvino/models/qwen-coder-0.5b-int4`; flags resolved from the
> `.openvino/venv` like `Makefile.openvino` (`make -f Makefile.openvino test-s1-5`).

> Update (2026-06-18): landed + verified (pure-Go build/vet/tests green) —
> - **`make build-modeld` wired:** CGO flags via shared `mk/openvino-flags.mk` +
>   `mk/llama-flags.mk`; `make deps-modeld` reproduces deps; `.llamacpp-vendor`
>   gitignored. Parallelism capped (`-p`) to avoid OOM.
> - **modeld single-backend made coherent:** `cmd/modeld` registry picks one
>   backend (`CONTENOX_MODELD_BACKEND` / preference); the owner lease + gRPC health
>   advertise it; `modeldprobe.Status.Backend` + `modeldconn.Backend()` let the
>   runtime detect the daemon's mode and gate each provider's `SessionAvailable()`.
> - **Typed transport handle:** `transport.OpenSessionRequest` now carries
>   `{ModelName, Type, Digest, Path}` (was a raw `ModelID` path); modeld validates
>   `Type` against its served backend (`transport.ErrBackendMismatch`).
> - **Both local backends registered:** `contenox init` registers `llama` AND
>   `openvino` at per-type dirs `~/.contenox/models/<type>/`; `openvino` is a
>   first-class backend type (validation + `runtimestate` dispatch).
> - **OpenVINO IR `model pull`** (handover Step 7): curated `-ov` registry entries +
>   multi-file HF-Hub HTTP fetch into `models/openvino/<name>/`.
> - **#6 below: eviction + class foundation done** (admission policy deferred to #7).

The remaining tracks are independent unless noted. Each lists where to work and how.

---

## #6 Semantic cache admission/eviction  (PARTIALLY DONE 2026-06-18)

**Goal:** replace plain-LRU with the coding-aware priority from
`local-coding-node-goals.md` (highest: system/tool schemas/repo instructions … low:
stale logs/old turns). Pin core segments for the workspace session; admit volatile
suffix material only when likely reused.

**DONE — eviction + class foundation:**
- **Bounded session cache** (`runtime/modelrepo/warmcache.go`, generic
  `WarmCache[S]`): idle-TTL reap + resident cap + LRU, never evicting a mid-turn
  session, closing evicted sessions so modeld releases the model. Both providers'
  previously-unbounded `sessionCache` in `{llama,openvino}/client.go` now share it.
  This fixed the model-switch leak (every distinct model used to stay resident in
  modeld until OOM). Verified in `warmcache_test.go`.
- **Cache-class types** (`runtime/contextasm/segments.go`): `CacheClass`
  (`task_pinned`/`repo_map`/`volatile`), `SegmentKind.CacheClass()`,
  `MoreEvictableThan`; `ManifestSegment` carries `CacheClass` + `Invalidation`
  (`manifest.go`), populated by `AssembleManifest` (additive; byte hashes unchanged).

**DEFERRED to #7 (no producer yet):** the budget-aware **admission** policy —
`AdmitSegments(segs, tokenBudget)` dropping highest-evictable classes first to fit
the window — plus its wiring into the chat assembler. Nothing produces the rich
segment kinds (`KindRepoMap`/`KindDiff`/`KindTerminal`) today; the chat providers
use a coarse role-based stable/volatile split. The #7 T3 planner is the producer;
only then can the policy be exercised + benched (`benchreport` `hit_rate` vs LRU).

---

## #2 Snapshot/restore wiring  (now unblocked — Phase 3 numbers exist)

**Goal:** L0 snapshot round-trip on the live session (suspend/resume, branch, crash
recovery). The blueprint L3 said decide *after* L0/L2 numbers — those now exist
(`benchreport`), so proceed.

**Where:**
- The native byte-state primitives live in
  `modeld/llama/llamacppshim/direct.go` — `StateSeqGetData` /
  `StateSeqSetData` are present; file save/load helpers still need direct shim
  wrappers if benchmark output should avoid moving large KV blobs through Go.
- Contract: extend `runtime/transport/session.go` `Session` with
  `Snapshot()/Restore()` (or `SaveState/LoadState`). This crosses the gRPC boundary —
  add methods in `runtime/transport/grpc/{wire,server,client}.go` (mirror the existing
  unary methods; the JSON codec handles structs, but raw KV bytes are large — consider
  a streamed or base64 field).
- `modeld/llama/llamasession/llama.go` — implement save/restore via the
  `llamacppshim` state funcs; gate restore on `ContextManifest` compatibility (reuse
  `contextasm` CompatibleRuntime + stable token hash) — refuse a mismatched snapshot.

**Hints:** kill gates are in `docs/blueprints/llamacpp-binding-ownership-options.md`
("Kill Gates"): tiny GGUF, non-empty state bytes, restore into fresh context, greedy
continuation equals original, wrong-manifest rejected, double-close safe. Add the
`snapshot_save`/`snapshot_restore` fields to `benchreport.Report` (already stubbed in
the shape) and fill them from a real round-trip test.

---

## #7 T3 context planner + coding-context eval gate  (large; needs GPU for latency)

**Goal:** prove "effective 200k" is real — the planner selects/cites/edits across more
text than the hot window holds.

**Where (new):** a planner package, `PlanTurn(workspace, task) -> []contextasm.Segment`
(repo map, symbol graph, pins, retrieval, summaries, diff/test/log budgeter) feeding the
existing `EnsurePrefix/PrefillSuffix/Decode`.

**Hints:** the segment/manifest plumbing is done (`contextasm`), so the planner only
emits `[]Segment` with cache classes (ties into #6). Build the eval harness for the gate
tasks in `local-coding-node-goals.md` ("Coding-context eval gate": cross-file bug
localization, edit-A-from-B/C, trace failing test, large refactor, architecture Q with
citations); record pinned/retrieved/summarized/cached/missed/cited per task. Latency
go/no-go needs the budget GPU; correctness/selection can be evaluated on CPU + tiny model.

---

## #4 OpenVINO capability gaps  (model-gated — need extra models to verify here)

Done: deterministic in-flight cancel test (`modeld/openvino/ovsession/genai_test.go`);
**OpenVINO is now a first-class registrable backend type** (2026-06-18) — `contenox
init` registers it, `backendservice` validation accepts it, `runtimestate` dispatches
it, and `contenox model pull` fetches curated IR models (see handover Step 7).
Remaining, each blocked on a model this env doesn't have:

- **Embeddings** — wire `TextEmbeddingPipeline` in `modeld/openvino/ovsession` for
  profiles that declare an embedding model; flip `runtime/modelrepo/openvino/provider.go`
  `CanEmbed()`/`GetEmbedConnection` off the not-wired stub. Mirror the llama
  `EmbedFunc` seam (`modeld/llama/session.go` `SetEmbedFunc`). *Needs an embedding IR
  model (e.g. bge-*) — the chat model can't verify it.*
- **Streaming incremental-reasoning parser bridge** — support the `*_incremental_*`
  protocols registered in `modeld/openvino/ovsession/genai.go`. *Needs a reasoning model
  (DeepSeek-R1 / Phi-4 reasoning) to verify.*
- **VLLMParserWrapper** — formalize: either a Python parser-object bridge or keep the
  explicit native-bridge error, documented (per `openvino-s2-7-protocol-registry.md`).
- **Per-model-family profiles** — ship `contenox-openvino.json` files declaring the
  right tool/reasoning protocol per shipped model.
- **Idle session GC** — the *runtime* side is now handled (#6): the bounded
  `modelrepo.WarmCache` reaps idle/over-cap sessions and `Close()`s them, so modeld
  releases the model. Still worth confirming the *daemon* side frees promptly on
  `Close` (lifecycle is via `OpenSession`/`Close` since the in-process pool was removed).

**Hint:** to verify locally, `make -f Makefile.openvino model` downloads the chat model;
embeddings/reasoning need their own `snapshot_download` of an appropriate IR.

---

## #5 follow-up: tool-result history  (extends the shipped model-native tool calls)

Sending tool *results* back in a multi-turn (`role:"tool"` messages, assistant messages
with `tool_calls`) is still rejected at `runtime/modelrepo/llama/prompt.go`
`validateMessage`. minja already renders these, so the work is:
- carry `tool_calls` / `tool` role + tool-call ids through the message→segment mapping
  in `prompt.go` and the modeld `stableMessages`/`volatileMessages` reconstruction
  (`modeld/llama/llamasession/llama.go`) — currently only `system/user/assistant`
  text roles survive;
- pass them as structured JSON to `chattmpl.Render` (minja consumes `messages[].tool_calls`
  and `role:"tool"`), not flattened text.

---

## Done: Contenox-owned llama.cpp shim

The live llama path now runs on `modeld/llama/llamacppshim` behind the
`llamacpp_direct` build tag. The old `llama_unsafe_abi` spike and
`modeld/llama/llamaabi` package are retired. Remaining work on this track is
GPU/runtime certification, snapshot transport, and benchmark reporting, not
replacement of the binding itself.
