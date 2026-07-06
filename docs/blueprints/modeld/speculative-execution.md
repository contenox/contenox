# Blueprint: speculative execution for modeld

> Status: product and architecture blueprint. Scope is two "guess ahead, verify or
> discard" mechanisms for the local daemon: lossless **speculative decode** (B) and
> anticipatory **speculative prefill** (C). Out of scope: NVMe-persisted KV (tracked
> separately and deprioritized), draft-model speculation, and free-text intent
> prediction beyond deterministic editor triggers.

## The Product Bet

modeld is a single-user daemon sitting under an IDE with a dedicated accelerator. That
gives it two things a shared cloud endpoint never has: **free idle GPU cycles** (the
human spends most wall-clock time typing, reading, and thinking) and **intent
telemetry** (editor buffer, terminal output, diagnostics, git state). Speculative
execution spends the free cycles to make the felt latency disappear:

```text
reactive   : user submits -> prefill -> decode -> first token        (waits twice)
speculative: [guess ahead during idle] -> user submits -> first token (waits ~never)
```

This is the CPU branch-prediction analogy made literal, and it is structurally an edge
play: a cloud vendor won't burn compute on guesses it bills nobody for, can't see the
editor, and has no idle surplus dedicated to one user. We do, can, and have.

Two mechanisms, two bottlenecks, one shared spine:

```text
B. speculative decode  -> decode throughput (tok/s)   -> lossless, mid-generation
C. speculative prefill -> TTFT of the next request    -> anticipatory, human-idle
shared spine: preemptible speculation on a single slot
```

They are orthogonal and stack: B makes each response stream faster; C makes it *start*
without waiting. Neither touches cold-prefill of the stable repo context (that is the
separate, deprioritized KV-store work).

## Mechanism B: Speculative Decode (lossless throughput)

Decode is memory-bandwidth bound: to emit one token the GPU streams the whole model
from VRAM, does tiny math, repeats. Speculative decoding amortizes those weight loads by
drafting several tokens cheaply and **verifying them in one batched forward pass** of
the resident model. Correctly guessed tokens are accepted; the first wrong one is
corrected. Output is identical to non-speculative decoding — it is a throughput trick,
not a quality trade.

### Prompt-lookup, not a draft model

There are two ways to produce the draft:

```text
draft model : a small second model proposes tokens   -> burns VRAM + a 2nd KV cache
prompt-lookup: match last-k tokens against the context -> zero extra model, zero VRAM
```

modeld's single-slot / max-context north star **selects** prompt-lookup. A draft model
fights the resident model for VRAM and forces a second KV cache — a direct violation of
the one-resident-model premise. Prompt-lookup drafts by matching the last few generated
tokens against the context already resident and copying the continuation. Code is the
ideal workload: boilerplate, repeated identifiers, imports, JSON, and the model quoting
the repo back are all high-hit-rate. The win is a **burst when output is predictable**
(commonly 2–5× on syntax) and ~1× on novel reasoning tokens — never a regression.

### Backend support already exists

Both backends ship native prompt-lookup; nothing is wired in modeld:

- **llama.cpp**: `tmp/ref/llama.cpp/common/speculative.h`
  (`common_speculative_init` / `common_speculative_gen_draft` /
  `common_speculative_are_compatible`) plus `common/ngram-cache.cpp` — the n-gram /
  prompt-lookup drafter that needs no second model.
- **OpenVINO GenAI**: `GenerationConfig.num_assistant_tokens` + `max_ngram_size` +
  `is_prompt_lookup()` (generation_config.hpp), supported on the
  `ContinuousBatchingPipeline` backend modeld drives. `num_assistant_tokens` defaults
  to 5; setting `max_ngram_size` selects prompt-lookup over a draft model.

### Where it plugs in

B does **not** change the seam. `transport.Session.Decode(ctx, DecodeConfig)` stays
exactly the same shape; the backend internally drafts/verifies and the runtime just
receives tokens faster, in bursts. The only surface change is an opt-in knob on
`DecodeConfig` (e.g. `SpeculativeNgram int`) that the backends translate to their native
params. The KV bookkeeping is the one subtlety: verification advances the resident KV by
several tokens per pass and rolls back on rejection — the backends own this internally,
but our resident-token accounting (`session.resident`, position tracking) must trust the
backend's accepted count, not assume one-token-per-step.

## Mechanism C: Speculative Prefill (anticipatory TTFT)

The expensive, serial part of an interactive turn is `PrefillSuffix` over the volatile
input (a 50-line stack trace, a diff, the new user turn) on top of the resident prefix.
C does that prefill **before the user submits**, during human idle time, so the real
submit lands on warm KV and TTFT drops toward zero.

### The honest version of "99% predictable"

The claim is not free-text precognition. It is: **for a constrained class of requests,
the editor state does not predict the next request — it determines it.**

```text
deterministic triggers (ship first):
  inline autocomplete  -> the "predicted suffix" IS the cursor context
  test run fails       -> "explain this error"  (templated from terminal output)
  diagnostic squiggle  -> "fix this"            (templated from the LSP range)
  selection + hotkey   -> a known canned action
statistical guess (later, riskier):
  predict the next free-text chat message
```

Scope C to the deterministic tier first: the "prompt" is a template filled from
telemetry, so hit-rate for those triggers is near-100% by construction. Open-ended chat
is not in scope for the first pass.

### The engine already exists; the driver does not

C is **not** a new compute path. It is `EnsurePrefix` / `PrefillSuffix` fired early, and
the seam's warm-reuse semantics make wrong guesses cheap rather than catastrophic:

- Speculatively `EnsurePrefix(predicted text)`. When the real text arrives,
  `EnsurePrefix` reuses the longest matching prefix and only re-prefills the divergence.
  A *partially* wrong guess is a partial cache hit, not a discard — this is exactly the
  autocomplete-debounce case the contract was built for.
- Or `Snapshot` the resident KV, fork a ghost, prefill the speculative suffix, and
  discard on miss. Cost of a miss = idle electricity already spent.

So the missing pieces are a **driver** (telemetry → templated turn) and a **scheduler**
(run it preemptibly), not a new kernel.

### Prefill-warming vs decode-ahead

Keep these separate (the source brainstorm conflates them):

```text
speculative prefill : warm the KV for the predicted turn -> TTFT ~0, decode still streams
speculative decode-ahead: also generate the body into a buffer -> instant full "splat"
```

Default to prefill-warming. Decode-ahead is far more expensive and only pays off for
short, near-deterministic completions (inline autocomplete), where the whole body is
small and the trigger is structural. Do not decode-ahead a chat answer to a 50-line
trace — warm its prefill and let it stream.

## The Shared Spine: preemptible speculation on one slot

This is the load-bearing design and the part the brainstorm skips. modeld is a **single
slot with serialized session operations**: every `EnsurePrefix` / `PrefillSuffix` /
`Decode` runs through `slot.Service.withSession` →
`lockOperation(ctx)` (`modeld/slot/service.go:540`), which admits **one operation at a
time**. A speculative operation therefore does not run *beside* a real request — it
*occupies* the slot. The scheduler can only **preempt**, never overlap.

That makes the center of gravity scheduling, not prediction:

1. **Speculation is best-effort and preemptible.** Tag speculative operations
   (a `Speculative bool` / priority on the request) so the slot knows they yield. When a
   real request arrives while a speculative op holds the operation lock, the slot
   **cancels** the speculative op (ctx cancel → `cx_genai_session_cancel` on OV; ctx
   cancellation on llama), lets it release the lock, and admits the real op. A real
   request must never wait on speculation for more than one in-flight prefill batch.
2. **The fork must not corrupt resident state.** A ghost prefill must leave the real hot
   prefix intact if the user does something else. Options: (a) `Snapshot`/`Restore`
   around the speculative prefill; (b) a scratch sequence — llama already opens
   `n_seq_max = 1 + min(coldMaxTokens, 1)` (`llamasession.NewWithAdapters`), but seq 1
   is earmarked for the cold store, so the ghost contends with eviction and that
   contention must be resolved. The simplest correct first cut is snapshot-guarded
   speculation on seq 0 with hard preemption.
3. **Layering: modeld executes, the runtime predicts.** Editor telemetry lives above
   modeld (runtime / ACP / IDE). The predictor there issues speculative ops; modeld
   provides preemptible execution and cheap-miss warm-reuse. modeld never reads the
   editor; it only learns "this op is speculative, cancel it for real work."

```text
runtime (has telemetry)        modeld (single slot)
  predict turn  ───────────►   EnsurePrefix/PrefillSuffix [speculative=true]
  user submits  ───────────►   real op preempts: cancel speculative, admit real
                               warm-reuse: real prefill reuses speculative KV -> TTFT~0
```

## Code Map

Real symbols this touches. Nothing below has speculative support yet.

### Mechanism B (speculative decode)

| Concept | Symbol | Change |
| --- | --- | --- |
| Decode seam | `runtime/transport/session.go` → `Session.Decode`, `DecodeConfig` | add opt-in `SpeculativeNgram int` (0 = off) |
| llama decode | `modeld/llama/llamasession/llama.go` → `Decode` / sampler loop | drive `common_speculative` + `ngram-cache` from `tmp/ref/llama.cpp/common/speculative.h` |
| OV decode | `modeld/openvino/ovsession/genai.go` → `Generate`/`generation_config_from`; `genai.cpp` gen sites | set `GenerationConfig.num_assistant_tokens` + `max_ngram_size` (generation_config.hpp) |
| resident accounting | `modeld/llama/llamasession` `session.resident` / position | trust backend accepted-token count, not 1/step |

### Mechanism C (speculative prefill) + the shared scheduler

| Concept | Symbol | Change |
| --- | --- | --- |
| Operation gate | `modeld/slot/service.go` → `withSession` (540) / `lockOperation` / `beginOperation` (563) | add preemption: a real op cancels an in-flight speculative op and takes the lock |
| Op priority | `runtime/transport/session.go` request types / `PrefixInput`/`SuffixInput` | a `Speculative bool` so the slot knows the op yields |
| Busy/state | `slot.Service` `busyOp` / `SlotBusy` / `generation` | a speculative `busyOp` that is interruptible; generation fencing already invalidates stale forks |
| Cancellation | OV `cx_genai_session_cancel` (genai.go); llama ctx cancellation | the preemption mechanism |
| KV fork | `Session.Snapshot` / `Restore`; llama `n_seq_max` (`NewWithAdapters`) | snapshot-guard the ghost, or resolve seq-1 contention with the cold store |
| Cheap miss | `Session.EnsurePrefix` longest-prefix reuse | wrong guess = partial cache hit, already the contract |
| Idle signal | `slot.Service` `lastActivity` / `touchLocked` / idle reaper | "human idle" window; speculation runs between the last op and the reaper |
| Driver (above modeld) | runtime / ACP / IDE telemetry | template the predicted turn; issue speculative ops; this is NOT in modeld |

## Capacity and Cost

- **B**: negligible memory (no second model); compute cost is the rejected-draft tokens,
  bounded by `num_assistant_tokens`. Net positive whenever acceptance > ~1 token/pass,
  which code clears easily.
- **C**: the cost of a wrong guess is the cancelled/discarded prefill — idle electricity
  already spent, plus the one-batch worst case a real request might wait for preemption.
  Memory is bounded if speculation is snapshot-guarded (one extra KV copy) rather than
  multi-branch. Do not fan out many speculative branches on a single slot; one
  in-flight speculation at a time matches the serialized operation model.

## Observability

`modeld status --json` and traces should expose: speculative acceptance rate (B),
speculative-prefill hit rate and TTFT-with/without (C), count of preempted speculations,
and wasted speculative token-seconds. These are the numbers that tell us whether the
guesses are paying for their electricity.

## Non-Goals

- NVMe-persisted / cross-session KV store (separate, deprioritized blueprint);
- draft-model speculation (violates single-slot / max-context);
- free-text intent prediction beyond deterministic editor triggers in the first pass;
- multi-branch / fan-out speculation on one slot (serialized ops make it pointless);
- decode-ahead for anything but short deterministic completions;
- letting speculation ever delay a real request beyond one in-flight prefill batch.

## Phased Plan

### Phase 0: Decode-level speculation (B), lowest risk

- Add `DecodeConfig.SpeculativeNgram`; wire llama `common_speculative`/ngram-cache and OV
  `num_assistant_tokens`/`max_ngram_size`.
- Verify byte-identical output vs non-speculative; measure acceptance on a coding corpus.

Proof point: code generation bursts (e.g. ~15→~50 tok/s on boilerplate) with identical
output to greedy.

### Phase 1: Preemptible scheduling (shared spine)

- Add a `Speculative` flag to session operations and preemption to `withSession` /
  `lockOperation`: a real op cancels an in-flight speculative op and admits within one
  batch.
- Snapshot-guard speculative prefill so it never corrupts resident state.

Proof point: a speculative `PrefillSuffix` in flight is cancelled and a real request is
served with no measurable added latency; resident KV is intact after a discarded guess.

### Phase 2: Anticipatory prefill from deterministic triggers (C)

- Runtime-side driver: map editor events (test-fail, diagnostic, autocomplete) to
  templated turns; issue speculative `EnsurePrefix`/`PrefillSuffix` during human idle.
- Measure TTFT with and without the warm speculative prefix.

Proof point: "explain this error" begins streaming with near-zero TTFT because its
prefill completed while the user was reading the trace; a wrong guess costs only idle
compute and the next real prefill reuses the matching prefix.

### Phase 3: Decode-ahead for autocomplete (optional)

- For inline autocomplete only, decode the small body into a buffer so an accepted
  completion "splats" instantly.

Proof point: accepted inline completions appear with no visible generation, byte-equal to
a non-speculative completion.

### Phase 4: Statistical intent (optional, research)

- A tiny local predictor (n-gram over user history or a small model) proposes the next
  free-text turn for speculative prefill, gated by a confidence threshold.

## Open Questions

- Should `Speculative` live on the existing request types or a distinct best-effort RPC?
- Snapshot-guarded ghost vs a dedicated speculative sequence — and how does the latter
  share `n_seq_max` with the cold store?
- What acceptance / hit-rate floor justifies keeping a trigger enabled (auto-disable a
  trigger whose guesses keep getting preempted or rejected)?
- Where does the deterministic trigger→template mapping live — runtime, ACP, or a thin
  modeld-adjacent "intent" service?
- Does OV CB prompt-lookup interact cleanly with prefix caching and (later) LoRA
  variants, or do they contend for the same generation config?
- Should speculative prefill be suppressed under memory pressure or thermal limits even
  when the GPU is idle?

## Recommendation

Ship **B first** (Phase 0): native in both backends, lossless, no seam change, immediate
felt speedup on code. Build the **preemptible scheduler** (Phase 1) as the shared spine,
then **C from deterministic triggers** (Phase 2) for the headline TTFT→0 result. Treat
decode-ahead and statistical intent as optional layers on top.

The load-bearing engineering rule is the single slot: speculation must be best-effort and
instantly preemptible, must never corrupt resident KV, and must never make a real request
wait. Get that scheduler right and the rest is wiring two capabilities the backends
already have to telemetry the daemon already sees.
