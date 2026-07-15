# Blueprint: ACP Chat Workspace — reusable components, layout, and beam re-engineering

This document covers two things that outlive any one UI: the **reusable
component library** extracted from beam, and the **chat workspace layout** those
components compose into. It also specifies how beam's chat is re-engineered onto
ACP so it stops being special.

It supersedes and folds in the prior workspace blueprints (sovereign-workspace,
sovereign-workspace-architecture, chat-canvas). Their durable ideas — the
brain/glass split, UX sovereignty, the renderer-agnostic canvas, the
two-artifact model, the terminology discipline, and the truth-gate — are carried
below. What is dropped is deliberate: those documents planned the workspace on
beam's **private** endpoints (`internalchatapi` mode/context, task-event SSE,
`WidgetHint` plumbing) and positioned `contenox serve` as the hero surface. This
blueprint routes the same workspace through **ACP** instead, which is what makes
the capability shared rather than beam-only.

## The load-bearing principle

Beam's chat must be a **client of `acpsvc`** — the same agent Zed and any other
ACP client talk to — not a consumer of private runtime endpoints.

The failure mode this prevents: adding one more beam-only endpoint is always
cheaper than landing a capability in the protocol, so every chat feature drifts
into a side door (`internalchatapi`, task-event SSE, approvals inferred
heuristically from later events). The result is a UI that out-delivers the
runtime's own protocol surface — at which point the capability has escaped the
product and Zed/API/CLI users don't have it.

Rule: **anything the chat UI can do that a conformant ACP client cannot is a
defect in the runtime's ACP surface, and the repair direction is always
downward** — into `acpsvc` (as standard methods or sanctioned extension points),
never sideways into a beam endpoint. If the UI ever needs to know it is talking
to contenox specifically to render something, the agent is wrong.

Corollary: the same components and the same client core serve three consumers
without divergence — beam's embedded chat, a future standalone ACP client, and
live demos on the website. None of them is privileged; contenox is one agent in
a list.

This "ACP client" is the browser/beam side driving contenox upward — not to be
confused with the Go runtime's own downward ACP client capability (contenox
driving *other* agents, including other contenox instances), which lives in
the runtime rather than in beam or any web client; see
`../acp/acp-client-engine.md`. The "future standalone ACP client" above is
scoped to this chat surface; it is not the shape the operator/ops console
takes — beam is the settled screen for that, see
`../opsclient/operator-console.md`.

---

## Part A — The reusable component library

### A.1 Three layers of unequal value

`@contenox/ui` is not one asset. Audited by intent, it splits three ways:

| Layer | Disposition | Why |
| --- | --- | --- |
| **Chat / terminal / diff / tool-call + tokens** (~3.2k LOC + token sheet + Storybook) | **Keep and extract** | No ecosystem equivalent; correct `aria-live` streaming, i18n-clean label props, dual-theme rigor enforced by `designTokenGuard` |
| **Generic overlays** (Dialog, Dropdown, Toast, Tooltip, …) | **Keep the API, back with headless primitives** | Hand-rolled versions lack focus traps / keyboard nav / portals; permission dialogs demand these. Style the ecosystem primitive with the same tokens |
| **Product-specific** (`visualization/`, `taskTypes.ts` mirroring Go schemas, admin scaffolding) | **Excise from the library** | Product code in the foundation layer; belongs in beam, not in a design system a second product imports |

The keep-layer is the only part with genuine, unique, competitive value. It is
also already proven in a second consumer (the VS Code webview). Everything else
is either replaceable by ecosystem defaults or is beam-specific.

### A.2 The extraction: `@contenox/chat-kit`

Extract the keep-layer into a **presentation-only, transport-agnostic** package.
Hard rules for what may live in it:

- **No data fetching, no protocol knowledge, no contenox types.** Components
  take plain props and render. A component that imports a `TaskEvent` shape, a
  chain type, or an API client does not belong here.
- **Semantic tokens are the styling contract.** The token CSS + `fonts.css`
  ship with the package; `designTokenGuard` moves *into* this package's test
  suite (today it lives in beam's tests, auditing ui from outside — wrong
  direction).
- **Every string is an overridable label prop defaulting to English.** The
  package carries no i18n framework; the consumer supplies translations by
  passing props. This is the pattern `ChatMessage` already models.
- **Headless where interaction is hard.** Overlays (dialog, menu, listbox,
  tooltip, toast) are composed over a headless primitive library and skinned
  with tokens. Do not hand-roll focus management again.

Component inventory (the seam an ACP client renders against):

| Component | Renders | ACP source it maps from (consumer wires this, not the component) |
| --- | --- | --- |
| `Transcript` / `Message` | streamed message list, thinking blocks, copy, collapse | `agent_message_chunk`, `agent_thought_chunk`, `user_message_chunk`, grouped by `messageId` |
| `StreamingCaret` / `TypingIndicator` | live-generation affordance | chunk arrival between turn start and `stopReason` |
| `ToolCallCard` | tool title/status/args/output, expandable | `tool_call` + `tool_call_update` (status, `rawInput`, `rawOutput`) |
| `DiffView` (real LCS line diff) | file diffs inside tool cards | `tool_call` content of type `diff` |
| `TerminalOutput` / `TerminalEmbed` | ANSI output, live terminal attach | `tool_call` content of type `terminal` |
| `PlanPanel` | ordered plan entries with status | `plan` updates |
| `PermissionDialog` | option list, keyboard-first accept/deny | `session/request_permission` (options with `kind`) |
| `ConfigOptionControls` | select/boolean dropdowns | `configOptions` in session responses + `config_option_update` |
| `SlashCommandMenu` / `Composer` | input, `/` and `@` completion, soft limit | `available_commands_update`; composer emits plain prompt text |
| `UsageMeter` | context budget indicator | `usage_update` (`used`/`size`) |
| `SessionList` | pick/rename/delete/resume | `session/list` + `session_info_update` |

Named gems to preserve as-is: `TerminalPromptInput` (no ecosystem equivalent),
the xterm lifecycle wrapper (`XTerminal` — Strict-Mode-safe deferred connect,
ResizeObserver fit, theme bridge), the slash-command registry (React-decoupled,
`/` vs `@` split, arg completions), and the token-sheet + `designTokenGuard`
pairing.

### A.3 The client core: `@contenox/acp-web-client`

A second, separate package: the transport-agnostic ACP **client** that turns a
connection into the props `chat-kit` renders. This is the implementation of the
long-dormant `BeamChatClient`-style seam.

- Depends on the official ACP TypeScript SDK for wire types and framing — do not
  hand-roll a third protocol implementation (the runtime already maintains
  libacp and the vscode agent's bespoke RPC; a third would drift the same way).
- Exposes a small interface: `listSessions / newSession / loadSession /
  resumeSession / deleteSession / prompt(handlers) / cancel /
  setConfigOption / respondPermission`. `prompt` streams via handlers
  (`onMessageChunk`, `onThoughtChunk`, `onToolCall`, `onPlan`,
  `onPermissionRequest → Promise<optionId>`, `onUsage`).
- **Connection is a pluggable adapter**, so the same core runs over: a
  WebSocket to `/acp` (browser / demo / remote), or a spawned stdio subprocess
  (a desktop shell, later). The renderer never knows which.
- Knows nothing contenox-specific. Chains arrive as slash commands, model/think/
  policy as config options, history as `session/load` replay — all standard.

Dependency arrow: `beam → chat-kit`, `beam → acp-web-client → chat-kit-types`.
`chat-kit` never imports the client; the client never imports React views.

### A.4 Package hygiene carried into the extraction

The current package has debt that must not survive the move: a junk `tsc`
runtime dep, a dead `tailwind-config` export, a stale compiled `components.css`,
and runtime/dev/peer deps that were mis-sorted. Extraction is the moment to fix
these, not replicate them.

---

## Part B — Experience: how it makes people productive

### B.0 The productivity thesis

The product is marketed as **"AI workflows you can run, review, and own"** — a
*local-first workflow runtime for specific, reviewable AI work*, explicitly not
"just a chatbot." The hero demo is the **human-in-the-loop approval gate**: the
agent asks to run a command, waits at the gate, and reports back. The stated
value is that **the chain is the contract** — prompts, tool allowlists, command
policy, budgets, and approval gates are visible, diffable, repeatable — and that
everything is **local, no account, no telemetry**.

That framing dictates the look and feel. Users do not arrive wanting a nicer
message list; they arrive wanting to *trust, review, and own* what an agent
does. So the workspace optimizes three moments, in priority order:

1. **The approval moment (trust).** A permission request is the product's
   signature interaction, not a modal afterthought. It must show *exactly* what
   will happen — the command, the diff, the tool and its arguments, the policy
   that gated it — with a keyboard-first accept/deny and an always-visible "what
   rule caused this?" affordance. This is where the "review" promise is kept or
   broken.
2. **The review moment (comprehension).** After (or during) a turn, the user
   inspects *what happened*: diffs as diffs, a run as a timeline, a plan as
   ordered steps, terminal output as a terminal. Reviewing must be as fast as
   reading — selection-driven, one click from a turn to its artifact, no hunting.
3. **The ownership moment (control).** The chain, policy, model routing, and
   providers that govern the session are visible and editable *from the same
   place you run them* — not hidden in a separate admin app. "The chain is the
   contract" is only true if the contract is one click away and diffable.

Everything below serves those three moments. A design choice that makes the
message list prettier but the approval gate weaker is the wrong choice.

Underneath the marketing thesis is a strategic one: **own the governance
surface, not the text buffer.** Editors (VS Code, Zed) are "open and extensible"
until you want AI features, at which point the vendor's agent chrome owns the
context window, onboarding, and review flow. Contenox does not need a text buffer
as its product — it needs ownership of *what the agent is about to do, what it
did, and the policy that governs it*. That is the governance surface, and it is
exactly the three moments above. Ceding it to an editor's Copilot pane is the
failure this workspace exists to prevent — while still speaking ACP *to* those
editors as a secondary surface, so an operator who prefers to type in Zed keeps
the same runtime, policy, and review semantics.

### B.0.1 The keep/reject filter

Every proposed workspace feature answers three questions; a "no" routes it
elsewhere rather than into the workspace:

1. **Can the runtime drive this headlessly (ACP / CLI / MCP), without an editor
   vendor's UI?** If no → fix the ACP surface first (repair downward, §the
   load-bearing principle). The UI never becomes the only way to reach a
   capability.
2. **Does it need the full context window — the run, the artifact, and the
   verification together?** If no → it belongs in the CLI or an editor
   attachment, not a bespoke panel here.
3. **Is the primary user action review / direct / steer, rather than
   character-by-character typing?** If no → it is editor work; attach an editor
   over ACP, do not rebuild one.

### B.0.2 Reclaimed assets (built, currently unreachable)

A recent redesign stripped beam's navigation and orphaned substantial built work.
The workspace does not start from zero — it *reclaims* these, which is also why
the polished experience is achievable incrementally rather than as new build:

| Asset | Current state | Role in the workspace |
| --- | --- | --- |
| Monaco integration (`ChainJsonEditor`, `PolicyEditor`, `ExpandedMessageEditor`, `monacoAppTheme.ts`) | orphaned pages + trapped in legacy chat | Code/diff **viewer** in the review pane; chain/policy **editor** in the ownership surface; expanded composer |
| Canvas/artifact system (`WorkspaceSplitPanel`, `CanvasPanel`, `useCanvas`, `lib/artifacts/canvas.ts`, `TimelinePanel`) | wired only into the dying legacy `ChatPage` | The review pane itself: click a turn/event → project to a canvas artifact. Re-point its data source from task-events to `session/update` |
| Graph/workflow visualizer (`ChainVisualizer`, dagre) | orphaned with `ChainsPage` | Visualize the chain-as-contract in the ownership surface and as a run's shape |
| `FileTree` file explorer (`@contenox/ui`) | built, **never imported** | Browse the session `cwd` / additional directories; anchor a file into a diff or the context artifact |
| `Cmdbar` / `CommandPanel` command palette (`@contenox/ui`) | built, **never imported** | The keyboard spine (§B.5): sessions, slash commands, config options, "jump to" — one surface |
| HITL policy editor (`PolicyEditor`) | orphaned `HitlPoliciesPage` | The "what rule caused this?" drill-down from an approval gate |

Rule: prefer reclaiming a built asset (re-pointing its data source to ACP) over
rebuilding. The excise-from-library rule (§A.1) still applies to the *packaging*
of the visualizer and Go-schema mirrors, but the *capability* is kept.

### B.1 What to keep from each existing surface

Beam has built the same chat three times; each contributed one good idea. The
polished layout is the union, not any single one:

- **From the console** (`pages/admin/console/`): the **turn model** (user
  command → work log → result), durable per-turn hydration, in-session run
  retention/scrollback, unified diff and approval verdicts inline, keyboard-first
  approvals, slash/bang commands. This is the best *rendering* prototype and the
  closest to ACP's shape.
- **From the legacy chat** (`pages/admin/chats/`): **live streamed tokens** with
  caret and a thinking box, the **workspace right rail** (Timeline / Canvas /
  Terminal), the blocking-setup guard, not-found/error states, and composer
  amenities (expanded editor, attachment pills, per-message copy).
- **From the dormant `ChatSurface`**: the **injected-client contract** and the
  ACP-shaped permission model (option arrays with `kind`), which is the seam the
  whole thing hangs on.

### B.2 The two-artifact model (load-bearing)

Keep the split cleanly, because collapsing it is the classic mistake:

- **Context artifact** — per-turn, model-visible; *what the user attaches to this
  message*. "I attached this file / terminal output to my prompt." In ACP terms
  it becomes prompt content blocks (text + resource links) plus, where
  sanctioned, `_meta`. Lives in the composer, sent at turn time.
- **Canvas artifact** — session-scoped, human-visible; *the thing being worked on
  or reviewed right now*. "This is the diff / plan / run we are looking at."
  Driven by `session/update` notifications, never by a private task-event reducer.

They are different objects with different lifetimes; some events produce both
(a preview URL is a canvas artifact; its screenshot becomes a context artifact
only when the user explicitly attaches it). Do not overload one for the other.

**The review pane is a renderer-agnostic slot, not a fixed viewer.** It starts as
an honest empty state ("No artifact selected — diffs, plans, run views, terminal
output, and files appear here.") and gains renderers incrementally. Each renderer
is a projection of what the protocol already sends — `projectUpdateToCanvas(update)
→ CanvasArtifact | null` — so nothing here needs a bespoke backend field:

| Canvas renderer | Projected from | Reclaimed component |
| --- | --- | --- |
| `diff` | `tool_call` diff content | Monaco read-only diff |
| `run` / timeline | the turn's `session/update` stream | `TimelinePanel` |
| `plan` | `plan` updates | plan list |
| `terminal_output` | `tool_call` terminal content | `TerminalOutput` / `XTerminal` |
| `file_excerpt` | a file opened from `FileTree` or a resource link | Monaco read-only |
| `markdown` / `message` | assistant/step text a user pins | markdown renderer |

Renderer priority follows the productivity thesis: **diff and run first** (the
review moment), then plan and terminal, then file and markdown. Media/preview
renderers are last — the canvas is a review surface, not a general document app.
State (open/closed, width, selection) persists per session in local storage.

Selection is the whole interaction: the timeline/transcript emits a selected id,
a small pure reducer (`useCanvas`) projects it to the current artifact, the pane
renders it. The pane never owns run state and never derives domain objects with
`useMemo` in the page tree.

### B.3 Layout — three zones plus a command spine, desktop-first

```
┌──────────┬───────────────────────────────┬─────────────────────┐
│ Sessions │         Transcript            │   Review / Workspace │
│  rail    │  (turns: cmd → work → result) │  (Diff / Timeline /  │
│          │                               │   Terminal / Files)  │
│ list +   │  live tokens, thinking box     │                     │
│ resume/  │  tool cards advance in place   │  selection-driven:  │
│ delete   │  plan entries inline           │  click a turn/event │
│ + fresh- │  ───────────────────────────  │  → project to one   │
│  ness    │  Composer: / and @ complete   │  canvas artifact    │
└──────────┴───────────────────────────────┴─────────────────────┘
     ╔═══ Permission gate (the signature moment) ═══╗
     ║ what will run (cmd / diff / tool+args)       ║
     ║ which policy gated it → drill to PolicyEditor ║
     ║ [Allow once] [Allow always] [Reject] — keys   ║
     ╚═══════════════════════════════════════════════╝
   ⌘K command palette: sessions · commands · config · jump-to
```

Zone rules, ordered by the productivity moments they serve:

1. **The permission gate is a first-class surface, not a rail.** It renders the
   full request: the command or the diff or the tool + arguments, plus the
   **policy name that gated it** as a link that drills into the (reclaimed)
   `PolicyEditor` — the literal "the chain is the contract, reviewable" promise.
   Keyboard-first: the option list from `session/request_permission` maps to
   accessible buttons with `kind`-aware default framing (allow-once / always /
   reject), Enter/Escape bound, focus trapped, focus restored. On a cancelled
   turn it tears down cleanly (the runtime now emits `$/cancel_request`).
2. **Transcript is the spine and renders live.** Turn-structured (console's
   model), but each turn renders as it happens — streamed `agent_message_chunk`s
   with caret, a thinking box, tool cards that advance through
   `tool_call_update` *in place* (never a new card per update), plan entries
   inline. Closes the single largest current gap (§C.0).
3. **Review pane is selection-driven, never a duplicate stream.** Clicking a turn
   or an event projects it into exactly one artifact — a diff in Monaco's
   read-only viewer, a run as `TimelinePanel`, a plan, terminal output, or a
   file via `FileTree`. Tabs: Diff / Timeline / Terminal / Files. This *is* the
   reclaimed canvas system, re-pointed from task-events to `session/update`.
   Reviewing must feel like reading: one click, no hunt, diffs shown *as diffs*.
4. **Sessions rail** is `session/list` with resume/delete and
   `session_info_update` freshness; collapses to a drawer on narrow viewports.
5. **Command palette (⌘K) is the keyboard spine** — see §B.5.
6. **Ownership is one click from running.** The chain/policy/model/providers
   governing the session are reachable *from the workspace*, not a separate app:
   the chain-as-contract opens in `ChainJsonEditor` + `ChainVisualizer`, the
   policy in `PolicyEditor`. Editing is diffable; local-first, no round-trip to a
   service.
7. **The permission gate is phone-usable.** Layout is desktop-first, but the
   gate is not desktop-only: on narrow viewports the workspace collapses to
   transcript + gate, with the option list from `session/request_permission`
   rendered as thumb-reachable accept/deny controls. This is what makes beam
   viable as the operator console for remote-administration sessions, not just
   coding sessions — see `../opsclient/operator-console.md`.

### B.4 Interaction & visual language (what makes it feel productive)

- **Keyboard-first, mouse-optional.** Every core action — send, stop, approve,
  deny, switch session, open a config option, jump to a turn — has a key
  binding, surfaced through the command palette and inline hints. The signature
  approval interaction is operable without leaving the keyboard.
- **Progressive disclosure.** Default view is calm: turns, results, the gate.
  Detail (raw tool args, full run timeline, thinking, the governing chain) is one
  deliberate expand away — never a wall of JSON by default, never hidden so deep
  it can't be reviewed.
- **State legibility over decoration.** Status is carried by the semantic token
  ramps (pending / in-progress / completed / failed, allow / deny), consistent
  across tool cards, plan entries, and the gate — the same vocabulary everywhere,
  enforced by `designTokenGuard`. No bespoke per-component color.
- **Motion is functional, not ornamental.** The streaming caret, a card
  advancing its status, an artifact sliding into the review pane on selection —
  motion signals *what changed*. Honor `prefers-reduced-motion`.
- **Trust signals are explicit.** "Local, no telemetry" is a product claim the UI
  should make legible: session/config/state visibly on-device, the governing
  policy always nameable from the gate, the model/provider for a turn visible in
  the usage strip.
- **Density is deliberate.** This is a review/ownership tool for a technical
  operator, not a consumer chat toy — favor information density (compact turns,
  monospace where it aids scanning) over whitespace-heavy chat styling, while
  keeping the calm-by-default disclosure.

### B.5 The command palette — the keyboard spine

Reclaim `Cmdbar`/`CommandPanel` (built, never wired) as the single ⌘K surface
that unifies what today is scattered across toolbar, sidebar, and composer:

- **Sessions:** switch / new / resume / delete.
- **Slash commands:** the `available_commands_update` set, with the same registry
  that powers `/` completion in the composer — one source, two surfaces.
- **Config options:** the `configOptions` list (model, think, policy, token
  budget) as searchable actions instead of a toolbar of dropdowns.
- **Jump-to:** any turn, any tool call, any open artifact.

This is also the accessibility and discoverability backbone: a new user finds
every capability by typing, an operator drives the whole workspace without the
mouse. It is the one component whose absence most visibly separates "a chat page"
from "a workspace."

### B.6 Polish gaps to close (present in none of the three surfaces today)

- **Syntax highlighting** in transcript code blocks and diffs — Monaco already
  ships (reclaimed) and provides it for the review pane; the inline transcript
  needs a lightweight highlighter.
- **Virtualization** of long transcripts / large tool outputs.
- **Consistent i18n** — several keep-layer components still carry hardcoded
  English aria-labels and status strings; these become label props during
  extraction.
- **Empty / error / not-found / broken-setup states** as designed surfaces, not
  blank panes — the blocking-setup guard the legacy page had, generalized.

---

## Part C — Re-engineering beam (incremental, truth-gated)

Non-negotiable: **no big-bang rewrite.** Each slice ships behind the existing
product-surface-truth gate — it must be demonstrably real (a reproducible
end-to-end check) and must not break plain chat, terminal, approvals, or the
VS Code webview. Slices are ordered so the runtime surface is repaired *before*
the UI is asked to consume it.

### C.0 Prerequisite runtime repairs (downward, in `acpsvc`)

The UI cannot render what the protocol does not emit. These land first, each
wire-verified against a conformant client:

- **Live token streaming over ACP** — DEFERRED BY DESIGN, not a repair. Contenox
  was designed for *managed agents running in the background with no human
  watching*, where a chain is a sequence of **steps that act as guardrails**: the
  next step (validator, router, policy gate, HITL check) evaluates the *complete*
  output of the previous one before anything proceeds. Streaming tokens to a
  human mid-step would surface output that has **not yet passed its guardrail** —
  which is antithetical to "reviewable, gated, repeatable work." So the blocking
  whole-message path is the architecturally honest default, not a bug: it is what
  makes a step's output a reviewable unit. Do not "fix" it thinking it is an
  oversight.
  Mechanically: the streaming branch (`taskexec.go` `executeLLM`) is gated on
  `len(tools) == 0`; ACP answer tasks carry `"tools": ["*"]`, so they take the
  blocking path and emit one `agent_message_chunk` at end-of-turn. Streaming
  works and is per-token at the provider; the gate (not the provider) is what
  turns it off. The gate is provider-agnostic — ALL providers are affected — and
  `StreamParcel` already carries a `ToolCalls` field for the eventual fix, but
  local llama's `Stream()` returns `UnsupportedFeatureError` for tool calls, so
  even opening the gate is real per-provider work.
  Interactive chat and the guardrail model are in genuine tension, not a missing
  feature. **Build streaming only when interactive UX actually demands it**, and
  when it does, the philosophically-aligned answer is chain-shaped, not
  core-shaped: stream ONLY the final, user-facing answer step — the one step with
  no downstream guardrail to feed — via a dedicated streaming answer task, while
  every intermediate (guardrail-feeding) step stays blocking. That avoids the
  high-blast-radius change to core `executeLLM` entirely. Decoupled from C.1–C.4:
  the UI renders the final answer without it, exactly as the console does today.
- **`/acp` transport on `contenox serve`.** A WebSocket endpoint that wraps the
  connection as an `io.ReadWriteCloser` and hands it to
  `libacp.NewAgentSideConnection(conn, acpsvc.New(deps))`. Serve already hosts a
  WebSocket path (terminal sessions) and already has every dependency acpsvc
  needs. Inherits serve's bearer-token auth; the token is mandatory off
  loopback (already enforced). This is the ~50-line brick every consumer needs.
- **Replay fidelity** where the console's journal hydration currently
  out-delivers `session/load` — richer replay (step granularity, tool-call
  detail) so the protocol matches what the side door provided, rather than the
  UI keeping a private advantage.

### C.1 Extract the library

Split `chat-kit` and `acp-web-client` out of `@contenox/ui`/beam per Part A.
Beam keeps importing them; the VS Code webview repoints to the new packages
(removing the current relative path-reaching). Excise `visualization/` and the
Go-schema mirrors from the library into beam. Move `designTokenGuard` into
`chat-kit`.

Truth gate: `@contenox/ui` consumers (beam, webview) build and render
unchanged; the token guard runs green inside the new package.

### C.2 Back the overlays with headless primitives

Replace the internals of Dialog / Dropdown / Toast / Tooltip with a headless
library, keeping the public props and token styling. The `PermissionDialog`
built on this is the acceptance case.

Truth gate: a permission dialog traps focus, is keyboard-operable end to end,
and restores focus on close; existing overlay consumers are visually unchanged.

### C.3 Adapter: beam chat speaks ACP

Implement `acp-web-client` against the `/acp` WebSocket and wire the workspace
layout's data layer to it — `session/update` handlers replace the task-event
reducers, `session/request_permission` replaces heuristic approval inference,
`configOptions` replace the hand-plumbed toolbar state. The console's turn/
retention/hydration model ports *behind* the client interface.

Truth gate: beam chat, driven entirely over `/acp`, reaches parity with the
current console on a real turn (≥3 distinct update kinds, live tokens, a tool
card, a plan, a permission round-trip) — and the identical turn works from Zed
and the headless CLI, proving no beam-only path remains.

### C.4 Demote and delete the side doors

Once C.3 holds, the legacy chat page and the private chat endpoints
(`internalchatapi` chat path, the bespoke approval inference) have no consumer.
Remove them. Beam shrinks to setup wizard + admin control plane + the ACP chat
workspace.

Truth gate: no chat code path reaches the runtime except through `/acp`; a grep
for the removed endpoints returns only history.

### C.5 Standalone client and demo (optional, same core)

With C.1–C.3 done, a standalone client is the same `chat-kit` + `acp-web-client`
in a different shell (web build, or Electron/desktop with a stdio adapter), and a
website demo is the same packages pointed at a hosted, hardened (`acpx` profile,
rate-limited) contenox over `/acp`. No new UI, no new protocol surface.

Scope note: this is a standalone *chat* client — a different shell for the same
components. It is not a standalone ops/administration app; that surface's
answer is beam itself, served from the operated box (see
`../opsclient/operator-console.md`), not a second client product.

---

## Non-goals

- **Fork or embed an editor core.** No custom text buffer, no competing on
  typing latency. Editors attach over ACP; they are not rebuilt here.
- **File-tree-first landing.** The workspace opens on the run and the gate, not a
  filesystem. `FileTree` is a review affordance, not the home screen.
- **Rebuild orchestration in the client.** Chains, HITL, sessions, taskengine
  stay in the Go runtime — the single source of truth. The client renders and
  steers; it never executes or owns durable state.
- **Monaco as the primary surface.** Monaco appears *only* inside a canvas
  renderer (diff / file / chain-or-policy editor), never as the workspace's main
  buffer.
- **Multiplayer / real-time collaboration in the first cut.** Review sharing is a
  later link-based feature, not co-editing.

## Terminology discipline

Use **workspace**, **transcript**, **canvas**, **timeline**, **verification**,
**gate**. Avoid **IDE** / **ADE** / "replacement for VS Code" in product copy,
marketing, and code comments — beam is the governance control center, not a
typist's desk. Do not call the run log or timeline the "primary pane"; the
transcript and the gate are primary, the canvas is the review surface.

## Invariants (anti-patterns to reject in review)

- A chat feature that lands as a beam endpoint instead of an `acpsvc`
  capability. (Repair downward.)
- A `chat-kit` component importing a contenox type, an API client, or a
  `TaskEvent`/chain shape. (Presentation only.)
- A hand-rolled focus trap / keyboard menu / portal. (Use the headless
  primitive.)
- A hardcoded user-facing string without an overriding label prop.
- The UI holding a capability no conformant ACP client can obtain. (The
  advantage has escaped the product.)
- A third hand-rolled ACP protocol implementation. (Use the official SDK.)
- Deriving core domain objects with `useMemo` in the page render tree instead of
  a pure reducer over `session/update`.
- A big layout rewrite before the panels have content, or deleting the terminal /
  legacy path before its replacement is demonstrably better.
