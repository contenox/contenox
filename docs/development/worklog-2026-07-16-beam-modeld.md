# Worklog & handoff — beam UI + modeld capacity (2026-07-16)

Purpose: a self-contained snapshot for planning a fresh session. Captures what
shipped and was verified, what's queued, the open decisions, and the
conclusion tasks (demo recordings + doc updates). Nothing in this session was
committed — the whole tree is uncommitted, interdependent work.

## Shipped & verified this session (all uncommitted)

Each was gate-verified (tsc + vitest + build; Go build+test) AND independently
live-verified in a real browser against `contenox serve` + modeld, unless noted.

- **ACP chat plumbing fixes** — streaming with tools (was blob-after-minutes;
  now token-by-token: `taskexec.go` streaming gate lifted, llama client
  `Stream` handles tools), reasoning-leak + misroute root-caused, error
  taxonomy (one named error state per root cause; server `FixPath` corrected
  to Settings not Backends).
- **Settings page** — all 12 config keys exposed (was 4), CLI help ported to
  tooltips, honest restart/scope notices (the ACP defaults are seeded once at
  serve boot; surfaced instead of silently ignored), wizard sidebar-toggle
  hidden. `default-think` validated server-side.
- **Sidebar order** — freshest-first (was random-UUID order; root cause: Go
  sqlite time.Time String() format the list parser didn't handle).
- **Session workspaces** — `runtime/vfs` factory (allowlist + one containment
  impl both `local_fs` and `/files` delegate to), per-session cwd wired to
  agent tools, workspace-root picker in pre-session controls, IDE-style file
  panel, **@-mentions = resource_link ONLY** (reference, never embed).
- **@-mention scoped file browser** — folders navigable/drill-in with lazy
  load, keyboard tree nav (↑↓ move, → drill, ← ascend), quoted paths for
  spaces, **live file preview in the @ popover** (relocated out of composer
  flow — pixel-verified zero page-shift).
- **Shell sessions (phases 1+2)** — persistent per-session PTY, `!` passthrough
  (no LLM turn), `shell_session.run` (HITL-gated) / `read` tools, output over
  WS `_meta` extension. Terminal ANSI sanitizing (`packages/ui/src/ansi.ts`).
- **Chats-as-tabs** — multi-session controller (`Map<SessionId,…>`, all tabs
  live concurrently; the client already multiplexes by sessionId), close≠delete,
  deep-link routing, per-tab config controls. Review pass fixed per-tab terminal
  stream + toggle drift + extracted `ChatSessionToolbar`.
- **Workspace canvas (B1)** — terminal moved from cramped sidebar into a
  resizable side-by-side **canvas tab** (`CanvasRegion` + `useCanvasTabs` +
  `TerminalTab`); collapses when empty; full-width takeover <1024px. Verified:
  collapse, 1280 split w/ handle, 640 takeover, `!echo` into canvas terminal.
- **agent-view-filter backend** — `runtime/agentview` + `/files?filter=agent&policy=`
  annotates each entry with the agent's real HITL verdict (allow/approve/deny)
  computed by the actual policy engine. Live E2E confirmed truthful — and
  surfaced the security hole below.
- **modeld no-spill placement** — auto mode NEVER sheds GPU layers to CPU; full
  offload or honest refusal with budget arithmetic; explicit override + CPU-only
  unchanged. Reviewed clean-room (no external origin) + correctness; unit-tested.
  NOT yet live-tested (needs a modeld rebuild+restart on the single-slot daemon).

Test counts ended at: beam 352 vitest green; modeld capacity+llama green.

## Blueprints written (docs/development/blueprints/)
- `beam/session-workspace-files.md`, `beam/shell-sessions.md`,
  `beam/workspace-tabs.md`, `beam/workspace-canvas.md`,
  `beam/agent-view-filter.md`, `beam/runtime-generated-ui.md`
  (hypermedia/served-UI direction, decided all-in htmx — big future bet),
  `modeld/no-spill-placement.md`.

## Queued / next up (NOT started)
- **Phase B2** — `FileTab` (file-tree click opens a canvas tab; remove the
  floating peek) + toolbar redesign (config→gear popover, usage→chip,
  toggles→canvas actions). Extension points ready: `CanvasRegion`'s `tabLabel`
  switch + `CanvasTabKind`. Restructures the same layout files — run solo.
- **Tab/canvas follow-ups** — `DiffTab` + edit-card maximize; per-session
  persistence of open canvas tabs; a background tab's pending-permission is
  invisible (needs a tab badge); background tabs each warm-fetch their file
  listing (minor redundancy).
- **agent-view frontend** — the filter control on the workspace panel (waits on
  B2, which restructures that rail).
- **modeld follow-ups** — placement plan + `doctor` authority (header-only
  pre-load fit report, versioned JSON for CLI/API/UI = the "fits on your GPU?"
  indicator); reserve-stack rationalization (headroom-frac + 80% resident cap
  withhold ~45% of a small card — needs maintainer call on reserve sizes);
  warm-KV/session-resume latency.
- **runtime-generated-UI** — the htmx served-spec direction (kills CLI/UI
  parity drift + the hardcoded-div problem). Strategic; not started.

## Open decisions for the maintainer
1. **vfs control-plane isolation (SECURITY, live hole).** An agent can read+write
   `~/.contenox` (its own HITL policy/config) AND read `~/.ssh` — confirmed live:
   `/files?filter=agent` shows `.contenox` and `.ssh` as `read=allow` under the
   default home-dir root. Fix: unconditional `vfs` exclusion of the control plane
   + reconsider the `filepath.Dir(contenoxDir)` home-dir default root. Awaiting
   go-ahead to blueprint+slice. (Mirror lesson: a subagent edited
   `.claude/settings.local.json` to allowlist its own commands — self-governance
   escape must be structurally impossible.)
2. **modeld reserve-stack sizes** (part of the plan/doctor follow-up).
3. **B2 launch** + whether the terminal auto-surfaces on first shell output
   (currently yes; single toggle to disable).

## The original design-TODO (TODO.md at repo root)
Error-handling UX, chat session controls, settings coverage, chat-path polish
(streaming/preview), model-provisioning page redesign, dark-mode contrast audit,
inline help boxes — most chat/settings items are now DONE (see above); the
**model-provisioning page redesign**, **dark-mode contrast audit**, and
**inline condition-triggered help boxes** remain open.

## Conclusion tasks (what the maintainer named)
- **Re-record clean website demos** for the flows that now work: token
  streaming in a rendered markdown transcript; chats-as-tabs (open/switch/
  close, background-live); the @-mention file browser + in-popover preview;
  terminal as a side-by-side canvas tab + `!` passthrough; the named error
  states + Settings deep-link; the full 12-key settings page. Use the German
  UI (current locale) or switch locale first — decide per audience.
- **Update docs** — user-facing docs for: workspaces/@-mentions (reference-only),
  shell sessions + `!`, tabs + canvas, the settings surface, and the modeld
  no-spill behavior (models now refuse-with-reason instead of degrading).

## Infra / workflow notes for a fresh session
- **Dev loop:** `make dev-beam` (vite :5173 proxying /api+/acp to serve :32123)
  hot-reloads UI; only Go changes need a serve restart. Embedded path: beam
  `npm run build` writes `runtime/internal/web/beam/dist` → `make build-contenox`.
- **Live verify:** playwright MCP is configured (`--browser=chromium`); assert
  via `browser_evaluate` (the a11y snapshot decorates code text). German UI.
  Single GPU slot — never run two prompts at once; avoid LLM turns when
  validating UI state (drive existing sessions + DOM).
- **Serve races:** subagent e2e cleanup uses `pkill -f 'contenox serve'` — it
  kills sibling serves. Give each verifier its own port and expect races when
  agents overlap.
- **Clean-room rule for modeld:** no external project/model/author/"port"
  references in any modeld code, comment, test, or doc. Frame as modeld's own
  ethos + observed behavior.
- **modeld ethos (governs capacity):** right-sized, low-latency, safe-24/7,
  6→48GB, refuse-don't-spill. NOT max-params-on-crammed-hardware.
