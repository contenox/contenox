# Envelope Compute Bounds — the mission envelope as the unit's TOTAL boundary

Date: 2026-07-21
Status: building. The schema + the minimal honest enforcement slice have landed
(see "Slice status"). Companion to `fleet-consolidation.md` (the mission envelope
concept), `mission-plans.md` ("determinism only at boundaries"), and
`tool-hardening.md` (advice-vs-gate vocabulary).

## The doctrine

The mission envelope is a named HITL policy bound to a mission (see
`missionservice.Mission.HITLPolicyName`). Until now it bounded only what a unit may
**do** — the per-tool action rules (`allow` / `approve` / `deny`) the unattended
answerer enforces. This slice makes the envelope the unit's **total boundary**: it
also bounds how much a unit may **spend** — its turns, its gated tool dispatches,
its tokens.

> **The envelope is the unit's total boundary. It bounds compute as well as
> actions. A mission's envelope is the one place an operator declares both what the
> unit may do AND how much it may spend, and neither half is enforced anywhere but
> at that envelope.**

Two things this doctrine is careful about:

- **Gates by declaration, not advice.** The attention layer (`toolguidance`)
  APPENDS advice to a tool result and never fails a call — advice, never a gate
  (`tool-hardening.md`). Compute bounds are the opposite by design: they are
  **gates**, the legitimate kind — *envelope-declared, operator-authored,
  deterministic at the boundary*. They are not the runtime second-guessing a
  model; they are the operator's stated ceiling, enforced. The distinction is the
  whole reason they live on the envelope (operator-authored) and bite at
  deterministic seams (turn start, tool dispatch), not in a heuristic.
- **Determinism only at boundaries** (`mission-plans.md`'s design law). A compute
  bound is a hard fact at the edge — a countable seam and a discrete terminal
  status — not a judgment inside the unit's loop. The runtime owns the discrete
  status (`stuck`), exactly as it owns the closed report-kind set without owning
  what counts as progress.

## The schema

The envelope (a `hitlservice.Policy`) grows an optional `compute` block. Verbatim:

```go
type Policy struct {
	DefaultAction Action         `json:"default_action,omitempty"`
	Rules         []Rule         `json:"rules"`
	Compute       *ComputeBounds `json:"compute,omitempty"`
}

type OnExhausted string

const (
	OnExhaustedFinishStuck OnExhausted = "finish_stuck"
	OnExhaustedPauseAsk    OnExhausted = "pause_ask"
)

type ComputeBounds struct {
	MaxTurns         int         `json:"maxTurns,omitempty"`
	MaxToolCalls     int         `json:"maxToolCalls,omitempty"`
	MaxTokens        int         `json:"maxTokens,omitempty"`
	ModelAllowlist   []string    `json:"modelAllowlist,omitempty"`
	BackendAllowlist []string    `json:"backendAllowlist,omitempty"`
	OnExhausted      OnExhausted `json:"onExhausted,omitempty"`
}
```

Example envelope (JSON):

```json
{
  "default_action": "approve",
  "rules": [ ... ],
  "compute": {
    "maxTurns": 40,
    "maxToolCalls": 300,
    "maxTokens": 2000000,
    "onExhausted": "finish_stuck"
  }
}
```

## Semantics

- **Per MISSION.** Every bound is a ceiling on the whole mission, not per turn or
  per session. The counting state is keyed on the mission id.
- **Checked at deterministic seams.** `maxTurns` at turn start (the drive loop),
  `maxToolCalls` at tool dispatch (the unattended answerer). `maxTokens` between
  turns, from the usage the unit reported.
- **Bounds only RESTRICT (additive compatibility).** A zero/absent field is
  unbounded. An envelope with no `compute` block — every policy written before this
  existed — runs exactly as before. A malformed `compute` block fails the *whole*
  policy to load, and evaluation falls back to the built-in default (which carries
  no bounds): a broken policy LOSES its ceiling rather than gaining a phantom one.
- **Exhaustion is never silent.** A mission that crosses a bound comes to rest
  through the REAL terminal machinery — `missionservice.Finish(id, stuck, reason)`
  — with a reason naming the bound (`compute bound exhausted: maxTurns=40 — …`).
  The board, the operator inbox, and a `mission fire --wait` all read that terminal
  status and its reason and tell the truth for free.
- **Validation** (in `hitlservice`): each ceiling non-negative and within a
  defensive cap (a typo like an extra digit fails to load, not runs at ten
  billion); `onExhausted` a known value; allowlist entries non-empty and bounded.
  The `compute` sub-object is held to `DisallowUnknownFields` — a typo in a NEW
  bound (`maxTurn`, `onExhaust`) fails the policy to load rather than silently
  running the mission unbounded on the field the operator thought they set. The
  strictness is scoped to the block being introduced; the rest of the policy stays
  laxly parsed, so an incidental extra top-level key (a `"//"`-style comment note)
  keeps loading as before.

## What is — and is NOT — reliably measurable today (honest scope)

This is the crux, stated plainly per the "measure it, don't assume it" discipline:

- **`maxTurns` — measured exactly.** The drive loop (`driveUnattendedMission`)
  issues the mission's prompt turns itself, so it counts them locally and exactly.
  `maxTurns` counts total host-driven prompt turns INCLUDING the intent turn:
  `maxTurns: 1` runs the intent turn and no nudge; `maxTurns: 2` is today's
  intent+nudge behavior. (Today the loop tops out at two turns anyway — the
  intent and one nudge; `maxTurns` is the ceiling on that, and grows with the
  resident-planner loop that will drive many turns, `mission-plans.md`.)
- **`maxToolCalls` — measured at the ENVELOPE seam.** It counts the mission's
  envelope-**gated** tool dispatches — the permission requests that reach the
  unattended answerer for a verdict. Under a fail-closed envelope (mission mode's
  default, `default_action: approve`), that is every consequential action. It does
  NOT count tool calls the envelope `allow`s outright, because those never reach
  the host for a decision. To bound ALL tool calls (including `allow`ed ones) one
  must either use a fail-closed envelope or add the subprocess-side seam named in
  the follow-ups. Documented, not hidden.
- **`maxTokens` — BEST-EFFORT.** It is enforced only from the usage the downstream
  unit actually REPORTS over ACP (`usage_update`, `Used`), read from the session
  journal between turns. Not every provider reports token usage, and a
  deterministic chain that resolves no model reports none at all — for such a unit
  `maxTokens` is INERT (enforced against nothing rather than a phantom zero). It
  also bounds only ACROSS turns, not within a single turn (the host sees usage only
  once a turn completes). This is the honest limit of what host-side accounting can
  do today; a provider-uniform token meter is a separate track.
- **`modelAllowlist` / `backendAllowlist` — DECLARED, not yet enforced.** Model and
  backend resolution happens inside the unit's OWN process (`llmrepo`), where the
  host cannot see it. These fields are parsed and shape-validated so an envelope can
  EXPRESS the intent and a later `llmrepo`-side seam can honor it — but nothing
  enforces them today. An envelope that sets them is not lying to itself: the
  schema records the intent; the follow-ups name the seam.

## The enforcement seams, and why

The dispatched mission unit is an ACP **subprocess**; the runtime is the ACP client
driving it. That boundary is what shapes the seams. The bounds are enforced
HOST-side — where the mission's envelope lives and is authoritative — at the two
seams a mission naturally passes through:

1. **The drive loop** (`fleetservice.driveUnattendedMission`) for `maxTurns` and
   `maxTokens`. It issues the prompt turns, so it counts them exactly (`maxTurns`),
   and it can read the unit's reported usage from the session journal between turns
   without attaching a viewer (`maxTokens`) — the `SessionJournal` accessor, a
   policy-free read reached by type assertion, the same precedent `SessionAgentText`
   set. Not attaching a viewer is load-bearing: a viewer would become the session's
   controller and hijack the unattended permission routing the answerer depends on.
2. **The unattended answerer** (`fleetservice.NewUnattendedPermissionAnswerer`) for
   `maxToolCalls`. It is the ONE host seam that sees a gated tool dispatch *before*
   it happens and can REFUSE it — so it is the only place a bound can "fail the CALL
   with a teaching outcome" rather than stopping between turns. It counts each
   mission's gated dispatches; the call that crosses the bound is refused (the model
   sees a hard permission-deny) and the mission is finished stuck with a reason
   naming the bound. This seam lands in `contenox serve` for free: serve already
   constructs the answerer with the real `hitlservice` (which implements
   `ComputeBoundsReader`) and the mission store, so no serve wiring changes.

Rejected seam: a `toolguidance`-style decorator INSIDE the subprocess. It would see
every tool call (not just gated ones) and could return a teaching error string the
model reads directly. But it is cross-process (the subprocess shares the DB via
`$HOME`, so it *could* read the mission's bounds), it would collide with the
sibling-owned `localtools`/`enginesvc` composition, and it is not where the
envelope is authoritative. It is named as the follow-up that would make
`maxToolCalls` count ALL tool calls and deliver a model-visible teaching string.

### The teaching refusal — where the text goes

The ACP permission response cannot carry free text back to the model: a denied
permission surfaces to the model as its `HITLWrapper`'s standard `DenyMessage`
("User denied the operation…"). So the *teaching* half of the refusal is authored
for the OPERATOR, on the stuck mission's `StatusReason` (which the board and inbox
render): `compute bound exhausted: maxToolCalls=300 — the mission reached its
envelope-gated action budget; this call and any after it are refused.` The model
gets a hard stop; the operator gets the teaching. A model-visible teaching string
requires the subprocess-side seam above — a named follow-up.

## `onExhausted`: `finish_stuck` now, `pause_ask` deferred

- **`finish_stuck` (default, enforced).** The mission is finished at `stuck`
  through `missionservice.Finish`. This is the only enforced behavior, and it is the
  right default: a mission out of budget is a discrete boundary an operator must
  judge (`stuck` is exactly OpenHands' "stuck == a wall to attend to," not a
  failure to post-mortem).
- **`pause_ask` (declared, deferred).** It will file a durable ask ("this mission
  hit its compute bound — extend it or let it stop?") instead of finishing, reusing
  the same ask machinery the answerer already drives. It is DECLARED in the schema
  (a forward-looking envelope parses today) but NOT yet enforced: at the seam it is
  honored AS `finish_stuck`. Wiring the ask (a new ask kind, a resume path that
  extends the budget) exceeds this slice — it is the next one. Setting it today is
  safe (it parses and finishes stuck); the deferral is recorded here so it is not
  mistaken for a bug.

## Slice status

- **Schema** (`hitlservice/policy.go`, `hitlservice.go`): `ComputeBounds` on
  `Policy.Compute`, `OnExhausted`, validation (ranges + deny-unknown-fields on the
  block), and `ComputeBoundsReader.ComputeBoundsFor`. — **LANDED**.
- **Enforcement** (`fleetservice/compute.go`, `fleetservice.go`, `unattended.go`):
  `maxTurns` + `maxTokens` in the drive loop (via `WithComputeBounds`), and
  `maxToolCalls` in the unattended answerer (per-call refusal + `Finish(stuck)`). —
  **LANDED** (maxToolCalls live in serve via the answerer; maxTurns/maxTokens await
  the one-line serve hookup below).
- **Tests**: schema parse/validate matrix + absent-block-unbounded + restrict-only
  (`hitlservice/policy_compute_test.go`); counting correctness + drive-loop
  maxTurns/maxTokens + answerer refusal (`fleetservice/compute_test.go`); the
  subprocess e2e tripping `maxTurns` end to end
  (`fleetservice/e2e_compute_bounds_test.go`). — **LANDED**.
- **Presets**: `hitl-policy-strict.json` and `hitl-policy-default.json` carry an
  inert `"//compute"` commented example (JSON has no comments; the key is dropped by
  the lax top-level parse, so the presets stay behaviorally unbounded). — **LANDED**.

## Follow-ups (named, not done)

1. **Serve hookup for the drive-loop bounds.** `contenox serve` does not yet pass
   `fleetservice.WithComputeBounds(hitlSvc.(hitlservice.ComputeBoundsReader))` to
   `fleetservice.New`, so `maxTurns`/`maxTokens` are enforced today only where the
   option is wired (tests). One additive line, deferred because serve wiring
   (contenoxcli) is another cycle's scope. `maxToolCalls` already lands in serve via
   the answerer.
2. **Subprocess-side tool-call seam.** A decorator in the unit's own process
   (keyed on `missiontools.WithMissionID`, reading bounds from the shared DB) would
   let `maxToolCalls` count ALL tool calls (not just gated ones) and return a
   model-visible teaching error string. It is the honest home for a per-call gate
   that reaches the model, at the cost of crossing into the sibling-owned
   `localtools`/`enginesvc` composition.
3. **`pause_ask` enforcement.** The durable ask + budget-extend resume path (above).
4. **`modelAllowlist` / `backendAllowlist` enforcement.** An `llmrepo`-side seam at
   model/backend resolution, inside the unit's process, that refuses a model or
   backend the envelope did not allow.
5. **A provider-uniform token meter.** So `maxTokens` is more than best-effort — a
   count that does not depend on which provider bothered to report `usage_update`.
6. **A subprocess e2e for `maxToolCalls`.** The stub agent raises at most one gated
   call per mission, so the per-call refusal is proven by unit test today; a fixture
   that makes repeated gated calls would let it be an end-to-end subprocess
   acceptance like the `maxTurns` one.
