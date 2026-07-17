# modeld: no-spill placement (refuse, don't degrade)

Status: draft blueprint, 2026-07-16. Grounded in a screen of the current
capacity/placement code. Slice 1 scoped; follow-ups listed.

## Ethos this enforces

modeld serves a right-sized model with good latency and is safe to run 24/7 on
a machine you also game/work on. When a model doesn't fit the device budget,
modeld should **refuse (or shrink context to fit) and say why** — not silently
degrade to hybrid CPU inference. Low latency + host safety beat "it technically
loaded."

## The bug this fixes (observed 2026-07-16)

Asking a 6 GB GPU to serve a ~4.8 GB model made modeld **shed GPU layers to the
CPU** to reach a usable-context floor — running weights *and* their KV in host
RAM, pegging ~430% CPU and driving the box toward swap, to answer "hi". That is
the ethos violated: it heroically loaded a model that doesn't fit instead of
refusing and pointing at a right-sized one.

## Root cause (screened)

- `modeld/capacity/capacity.go` `Resolve` is fine — it already shrinks the
  context window to the memory budget and flags `Clamped`/`Reason`.
- `modeld/llama/service.go` `resolveGPULayersForBudget` is the culprit: in AUTO
  mode (no explicit `num_ctx`) it walks GPU layers *down* from full offload,
  trading offloaded layers for a bigger KV window to reach
  `MinHotContextTokens` — i.e. it puts weights on the CPU to buy context.
- `resolveSession` already refuses when even maximal shedding can't reach the
  floor (the `HotContextTokens < floor` check) — so the machinery to refuse
  exists; it just tries to spill first.

## Slice 1 — no-spill auto placement (careful, focused)

The single behavioral change, plus honest messaging. Do NOT touch the working
inference path beyond this.

1. **Auto mode never sheds layers.** In `resolveGPULayersForBudget`, when there
   is no explicit GPU-layer cap and no explicit `num_ctx`, evaluate placement at
   **full offload only** and return the full (all-layers-that-fit-the-model)
   count. Do not walk down to buy context. The resulting context is whatever
   fits at full offload (`capacity.Resolve` already computes it).
2. **Refuse, don't partial-offload, when full offload doesn't fit.** If weights
   + minimum-KV + overhead exceed the device budget at full offload (context
   resolves to ~0), REFUSE — do not shed to a partial-CPU placement.
3. **Honest refusal.** Improve the `resolveSession` refusal (and the
   `EffectiveContext<=0` path) to state the budget arithmetic and the action:
   e.g. "model X needs ~A GiB (weights B + min-KV C + reserve D) but the device
   has ~E GiB free — use a smaller model or quant, or free VRAM." Actionable,
   not a bare error code.
4. **Shedding stays an explicit operator override only.** `CONTENOX_LLAMA_GPU_LAYERS=n`
   (and a model-profile cap) still work exactly as before — the walk-down logic
   remains reachable *only* when an explicit cap is set. Auto is no-spill;
   forcing hybrid is a deliberate operator choice.
5. **Tests.** Unit-cover: a model that fits full-offload with room → full
   layers, sensible context; a model too big for full offload → refuse with the
   arithmetic in the message (NOT a partial-layer placement); an explicit
   `CONTENOX_LLAMA_GPU_LAYERS` cap → still honored (walk-down still reachable);
   CPU-only host (no accelerator) → unchanged (0 GPU layers by design, runs on
   CPU). Keep the existing capacity/service tests green.

Net effect: qwen3-8b on a 6 GB card now refuses with a clear message (correct);
qwen3-4b loads fully on-device with a real context (correct); nobody's gaming
rig melts answering "hi".

## Follow-ups (later slices, NOT slice 1)

- **Placement plan + doctor authority.** A read-only, header-only pre-load
  report (weights/KV/overhead footprint, achievable context per device, a
  fit/won't-fit verdict) as versioned JSON shared by CLI/API/UI — the basis for
  the UI "fits on your GPU?" indicator and an honest `contenox doctor` placement
  check that runs before loading. (Extends the existing `Describe`/ModelInfo
  budget breakdown, which already logs all the pieces.)
- **Reserve-stack rationalization.** The current budget stacks min-free +
  `DefaultMaxResidentFrac` (80%) + `DefaultHeadroomFrac` (10%) + a batch-scaled
  compute reserve — together withholding ~45% of a small card. Decide: keep
  min-free + compute reserve (the real costs), drop/floor the headroom fraction
  and the 80% resident cap (redundant with min-free). Needs its own careful
  pass + the maintainer's call on the reserve sizes.
- **Warm-KV / session-resume latency** for agentic multi-turn (compare against
  the existing prefix-stable/coldstore/snapshot path).
