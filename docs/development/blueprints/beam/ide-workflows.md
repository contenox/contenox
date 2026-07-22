# Beam IDE workflows — the oversight cockpit arcs

Date: 2026-07-21
Status: blueprint (mined + sized; not yet built). Directive: "replicate as many
VSCode/Zed/IDE workflows as meaningful into beam otherwise it's half-baked" —
with the filter that decides *meaningful*: **Beam's job is the
supervise–review–intervene loop, never editing.** Real IDEs are ACP clients of
the same runtime; Beam competing on editing would be a worse VS Code instead of
a better cockpit. OpenHands validates the filter from production: their
oversight UI never lets the diff view edit — actual editing is a cleanly
separate escaped-to iframe.

Sources: mined 2026-07-21 from OpenHands/frontend, bolt.diy, gitea (diff UI),
theia — grounded against what this repo already ships. The sizing surprise:
most of the backend already exists.

## Arc 1 — Changed-files + diff review (flagship; this IS the state-diff bet)

**Already in the tree:** `runtime/acpsvc/events.go` `diffContentFromResult()`
parses every file-write tool result into
`libacp.ToolCallContent{Type: Diff, Path, OldText, NewText}` — the exact
`{original, modified}` shape OpenHands' diff endpoints serve — flowing per-edit
through the session event stream. Missing: per-mission aggregation.

**Contract (mirror OpenHands' two-endpoint shape):**
- `GET /missions/{id}/changes` → `[{path, status: added|modified|deleted}]`
- `GET /missions/{id}/changes/diff?path=` → `{original, modified}`
Aggregation: fold the mission session's diff events by path — first `OldText`,
last `NewText`; status from create/edit/delete. Cap the list; per-file
`incomplete` suppression flags for huge files (gitea's pattern).

**Rendering:** Monaco `DiffEditor` (`@monaco-editor/react`) — the proven
choice for exactly this use (OpenHands). Collapsed rows by default, lazy
per-file diff fetch on expand, old/diff/new toggle. Do NOT hand-roll diffing
(bolt.diy's regret) and do NOT add editing or a VS Code iframe.

**The differentiator:** rank the changed-files list by
Degree-of-Interest — weight and decay the unit's interactions per path (reads,
edits, tool dwell) from the already-journaled events, so review starts where
the agent's attention concentrated, not alphabetically. Landed-vs-planned
joins here (mission plan entries beside the changed files).

Size: Go aggregation + endpoints ~0.5–1d; UI ~1–2d (near-port of OpenHands'
`file-diff-viewer.tsx`).

## Arc 2 — Workspace-wide search

Backend: shell to `rg --json` under the mission/workspace root, stream matches
incrementally (SSE/NDJSON — theia's push model; never block on full scan).
Shape per match: `{path, line, column, length, preview}`; server- or
client-grouped by file; result cap (~500, theia's default).
Frontend: bolt.diy's `Search.tsx` anatomy — 300ms debounce, per-file
collapsible groups with match-count badges, highlighted substring with context
window. Click-through routes into Arc 1's diff view when the file is a changed
file; otherwise inline context in the row — no general file viewer (editor
territory). Size: ~0.5d Go, ~1d UI.

## Arc 3 — Terminal

**Backend is COMPLETE:** `runtime/terminalservice` (real PTY, `creack/pty`,
principal-scoped reattach) + `runtime/internal/terminalapi` (POST/GET/DELETE
sessions, `GET /terminal/sessions/{id}/ws`, binary frames, in-band JSON
`{"type":"resize",cols,rows}`, token-gated).

Frontend: `@xterm/xterm` + `@xterm/addon-fit` against the existing WS —
bolt.diy's bridge nearly verbatim (`onData` → send; `onmessage` → write;
ResizeObserver → fit → in-band resize frame). Multi-terminal via id-keyed tabs
if wanted. Size: ~0.5–1d (+0.5d multi-tab).

**DECIDED 2026-07-21 (option b, taken under the land-everything mandate,
flippable):** the terminal ships **read-only by default** ("Nur Lesen" badge,
xterm `disableStdin`) with an explicit **"Shell übernehmen"** affordance that
flips the same view interactive behind an honest confirmation naming what it
grants (a real shell on the host, bounded by the serve credential). The
default lives at `TERMINAL_READ_ONLY_DEFAULT` in
`packages/beam/src/lib/hostTerminal.ts` with a register-quality comment —
flip it there if the posture should change. Same-day fix that unblocked it:
`terminalapi` now authenticates via the injected serverapi gate (raw token OR
session JWT — one login, every surface), replacing its raw-only compare that
401'd the browser cookie.

## Arc 4 — Layout: inspector tabs, not docking

Theia's Lumino docking shell is confirmed overkill (multi-week subsystem).
Beam's pages+tabs architecture is already the right shape (OpenHands proves
the flat model suffices). Two cheap borrows: OpenHands' dependency-free
`useResizablePanels` hook (~80 lines, localStorage-persisted) for a two-pane
split; `react-resizable-panels` only if a third pane ever exists. The
**mission inspector**: a flat tab bar on `MissionDetailPage` hosting
Changes / Search / Terminal / (transcript via the adoption affordance) as tab
content. Size: hours.

## Cross-cutting steal

Inline risk annotation on the action feed (OpenHands renders `security_risk`
beside each bash command as a first-class part of action rendering): when our
events carry gate/envelope decisions, render them inline in the transcript and
timeline, not in a separate security view.

## Build order

Arc 1 (with DOI ranking stubbed as ordering-by-edit-count first), then Arc 4's
inspector shell to host it, then Arc 3 (after the interactivity decision),
then Arc 2. Every arc lands behind the same gates as the rest of Beam
(typecheck, vitest, build-ui) and each gets a Playwright pass at both widths.
