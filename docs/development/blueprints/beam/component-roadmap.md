# Beam component roadmap — plumb-ready list

Date: 2026-07-21
Status: backlog distilled from the day's landings, minings, and blueprints
(`ide-workflows.md`, `attention-layer.md`, `../acp/mission-plans.md`). Every
entry names its data source and whether that source EXISTS — the list is
ordered so nothing waits on undesigned backend work.

## Tier 0 — landed or in flight today (finish line, not backlog)

- **`UnitStatus` composed-status atoms** (landed) — the one truth for
  process × verdict × liveness × blocker, used by board/missions/inbox. The
  designated extension point for plan-progress and attention badges.
- **Plan panel + step-progress fragment** (in flight) — `Mission.Plan` on the
  mission API; checklist chips, revision + explanation line.
- **Session adoption** (in flight) — "Sitzung öffnen" from board/mission into
  the chat surface; übernommen/beobachten header; delete-guard; durability
  notice. Wire contract landed server-side.
- **Fleet board / missions / inbox operability** (landed) — running-first
  board, triage inbox, mobile + house patterns.

## Tier 1 — API already exists, pure frontend (days, start anytime)

1. **Workspace picker** — source: `GET /workspace/roots` (landed today).
   Components: root chip in explorer/dispatch/chat headers; root selector on
   the dispatch form and new-chat flow (ACP `session/new` cwd already carries
   it); "outside roots" designed refusal state replacing the raw 422.
2. **Terminal tab** — source: `terminalapi` WS (fully built: PTY, reattach,
   in-band resize). xterm.js + fit addon, bolt.diy's bridge verbatim.
   **Blocked only on the interactivity decision** (interactive vs read-only —
   flagged in `ide-workflows.md`).
3. **Mission inspector shell** — no backend at all: flat tab bar +
   OpenHands' ~80-line resizable-split hook on `MissionDetailPage`, hosting
   Changes/Search/Terminal/Transcript tabs as they land. Hours.

### The instant-feel law (binding on every tier)

The editors that won without being IDEs won on:
instant latency, fuzzy-everything navigation over dumb text, zero ceremony
(the folder is the project), shell composability, and plain-state honesty —
while Eclipse-era IDEs traded the inner loop away for a semantic model. VS
Code resolved it with **progressive semantic enhancement**: instantly usable
dumb-fast, the model arrives async as an upgrade, never a prerequisite. Beam
follows that resolution: every view works at record-speed over plain state
(board, inbox, rg-streamed search — no index); the semantic layer (DOI
ranking, plans, landed-vs-planned) is asynchronous enhancement that never
blocks, gates, or delays the basic view.

3b. **Goto-anything palette** (Tier 1½, no backend gap — data all served
   already): a command palette fuzzy-matching over missions, agents,
   sessions, workspace roots, inbox items, and page actions. Keyboard-first,
   zero taxonomy (blind-spot compliant), the single biggest inner-loop
   upgrade per line of code the editors' history offers. Size: ~1d.

## Tier 2 — small named backend gap + frontend (the flagship arc)

4. **Changed-files list + DiffViewer** — the state-diff bet. Backend gap
   (small, designed): aggregate the per-edit `ToolCallContent{Diff, Path,
   OldText, NewText}` already flowing in acpsvc events into
   `GET /missions/{id}/changes` + `/changes/diff?path=`. Frontend: Monaco
   `DiffEditor`, collapsed rows, lazy per-file fetch, old/diff/new toggle,
   incomplete-suppression for huge files. First DOI hook: order the list by
   per-path edit/read counts (upgrade path in `attention-layer.md`).
5. **Search panel** — backend gap (small, designed): `rg --json` under the
   workspace root streamed as SSE/NDJSON. Frontend: bolt.diy's Search.tsx
   anatomy (debounce, per-file groups, count badges, highlighted preview);
   click-through into #4's diff when the hit is a changed file.
6. **Plan-revision feed in the inbox** — backend gap (small, needs a
   decision): `plan_revised`/`status_changed` are bus events with no REST
   history. Either (a) render only current `Plan.{Revision, Explanation}` on
   mission rows (zero backend), or (b) persist last-N revision summaries on
   the mission record for a real "+2/−1 — why" feed. (b) is the honest
   overnight-skim answer; size ~0.5d.

## Tier 3 — rides on the attention layer (`attention-layer.md`)

7. **Attention badges** — "concentrated in 3 files / wandered 14 dirs" chips
   on board rows and inbox groups, from the same per-path aggregation as #4.
8. **Scope-anomaly flag** — the derailment early-warning ("unit left its
   expected scope") as a first-class `UnitStatus` atom + inbox condition.
9. **Transcript anchors** — jump-to-where-attention-concentrated markers in
   the adopted-session view and journal rendering.

## Tier 4 — post-decision / post-dependency

10. **Batch/plan roll-up view** — parent mission with children grouped
    (ParentSessionID exists; waits on the sub-mission/a2a slice).
11. **Inline risk annotations** — envelope/gate decisions rendered beside
    actions in transcript and timeline (OpenHands' security_risk pattern);
    waits on events carrying the gate verdicts explicitly.
12. **Checkpoint/rollback affordances** — waits on the state-diff design
    maturing (see `attention-layer.md` and the beam criterion).

Deliberately absent (the meaningful-filter): text editing, LSP anything,
debugging, VS Code iframes, docking layouts.
