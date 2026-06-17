# Remaining Work — local-node roadmap (post Phase 5)

> Status note (this session): the codebase was replaced with the completed `modeld`
> refactor and the following landed + verified: daemon wiring (cmd/modeld now imports
> `modeld/llama/llamasession`), manifest token-range population, the common benchmark
> harness (`runtime/benchreport` + real llama driver), the OpenVINO in-flight cancel
> test, and **model-native llama tool calls** (owned minja engine in
> `modeld/llama/chattmpl`, tools across the transport, output parser).
>
> Build/test recipe for the tagged llama path:
> `make -f Makefile.llamacpp vendor-headers` then
> `CGO_CPPFLAGS=-I$PWD/.llamacpp-vendor CONTENOX_LLAMA_TINY_GGUF=/home/naro/.libollama/models/tiny/model.gguf go test -tags 'llamanode llama_unsafe_abi' ./modeld/llama/...`
> OpenVINO: model at `.openvino/models/qwen-coder-0.5b-int4`; flags resolved from the
> `.openvino/venv` like `Makefile.openvino` (`make -f Makefile.openvino test-s1-5`).

The remaining tracks are independent unless noted. Each lists where to work and how.

---

## #6 Semantic cache admission/eviction  (pure-Go, verifiable here — recommended next)

**Goal:** replace plain-LRU with the coding-aware priority from
`local-coding-node-goals.md` (highest: system/tool schemas/repo instructions … low:
stale logs/old turns). Pin core segments for the workspace session; admit volatile
suffix material only when likely reused.

**Where:**
- `runtime/contextasm/manifest.go` (`ManifestSegment`) — add a `CacheClass` field
  (e.g. `task_pinned`, `repo_map`, `volatile`) + an `Invalidation` hint.
- A new policy type in `contextasm` (or `runtime/modelrepo/llama/client.go`'s
  `sessionCache`) that decides admit/evict by class, not recency.

**Hints:** the session cache today is `client.go: sessionCache` (keyed by
model+config). The eviction decision is a good seam. Keep it backend-neutral by
putting the policy in `contextasm` and having both backends consult it. Verify with
unit tests + the Phase 3 `benchreport` `hit_rate` improving on a repeated-workspace
scenario vs an LRU baseline.

---

## #2 Snapshot/restore wiring  (now unblocked — Phase 3 numbers exist)

**Goal:** L0 snapshot round-trip on the live session (suspend/resume, branch, crash
recovery). The blueprint L3 said decide *after* L0/L2 numbers — those now exist
(`benchreport`), so proceed.

**Where:**
- The native primitives already exist: `modeld/llama/llamaabi/llamaabi.go` —
  `StateSeqGetData` / `StateSeqSetData` / `StateSeqSaveFile` / `StateSeqLoadFile`
  (currently unused).
- Contract: extend `runtime/transport/session.go` `Session` with
  `Snapshot()/Restore()` (or `SaveState/LoadState`). This crosses the gRPC boundary —
  add methods in `runtime/transport/grpc/{wire,server,client}.go` (mirror the existing
  unary methods; the JSON codec handles structs, but raw KV bytes are large — consider
  a streamed or base64 field).
- `modeld/llama/llamasession/llama.go` — implement save/restore via the llamaabi
  StateSeq funcs; gate restore on `ContextManifest` compatibility (reuse
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

Done: deterministic in-flight cancel test (`modeld/openvino/ovsession/genai_test.go`).
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
- **Idle session GC** — confirm the daemon garbage-collects idle daemon-held sessions
  (the old in-process pool was removed in the refactor; lifecycle is now via
  `OpenSession`/`Close`).

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

## Deferred: Contenox-owned llama.cpp shim  (replace the `llama_unsafe_abi` spike)

The live llama path runs on `modeld/llama/llamaabi` — a quarantined `unsafe` read of
ollama's private `llama.Context`, behind `llama_unsafe_abi`. It works and is tested, so
this is a hardening/robustness item, not a functional blocker. The decode-status fidelity
gap (ollama's `Decode` collapses `no_kv_slot`/`aborted`/`fatal`) also rides here.

**Hint:** `docs/blueprints/llamacpp-binding-ownership-options.md` is the spec
(`runtime/modelrepo/llama/llamacppshim/` → now `modeld/llama/llamacppshim/`). Note the
minja vendoring done for tool calls (`Makefile.llamacpp` pinned to commit `ec98e2002`,
`modeld/llama/chattmpl`) is the same pattern this shim will use to own its llama.cpp
sources rather than piggyback ollama's stripped module.
