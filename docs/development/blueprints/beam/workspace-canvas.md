# Workspace Canvas: a side-by-side working area for terminals, files, diffs

Status: landing in slices — the canvas tab-model (slice B1) is in
`packages/beam` (`useCanvasTabs`); later slices follow the body below.
Phase B of the tab work — supersedes the terminal-sidebar and the file-tree
floating-peek. Decisions confirmed with the maintainer 2026-07-16.

## Problem (observed in the live UI)

Auxiliary content has no real home: the **terminal is a cramped ~200px right
sidebar** (wrong container for a terminal), **clicking a file in the tree
pops a floating preview that overlaps the transcript** (a peek, not a place to
work), and the **header toolbar is a crammed wrapping strip** (two toggles +
usage + four config dropdowns). All three are symptoms of wedging working
surfaces into leftover chrome.

## Concept

A **secondary "canvas" region beside the chat**, holding terminal / file /
diff as **tabs** (mirroring chats-as-tabs in the primary region). It appears
side-by-side when something is open and collapses to just-chat when empty.
The VS Code shape: editor area (chat) + a working pane (canvas).

- **Terminal** → a canvas tab (full height, real width). The right sidebar is
  removed entirely.
- **File** (tree click / @-maximize / edit card maximize) → a `FileTab` in the
  canvas (real editor view). The tree's floating peek is removed.
- **Diff** → a `DiffTab` (deferred — no producer wired yet; edit-card maximize
  is later).
- **Chat + canvas simultaneity** is the point: watch the conversation and the
  shell/file at once.

## Decisions (maintainer, 2026-07-16)

- **Canvas is side-by-side, NOT a flat single-tab strip.** Chat stays visible;
  the canvas opens beside it.
- **This does NOT reverse "no split view."** That decision was chat-vs-chat
  (never two conversations side by side — still holds). This is
  chat-vs-working-canvas, a different axis.
- **File-tree rail stays** as the left navigator; a file click opens a
  `FileTab` in the canvas (kill the floating peek).
- **Responsive:** on a narrow viewport the canvas stacks under the chat or
  takes over full-width on demand — never a cramped side rail on small screens.

## Canvas scoping (design note)

The canvas is **scoped to the focused chat session**: a terminal is that
session's PTY (`useTerminalStream(sessionId)` already exists), a file is rooted
at that session's cwd. Switching chat tabs updates the canvas to the newly
focused session's terminal/files. B1 target: the canvas reflects the focused
session (single canvas instance re-pointed on focus change). Per-session
persistence of *which* files were open (restore on switch-back) is a later
refinement, not required for B1.

## Component architecture & reuse

- **`CanvasRegion`** (new): the secondary pane. Uses `@contenox/ui`
  `ResizablePanel` for the chat|canvas split (collapses when no canvas tabs).
- **`useCanvasTabs`** (new): the canvas's own tab manager (open list, active,
  open/close/focus/dedup by identity) — same shape as `useWorkspaceTabs`,
  reuse the pure-reducer pattern (`workspaceTabs.ts` is the template; consider
  generalizing it rather than copy-paste).
- **Canvas tab strip**: reuse `@contenox/ui` `Tabs` (now `closable`) +
  `TabPanel`.
- **Surfaces**: `TerminalTab` (reuse `TerminalOutput` + the terminal
  components; `useTerminalStream(sessionId)` already threads per-session),
  `FileTab` (reuse the file-view renderer — `CodeBlock` /
  `InlineAttachmentRenderer` file_view — over `useWorkspaceFiles.readFile`;
  real scroll, line count), `DiffTab` (reuse `DiffView`; deferred).
- **Toolbar redesign** (`ChatSessionToolbar.tsx`, already extracted): config
  controls (Model/HITL/Think/Token-Limit) → a compact **session-settings
  popover** behind one gear button (reuse a `@contenox/ui` popover/dropdown);
  usage (`0/15.709`) → a small chip; the Dateien/Terminal buttons stop being
  sidebar toggles and become **"open in canvas"** actions (or move onto the
  file-tree rail + the canvas tab strip). Header ends as: tab strip +
  connection status + settings gear + usage chip + New Chat.

## Slices (sequential — all restructure the same layout files)

### Slice B1 — Canvas infrastructure + Terminal surface (FIRST)
`CanvasRegion` (ResizablePanel split, collapses when empty, responsive
stack/full-width on narrow) + `useCanvasTabs` + `TerminalTab`; move the
terminal OUT of the sidebar INTO a canvas tab; wire "open terminal" (the
header Terminal action and/or a shell-card maximize if cheap). Remove
`TerminalPanel` as a sidebar. Canvas reflects the focused session's PTY.
Delivers: terminal is a proper side-by-side canvas tab, not a rail.

### Slice B2 — File surface + Toolbar redesign (SECOND; consumes B1)
`FileTab` (file-tree click opens it in the canvas; remove the floating
tree-peek and the workspace-panel inline preview) + the toolbar redesign
(config→popover, usage→chip, toggles→canvas actions). Delivers: files open as
canvas tabs; header is clean.

### Deferred
`DiffTab` + edit-card maximize gesture; per-session canvas-tab persistence;
the agent-view *frontend* filter (waits on this since it touches the file
rail); the still-open `vfs` control-plane hardening (independent, backend).

## Sequencing

1. (running) composer file-preview reposition — owns the composer/mention area.
2. B1 — after (1) lands.
3. B2 — after B1 lands.
No Phase B parallelism: every slice restructures `AcpChatPage` /
`WorkspaceTabs` / `ChatSessionTab` layout, so concurrent editors would corrupt.
