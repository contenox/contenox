# Blueprint: The supervision bus — reporting, asking, and the agent primitive

Owner: runtime

Status: PROPOSED (2026-07-20). Scope draft awaiting maintainer ratification.
Every decision herein is proposed unless explicitly marked DECIDED or RULING.
Depends on [`acp-client-engine.md`](acp-client-engine.md) (shape (a), the
permission-routing invariant) and
[`declared-agents-and-harnesses.md`](declared-agents-and-harnesses.md) (the
`forward | rule | queue` permission routes). Supersedes nothing; the fleet
board's passive-telemetry ruling
([`../beam/fleet-manager.md`](../beam/fleet-manager.md)) stands unchanged — see
"Bus is not telemetry" below.

## Purpose

The operator does not manage many agents by watching them. Watching does not
scale past one, and the fleet board proved it: a page that renders process
state answers "is it up," not "does it need me." Management at any number
above one means **being reported to** — units speak, the operator answers, and
silence means nothing needs attention.

That requires a channel that runs in both directions, and it requires the
agent to be a participant that can *speak*, not an object that gets observed.

The claim this blueprint rests on: **supervision is uniform and recursive.**
The relation — a unit reports and asks; a supervisor answers and instructs —
is identical whether the supervisor is a human at beam, a `tail` in a
terminal, a chain step driving a sub-agent, or (later) a JS orchestration
script looping one unit fifty times. Making that one relation, rather than
four features, is what turns a notification feed into a bus, and it produces a
**supervision tree with the operator at the root**. Exceptions escalate
upward until something can answer them; everything answered lower never
reaches the operator. "Green silence" stops being a UI filtering aspiration
and becomes a property of the topology.

The taskengine agent primitive is in this blueprint and not a separate one
because it is what creates the **first non-human supervisor**. Until a chain
can be an agent's parent, the bus has exactly one consumer and "dual-sided"
collapses into HITL with extra ceremony.

### Bus is not telemetry

The fleet blueprint RULED that lifecycle telemetry is passive: observed,
recorded for after-the-fact audit, triggering nothing, and content-free
("report identity and fact — never content. No prompt text, no output"). That
ruling stands and is not reopened. This bus is a different mechanism with
opposite properties, and the distinction must stay legible in review:

| | Telemetry (tracker) | Bus |
|---|---|---|
| Origin | derived by observation | authored — by the kernel at an event, or by the agent deliberately |
| Purpose | audit afterwards | act now |
| Payload | identity and fact only | **content, necessarily** — a report with no content is a status light |
| Triggers behavior | never | that is its entire point |

The earlier rejection was of a *classifier* deriving curated events from
telemetry. Nothing here derives anything. The tracker keeps working exactly
as it does today, unchanged, alongside this.

## Verified Current State

Established by reading the code (2026-07-20), not inferred from names.

**The reporting half of this bus already exists.** `TaskEvent` is already a
bus message: `BusTaskEventSink` publishes every chain event to `libbus` on
`taskengine.events` and `taskengine.events.request.<requestID>`
(`runtime/taskengine/events.go:93-142`, subjects at `:14` and `:105-111`),
and four consumers already subscribe — the SSE route
(`runtime/internal/taskeventsapi/routes.go:61`), the ACP `session/update`
translator (`runtime/acpsvc/prompt.go:130`), the CLI trace renderer
(`runtime/contenoxcli/trace_render.go:86,126`), and the VS Code bridge. Its
kind set already includes `approval_requested` and `hitl_decision`
(`events.go:18-33`). **HITL escalation already rides a bus today — for
chains.** Building a second bus would be the parallel-registry mistake
`acp-client-engine.md:212-215` names as reject-in-review.

**`libbus` has first-class request-reply.** `Request` / `Serve`
(`libbus/bus.go:95-100`) are not decorative — they carry the entire MCP
tool-execution path (`runtime/internal/tools/remoteprovider.go:177,342` →
`runtime/mcpworker/mcpworker.go:257,275`). The production backend is
**SQLiteBus** (`libbus/sqlite.go:67`; every production construction site uses
it — `serve_cmd.go:203`, `enginesvc/engine.go:37`; the NATS backend is wired
nowhere).

**But `libbus` cannot be the ask queue.** Three properties disqualify it for
human-scale waiting, all documented behavior rather than bugs:

- Unanswered `bus_requests` rows are **deleted by the 5-minute cleanup
  sweep** (`libbus/sqlite.go:417-434`). A pending human approval is silently
  garbage-collected.
- `Request` without an explicit deadline defaults to **10 seconds**
  (`libbus/sqlite.go:52,358-361`).
- **No inbox query exists.** Requests are claimed by `DELETE ... RowsAffected
  == 1` (`sqlite.go:307-315`), not listed. There is no way to enumerate
  pending work.

Also load-bearing for the report half: SQLiteBus `Stream` snapshots
`MAX(id)` at subscribe time and delivers only `id > cursor`
(`sqlite.go:140-157,175`) — **a late subscriber never sees what it missed** —
and `bus_events` rows are swept at 5 minutes. `BusTaskEventSink` publishes
with a 100 ms timeout and returns only the first error (`events.go:101,132-140`),
so a slow bus drops events silently. All acceptable for telemetry; all
disqualifying for an ask.

**The durable-ask substrate does not exist, and its absence is currently a
hang.**

- `hitlservice` pending approvals are an **in-process map**,
  `pending map[string]chan bool` (`runtime/hitlservice/hitlservice.go:43-44`).
  No table, no KV key. A process restart loses every in-flight approval.
- **`hitlservice.Respond` is dead code** — zero callers anywhere in the repo,
  including tests (`hitlservice.go:170`). It also has a latent defect: the
  send is `select { case ch <- approved: ...; default: return false }` on an
  **unbuffered** channel (`:177-182`), so unless the requester goroutine is
  parked on the receive at that exact instant, the answer is dropped.
- **The headless approval fallback does not work.** `serve_cmd.go:279-284`
  falls back to `hitlSvc.RequestApproval` when no ACP session is bound, and
  its comment (`:277-278`, repeated at `acpsvc/router.go:13-16`) claims this
  reaches "the approval-API path for headless/API callers." **That approval
  API does not exist.** The fallback publishes an `approval_requested` event
  and then blocks forever on a channel nothing can write to. For a headless
  caller, this is an unconditional hang until context cancellation.

**No inbox surface exists on any channel.** No REST route (`grep -i approv`
across `runtime/internal/` returns nothing outside tests; the only HITL routes
are policy CRUD, `runtime/internal/hitlpolicyapi/routes.go`). No CLI verb. No
beam page. Beam even *parses* the SSE `approval_requested` into
`state.pendingApproval` (`packages/beam/src/lib/taskEvents.ts:113-121`) — and
its only consumer, `ExecPromptPage.tsx:27`, never reads it. The request is
decoded and dropped on the floor.

The two answer surfaces that do work are both **live-connection-scoped**:
`PermissionCard` (`packages/beam/src/pages/chat/components/PermissionCard.tsx:34`)
and `ChatSurface`'s `ApprovalCard` (`packages/beam/src/chat/ChatSurface.tsx:1161`).
Neither is an inbox; both answer one in-flight request on one attached
session.

**There is no run→run parentage.** `libtracker.ContextKeyRequestID` is a flat
string (`libtracker/context.go:9`), and `WithNewRequestID` **overwrites** it
(`:36-39`). Every prompt turn mints a fresh id and discards the old
(`acpsvc/prompt.go:94,98`, plus ~60 CLI sites). Grep for
`ParentID|ParentSession|SpawnedBy|Subagent` across `runtime/` and `libacp/`
returns **zero** matches. The supervision tree has no existing correlation
substrate.

**The supervision that does exist is instance→session→viewer.** One
controller per session, first-attached wins, promoted on detach
(`runtime/agentinstance/viewer.go:81-86,169-171,209-216`); permission routes
to the controller and, with none, returns cancelled and fires
`EventUnsupervisedDeny` (`viewer.go:240-258`, `manager.go:40-45`). The
`adopt` verb (`runtime/acpsvc/adopt.go`) binds a running instance+session to a
new upstream session so a dispatched unit can acquire a supervisor after the
fact. A `Mission` is a flat many-to-many tag
(`runtime/missionservice/missionservice.go:43-48`), **not** a parent/child
relation.

**Two unrelated event systems.** Chains publish `TaskEvent` to `libbus`;
agent instances fire `EventSink` straight into the tracker with no bus
involvement (`runtime/contenoxcli/fleet_telemetry.go`). Every event this
blueprint wants on the bus from the fleet side — crash, exit, unsupervised
deny — is in the second system. Every consumer that already exists is on the
first.

**The taskengine primitive is blueprinted, unimplemented, and cheaper than it
looks.** `TaskHandler` is a closed const set
(`runtime/taskengine/tasktype.go:15-24`: `raise_error`, `route`,
`chat_completion`, `execute_tool_calls`, `noop`, `tools`) dispatched by one
switch (`taskexec.go:525`). `acp-client-engine.md:60-64` already specifies a
new `TaskHandler` and a new switch case, "not a plugin/registry layer."
`agenthost.DriveTurn` (`runtime/agenthost/drive.go:152`) already implements
the full `initialize → session/new → session/prompt` turn a handler would
call. Adding a type touches ~21 sites, of which the six mandatory ones are
~150 lines across two files.

The real cost is elsewhere: **`TaskEvent` has no field naming which
sub-agent or session an event came from** (`events.go:35-74`), and
`acp-client-engine.md:216` makes "a chain step that cannot name which
sub-agent, session, or policy backs it" a reject-in-review anti-pattern. So
the primitive forces identity fields onto `TaskEvent`, which touches all four
consumers — and that is exactly the addressing the bus needs anyway. The two
are one investment, not competing ones.

Also: hooks no longer exist (renamed wholesale to tools-providers, commit
`4713aab`), so "could this be a tools-provider instead" reduces to the ruling
`acp-client-engine.md` already made — an agent is stateful and bundles an
arbitrary number of its own tool calls and permission requests behind one
interaction, and hiding that inside `ToolsRepo.Exec` would violate the
taskengine's own doctrine that state is "never mutated invisibly on the
forward path where no event would see it" (`taskengine.go:7-12`).

## The model

### Four flows

| Direction | Kind | Blocking | Substrate |
|---|---|---|---|
| unit → supervisor | **report** — I finished; I found this; I am 40% through | no | existing lossy fan-out (`TaskEvent` on `libbus`) |
| unit → supervisor | **ask** — may I run this; which do you want | **yes** | durable pending store (new) |
| supervisor → unit | **answer** — correlated reply to an ask | resolves a block | durable store + wake-up |
| supervisor → unit | **instruct** — stop; change course; here is context | no | later slice |

The asymmetry that matters is blocking. A report is a notification and the
existing fan-out handles it. An ask suspends a unit until someone replies,
which needs a correlation id, a durable pending record, an expiry policy, and
a defined behavior when nobody ever answers.

RULING (proposed): **the bus is honestly two mechanisms behind one
vocabulary** — a lossy stream for reports and a durable queue for asks — and
the blueprint says so rather than pretending one pipe does both. `libbus`
provides the fan-out and the wake-up; it does not provide the queue.

### Two producers

- **Trigger-sourced.** The kernel authors these at moments that are
  definitionally events: crash, exit, state change, permission request. The
  seams exist (`agentinstance.EventSink`, the permission callback). Wiring
  them is plumbing.
- **Agent-authored.** The agent deliberately calls a tool to report or ask.
  Nothing like this exists today.

An external ACP agent's tools arrive through the `mcpServers` list its
declaration forwards at `session/new` (`agentinstance/drive.go:58-62`,
`acpsvc/external.go:1451-1455`). So the report/ask capability is **an MCP
server contenox itself serves**, forwarded like any other. Contenox serves
none today (`mcpworker` is a client), so this is real new surface — but it
makes reporting a **per-agent declaration** through the existing explicit
allowlist. Equip, don't govern: an agent whose declaration omits it simply
cannot author messages, and still produces trigger events.

### The hard line: two kinds of ask

DECIDED (proposed as an invariant, not a preference):

A **question-ask** is free-form — "which config," "should I continue." A
supervisor, including a parent agent, may answer at its own discretion. That
is delegation working.

A **permission-ask** is governed. `acp-client-engine.md:115-131` already
states it: a sub-agent's `session/request_permission` must be answered by
contenox's own HITL policy machinery, and auto-approve-because-it-is-just-a-
sub-agent is named an anti-pattern that destroys the entire governance value.
A permission-ask therefore routes to the policy evaluator, which answers where
a rule allows and escalates to a human otherwise — **never to agent
discretion, at any depth**. A parent agent may *forward* a permission-ask
upward. It may not *grant* one.

Conflating the two would build a machine that can approve `rm -rf` by LLM
judgment. This design makes that failure mode newly reachable, which is
exactly why the line is drawn here rather than left implicit.

## Invariants

- **Equip, don't govern** holds: the bus carries what a unit chose to say and
  what the kernel observed. It does not police agents.
- **Permission-asks never resolve by agent discretion** (above). An operator
  must always be able to name which policy gated the last action.
- **The tracker stays content-free and passive.** The bus carrying content
  does not license the tracker to.
- **No second bus.** New message classes extend the existing `libbus` subjects
  and event vocabulary; a parallel transport is reject-in-review.
- **No silent drops on the ask path.** A report may be lost (documented, lossy
  by design). An ask that is lost is a hang or a wrongly-denied action, and
  must be either answered, expired by an explicit policy, or surfaced.
- Control-plane isolation holds: nothing here gives an agent fs-reach into its
  own governing policy.
- Every new seam takes `context.Context` and returns `error`; no `lib*`
  package imports `runtime/`.

## Slices

Sequenced walking skeleton. The thinnest thread through the whole vision:
**a unit asks → the ask lands durably → the operator sees it in an inbox →
answers → the unit proceeds.** That is S1+S2; everything after deepens it.

### S1 — The durable ask, and the hang it fixes

Replace `hitlservice`'s in-process `pending` map with a durable pending-ask
store (a table beside the existing registries), wire `Respond` to it, fix the
unbuffered-channel drop (`hitlservice.go:177-182`), and give asks an explicit
expiry policy sourced from the rule's `TimeoutS` / `OnTimeout`
(`policy.go:86-91`) rather than from a caller's context alone.

This is first because it is simultaneously a **live bug fix** — the headless
fallback at `serve_cmd.go:279-284` currently blocks forever — and the
substrate everything else needs.

Acceptance: an approval requested with no attached session survives a serve
restart, is answerable afterwards, and expires by its rule's policy rather
than hanging. `Respond` has a caller and a test.

### S2 — The inbox

The operator surface for S1: list pending asks, answer by id, see which policy
rule escalated each one. Three renderings of one service — REST routes, a CLI
verb, and a beam page. Beam's `taskEvents.ts` already reduces
`approval_requested` into state nothing reads; this gives it a consumer.

Acceptance: a dispatched agent's permission request reaches a human who never
attached to its session, and is answerable from CLI and beam. A fleet with
nothing pending renders empty.

Note this closes the dispatch black hole from the opposite side to `adopt`:
`adopt` lets a human show up and answer live; the queue lets the ask wait for
anyone, including a rule. Both are wanted.

### S3 — Fleet events onto the bus

Bridge `agentinstance.EventSink` onto the bus so crash, exit, state change,
and unsupervised-deny become reports the operator feed can render. The
passive tracker sink stays as it is — this is additive, a second sink, not a
replacement.

Acceptance: an instance crash appears in the operator feed within one poll
interval, and in the tracker exactly as it does today.

### S4 — Correlation: the supervision edge

Stop discarding parentage. Introduce a parent reference threaded through the
places that currently mint a fresh request id and drop the old
(`libtracker.WithNewRequestID`, `libtracker/context.go:36-39`), plus a
durable edge recording who supervises a unit — set to the operator at
dispatch, to the run at an agent-step spawn.

This is small in concept and wide in blast radius (~60 call sites mint request
ids). It is the foundation of the tree; S5 can land ahead of it with a
single-level edge if sequencing demands.

### S5 — The taskengine `agent` primitive

Shape (a) of `acp-client-engine.md`: a new `TaskHandler`, a new case in the
`taskexec.go` switch, calling `agenthost.DriveTurn`, plus the resolver
dependency `SimpleExec` has never had (`taskexec.go:53-58` → `NewExec` →
`enginesvc.Build` → three CLI bootstraps). Includes the `TaskEvent` identity
fields naming the sub-agent and session, and updating the four consumers.

Permission routing for the driven sub-agent goes to the HITL policy machinery
per the invariant — which by then means S1's durable queue, so a sub-agent's
escalation reaches the operator inbox like any other.

Acceptance: a chain runs an external ACP agent as one step; the chain sees the
sub-agent's tool calls as they happen rather than one opaque result; a
guardrail step between two agent actions works; and the operator can name
which sub-agent and which policy backed the last gated action.

### S6 — Agent-authored reports and asks

The MCP server contenox serves, exposing `report` and `ask`, forwarded to a
declared agent through its existing `McpServers` allowlist. Native chains
reach the same capability through `localtools`.

Acceptance: an agent given the tool reports progress mid-turn and it appears
in the operator feed; an agent asks a question and blocks until answered; an
agent whose declaration omits the tool cannot do either.

### S7 — Instruct

The remaining down-channel: steer a running unit without answering a specific
ask. Deferred deliberately until S1–S6 prove the addressing.

## Decision Points

RESERVED to the maintainer unless marked. Recommendations given.

- **Ask store location.** A durable table owned by `hitlservice` (it already
  owns evaluation, timeout, and `OnTimeout` semantics) versus a new
  `askservice` package generalizing beyond permissions. *Recommend
  `hitlservice` for S1* — permission-asks are the only asks until S6 — with
  the interface shaped so question-asks slot in without a second store.
- **Report durability and replay.** SQLiteBus sweeps `bus_events` at 5 minutes
  and never replays to late subscribers, and `useTaskEvents` has no resume
  token, so a beam reconnect silently loses the gap. Options: leave `libbus`
  alone and give the operator feed its own durable log (the
  `KVJournalTaskEventSink` pattern, `events_journal.go:28-93`, already does
  exactly this per-request), versus adding cursor/resume to the bus itself.
  *Recommend the journal pattern* — it is built, and changing bus semantics
  affects five unrelated subscribers.
- **Unify versus bridge the two event systems.** `TaskEvent` is called
  de-facto public API and is pinned by a contract test
  (`dsl_contract_test.go:15-22`); `agentinstance.Event` is a separate shape
  feeding the tracker. *Recommend bridging into a shared envelope* rather than
  merging the types, since a merge breaks a contract-tested surface for four
  consumers.
- **Nesting depth in v1.** Cap at one level (operator → agent) with the
  general tree deferred, versus uncapped with loop and budget guards from the
  start. *Recommend capping*: it validates the model and defers the guards,
  and S4's edge record is forward-compatible either way.
- **Does controller-viewer routing survive?** The attached-controller path
  (`viewer.go:240-258`) works today and backs every live beam chat.
  *Recommend keeping it unchanged as the `forward` route*, with the durable
  queue added as a sibling for the no-controller case — replacing the working
  path would be a large behavior change for no gain.
- **Where the supervision edge lives.** On the `Mission` record (already
  bound to sessions and instances, but flat and many-to-many), on the
  instance, or as its own edge table. *Recommend its own record* — a mission
  is a grouping tag and overloading it with topology conflates two ideas.
- **Whether `contenox` serving an MCP server is acceptable surface** at all,
  or whether agent-authored reporting should ride an ACP `_meta` extension
  instead. *Recommend MCP*: it reuses the existing per-agent allowlist and
  works for any conformant agent without protocol extension.

## Acceptance

The supervision bus exists when:

1. An agent working with no human attached can ask a question or request a
   permission, and that ask waits durably, is visible in an inbox, is
   answerable from CLI or beam, and expires by an explicit policy rather than
   hanging or being silently denied.
2. A crash, an exit, and an agent-authored report all arrive at the operator
   through one feed with one vocabulary, and a healthy fleet's feed is empty.
3. A chain step drives a sub-agent, sees its actions as they happen, and its
   sub-agent's permission escalations reach the same inbox as everything
   else — proving the supervisor need not be human.
4. No permission-ask anywhere in the tree was resolved by an agent's
   discretion, and every gated action can name the policy that gated it.
5. Nothing here required a second bus, a second registry, or a parallel event
   vocabulary. If a slice needs one, the slice is mis-scoped.
