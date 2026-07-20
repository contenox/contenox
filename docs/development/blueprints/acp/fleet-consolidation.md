# Blueprint: Fleet consolidation — closing the loop before widening it

Owner: runtime

Status: PROPOSED (2026-07-20). Slices C1–C8 are specified to be executed;
technical decisions inside them are marked EXECUTED and are mine to make under
delegation. The four questions in "Reserved" are product decisions and are the
maintainer's.

Precedes [`supervision-bus.md`](supervision-bus.md): that blueprint's S1 and S2
are absorbed here as **repairs** (C1, C2) rather than new capability. The bus
picks up at its S3 once this lands. Continues
[`../beam/fleet-manager.md`](../beam/fleet-manager.md).

## Purpose

The `agentinstance` kernel was designed against requirements and use cases,
and it shows — attach-bound control with the reasoning for rejecting a time
lease, a documented port map from its reference implementation, an enumerated
list of where ACP forced divergence. Everything downstream of it accreted
slice by slice without that step, and it shows there too.

The result is a subsystem with more open loops than closed ones. Each slice
shipped was individually verified; none completed a journey. Dispatch creates
work that cannot be watched. The board renders state and affords no operation
that reaches the session. Missions are stored and rendered nowhere. Approval
requests reach beam and are discarded unread. The fleet has no CLI in a
product whose identity is that the shell is the operating surface.

That is not a backlog of missing features. It is a correctness problem, and
there is direct evidence: the `InstanceStatus.SessionIDs` defect shipped,
survived a second slice that consumed it, and surfaced only when `adopt`
became the first code to use it end to end. **A primitive cannot be validated
until a loop closes over it.** Building more primitives first compounds that.

This blueprint closes one loop — *dispatch an agent, see it ask for something,
answer it, watch it finish* — and repairs what is actively broken along the
way. Method: **state the requirement, then fix to it**, which is the kernel's
method and the reason the kernel is the good part.

## Verified defects

Established by reading the code (2026-07-20), file:line cited. Not a wish
list — every item is a shipped path that misbehaves or a declared capability
that cannot run.

| # | Defect | Evidence |
|---|---|---|
| D1 | Headless approval **hangs forever**. The fallback blocks on a channel nothing can write to; its comment claims an approval API that does not exist. | `runtime/contenoxcli/serve_cmd.go:279-284`, comment `:277-278`, repeated `runtime/acpsvc/router.go:13-16` |
| D2 | `hitlservice.Respond` is **dead code** — zero callers repo-wide — and would drop answers if wired: `select` with `default:` on an **unbuffered** channel. | `runtime/hitlservice/hitlservice.go:170`, `:177-182` |
| D3 | Pending approvals are an **in-process map**; a restart loses every in-flight approval. | `runtime/hitlservice/hitlservice.go:43-44` |
| D4 | Beam **parses `approval_requested` and discards it**: `taskEvents.ts` reduces it into `state.pendingApproval`; its only consumer never reads it. | `packages/beam/src/lib/taskEvents.ts:113-121`, `packages/beam/src/pages/admin/prompt/ExecPromptPage.tsx:27` |
| D5 | No pending-approval surface exists on **any** channel — no REST route, no CLI verb, no beam page. | `grep -i approv` over `runtime/internal/` returns nothing outside tests |
| D6 | `Enabled` had **two independently drifted implementations**, not one gap. CORRECTED 2026-07-20: an earlier draft of this row claimed the chat path "never checks" — that was wrong, drawn from a truncated grep. Both spawn paths did check; they had diverged in message text, neither offered the remedy, and neither had test coverage. Consolidated in C5 to one shared judgment. | `runtime/fleetservice/fleetservice.go`, and `acpsvc`'s external resolve (present at HEAD before this work) |
| D7 | Beam's chain editor declares **task handlers that no longer exist in Go** (`hook`, `prompt_to_string`, `prompt_to_int`). | `packages/beam/src/lib/types.ts:1042-1061` vs `runtime/taskengine/tasktype.go:15-24` |
| D8 | `adopt` works but is reachable **only by hand-crafting `_meta`** — no button, no verb. | `runtime/acpsvc/adopt.go`; no consumer in `packages/beam` or `runtime/contenoxcli` |
| D9 | The fleet has **no CLI**. | only `serve_cmd.go` and `fleet_telemetry.go` mention fleet in `runtime/contenoxcli/` |
| D10 | The board cannot tell a dispatched instance from **the chat you have open** — fleet reports downstream ACP session ids, beam routes on upstream contenox ids, and the KV mapping is one-directional. | `runtime/acpsvc/session.go:426-427` |
| D11 | **No e2e coverage** for agents, fleet, missions, or sessions. | `apitests/` covers backends, health, hitl_policies, mcp_servers, model_registry, taskchains, tools |

## Invariants

- The kernel stays **policy-free**. `agentinstance` resolves and spawns what it
  is told; judgments (Enabled, envelopes, approval routing) live in the service
  layer above it. A fix that adds policy to the kernel is mis-placed.
- **No second mechanism.** Approvals extend `hitlservice`; fleet verbs extend
  `fleetservice`; events extend `TaskEvent`. A parallel store, bus, or registry
  is reject-in-review.
- **An ask is never silently lost.** It is answered, expired by an explicit
  policy, or visible as pending. Silence is the one outcome that is a defect.
- The tracker stays passive and content-free; nothing here changes it.
- Every new seam takes `context.Context` and returns `error`.

## Slices

### C1 — Durable asks (repairs D1, D2, D3)

**Requirement.** An agent working with no human attached must be able to
request permission, have that request survive a restart, be answerable
afterwards from a surface that is not a live session, and expire by an
explicit policy rather than hang.

**EXECUTED decisions.** The store is a **table**, not KV records — asks are
listed and filtered by state, which is precisely the criterion the
fleet-manager blueprint used to choose a table over KV. It is owned by
`hitlservice`, which already owns policy evaluation, `TimeoutS`, and
`OnTimeout` (`runtime/hitlservice/policy.go:86-91`); a separate `askservice`
would split one lifecycle across two packages. The answer channel becomes
buffered (capacity 1) with the answer also persisted, so a `Respond` arriving
while the requester is not parked is recorded rather than dropped — the
`default:` arm at `hitlservice.go:177-182` is the bug, not the mechanism.

**Shape.** `RequestApproval` writes a durable pending row (id, tool, args
summary, diff, policy name, matched rule, requested-at, expires-at, state),
publishes `approval_requested` as it does today, then blocks on the wake-up.
`Respond(id, approved)` transitions the row and wakes any parked requester;
answering an already-resolved or expired ask is an explicit, non-panicking
outcome. A sweeper expires rows past their deadline applying the rule's
`OnTimeout` (default deny, per `runtime/localtools/hitl.go:151-159`).

**Acceptance.** An approval requested with no bound ACP session survives a
`contenox serve` restart and is answerable afterwards; an unanswered ask
expires by its rule rather than hanging; `Respond` has callers and tests; the
`serve_cmd.go:279-284` fallback provably terminates.

### C2 — The inbox (repairs D4, D5)

**Requirement.** The operator must find and answer pending asks from any
surface without attaching to the session that raised them, and must be able to
name the policy rule that escalated each one.

**Shape.** `GET /api/approvals` (pending, newest first) and
`POST /api/approvals/{id}` (answer) over C1's store; `contenox approvals
list|answer`; a beam page consuming the same routes. Each row shows agent,
tool, args summary, diff when present, and the matched rule — the
`acp-client-engine.md:127-130` requirement that an operator can always name
which policy gated an action.

**Acceptance.** A dispatched agent's permission request reaches a human who
never attached to its session and is answerable from both CLI and beam; a
fleet with nothing pending renders empty.

**This is the slice that closes the loop.** With C1 and C2, dispatch →
ask → answer → finish works end to end for the first time.

### C3 — `contenox fleet` (repairs D9)

**Requirement.** The fleet must be operable from the shell, because the shell
is this product's operating surface. Output must compose: stdout is data,
diagnostics are stderr, `--json` is the machine contract, exit codes are
meaningful.

**EXECUTED decision.** Unlike `contenox state` / `session`, which open the
SQLite DB directly, the fleet lives in `contenox serve`'s memory and is
reachable **only over HTTP**. This slice therefore introduces the first
serve-API client in `contenoxcli` (address + token resolution), which later
verbs reuse.

**Scope.** `list`, `show`, `stop`, `cancel`, `dispatch` over the existing
routes. `attach` is deliberately **not** here — see C4.

### C4 — Attach, on both surfaces (repairs D8)

**Requirement.** A running unit must be observable without hand-crafting
protocol: a button on the board, and a `tmux attach`-shaped verb in the shell.

**EXECUTED decision.** Both go through the existing ACP `adopt` verb rather
than a new REST journal-tail route. Attach is ACP-native per
`declared-agents-and-harnesses.md`, beam already holds an app-wide ACP
connection, and `libacp.ClientSideConnection` plus the `/acp` WebSocket shim
already exist for the CLI side. A REST tail would be a second mechanism for
something already built.

**Shape.** beam: a board action per session row → adopt → navigate to the
returned upstream session. CLI: `contenox fleet attach <instance> [session]` →
journal replay as scrollback, then live tail. Beam's adopt returns the upstream
session id, which also resolves D10's click-through as a side effect.

### C5 — Enforce `Enabled` where agents spawn (repairs D6)

**Requirement.** A disabled agent must not spawn, on any path.

**EXECUTED decision.** Not in `Manager.Start` — that would put policy in the
kernel, against the invariant above. Instead a single shared
resolve-declared-agent-for-spawn helper in the service layer, used by both
`fleetservice.Dispatch` and `acpsvc.bringUpExternal`, so the judgment has one
implementation and cannot drift between the two spawn paths.

### C6 — Chain-handler drift (repairs D7)

Delete the three dead handlers from beam's types and any UI that offers them;
align with `taskengine/tasktype.go`'s closed set. Small, mechanical, and it
stops the chain editor from writing chains the engine rejects.

### C7 — e2e for agents and fleet (repairs D11)

Extend `apitests/` to cover declaring an agent, dispatching it, listing the
fleet, cancelling, stopping, and answering an approval through C2's routes.
Nothing outside Go unit tests currently proves any of this works against a
running server.

### C8 — Board truthfulness (repairs D10)

With C4 landed, the upstream/downstream mapping exists. Use it: link a session
row to its chat, and make the Stop confirm state whether the instance backs an
open chat instead of admitting it cannot tell.

### C9 — Chains as agent templates (resolves the `chain` kind)

DECIDED (maintainer, 2026-07-20): **implement it.** The kind was deliberately
deferred until the ACP proxy path was validated; `adopt` plus the instance
kernel now validate it. The intended usage model, in the maintainer's words:
*you declare multiple chains as agent templates and launch agent-units for the
fleet from that chain.*

**Requirement.** An operator authors chains as files today. Those chains must
be declarable as agents, so a fleet unit can be launched from one — many units
from one template, and many templates side by side.

**Why it matters beyond closing an ambiguity.** It makes fleet units *cheap*.
An `external_acp` unit is a subprocess with its own model and its own
credentials; a chain is the native primitive the operator already writes and
reviews. Fleet-of-many stops requiring subprocess-of-many, which is what makes
the fleet coherent on one workstation with one GPU slot — and it is the unit
the later JS orchestration layer will loop over.

**EXECUTED design — the kernel stays ACP-generic.** `agentinstance` Layer A
depends only on `libacp` + `agenthost` by construction (`doc.go:20-33`), and a
chain-backed instance must NOT teach it about `taskengine`. Instead a
chain-kind agent resolves to an `agenthost.Agent` that connects to an
**in-process contenox ACP agent bound to that chain** — a real ACP connection
over an in-memory pipe, no subprocess. This is exactly the "contenox driving
contenox" composition `acp-client-engine.md:190-202` already names as
first-class, and it is what validating the proxy path unblocked. The kernel
sees an ACP peer; journal, viewers, controller promotion, and permission
routing all work unmodified.

**Shape.** A chain-kind `ConfigJSON` naming the chain (sibling of
`ExternalACPConfig`), validated by `agentregistryservice` — which today hard-
rejects the kind (`:143-144`). `Manager.Start`'s process-less stub
(`manager.go:284-285`) becomes a real spawner, and `OpenSession` / `Prompt`
stop erroring for native instances because a connection now exists.

**Acceptance.** Two chain files declared as two agents; units dispatched from
each; both appear on the board, are adoptable, cancellable, and stoppable, and
route permissions through the same HITL path as external agents — with no
subprocess spawned.

**Open sub-question for review:** whether each chain-backed instance gets its
own `acpsvc.Transport` (simplest, mirrors one-per-connection, costs a little
per unit) or shares one across units. I lean per-instance for isolation.

## Mission mode — the second way to interact with an agent

DECIDED (maintainer, 2026-07-20). This supersedes slice F4 of
[`../beam/fleet-manager.md`](../beam/fleet-manager.md), which modelled a
mission as "a durable note bound to work." **A mission is not a note. It is
the headless interaction model**, and it is why the mission record exists at
all. It is large enough to graduate into its own blueprint once these slices
are ratified.

There are two ways to work with an agent, and they are duals:

| | **Chat mode** (built) | **Mission mode** (this section) |
|---|---|---|
| How work starts | you prompt, turn by turn | you **fire a mission** and detach |
| Your presence | attached; you are the controller viewer | absent by default |
| What governs the agent | you, live, per permission request | the **envelope**: a HITL policy attached to the mission |
| How the agent reaches you | `session/request_permission` to your live client | **mission tools** — it reports back, or asks for attention |
| Where you answer | the chat transcript | an attention inbox |

The envelope is the load-bearing idea. In chat mode you are the envelope — you
see every request and answer it. In mission mode a **HITL policy is the
envelope**, so the unit can act unattended inside declared bounds and only
crossing them costs your attention. That is "intent down, autonomy inside the
envelope, exceptions up" made concrete rather than aspirational.

**Mission tools are granted by the mission, not by the agent.** An agent gets
`report` and `ask-for-attention` **only while on a mission** — they are not
part of its standing tool set. That keeps the grant per-unit-of-work and makes
the mission the thing that equips, which is the same equip-don't-govern shape
harnesses use. This is also the agent-authored producer that
[`supervision-bus.md`](supervision-bus.md) describes; that blueprint's S6 is
therefore **promoted from a late slice to core**, and its inbox (S2) and this
mode's attention surface are one surface, not two.

Consequence for the board: it becomes **mission-first**. A row's primary fact
is what a unit was sent to do and whether it needs you — process state is
secondary. This is the direct answer to the board's original defect, that it
rendered lifecycle rather than work.

### M1 — The mission record carries the envelope

**Requirement.** A mission must name its envelope and its work, because
nothing downstream can render or enforce what the record does not carry.

Today's `Mission` has intent, agent name, session/instance ids, and a status;
it has **no HITL policy** and no report storage. Add the envelope reference,
and reshape dispatch from "bring up an agent and fire a raw prompt" into
**fire a mission** — agent, intent, envelope, working directory. Status stops
being a hand-typed label and becomes agent-reportable.

### M2 — Mission UI

**Requirement.** Firing a mission, seeing what is running and why, and reading
what came back must all be possible from beam.

Fire-a-mission form (agent or chain template, intent, envelope, cwd); mission
list; mission detail showing reports, asks, and outcome; mission intent on
board rows. This is the plumbing that ends missions' zero-consumer status.

### M3 — Mission tools and the attention inbox

**Requirement.** A unit on a mission must be able to report progress and ask
for attention, and the operator must have one place where asks land.

The `report` / `ask` tools, granted per-mission and forwarded at session setup
(via `mcpServers` for external ACP agents, via the tools allowlist for chain
templates per C9); durable storage of reports against the mission; and the
attention inbox — which is C2's approval inbox, extended, not a second
surface.

### M5 — Unattended permissions reach the inbox (prerequisite for everything else)

**The defect.** `viewerHub.requestPermission` (`runtime/agentinstance/viewer.go:239-258`)
routes a downstream permission request to the session's controller viewer, and
when there is **no controller it auto-denies** — returning `cancelled`, with the
`onUnsupervisedDeny` hook explicitly documented as recording the decision and
"never changing the outcome." The kernel has no HITL path whatsoever (grep for
approval/hitl across `runtime/agentinstance` finds nothing).

A mission runs with no viewer attached **by design**. So today, the first
permission-gated action any unattended unit attempts is refused, C1's durable
ask store never sees it, and the inbox has nothing to show. **Mission mode is
broken for precisely the case it exists for**, and no amount of UI on top will
reveal it — the ask dies below the surface that would render it.

Note the asymmetry this leaves: native chain sessions already reach
`hitlservice` through serve's `AskApproval` fallback, so C1 fixed *their*
headless case. External-agent sessions, which are the ones missions actually
fire, never get there.

**EXECUTED design — the kernel stays policy-free.** Do not teach
`agentinstance` about approvals. Inject a fallback permission answerer,
defaulting to the current deny behavior, in the shape of the existing
`WithEventSink` option: the kernel calls it when there is no controller and
knows nothing about what it does. `serve` wires that fallback to
`hitlservice`, turning an unsupervised request into a durable ask with the
mission's envelope as its policy. The auto-deny stops being the default and
becomes the *timeout* outcome, which is what the supervision-bus blueprint
already argued for.

**Acceptance.** A dispatched unit with no viewer requests permission; the ask
appears in the inbox naming the policy that escalated it; answering it
unblocks the unit; leaving it unanswered expires by policy rather than hanging
or silently denying.

**Sequencing.** This lands before M3 and before any live end-to-end run.
Without it an e2e proves only that missions fail quietly.

### M4 — Mission CLI and documentation

**Requirement.** Mission mode must be operable and *learnable* from the shell.
A headless mode you can only reach from a browser contradicts the product's
own centre of gravity, and an operator cannot adopt a second interaction model
they have to reverse-engineer from route definitions.

**EXECUTED decision — `mission` owns firing, `fleet` does not.** The two verb
families split along the two nouns, not along the routes that happen to back
them:

- `contenox fleet` operates on **units** — list, show, stop, cancel, attach.
  The process-management verbs.
- `contenox mission` operates on **work** — fire, list, show (with its
  reports), and status transitions.

Firing therefore lives at `contenox mission fire`, even though it is served by
`POST /fleet/dispatch`. The route is an implementation detail; the verb should
read as what the operator is doing. Consequence: **C3 drops `dispatch` from
its scope** — a `fleet dispatch` alongside `mission fire` would be two names
for one act, and the first thing to rot.

**Shape.** `mission fire --agent <name> --intent <line> --policy <envelope>
[--cwd <dir>]`; `mission list`; `mission show <id>` rendering the mission plus
its reports newest-first. Same output discipline as every other verb: stdout is
data, diagnostics are stderr, `--json` is the machine contract, exit codes are
meaningful.

**Documentation is part of this slice, not a follow-up.** Two audiences, two
places: the verb reference in `docs/reference/contenox-cli.md` alongside the
existing subcommand sections, and a concept page explaining the **two
interaction modes** — when you prompt an agent turn by turn versus when you
fire one at an intent and detach, and what the envelope is protecting you from.
The second matters more; mission mode is a new way to work, not a new flag.

**Sequencing.** Depends on C2, which builds the first serve-API HTTP client in
`contenoxcli`. The fleet lives in `contenox serve`'s memory, so unlike
`contenox state` or `session` — which open the SQLite DB directly — these verbs
must go over HTTP. Writing a pending approval's row from another process would
change its state without waking the goroutine parked in `RequestApproval`,
which would then block to its ceiling. One client, built once, reused by C3 and
M4.

Documentation is written *after* the commands exist and against their real
behavior. Documenting unwritten verbs is how a repo ends up with comments
claiming APIs that were never built — a defect this blueprint already exists to
repair (D1).

### Primitives weighed and deliberately deferred

Design positions taken 2026-07-20, recorded so they are not re-litigated.
Each stands on its own reasoning.

**DEFERRED: durable suspend/resume.** A step that could yield — persisting its
position and being resumed later by an external stimulus — would collapse
"waiting for a human", "waiting for a sub-agent", and "backing off before a
retry" into one mechanism. That is attractive, and we are still not building
it now, for a reason specific to our units: a mission's unit is an **external
ACP subprocess**, and `Manager.Close` kills every subprocess at shutdown
(`agentinstance/manager.go:522-547`). Suspending our side of the conversation
saves nothing when the agent's own context dies with the restart regardless.

Revisit at C9, when chain-backed units become in-process and the run itself is
finally worth preserving — or when missions must survive a deploy. Until then,
durability lives in the mission record and the ask store, not in the execution.

**ADOPTED: enqueue-then-nudge.** Write the durable record inside the
transaction first; only then publish a notification, and give that
notification **no payload** — it says only that something changed. The
notification becomes a latency optimization rather than a correctness
dependency: losing one costs seconds, because a sweeper re-nudges anything
that has gone stale. This is already C1's shape, and it is why C1 persists the
answer rather than only signalling it.

**ADOPTED: the envelope is enforced at construction, not by a check.**
Privileged capabilities are bound into a unit's session when that session is
built; an unbound capability is *absent*, not refused. So mission tools are
not "the agent calls `report` and we verify it is on a mission" — the
mission's session is constructed with those tools bound, and a unit not on a
mission has nothing to call. That makes the envelope unforgeable from the
agent's side instead of policed, and removes a whole class of check-ordering
bugs. M3 must implement it this way.

**NOTED for the future scripting layer, not now:** an embedded interpreter
needs a watchdog able to interrupt runaway user code, since a single-threaded
VM lets an unguarded loop pin a core indefinitely. And a step transition must
return a value the *engine* dispatches rather than invoking the next step on
the interpreter's own call stack — the latter bounds total transitions by
stack depth, which would cap precisely the long-running loop the scripting
layer exists to enable.

## Reserved — the maintainer's calls

These are product decisions, not technical ones. Each is cheap if the answer
is "remove"; each costs more than either resolution while it stays ambiguous.

1. **`endpoint` transport.** Validates and persists with only a URL
   (`runtimetypes/agents.go:72-75`), then fails at connect with "not
   implemented yet" (`agenthost/externalacp.go:67-68`) — you can register an
   agent that can never run. Implement, or reject at validation?
DECIDED (maintainer, 2026-07-20): **restart stays off.** Re-spawning loses the
downstream conversation (`agentinstance/doc.go:84-88`), which makes it a
change with real cost and no urgency; it is not taken lightly and not taken
now. `warning` stays in the state vocabulary (it is reachable the moment
restart is ever enabled) and the board keeps rendering reality. Consequence
for C8: no surface may imply a restart action exists.

## Acceptance

Consolidation is done when:

1. Dispatch → ask → answer → finish works end to end, from CLI and from beam,
   with no hand-crafted protocol and no attached session required.
2. Every declared capability either runs or is rejected at declaration time —
   no registry entry that cannot spawn, no state that cannot be reached.
3. The fleet is operable from the shell with composable output.
4. An operator can always name the policy that gated the last action.
5. `apitests` exercises the agent and fleet paths against a running server.
6. Nothing in this blueprint required a new mechanism — only completing,
   wiring, or deleting existing ones.
