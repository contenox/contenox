# Sovereign Workspace: Architecture Plan

Companion to [sovereign-workspace.md](sovereign-workspace.md) — that document holds the decision record and PR plan; this one holds the engineering boundaries.

**Goal:** Define clean boundaries so every slice is intentional, not "just add a useMemo".

## Core Principles (Non-Negotiable)

1. **Brain owns everything.** Go runtime (taskengine, agentservice, hitlservice, messagestore, modeldconn, etc.) is the single source of truth for execution, state, policy, and side effects. Beam never executes, never talks to modeld directly, never owns durable state.

2. **Glass is thin presentation + steering only.** React renders views, collects user intent (message + mode + explicit context artifacts), and sends steering commands (approve/deny, stop). No orchestration logic in TS.

3. **Two distinct artifact concepts (keep them separate):**
   - `ChatContextArtifact`: Per-turn, model-visible context (what gets injected into the prompt). Lives in `lib/artifacts/registry.ts` + `types.ts`. Used at send time.
   - `CanvasArtifact`: Session-scoped, human-visible objects for review/inspection (plans, diffs, run summaries, previews). Lives in a new `lib/artifacts/canvas.ts` (or `canvasTypes.ts`). Driven by TaskEvents + explicit user actions.

4. **Event-driven, not polling or duplicated state.** Live data comes exclusively from `useTaskEvents` (SSE over `taskeventsapi` + libbus). After completion, fall back to persisted chat history + `CapturedStateUnit`.

5. **Incremental evolution, not big-bang rewrite.** Per the blueprint: evolve inside the existing `WorkspaceSplitPanel` / right rail first. Coexist with `ChatRunLog`, Terminal, and current chat thread. Full three-column only after renderers and server wire prove truthful.

6. **State ownership rules:**
   - Live run state: `useTaskEvents` + `reduceTaskEventState` (extend the reducer when needed; keep it pure).
   - UI chrome (panel open/width, selection): local state in `ChatPage` or a narrow `useWorkspaceChrome` hook + localStorage. No global store yet.
   - Canvas history/selection: dedicated small reducer or `useCanvas` hook that consumes events + selection. Never derive complex UI objects with useMemo directly in the page render tree for core domain objects.
   - Selection drives canvas: `selectedEventId` or `selectedCanvasArtifactId` → lookup/projection into a `CanvasArtifact`.

7. **Layout strategy (desktop first):**
   - Preserve existing `ResizablePanelGroup` chat | workspace for now.
   - Inside the right "workspace" area: introduce tabs or vertical split for Timeline view + Canvas view (initially).
   - Later (PR5+): promote Timeline to first-class left column when the feature flag / truth gate passes.
   - Mobile: drawers or collapse everything into the existing mobile workspace pattern.
   - Terminal stays accessible (do not delete the current `WorkspaceSplitPanel` terminal path until Canvas + run log are proven).

8. **Product-surface-truth gates for every slice:**
   - A user can `contenox serve`, open a chat, send a message that produces ≥3 distinct TaskEvent kinds, see live structured events, select one, and see a corresponding artifact appear in the canvas area.
   - Existing plain chat, terminal, approvals, and VS Code webview paths must continue to work.
   - No new "hero" claims in docs/marketing until the slice has an explicit truth test (in code or manual reproducible steps).

## Data Flow (Canonical)

```
User action (composer)
  → collect ArtifactRegistry (context for model)
  → POST /chats/{id}/chat {message, mode?, context?} + X-Request-ID
  → Go: internalchatapi (now accepts mode/context) → agentservice.Prompt
  → taskengine emits TaskEvent via BusTaskEventSink (per-request subject)
  → SSE /api/task-events?requestId=... delivers to Beam
  → useTaskEvents → reduceTaskEventState → liveTask
  → TimelinePanel renders / groups events
  → Selection (click) → project event → CanvasArtifact (via mapping rules)
  → CanvasPanel renders the artifact
  → (later) approvals surface in Verification rail or Canvas
```

**Event → Canvas mapping (start minimal, per blueprint table):**
- `chain_started` → {kind: 'run', requestId, summary: chain}
- `step_completed` + content or attachments → {kind: 'markdown' or 'message'}
- tool diffs → {kind: 'diff'}
- `approval_requested` → {kind: 'message', title: 'Approval pending'} + drive verification
- token_usage / status updates → mutate active 'run' artifact

## Component Boundaries (Proposed)

- `lib/artifacts/canvas.ts` (new)
  - `CanvasArtifact` discriminated union (as specified in blueprint)
  - `projectEventToCanvas(event: TaskEvent, runState): CanvasArtifact | null`
  - `CanvasState` interface (open, current, history ring, selection)

- `hooks/useCanvas.ts` (new, narrow)
  - Takes live events + requestId
  - Returns { artifacts: CanvasArtifact[], current: CanvasArtifact | null, select(id) }
  - Manages ephemeral history + localStorage for selection persistence per chat/run.

- `components/TimelinePanel.tsx`
  - Pure(ish) view over events.
  - Emits selection via callback (does not own canvas state).
  - Reuses / wraps `ExecutionTimeline` + a selectable list.
  - Virtualization later.

- `components/CanvasPanel.tsx`
  - Receives `currentArtifact` + optional `onEditIntent` etc.
  - Initial renderers: run summary, simple markdown, basic diff (unified), event detail.
  - Empty state + "no artifact selected" guidance.
  - Monaco **only** inside specific renderer (e.g. editable file_excerpt later).

- `pages/admin/chats/ChatPage.tsx` (orchestrator only)
  - Owns: activeRequestId, liveTask subscription, toolbar state, artifactRegistry (for *context*).
  - Wires: `<TimelinePanel events={liveTask.events} onSelect={...} />`
  - Wires: `<CanvasPanel artifact={canvas.current} />`
  - Does **not** contain complex derivation useMemos for domain objects. Delegates to hooks.

- Keep `WorkspaceSplitPanel.tsx` as the host for now (evolve it to contain Timeline + Canvas tabs or split, or render CanvasPanel and keep terminal as a sibling tab).

- `ChatRunLog.tsx` remains available during transition (tab or debug toggle).

## Server Side (Minimal, Order Matters)

- PR2 equivalent first (or in same small slice if tiny): Accept `mode` + `context` in `chatRequest`. Pass through or log. Do **not** yet change `agentservice.Prompt` or `buildChatInput` unless the truth test requires it.
- TaskEvent struct: additive fields only (`attachments?`, `widgetHint?`) when needed for canvas drive. Existing events are sufficient for first slice.
- No schema changes.

## Recommended First Meaningful E2E Slice (Scoped)

**Name:** "Canvas slot foundation + live timeline inside the existing workspace rail" (aligns to PR 3 + early PR 5)

**Scope (tight):**
- Introduce `lib/artifacts/canvas.ts` with the union type + a `projectEventToCanvas` function (minimal cases only).
- Introduce `hooks/useCanvas.ts` (or local state + effect inside the workspace component) that consumes `liveTask.events` and produces `currentArtifact` + `selectEvent`.
- Evolve `WorkspaceSplitPanel.tsx` (or a new `WorkspaceInspector.tsx` that it renders) to show:
  - A "Timeline" section using `ExecutionTimeline` + selectable list (or embed `TimelinePanel`).
  - A "Canvas" section below or tabbed that renders the projected artifact (at minimum: run header + selected event detail as markdown-like or raw).
- Wire selection: clicking a step in the timeline updates the canvas view with that event's content/diff/whatever.
- Preserve: current terminal (keep as default or tab), ChatRunLog path, mobile behavior, no breakage to send/receive.
- Add a localStorage flag or simple toggle (`?sovereign=1` or button) so the new inspector is opt-in during development.
- Truth gate (explicit):
  - Start `contenox serve`.
  - Open/create chat, pick a chain that produces ≥3 events (step_started/completed, token_usage, etc.).
  - Send message.
  - See live events appear in the timeline section of the right panel.
  - Click an event → canvas area updates to show its content or a derived artifact.
  - Stop / complete the run; canvas still shows last state.
  - Plain message path, approvals, and terminal still work.

**What this slice deliberately does NOT do:**
- Full three-column layout change in ChatPage.
- Any server-side mode resolution or context injection into the model prompt.
- Rich renderers (no ReactMarkdown, no real diff viewer, no Monaco yet).
- Persistence of canvas artifacts across refresh.
- Verification rail.
- Changes to agent.go or taskengine.
- Removing or hiding ChatRunLog / terminal.

**Files touched (minimal blast radius):**
- New: `packages/beam/src/lib/artifacts/canvas.ts`
- New: `packages/beam/src/hooks/useCanvas.ts` (or keep minimal in component if truly tiny)
- Edit: `packages/beam/src/pages/admin/chats/components/WorkspaceSplitPanel.tsx` (host the new views)
- Edit: `packages/beam/src/pages/admin/chats/ChatPage.tsx` (pass live events + selection callbacks only; minimal)
- Optional small: extend `reduceTaskEventState` if selection needs extra data.
- Docs: note the slice landed + how to exercise the truth gate.

**Why this is "properly planned":**
- Follows the "evolve inside existing split" guidance.
- Introduces the CanvasArtifact model before any rendering.
- State lives in a hook/reducer, not a giant useMemo in the page.
- Coexistence with existing UI.
- Clear, testable E2E user journey.
- Leaves room for PR 2 server work and later full-column promotion.

## Next Slices (after this one proves truthful)

- Server wire (mode + context passthrough + basic injection).
- Promote Timeline to its own resizable column.
- Add first real renderer (markdown from step content or plan).
- Surface approvals in the inspector.
- Etc.

## Anti-Patterns to Avoid

- Deriving domain UI objects with useMemo directly in ChatPageImpl.
- Big layout rewrites before the panels have content.
- Deleting terminal or run log paths before the replacement is better.
- Starting with full three-column before the canvas slot is solid inside the current split.
- Adding useMemo "just to make the demo work" without a hook or type for the concept.

This document exists so the next code change has an explicit target and boundaries.
