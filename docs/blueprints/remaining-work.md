# Remaining Work — local-node roadmap (post Phase 5)

> Status note (this session): the codebase was replaced with the completed `modeld`
> refactor and the following landed + verified: daemon wiring (cmd/modeld now imports
> `modeld/llama/llamasession`), manifest token-range population, the common benchmark
> harness (`runtime/benchreport` + real llama driver), the OpenVINO in-flight cancel
> test, and **model-native llama tool calls** (llama.cpp common chat template
> path, tools across the transport, output parser).
>
> Build/test recipe for the tagged llama path:
> `make build-modeld-llama test-llamacpp-direct-cpu`, then
> `CGO_ENABLED=1 CGO_CPPFLAGS="-I$PWD/tmp/ref/llama.cpp/common -I$PWD/tmp/ref/llama.cpp/vendor -I$PWD/.llamacpp-runtime/cpu/include" CGO_LDFLAGS="-L$PWD/.llamacpp-runtime/cpu/lib -Wl,--disable-new-dtags -Wl,-rpath,$PWD/.llamacpp-runtime/cpu/lib -Wl,-rpath-link,$PWD/.llamacpp-runtime/cpu/lib -l:libcommon.a -l:libllama.so -l:libggml.so -l:libggml-base.so -l:libggml-cpu.so -lstdc++ -lm -ldl -lpthread" go test -tags 'llamanode llamacpp_direct' ./modeld/llama/...`
> OpenVINO: model at `.openvino/models/qwen-coder-0.5b-int4`; flags resolved from the
> `.openvino/venv` like `Makefile.openvino` (`make -f Makefile.openvino test-s1-5`).

> Update (2026-06-18): landed + verified (pure-Go build/vet/tests green) —
> - **`make build-modeld` wired:** CGO flags via shared `mk/openvino-flags.mk` +
>   `mk/llama-flags.mk`; `make deps-modeld` reproduces deps. Parallelism capped
>   (`-p`) to avoid OOM.
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

## Done: Snapshot/restore wiring

L0 live snapshot round-trip is wired through `transport.Session` and the gRPC
transport, with `modeld/llama/llamasession` restoring into a fresh llama.cpp
context and matching the original greedy continuation on a tiny GGUF.

The session snapshot uses full llama.cpp context state (`llama_state_get_data` /
`llama_state_set_data`) through `modeld/llama/llamacppshim/direct.go`, not only
`llama_state_seq_*`, because sequence state restores KV memory but not the last
logits buffer needed for exact next-token equality.

Remaining optional work: add file save/load wrappers if benchmark output should
avoid moving large state blobs through Go, and fill the `snapshot_save` /
`snapshot_restore` fields in `benchreport.Report`.

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
Embedding transport/provider wiring is done: profiles opt in with `can_embed`,
runtime calls `modeldconn.Embed`, and modeld runs OpenVINO GenAI
`TextEmbeddingPipeline` as a stateless request. System verification passed with
`.openvino/models/bge-small-ov`. Stream/tool history parity is done: modeld
receives assistant `tool_calls` and tool-result IDs in chat-template rendering,
and tool-enabled streams emit a final parsed `ToolCalls` parcel.
Streaming incremental reasoning is also wired: `DecodeConfig.ParserProtocols`
reaches OpenVINO GenAI, native incremental parsers split `reasoning_content`
into `StreamChunk.Thinking`, and the runtime only displays it when `WithThink`
requests a visible level. Verified with
`.openvino/models/deepseek-r1-distill-qwen-1.5b-int4-ov`.
Remaining:

- **VLLMParserWrapper** — formalize: either a Python parser-object bridge or keep the
  explicit native-bridge error, documented (per `openvino-s2-7-protocol-registry.md`).
- **Per-model-family profiles** — ship `contenox-openvino.json` files declaring the
  right tool/reasoning protocol per shipped model.
- **Idle session GC** — the *runtime* side is now handled (#6): the bounded
  `modelrepo.WarmCache` reaps idle/over-cap sessions and `Close()`s them, so modeld
  releases the model. Still worth confirming the *daemon* side frees promptly on
  `Close` (lifecycle is via `OpenSession`/`Close` since the in-process pool was removed).

**Hint:** to verify locally, `make -f Makefile.openvino model` downloads the chat model;
set `CONTENOX_OPENVINO_EMBED_MODEL` to a BGE OpenVINO IR and
`CONTENOX_OPENVINO_REASONING_MODEL` to an official DeepSeek/Phi reasoning IR with
`openvino_tokenizer.xml`.

---

## Done: tool-result history

Tool results in multi-turn history (`role:"tool"` messages, assistant messages
with `tool_calls`) now flow through the message→segment mapping and modeld
reconstruction. The llama backend renders them via llama.cpp's common chat
template layer, so the same upstream path handles tool definitions, assistant
tool calls, tool result IDs, and model-specific template handlers.

---

## Done: Contenox-owned llama.cpp shim

The live llama path now runs on `modeld/llama/llamacppshim` behind the
`llamacpp_direct` build tag. The old `llama_unsafe_abi` spike and
`modeld/llama/llamaabi` package are retired. Remaining work on this track is
GPU/runtime certification, snapshot transport, and benchmark reporting, not
replacement of the binding itself.
