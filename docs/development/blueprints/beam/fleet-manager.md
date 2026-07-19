# Blueprint: Beam as Fleet Manager

Owner: runtime

Status: PROPOSED (2026-07-19). Scope draft awaiting maintainer ratification.
Every decision herein is proposed unless explicitly marked DECIDED. Refines
"Wave 3 — Beam fleet MVP" of
`docs/development/blueprints/acp/declared-agents-and-harnesses.md`.

## Purpose

The maintainer operates a fleet: many concurrently running agents across
missions, supervised by mission command — intent and constraints down,
autonomy inside the envelope, exceptions up. Fleet management in every mature
domain (trucking, naval, airline) rests on the same structures: a **manifest**
(registry of units and missions), **dispatch** (allocation, not operation),
**envelopes** (declared bounds whose crossing is automatically an event),
**telemetry → ops board → exception** (readiness states, green silence), and
**maintenance** as one subsystem, never the center. The codebase already chose
this ontology — `runtime/agentinstance/doc.go:84-88`: "Restart keeps the fleet
alive, not the conversation" — and the kernel's own doc comments name "beam's
live fleet view" as the intended consumer of seams that exist but are surfaced
nowhere (`manager.go:45-52`, `doc.go:50-52`).

This blueprint scopes the ops board and its wiring. The maintenance bay
(probes, claims, state-diff forensics) is deliberately out of scope here and
gets its own blueprint once the board exists to surface it.

DECIDED (maintainer, 2026-07-19): the yard speaks ACP. If a UI is needed that
does not speak ACP, we build it ourselves. No design accommodation for
foreign-UI agents.

## Verified Current State

Established by reading the code (two mapping passes, 2026-07-19), not
inferred from names.

**The fleet substrate exists and is unexposed.**

- `agentinstance.Manager.List(ctx) ([]FleetEntry, error)`
  (`runtime/agentinstance/manager.go:114`, impl `:425-471`) returns the full
  config+runtime join: declared-but-idle agents, live instances, and orphans.
  `FleetEntry{AgentID, AgentName, Kind, Instances}` (`manager.go:71-79`),
  `InstanceStatus{ID, AgentID, AgentName, Kind, State, Sessions, Viewers,
  StartedAt}` (`instance.go:34-43`), states
  `starting|running|stopped|error|warning` (`instance.go:24-30`). All
  JSON-tagged. **Zero consumers** — no HTTP route, no CLI, no bus subject;
  `acpsvc` holds the only Manager reference and never calls `List`.
- Lifecycle events exist and fire nowhere: `EventSink` with
  `state_change|attach|detach` and a rich `Event` struct
  (`manager.go:31-65`), installed per-instance at `bringUp`
  (`manager.go:301-309`) — but serve constructs the Manager with no sink
  (`runtime/contenoxcli/serve_cmd.go:330`).
- The Manager is reachable only from `acpsvc` (`serve_cmd.go:362`); it is not
  in `serverapi.Dependencies` (`runtime/serverapi/server.go:75-100`).

**The surfacing substrate exists.**

- SSE pattern to copy: `runtime/internal/taskeventsapi/routes.go` (bus
  subject → `data: {json}\n\n`), consumed by `EventSource` +
  `packages/beam/src/hooks/useTaskEvents.ts` (reconnect/backoff `:95-106`).
  Note it is per-requestId; a fleet feed is a global subject.
- Bus sink pattern to copy: `taskengine.BusTaskEventSink`
  (`runtime/taskengine/events.go:93-142`).
- Beam registration is a table: route entry in
  `packages/beam/src/config/routes.tsx:88`, nav via `adminRouteDefinitions`
  (`:51` → `ControlPlaneDropdown` + `ControlPlanePage`), `headerTitle` case in
  `components/Layout.tsx:92-100`, en+de blocks in `src/i18n.ts`.
- UI kit covers the board compositionally: `StatusIndicator` (statuses map
  ~1:1 onto instance states, `packages/ui/src/components/StatusIndicator.tsx:5-11`),
  `Table`, `Badge`, `Inbox` (the pending/approval pattern),
  `visualization/TaskEventFeed`, `ExecutionTimeline`, panel/layout kit. No
  stat-tile/chart component exists (acceptable; compose `Card`/`KeyValue`).
- Forensics click-through already works: board row → `<Link to="/chat/:id">`
  is the same mechanism the session sidebar uses
  (`components/sidebar/AcpSessionSidebar.tsx:135`).

**What is genuinely missing** (all of it mechanical, none architectural):

| Board need | Status |
|---|---|
| Enumerate units + states over HTTP/CLI | missing surface over an existing primitive |
| Instance-lifecycle events on the bus + SSE | missing wiring of an existing seam |
| Board page/route/hook/i18n in beam | net-new, composed from existing kit |
| Mission records | net-new concept (no `Mission` anywhere; nearest pattern: `acp:session_*` KV prefixes, `runtime/acpsvc/external.go:218,263`) |
| Dispatch by API | missing — instance/session creation exists only via the ACP WebSocket (`external.go:1536-1557`); `POST /tasks` is native-chains-only; `jobqueue` (`runtimetypes/jobqueue.go`) exists unwired; doc comments reserve "a future scheduler (cron/bus → Start)" |

Also load-bearing: restart is **disabled by default** (`WithRestart` unused at
`serve_cmd.go:330`), so today a crashed instance goes terminal `error` and
`warning` ("gave up restarting") is unreachable in practice.

## Invariants

- **Equip, don't govern.** The board observes state and surfaces exceptions;
  it never polices agents. Trust in an agent remains the operator's vendor
  decision. Envelope enforcement stays where it lives today (workspace-root
  allowlists, sandboxed tool paths, HITL policy) — the board *renders* those
  facts, it does not add a policy engine.
- Control-plane isolation holds: nothing here gives an agent fs-reach into
  its own governing config.
- No `lib*` package imports `runtime/`. Fleet code is `runtime/` + beam.
- Every new seam takes `context.Context` and returns `error`.
- The board must be truthful about the restart cost it renders: a restarted
  instance lost its downstream conversation context (`instance.go:243-247`).

## Telemetry Model

Events are defined backwards from the decision loop they feed; anything that
feeds no loop is noise and is not emitted. Four loops, four consumers:

| Loop | Consumer | Signal needed | Latency | Healthy-day volume |
|---|---|---|---|---|
| Exception handling | the maintainer (Inbox feed) | "a unit needs me", nothing else | seconds | ~0 |
| Board sync | beam FleetPage / CLI watch (machine) | every state transition | sub-second | all transitions |
| Mission accounting / forensics | durable record, read on incident | per-mission timeline | none (durable) | small |
| Automation (future: scheduler, probe daemon, auto-restart) | machine bus subscribers | actionable facts | seconds | varies |

RULING (maintainer, 2026-07-19): **telemetry is passive.** It observes and
records for after-the-fact audit; it never triggers product behavior. An
earlier draft of this section proposed an exception classifier that derives
and publishes curated events — REJECTED: that is product machinery, not
telemetry, and is deferred until the board proves the need. The vehicle is
the established tracker pattern (`libtracker.ActivityTracker`, as used by
every other subsystem): agentinstance lifecycle facts (state transitions,
attach/detach, unsupervised denies) are *reported* through an injected
tracker — recorded, queryable afterwards, triggering nothing.

Payload discipline still holds: report identity and fact — ids, agent name,
state, one-line summary — never content. No prompt text, no output. This
keeps the redaction question out of the telemetry path entirely.

Consequences for the board: live state comes from polling `GET /api/fleet`
(TanStack refetch), which is truthful by construction since `Manager.List`
is an in-memory join. Push channels (bus subjects, SSE) are not part of this
blueprint's committed scope; if later slices want them, that is a separate
proposal, not an assumption.

## Slices

Sequenced walking-skeleton; each independently landable and useful without
the ones after it.

### F1 — Expose the manifest (read path)

Add `Instances agentinstance.Manager` to `serverapi.Dependencies`
(`server.go:75`), pass it in `serve_cmd.go:373`, and add a `fleetapi` package
(shape of `agentregistryapi`, `runtime/internal/agentregistryapi/routes.go`)
registered in `registerProductRoutes`: `GET /api/fleet` → `Manager.List`,
`GET /api/fleet/{instanceID}` → `Manager.Get`. A `contenox fleet` CLI verb
over the same data for beam-less verification.

Acceptance: with two declared agents and one live instance, `GET /api/fleet`
returns the config+runtime join including the idle agent; CLI renders it.
~150–250 LoC Go + tests.

### F2 — Passive telemetry via the tracker pattern

Install an `EventSink` at `serve_cmd.go:330` that reports lifecycle facts to
the injected `libtracker.ActivityTracker` — identity and fact only, per the
Telemetry Model ruling. No bus subjects, no SSE, no classifier: recorded for
after-the-fact audit, triggering nothing.

Acceptance: an instance crash and an unsupervised permission deny are both
findable in the tracker log after the fact; nothing in the product changes
behavior because of them. ~80–120 LoC Go + tests.

### F3 — The board page

`/fleet` route + `FleetPage` in beam: `api.getFleet` + `useFleet` (TanStack)
+ an SSE hook (pattern of `useTaskEvents`), rendered with
`StatusIndicator`/`Table`/`EmptyState`, per-instance click-through to
`/chat/:sessionId`, nav entry, `headerTitle`, en+de i18n. Mechanical surface
only — layout/placement decisions delivered as decision-ready facts, not
invented (see Decision Points).

Acceptance: the maintainer answers "what is my fleet doing right now" from
one page, live, including pending-permission and warning/error rows first.
~400–600 LoC TS.

### F4 — Mission records (the manifest's other half)

Net-new, smallest honest version first: a mission is a durable note bound to
work — `{id, intent (one line), agentName, sessionIDs, status:
open|landed|derailed|abandoned, createdAt, updatedAt}`. Attachable to a
session at creation or after; rendered on board rows; editable from the
board. This delivers the "at least allow to note stuff and plans" floor of
the product. Envelope references and richer structure come later, after use.

Acceptance: dispatching or adopting a session with a mission note shows the
intent on the board; a mission outliving its sessions remains listed as open.
~200–300 LoC + storage decision (see Decision Points).

### F5 — Dispatch by API

`POST /api/fleet/dispatch` → `Manager.Start` + `OpenSession` + first
`Prompt`, returning instance/session/mission ids. This is the "future
scheduler (cron/bus → Start)" seam the kernel docs reserve
(`manager.go:45`, `doc.go:37,50`). Queued dispatch via `jobqueue` is a later
increment; not in this slice.

Acceptance: a curl dispatches a declared agent on a mission and the board
shows it without beam involvement in the creation path. ~150–250 LoC + tests.

### F6 — The exception feed

Exception v1, defined narrowly: `state_change` into `error` or `warning`,
plus a pending permission request with no attached controller. Rendered with
`Inbox`/`TaskEventFeed` on the board (and nothing else pushed anywhere).
This is the socket the maintenance bay (probes, claims, state-diff) plugs
into later — future exception sources publish to the same subject and appear
in the same feed without board changes.

Acceptance: an unsupervised permission request and a crashed instance both
appear as exceptions; a healthy fleet renders an empty feed. Mostly
composition; ~100–200 LoC.

## Decision Points

All RESERVED to the maintainer unless marked DECIDED.

- **DECIDED (2026-07-19): ACP-only yard.** Foreign UIs are replaced with our
  own rather than integrated.
- **Board placement:** `adminRouteDefinitions` (control-plane dropdown) vs a
  top-level navbar entry. Facts: admin route registration is one line and
  auto-feeds dropdown + hub; a top-level entry is a bespoke navbar change.
  Fleet-as-daily-surface argues top-level; mechanism argues admin-first and
  promote later.
- **Mission storage:** KV prefix keyed by mission id (pattern of
  `acp:session_*`, zero migration) vs a table beside `agents` (queryable,
  matches the reserved `HarnessID`/`WorkspaceID` seam style,
  `runtimetypes/agents.go:97-118`). Facts favor a table if missions are ever
  listed/filtered server-side; KV if they stay board-rendered only.
- **Restart default:** with restarts disabled, `warning` is unreachable and
  every crash is terminal `error`. The board makes the
  `warning`/"gave up restarting" distinction visible and useful for the
  first time — decide whether serve should now pass `WithRestart(n)`, and
  what n is. Restart cost (lost downstream conversation) renders on the
  board either way.
- **Dispatch semantics:** synchronous first-prompt (simple, blocks the
  caller) vs accepted-and-async (returns ids immediately, outcome arrives as
  events). F5 proposes async-after-OpenSession; ratify or simplify.

## Acceptance

The fleet manager exists when:

1. Every declared agent and every live instance, with state, session count,
   viewer count, and mission intent, is enumerable from one board, live.
2. Exceptions (error, warning, unsupervised permission) reach the maintainer
   through the feed without any polling of sessions; a healthy fleet is
   silent.
3. Dispatch of a declared agent on a mission requires no vendor UI.
4. The board's data path is REST/SSE over primitives that already existed
   (`Manager.List`/`Get`/`EventSink`) — if a slice requires teaching the
   kernel new concepts beyond mission records and the
   `unsupervised_permission` event kind (surfacing a judgment the kernel
   already makes), the slice is mis-scoped.

Estimated total: ~1.2–1.8k LoC across Go and TS, no new architecture, no new
dependencies. The kernel side was already built and documented for exactly
this consumer; the work is surfacing it.
