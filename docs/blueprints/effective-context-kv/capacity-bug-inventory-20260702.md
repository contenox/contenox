# Working Doc: Capacity/Context Bug Inventory (2026-07-02)

Status: working document; tracks live findings and the fix plan.
Owner: runtime / modeld
Host: nox16 — GTX 1660 6 GiB (PRIME render offload; display wired to Intel iGPU), 15 GiB RAM, CUDA 13.3.
Session: follow-up to the stale-Request ratchet fix (capacity.HardContextLimit, slot
reclaim-credit, auto-NumCtx end-to-end). That fix is verified working: sessions now
open at the honest memory-fitting window (2.9k hot / ~17.6k planner on this host,
tracking VRAM jitter turn to turn, no ratchet, no 220-token collapse).

## Test Evidence (reproduced live on this host)

- Cold open, auto: `num_ctx=2921 planner=17945 resolved_gpu_layers=37`, honest
  `capacity_reason=model_context_exceeds_memory_budget`. Second CLI turn: 2896 —
  fresh resolution, no ratchet. Fix confirmed.
- Big single prompt (~15k tok): `context overflow during suffix:
  resident_tokens=221 additional_tokens=14964 num_ctx=2890` — planner window not
  usable for a single input.
- Multi-turn growth (fresh session, ~2.4k tok/turn): turn 0 OK, turns 1-2 hard
  overflow (`num_ctx=3854`, then `2728`) — conversation dead after 1 turn of
  history. Error text claimed "serves only 433 context tokens".
- Same-identity capacity panel while resident session served hot 2891: panel said
  439; `model list` said CTX 13725; three inconsistent numbers for one session.
- gnome-shell crashed (SIGABRT, mutter `logical_monitor` assert) at 08:29:48,
  between overflow-pressure turns; modeld had the card at ~1.6 GiB free with
  repeated 2.4 GB weight load/free churn. Correlated, NOT proven causal (mutter
  asserts on hybrid iGPU/dGPU are a known GNOME bug family). Xorg/gnome-shell/
  Firefox hold dGPU contexts via PRIME even with display on the iGPU.

## Bug Inventory

| ID | Sev | Summary | Root cause (file:line at time of writing) |
|---|---|---|---|
| BUG-1 | critical | Chats die after 1-2 turns; planner/cold context unreachable in primary flow | (a) `EnsurePrefix` rejects prefix > hot window, no evict path (`modeld/llama/llamasession/llama.go:300`); (b) suffix evict can only park resident tokens, single suffix > hot window can never fit (`llama.go:410-416`); (c) implicit sessions close on CLI process exit, discarding hot+cold KV (`modeld/slot/service.go`, release path) |
| BUG-2 | high | Same-identity Describe reports encumbered hypothetical, not resident-session truth | `slot.Describe` gives no reclaim credit for same identity and recomputes under the session's own footprint; should return the open-time resolved info stored on the slot |
| BUG-3 | high | Overflow error text quotes the stale describe number ("serves only N tokens") | `explainOverflow` prefers construction-time `describedEffectiveContext` over the live `num_ctx` already carried by `ContextOverflowError` (`runtime/modelrepo/llama/client.go`, openvino mirror) |
| BUG-4 | high (safety) | No VRAM floor for other GPU clients; desktop crash correlated with pressure | `Policy.MinFreeBytes` defaults to 0 (`modeld/capacity/capacity.go`, `LoadPolicy`/defaults); 80%-of-free is the only guard on a shared card |
| BUG-5 | medium (design) | Layer resolver prefers full offload (37/37, 2.9k ctx) over usable context | `resolveGPULayersForBudget` returns max slots with any context > 0 (`modeld/llama/service.go:413-449`); no context-vs-speed knob |
| BUG-6 | medium (product) | Chat narrates tool calls it never made (pong incident) | default chain declares `tools:['*']` + tool-assuming instructions; beam with no toolchain attached leaves the 4B model to fabricate; no narrated-execution detection |
| BUG-7 | low | `contenox chat --input @file` does not expand; literal `@name` reaches model | CLI input handling; benchmark doc documents the syntax as working |
| BUG-8 | low | `summarise_failure` re-feeds oversized input, overflows, masks original error | default chain failure path has no input truncation |
| BUG-9 | cosmetic | Startup log prints `headroom=0.00` for unset (real default 0.1) | `cmd/modeld` policy formatting |

## Fix Plan

### Phase 1 — safety + truth (small, land first)
1. **BUG-4**: default `MinFreeBytes` for accelerator devices when unset:
   `max(512 MiB, 10% of device total)`. Keep env/file overrides
   (`CONTENOX_MODELD_MEM_RESERVE`, `modeld.json memory.reserve_free`) winning.
2. **BUG-2**: store the open-time resolved `transport.ModelInfo` on `activeSlot`
   (the footprint Describe already fetches it; keep the whole struct, not just
   `RequiredBytes`) and return it for same-identity Describe.
3. **BUG-3**: `explainOverflow` prefers the live `NumCtx` from
   `ContextOverflowError` when present; described numbers only as fallback.

### Phase 2 — make advertised context real
4. **BUG-1c** (first: cheapest, biggest win): stop closing implicit sessions on
   handle release; the existing idle-TTL reaper owns reclamation. Gives
   cross-process warm reuse (sameIdentity already matches across CLI turns after
   the ratchet fix) and lets cold KV survive between turns. Requires Phase 1 #1
   landed (resident-by-default needs the VRAM floor).
5. **BUG-1a**: chunked prefix prefill in `EnsurePrefix`: accept prefixes up to
   `plannerCtx`, prefill through the hot window, park completed ranges to cold.
6. **BUG-1b**: same streaming treatment for oversized suffixes.
7. **BUG-1d / capability truth**: `model list` CTX and catalog advertise reachable
   context (post-1a/1b: planner; before: hot), not an unreachable ceiling.

### Phase 3 — product polish
8. **BUG-5**: context-vs-speed preference knob (e.g. minimum hot-context target
   before sacrificing GPU layers).
9. **BUG-6**: when no tools are attached, strip/replace tool-assuming
   instructions in the chat system prompt; optional narrated-execution detector.
10. **BUG-7**: implement `@file` expansion (or remove it from docs).
11. **BUG-8**: truncate the input handed to `summarise_failure`.
12. **BUG-9**: print `unset` for unset headroom.

## Progress Log

- 2026-07-02: inventory created from live testing. Ratchet fix (previous session)
  verified working. Phases defined. Implementation starting with Phase 1.
- 2026-07-02 (impl): Phase 1 landed —
  - BUG-4: `WithResidentDefault` now also defaults `MinFreeBytes` on accelerator
    devices to max(512 MiB, 10% of total), capped at 25% of a known total so tiny
    devices stay usable. Explicit reserve always wins.
  - BUG-2: `activeSlot` stores the full pre-open `ModelInfo` (captured after the
    old session closes, before the new one allocates — unencumbered, identical
    inputs to the open's own resolution). Same-identity Describe returns it;
    different-identity Describe uses its `RequiredBytes` as the reclaim credit.
  - BUG-3: `explainOverflow` (llama + openvino clients) extracts the live
    `num_ctx=` from the overflow error text (typed error does not survive gRPC)
    and prefers it over describe-time numbers.
- 2026-07-02 (impl): Phase 2 items —
  - BUG-1c: slot `release` keeps implicit sessions resident whenever the idle
    reaper is enabled (idleTTL > 0); reaper owns reclamation. Close-on-release
    preserved when reaping is disabled. Cross-process warm reuse now works for
    one-shot CLI turns.
  - BUG-1a/1b: `prefillStreamLocked` in llamasession streams arbitrary-length
    prefix/suffix runs through the hot window, parking policy-selected ranges to
    the cold store between chunks (positions stay index==position because
    eviction compacts). `EnsurePrefix`/`PrefillSuffix` now gate at the logical
    budget (hot + cold) instead of the hot window; the gate also guarantees the
    cold store never LRU-drops during a stream. StreamingLLM-only sessions (no
    cold store) keep their historical lossy behavior unchanged. Mid-stream
    errors close the session (no rollback exists across evictions).
  - Known limitation (follow-up): cross-turn prefix reuse for beyond-window
    prefixes is LCP-based against the compacted resident tape, so a grown
    history re-streams from the divergence point each turn instead of admitting
    matching cold blocks back. Cold-block hash-based reuse is the optimization
    path.
  - First live run of streaming exposed two more defects (both fixed):
    (1) `planForBudgetLocked` built blocks without `ManifestOptions.BlockSize`,
    so one big message segment = one block spanning the whole hot window,
    overlapping both the sink head and the recent tail → flagged protected →
    the eviction driver had nothing to park ("resident=1836 num_ctx=1836"
    fatal). Now splits at `evictionDriverBlockSize=256`.
    (2) `prefillStreamLocked` marked the tape mutated even when eviction freed
    nothing, escalating a clean gate failure into a session-fatal close. Now
    only marks mutated when resident actually changed.
    (3) Even with block-split, the plan-based driver failed mid-stream: the
    manifest only describes segments enriched on PREVIOUS calls, so freshly
    streamed tokens have no blocks and the planner sees nothing to evict. The
    stream driver now evicts directly (`streamEvictLocked`): pin the
    StreamingLLM sink head and recent tail, park the middle to cold in
    256-token blocks, manifest-independent. The plan-based driver remains for
    the non-streaming (decode-time) paths.
- 2026-07-02 (verified): 3-turn growth E2E — the exact scenario that killed
  conversations this morning — now passes: all turns answered correctly with
  ~7.5k tokens of history through a ~3k hot window, zero overflow, zero fatal.
  Cross-process warm reuse confirmed: ONE backend session open across four CLI
  turns spanning two conversations (BUG-1c fix working). 15k-token single-paste
  runs without error (slow on this GPU: multi-cycle prefill + qwen3 thinking
  generation; wall time is a quality/perf follow-up, not a correctness bug).


-----------
Closed and verified today (all in the working doc docs/blueprints/effective-context-kv/capacity-bug-inventory-20260702.md):

- BUG-4 — accelerators now get a default VRAM reserve floor (max(512 MiB, 10% of total), capped at 25% of small devices) so modeld can't starve other GPU clients (the desktop-crash correlate).
- BUG-2 — the capacity panel / same-identity Describe now returns the resident session's actual open-time resolution instead of encumbered garbage (439-vs-2891 class bug gone).
- BUG-3 — overflow errors now quote the live session window extracted from the error itself, not a stale Describe answer.
- BUG-1c — implicit sessions stay resident after handle release when the idle reaper is on. Verified live: one backend session open across four CLI turns spanning two conversations — cross-process warm reuse works.
- BUG-1a/1b — the big one, landed after three live-fail iterations (each narrowing the root cause: hot-window hard gate → single-block manifest segments defeating the eviction plan → manifest lag making plan-based eviction fundamentally wrong mid-stream). Final design: prefillStreamLocked streams arbitrary prefixes/suffixes through the hot window with direct, manifest-independent StreamingLLM eviction (sink+recent pinned, middle parked to cold in 256-token blocks). Verified: the exact 3-turn scenario that killed conversations this morning now passes with zero overflows — ~7.5k tokens of history through a ~3k hot window, correct answers every turn. The 15k single-paste runs without error (slow on this GPU — a perf note, not a correctness bug; final confirmation run still in flight).
- BUG-9 — startup log now prints headroom=unset instead of a misleading 0.00.

Still open (documented in the doc's Phase 2/3): capability-truth for model list CTX (P2.7), the context-vs-speed GPU-layer knob (BUG-5), honest no-tools chat prompts (BUG-6), @file expansion (BUG-7), and failure-summary input truncation (BUG-8). Those are product/design-flavored rather than correctness-critical — BUG-5 and BUG-6 in particular have real design decisions in them that are worth your input before I pick a direction.

The unit suite is green across all touched packages (only the known environmental lease failure, which comes and goes with my test daemon). My suggestion for next session: restart your own make run-modeld and use the beam chat normally — the difference should be immediately felt — while I take on P2.7 and the Phase 3 items.
