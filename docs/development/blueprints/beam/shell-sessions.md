# Shell Sessions: a live terminal surface for agent and user

Status: draft blueprint, 2026-07-16. Decisions settled; phases 1+2 planned
for implementation. Phase 3 (user co-input) explicitly deferred.

## Definition

Each chat session can own a **persistent shell session**: a real PTY, rooted
at the session's workspace root (see session-workspace-files.md), with a
lifetime beyond single commands — cwd, env, history, long-running processes.
It is a **session surface**: a named, stateful object attached to the session
that both the user and the agent observe and act on, rendered in its own
IDE-style toggleable panel (the workspace file panel is the first such
surface; this is the second; no generic "surface framework" is built until a
third instance proves the abstraction — rule of three).

## Decisions (maintainer, 2026-07-16)

- **Not a canvas.** A canvas is a shared document (idempotent, versionable,
  diffable); a shell is a shared process (append-only stream, side effects).
  Kept as distinct concepts under the session-surface umbrella.
- **Reference-only context, same principle as files/@-mentions:** terminal
  output NEVER streams into model context. The agent reads scrollback
  explicitly via a tool (`read` with tail-N / since-marker semantics). One
  ingestion path, policy-visible, always fresh.
- **Line-gated agent input.** The agent proposes one line; the HITL gate
  (policy or human) approves; the runtime types it into the PTY. The
  approval unit stays "a command", exactly like one-shot local_shell today —
  the PTY just persists between approvals. Free-typing/keystroke access for
  the agent does not exist.
- **User co-input (typing into the same PTY) is Phase 3, out of scope now.**
  Keyboard arbitration and agent-visibility-of-user-keystrokes are only
  worth solving once observing (Phase 2) feels right. User interaction in
  phases 1–2 is via `!` passthrough, not raw typing.

## What exists to build on

- `local_shell` one-shot tool + HITL gating (`runtime/localtools/commandrunner.go`),
  `--local-exec-allowed-dir` containment, `--shell` opt-in flag.
- libacp already implements the ACP terminal method shapes: `terminal/create`,
  `terminal/output`, `terminal/wait_for_exit`, `terminal/kill`,
  `terminal/release` (`libacp/methods.go:38-42`) — client-hosted terminals
  for editor clients. In serve, the RUNTIME hosts the PTY; align field
  shapes with these types where sensible so an editor-hosted variant stays
  possible.
- Terminal UI components in `packages/ui/src/components/terminal/`
  (`TerminalLine`, `terminalMarkdown`, `TerminalPromptInput`, glyphs) +
  `TerminalOutput.tsx` + `InlineAttachmentRenderer`'s `terminal_excerpt` —
  built during the console generation, currently unused by beam.
- A `!` + shell integration existed in an earlier generation (git history) —
  semantics reference, not code.
- Session workspaces (in flight): the PTY roots at the session's workspace
  root and inherits its allowlist guarantees.

## Phase 1+2 scope (what ships now)

**Phase 1 — `!` passthrough:** typing `!<command>` in the composer is NOT a
prompt: it runs the command in the session's shell (creating it on demand),
renders the output in the terminal panel, and drops a compact terminal card
into the transcript so the conversation records what happened. No agent
involvement, no LLM turn, no token cost.

**Phase 2 — persistent PTY + observer panel + gated agent access:**
- Backend: a shell-session manager — one PTY per chat session, created on
  demand, rooted at the session workspace root; scrollback ring buffer;
  idle-timeout + kill-on-session-close lifecycle; line-execution API
  (write approved line, read output since marker).
- Agent tools (replacing nothing; local_shell one-shot remains):
  `shell_session.run` (line-gated through existing HITL machinery) and
  `shell_session.read` (ungated scrollback read — reference-only principle).
- Live output to the client rides the existing ACP WebSocket as an
  extension `session/update` kind under `_meta` (same spec-safe extension
  pattern as `contenox.workspaceConfigOptions`) — one transport, session
  scoping and reconnect logic already exist. Conformant foreign clients
  ignore it.
- Frontend: second IDE-style toggleable panel (same pattern/affordance as
  the workspace panel), rendering the stream with the packages/ui terminal
  components; agent-run commands appear in the same stream the user watches.

## Non-goals (phases 1+2)

- No user free-typing into the PTY (Phase 3).
- No multiple shells per session (one, on demand).
- No PTY resize/interactive TUI fidelity guarantees (top/vim); plain
  line-oriented streams are the target. Document the limitation.
- No surface framework.

## Implementation status (2026-07-16)

Phases 1+2 implemented (agent build interrupted by a session limit during its
own e2e; completed and verified by the main loop). Backend: `runtime/shellsession`
(PTY manager, ring buffer, lifecycle), `shell_session.run` (HITL-gated) /
`shell_session.read` (ungated) tools, `runtime/acpsvc/terminal.go` (`_meta`
output updates + `!` passthrough). Frontend: `TerminalPanel` (sibling of the
workspace panel), `useTerminalStream`, `terminalPassthrough` `!` interception,
transcript `terminal_excerpt` card.

Gates: `go build ./...` clean; `runtime/shellsession` + `runtime/acpsvc`
tests green; 266 beam vitest; `npm run build`. Live browser e2e on the
embedded binary: `!echo …` runs a real shell with NO LLM turn (no plan rail),
output lands in the terminal panel AND a transcript card, panel toggle
persists.

**Defect found and fixed during e2e:** the PTY emits full terminal control
sequences (bracketed-paste `ESC[?2004h`, OSC window-title `ESC]0;…BEL`, SGR
colors) and both surfaces rendered them raw as literal garbage. Fix:
`packages/ui/src/ansi.ts` — `sanitizeTerminalText` (strips non-SGR control
sequences, keeps SGR for a colorizer) used by `TerminalOutput`'s `colorize`
(panel keeps colors), `stripAnsi` (full strip) used by the transcript
`CodeBlock` card (no colorizer there). 9 unit tests; verified live (zero raw
escapes on the page).

Not verified live: @-mention menu under synthetic input (inconclusive — the
menu filters over already-loaded workspace files, and the pure mention logic
is unit-tested green); left for a real-typing pass. HITL gating of
`shell_session.run` verified at the Go wire level, not the browser.
