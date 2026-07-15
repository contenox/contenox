# Blueprint: Beam on ACP — chat re-engineering

## Scope

Re-engineer Beam's chat around the Agent Client Protocol: the chat surface
becomes an ACP client of the same agent (`acpsvc`) that Zed and every other
editor speaks to, over a WebSocket transport hosted by `contenox serve`.
Beam's scope settles to three things: the setup wizard, the admin control
plane, and this ACP chat client. This document designs (A) the reusable
component extraction, (B) the target surface layout, and (C) the migration.

Out of scope: a standalone agent-agnostic client product, desktop packaging,
and public demo hosting. The extraction in Part A is deliberately shaped so
those remain possible later without rework, but nothing here commits to them.

Also out of scope, settled elsewhere rather than open: the Go runtime's own
downward ACP client capability — contenox driving *other* ACP agents as a
taskengine step or a modelprovider implementation — is not a Beam feature and
is not built as a new client app; it lives in the runtime
(`../acp/acp-client-engine.md`). Where that capability needs a human screen
(reviewing a driven agent's permission requests, e.g. for remote-host
administration), Beam is that screen — the same ACP chat workspace this
document specifies, not a separate ops client
(`../opsclient/operator-console.md`).

## The rule that forces this

Beam's chat historically consumed a private REST+SSE surface
(`internalchatapi`, task-event streams, approval heuristics). A private side
door is always cheaper than the protocol, so features take it — and every time
Beam out-delivers ACP, a runtime capability degrades into a UI feature that
Zed users, acp-cli users, and API users don't have. The runtime stops being
the product.

**Doctrine: any capability reachable only through Beam's private API is a
defect in the runtime's protocol surface. The repair direction is always
downward** — into standard ACP where it fits (plans, permissions, config
options, replay), or through sanctioned extension points (`_meta`, namespaced
methods) when genuinely contenox-specific. Corollaries:

- A chat feature PR that touches Beam without a corresponding protocol
  capability is wrong by construction.
- The chat surface imports nothing from Beam's REST API layer. Its only data
  dependency is the ACP client.
- If the chat surface must know it is talking to contenox in order to render
  something, the agent implementation is wrong — never the client.

## Runtime prerequisites (repairs, not Beam features)

These are protocol-surface gaps the target design depends on. Each is a
runtime/acpsvc work item; Beam must not work around them client-side.

1. **Token-level streaming.** The agent emits assistant output as a single
   `agent_message_chunk` at end of turn (wire-verified: the chunk precedes the
   prompt response by microseconds). Provider-level streaming exists inside
   the engine but is not forwarded as incremental task events. Without this,
   no client — Beam, Zed, acp-cli — renders live tokens, and a "streaming
   caret" would be theater. Repair: the engine's step-chunk events must carry
   incremental deltas for streaming-capable tasks, and `translateEvents`
   forwards them as they arrive.
2. **Journal-grade replay.** `session/load` replays messages and tool calls
   but not full execution-event fidelity (step granularity, retries,
   timings). Richer replay benefits every client; until then the degradation
   is accepted — not compensated for with a private endpoint.
3. **`/acp` WebSocket transport** (Part C, phase 0).

## Part A — reusable component extraction

### Boundary rule

`@contenox/ui` must not know contenox runtime schemas. Anything that imports
or mirrors task-engine types is product code and lives in Beam. The package
is presentational: props in, DOM out, strings overridable, tokens for all
color/spacing/type decisions.

### Target layers inside `@contenox/ui`

| Layer | Contents | Notes |
| --- | --- | --- |
| `tokens` | the semantic token sheet (`src/index.css`), fonts, and the design-token guard test | the guard moves into the package it guards; consumers inherit enforcement |
| `chat` | ChatThread (`role=log`/`aria-live` streaming pattern), ChatMessage, ChatComposer, streaming caret/typing/processing indicators, useChatScroll, date separator, transcript markdown components | already i18n-clean via label props; stays transport-ignorant (renders strings) |
| `agent` | ToolCallCard, DiffView + line-diff util, PermissionCard, PlanPanel, UsageMeter, CommandMenu | ACP-*shaped*, not ACP-*coupled*: props mirror the wire structurally (e.g. tool statuses `pending/in_progress/completed/failed`, permission **option arrays** with `allow_once/allow_always/reject_once/reject_always` kinds) but the package imports no protocol library; the client layer maps wire→props |
| `terminal` | TerminalOutput (ANSI), TerminalPromptInput, XTerminal | XTerminal generalizes: connection/transport injected; no Beam token storage or layout-event coupling |
| `overlay` | Dialog, Dropdown/Select, Toast, Tooltip, command palette | requirement, not implementation mandate: focus trap, Escape, portals, full keyboard nav, correct ARIA. Hand-rolled implementations must meet the bar; a headless library styled by the tokens is an equally valid way to meet it. The permission dialog is the highest-stakes consumer — it gates tool execution |

### Renames and reshapes

- ToolCallCard statuses align to ACP's four (`pending/in_progress/completed/
  failed`); ad-hoc status vocabularies are removed.
- PermissionCard replaces the binary approve/deny ApprovalCard: it renders an
  option array and returns the chosen option id. Keyboard bindings (y/n/Esc)
  map onto option *kinds*, not hardcoded buttons.
- PlanPanel is new: renders plan entries (content, priority, status) with
  live status transitions. It is the protocol-native successor of the
  timeline rail.
- The slash-command registry (already React-decoupled) moves next to the chat
  kit; it merges *client-side* commands with the agent's
  `available_commands_update` — agent commands are authoritative, client
  commands exist only for pure client concerns.

### Excisions

- `visualization/` (workflow visualizer, task-event feeds) and the mirrored
  task-engine types move out of `@contenox/ui` into Beam's admin area, taking
  the graph-layout dependency with them. The design system loses all
  knowledge of the runtime.
- Components below the overlay bar with no consumer in the target surface are
  candidates for deletion rather than repair.

## Part B — the surface

One chat surface. It replaces the console, the legacy chat page, and the
dormant `ChatSurface` component (whose injected-client contract is absorbed
into the client package; its view layer is not revived).

### What earned its place (evidence from the surfaces it replaces)

- **Turn-block transcript** (from the console): user command → collapsible
  work section → result. Dense, scannable scrollback.
- **Live streamed body with caret + thinking block** (from the legacy page):
  the one thing legacy did better; depends on runtime prerequisite 1.
- **Keyboard-first interaction** (console): y/n/Esc on permissions, slash
  completion, Enter-to-send with an explicit guard against killing a running
  turn (submitting during a run must never silently cancel it).
- **Bang-shell** (console): `!cmd` runs in a client-owned terminal via the
  terminal kit. This is a client feature by nature; it does not ride ACP.
- **Injected client contract** (dormant ChatSurface): the surface receives a
  client interface, never a transport.

### Data mapping (exhaustive — nothing else feeds the transcript)

| Wire | Rendering |
| --- | --- |
| `agent_message_chunk` / `agent_thought_chunk` | streaming body / collapsible thought block, grouped by `messageId` |
| `tool_call` / `tool_call_update` | ToolCallCard in the turn's work section; diff content → DiffView; terminal content → embedded terminal |
| `plan` | PlanPanel (right rail), statuses advance live |
| `session/request_permission` | PermissionCard, modal; resolves with an option id; torn down on `$/cancel_request` |
| `available_commands_update` | slash completion source |
| `config_option_update` + session config options | status-line selects (model / think / policy); writes via `session/set_config_option` |
| `usage_update` | UsageMeter |
| `session_info_update` | sessions-rail freshness |
| replay on `session/load` | transcript reconstruction, grouped by `messageId` |

User-message echo is client-owned: the client renders its own sent message
immediately and reconciles against replay by `messageId`. No content-matching
heuristics, no timestamp windows — those existed only because the old data
layer ech