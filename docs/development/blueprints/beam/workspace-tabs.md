# Workspace Tabs: an in-app tabbed workspace

Status: draft blueprint, 2026-07-16. Decisions settled with the maintainer.
Supersedes the terminal-sidebar and side-panel-file-preview parts of
shell-sessions.md / session-workspace-files.md.

## Concept

The chat page's center becomes a single **in-app tabbed workspace** — one
React tab-manager component (`WorkspaceTabs`) holding N tabs, each a
polymorphic **surface**. Not browser tabs; an editor-style tab area. A chat
session is one kind of surface. Files, terminals, and diffs are others. Open
as many as you want, close them, switch between them.

- **Navigators (rails) list; tabs hold.** The session list and file tree stay
  as collapsible rails whose job is to *open* things into tabs. Opening the
  same thing focuses its existing tab (dedup by identity).
- **Surfaces:** `ChatSessionTab` (a conversation — today's chat body,
  extracted), `FileTab` (real editor view, replacing the toy preview),
  `TerminalTab` (live PTY, replacing the sidebar), `DiffTab` (an agent edit).
- **Maximize is the universal "expand into a tab" verb:** a shell card's
  maximize opens a `TerminalTab`; a file reference opens a `FileTab`; an edit
  card opens a `DiffTab`. The transcript card stays the compact inline record.
- **Closing a chat tab ≠ deleting the session** (`session/close` vs
  `session/delete`, already distinct). The session stays in the list.

## Decisions (maintainer, 2026-07-16)

- **Single virtual screen, NO split view** for now (one visible tab at a
  time). Split is a clean later extension via `ResizablePanel`.
- **All open chat tabs stream live concurrently** — NOT load-on-focus/suspend.
  Justified by the code (below): the wire already multiplexes.

## Why "all tabs live" is cheap (the multiplexing finding)

- `transport.ts` is a dumb pipe — one JSON-RPC message per WS frame, no
  session awareness (`transport.ts:8`).
- The client ALREADY namespaces by sessionId: `subscriptions = new
  Map<SessionId, SessionEventHandlers>()` (`client.ts:263`); every
  `session/update` is demuxed via `handlersFor(params.sessionId)`
  (`client.ts:420,434`). N concurrent subscriptions over one connection are
  already supported.
- The single-active limit is a CONTROLLER policy, not a wire limit:
  `acpWorkspaceController.ts` keeps one `activeSessionId`/`unsubscribeActive`
  and documents "one subscribed session at a time … the standing subscription
  always wins" (`acpWorkspaceController.ts:28-33,177-178`).
- So: lift the controller to hold `Map<SessionId, {unsubscribe, state}>` — one
  standing subscription per open `ChatSessionTab`. The only thing that
  serializes is generation (single GPU), a server concern, not the client's.

## Component reuse inventory (packages/ui)

- **Reuse as-is:** `Tabs` (controlled strip `{tabs:[{id,label,disabled}],
  activeTab, onTabChange}`, keyboard nav ←→/Home/End, `TabTrigger` styling);
  `TabPanel` (`{tabId, activeTab, lazy}` — inactive = `hidden` NOT unmounted,
  so a backgrounded chat/terminal tab keeps its live state + subscription;
  active panel already gets `flex min-h-0 flex-1 flex-col`). `ResizablePanel`
  / `SidePanel` for rails and the future split.
- **Add to the lib (small):** a per-tab **close** affordance — extend the
  `Tab` type / `Tabs` with `closable` + `onClose(id)`, close ✕ on the
  `TabTrigger` (its own button, stop-propagation so ✕ doesn't switch tabs).
  Storybook story. This is the only real ui-lib gap.
- **Tab-manager state is app-level** (caller owns the tabs array): a
  `useWorkspaceTabs` hook — open list, active id, `open(surface)` (dedup by
  identity), `close(id)`, `focus(id)`. Pure/testable.
- Surface content reuses existing components: chat body (ChatThread etc.),
  file view (`InlineAttachmentRenderer` file_view / CodeBlock), terminal
  (`packages/ui/.../terminal/*` + `TerminalOutput`), diff (`DiffView`).

## Two slices

### Slice 1 — Multiplexing (controller multi-session) — FIRST, foundation
Additive refactor of the controller/state/provider layer so multiple sessions
can be subscribed and rendered concurrently. Keep the existing single-active
API working so `AcpChatPage.tsx` still compiles unchanged (this slice must NOT
restructure the page). New capability: `Map<SessionId, {unsubscribe, state}>`,
open/close/focus of multiple sessions, per-session state slices. The client's
`subscriptions` Map already supports it; this is controller/state work
(`acpWorkspaceController.ts`, `acpWorkspaceState.ts`, `useAcpWorkspace.ts`,
`AcpWorkspaceProvider.tsx`). Stay out of the composer/mention/file-preview
files (another effort owns them). Tests for concurrent-subscription routing.

### Slice 2 — Tab UI — SECOND, consumes Slice 1
`WorkspaceTabs` + `useWorkspaceTabs` + the four surface components; the ui-lib
closeable-tab addition; extract today's chat body into `ChatSessionTab`; wire
the session-list rail and file-tree rail to open tabs; shell/edit card
maximize → terminal/diff tabs; remove the terminal sidebar and the header
Dateien/Terminal toggle buttons (their instability — `flex-wrap` toolbar
reflow — is the bug being fixed). Single visible tab; no split. Restructures
`AcpChatPage.tsx`, so it must land AFTER the composer-file-preview effort and
Slice 1 (three concurrent editors of that file would corrupt it).

## Sequencing (no git isolation; shared uncommitted tree)

1. (running) composer file-preview effort — owns AcpChatPage composer region.
2. Slice 1 (multiplexing) — controller layer, non-overlapping; may run
   concurrently with (1).
3. Slice 2 (tab UI) — after (1) and Slice 1 land.
