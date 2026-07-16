# Blueprint: Declared agents and harnesses — beam as agent manager, the runtime as harness factory

## Scope

This document sets the direction that sits on top of the client-side engine
capability specified in [`acp-client-engine.md`](acp-client-engine.md). That
document defines *how* the Go runtime drives another ACP agent (the libacp
client core, the taskengine-step and modelprovider shapes, the permission
invariant). This document defines *what that capability is for*:

- **Agents become declared resources.** The operator registers an agent in
  the runtime — a native chain, or an external ACP agent given as a spawn
  command or endpoint — the same way backends, models, and MCP servers are
  registered today.
- **Harnesses become declared resources.** A harness is a named bundle of
  everything the runtime provides to an agent through the ACP client role:
  workspace, filesystem, terminals, MCP tools, permission routing.
- **Beam becomes an agent manager**, not a one-agent-at-a-time chat: the
  fleet of declared agents and their live sessions is a managed surface.
- **Headless agents fall out** of the same machinery: a harness whose
  permission route is a rule instead of a human, and whose trigger is a
  schedule or bus event instead of a keystroke.

Out of scope, permanently: policing external agents (see the trust stance
below), multi-hop session-semantics completeness for proxy chains before a
consumer exists, and any aggregate-agent multiplexing that hides which agent
a session belongs to.

## The trust stance

**The runtime does not govern external agents. It equips them.** Connecting
an external ACP agent is the operator's explicit vendor-trust decision, made
per declared agent, exactly like choosing a cloud model provider. Work that
must be owned and inspected end-to-end runs as chains — that is what chains
are for. There is no sandboxing layer, no policy middlebox, and no pretense
of containment in this direction: an external agent is a process the operator
chose to run.

What the runtime *does* own is the harness contract: an agent can only reach
the filesystem, terminals, and tools that its harness advertises and serves,
**through ACP**. Capability advertisement at `initialize` and the per-session
`mcpServers` list at `session/new` are the contract on the wire — an agent
cannot call `terminal/create` at a client that never advertised terminals.
That is a protocol contract, not a security boundary, and this document never
claims otherwise.

## The harness is the client role

Everything a coding agent needs from its environment arrives through the
client side of an ACP connection. That makes ACP's client role a harness
interface, and the runtime — which implements that role in Go
(`libacp.Client`, served by `libacp.ClientSideConnection`) — a harness
factory. A harness declaration names:

| Harness field | ACP mechanism | Runtime source |
| --- | --- | --- |
| Workspace | `cwd` + `additionalDirectories` on `session/new` / `load` / `resume` | operator-declared roots |
| Filesystem | `fs/read_text_file` / `fs/write_text_file` served (or capability withheld) | host fs, a worktree, or a virtual store |
| Terminals | `terminal/*` served (or capability withheld) | runtime execution layer |
| Tools | `mcpServers` passed down at session setup | the MCP server registry |
| Permission route | `session/request_permission` answered | forwarded to a human surface (beam, VS Code bridge), or answered by a declared rule |
| Contenox extras | `_meta`, namespaced extension methods | the sanctioned extension points from [`../beam/beam-on-acp.md`](../beam/beam-on-acp.md) |

The same agent binary gets a different harness per job, and the harness is
declared in the runtime rather than hardcoded per editor. This is the point
of using ACP rather than N vendor APIs: capability negotiation already *is*
the harness contract, so "give whatever ACP agent the right harness for the
job" costs a declaration, not an integration.

A harness is implemented as a `libacp.Client` implementation assembled from
these declared parts. The seam is the `Client` interface
(`libacp/client.go`); the connection machinery
(`libacp/clientconn.go`) is shared and harness-agnostic.

## Beam as agent manager: sessions are server-resident, beam attaches to screens

Beam's chat doctrine ([`../beam/beam-on-acp.md`](../beam/beam-on-acp.md))
already binds the chat surface to ACP. The manager direction extends the
*scope* of what beam fronts, not the protocol rules. The model is
tmux-shaped: sessions live and run at the server; a viewer attaches to a
screen, and detaching changes nothing about the session.

- **Fleet is management plane, not ACP.** Declaring agents and harnesses,
  and listing declared agents, is registry work — REST under `/api`, like
  every other declared resource. ACP has no agent-declaration vocabulary and
  should not be bent into one.
- **The runtime is always the driver.** Every running session — native
  chain or external agent via the downward client — is driven by the
  runtime's own connection, never by a viewer. This matters because
  `session/update` is an unreplayable notification stream: whoever is not
  listening at that moment loses it, and a browser tab is the least reliable
  listener there is. The driver is the one witness that is always present —
  for headless sessions, the only one.
- **The virtual screen is a per-session journal at the driver.** The runtime
  records each session's update stream (updates, tool calls, permission
  events). That journal *is* the screen a viewer attaches to, and it is the
  same artifact whether the session was interactive or headless.
- **Attach is ACP-native; the transport is not the design.** Beam holds one
  upward ACP connection to contenox; the fleet's sessions are its
  `session/list` inventory. Attaching is `session/load`: the runtime replays
  the journal as the spec's load-time `session/update` replay, then tails
  live. Multiple viewers may attach to one session; attaching to a headless
  run mid-flight with full history is the same operation. ACP here is
  JSON-RPC over any bidirectional byte stream — both libacp connection types
  take an `io.ReadWriteCloser`, which is why stdio and the WebSocket shim
  (`runtime/contenoxcli/acp_ws.go`) already share one implementation. The
  WS shim is the browser-reality adapter, not a commitment: any transport
  that carries the frames (stdio, TCP, a future HTTP transport as the spec's
  transports work matures) plugs into the same attach model unchanged.
  Nothing about virtual screens, journals, or harnesses may ever assume a
  particular transport.
- **Attribution is never hidden, capability truth is never faked.** Every
  session carries which declared agent runs it. Connection-level
  capabilities advertised upward are contenox's own; per-session differences
  — modes, available commands, config options — flow through the
  session-scoped updates ACP designed for exactly this. A per-prompt
  capability mismatch (an image sent toward a text-only downward agent)
  surfaces as a clear per-prompt error, never as silent degradation.
- **Permissions while unattached** route per harness: answered by a declared
  rule, or queued into a pending-permission inbox that beam surfaces. The
  driving client is always present to receive them, so nothing blocks on a
  viewer existing.

The session-management surface — `session/list`, `session/load`,
`session/resume`, `session/close`, `session/set_mode` — is the vocabulary a
manager renders, and load-replay fidelity in `acpsvc` becomes load-bearing:
advertising `loadSession` without faithful journal replay would break the
attach model at its root. The journal needs an explicit retention policy;
that is a cost this direction accepts knowingly.

## Headless agents

A headless agent is a declared agent bound to a harness whose permission
route is a rule and whose trigger is not a human: a schedule, a bus event, a
webhook. Nothing else changes — the same client core drives the session, the
same harness serves fs/tools/terminals, the same session record is available
for beam to attach to *afterwards* (or live, mid-run, since attach is just an
upward connection to a session the runtime is already driving). `contenox
acpx` established the headless/untrusted-driver profile for the agent role;
this is its mirror image on the client role.

The ramp is deliberate: **manager → harnesses → headless.** Each stage is a
consumer of the previous one, and nothing in a later stage requires
speculative work in an earlier one.

## Why this lands in the Go runtime

Both ACP roles live in one process and one language: `AgentSideConnection`
upward and `ClientSideConnection` downward share the wire layer, the DTOs,
and the test surface in `libacp`. Binding an upward editor connection to a
downward agent session is goroutine plumbing, not a service boundary; a
fleet of concurrent sessions is the runtime's native concurrency model; and
the whole thing ships in the single binary that already embeds beam. The
conformance harness runs both directions against the reference Rust
implementations (deterministic test agent downward, validator client upward),
so "any conformant agent" is a tested claim, not an aspiration.

## Prerequisites and seams

| Prerequisite | Where it lives | Depended on by |
| --- | --- | --- |
| Client core: `Client`, `ClientSideConnection`, cancellation parity | `libacp/client.go`, `libacp/clientconn.go` | everything below |
| Full v1 session surface both roles (list/load/resume/close/set_mode, logout, additionalDirectories) | `libacp` | manager UI, harness workspace field |
| Extension passthrough (`_meta`, namespaced methods) | `libacp` connection seams | contenox-specific harness extras |
| Agent registry (declared agents: chain / spawn command / endpoint) | runtime state store + REST | fleet surface, harness assignment |
| Harness declarations | runtime state store + REST | per-job equipment, headless routes |
| Session journal + faithful `session/load` replay | runtime, surfaced through `acpsvc` | attach model, headless observability |
| Upward session surface spanning driven sessions | `acpsvc` + `runtime/contenoxcli` serve surface | beam manager |
| Rule-based permission answerer + triggers | runtime (bus/cron already exist) | headless agents |

The first two rows are the libacp work; the registry and binding rows are
where this blueprint turns into runtime features; the last row is the
headless stage. Consumers named in
[`acp-client-engine.md`](acp-client-engine.md) (taskengine step,
modelprovider) and [`../opsclient/operator-console.md`](../opsclient/operator-console.md)
(remote-host administration) plug into the same registry and harness
declarations rather than growing parallel ones.

## Build order: the walking skeleton and its waves

The direction is built as independently landable slices, ordered so one
thread through the whole vision exists as early as possible. **The walking
skeleton rule: declare one external agent → the runtime drives it under a
trivial harness → every update lands in the journal → beam attaches to the
screen.** Everything after that deepens the thread; nothing before it is
allowed to gold-plate.

**Wave 1 — substrate** (parallelizable):

- **Agent registry.** Declared agents as CRUD resources following the
  existing registry idioms (backends, models, MCP servers): name, kind
  (`external-stdio` command/args/env first; `endpoint` and `native-chain` as
  declared kinds that may land later), default harness reference. Verified
  through `apitests` like every other registry.
- **Session journal.** The virtual screen as storage: an append-only
  per-session record of every `session/update`, permission event, and
  tool-call event, with a retention policy and a replay iterator. The design
  constraint: replay fidelity is load-bearing, so the journal stores
  wire-shaped frames — what came in is what replays out — never a lossy
  projection. Verified by replay-equals-input tests.
- **Harness compiler.** A harness declaration (workspace roots, fs mode,
  terminals on/off, MCP server references, permission route:
  `forward` | `rule` | `queue`) compiles into a `libacp.Client`
  implementation: the fs and terminal handlers, the permission answerer,
  and the `mcpServers` / `additionalDirectories` payload for `session/new`.
  Verified by loopback tests showing different declarations produce
  observably different capability advertisements and callback behavior.

**Wave 2 — the driver** (the keystone, after wave 1):

- **Session supervisor.** The runtime component that owns downward
  connections: spawns or dials the declared agent, runs
  `ClientSideConnection`, applies the compiled harness, journals everything,
  tracks live sessions, and closes the journal cleanly on agent-process
  death. This is "the runtime is always the driver" as code; the
  supervision idioms in `libroutine`/`liblease` are the reuse candidates.
- **Upward attach surface.** `acpsvc` lists driven sessions in
  `session/list` (agent attribution via `_meta`) and serves `session/load`
  by replaying the journal, then tailing live to N attached viewers.
  Attaching mid-run must be indistinguishable from having watched live —
  that is the acceptance bar, proven with a loopback viewer plus the
  reference test agent as the driven side.

**Wave 3 — visible product:**

- **Beam fleet MVP.** Registry views (agents, harnesses, sessions) from
  REST; attach is the existing beam ACP client calling `session/load` — per
  [`../beam/beam-on-acp.md`](../beam/beam-on-acp.md) the chat surface gains
  no private API, only the registry endpoints are new. Includes the
  pending-permission inbox for `queue`-routed harnesses.

**Wave 4 — headless closes the loop:**

- **Rules and triggers.** The `rule` permission route (allow/deny/match
  patterns) and triggers (cron or bus event → supervisor starts a session
  with agent + harness + prompt). The test of the earlier slices' shape is
  that this wave adds no new machinery: a headless run is a trigger, the
  same driver, the same journal, and a viewer that may never come.

Decisions owned by the operator of this plan, to be settled before wave 1:

1. **Journal storage** — append-only in the durable SQL store versus the
   kv store. Retention, replay scans, and the journal doubling as the audit
   trail all argue for the durable store.
2. **Harness spec shape** — a harness as its own declared resource
   referenced by agents (reusable across agents; the lean of this
   document), versus embedded per-agent configuration.
3. **Skeleton target agent** — which external agent the walking skeleton
   demos against: the reference test agent is the hermetic default; a real
   coding agent is the better demo at the cost of auth and model
   configuration entering the story.
