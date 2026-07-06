# VS Code Extension — UX Shape Review

Sanity check of the whole extension's UX surface: activation, commands, menus,
status bar, sidebar (Runtime + Sessions), setup/onboarding, config selectors,
autocomplete, and the runtime-controls webview. Scope is **UX/shape**, not the
protocol/permission correctness covered in
[bridge-review-findings.md](./bridge-review-findings.md).

Date: 2026-06-14

## Summary

The surface is broad and coherent: activity-bar container with Runtime + Sessions
views, a chat participant with slash commands, editor-context actions, inline
autocomplete with a toggle, a model-provider registration, MCP provider, and a
walkthrough. Solid hygiene throughout (webview CSP+nonce, no remote telemetry,
restricted configs in untrusted workspaces, throttled autocomplete warnings,
stale-result guards, helpful spawn-error messages). The items below are the reviewed polish and
consistency gaps with their current WIP status, ordered by user impact.

| # | Severity | Finding |
|---|----------|---------|
| 1 | Medium | Terminal `Run Setup` desyncs from bridge state; walkthrough marks setup done on launch |
| 2 | Low–Med | Dead context-key plumbing → no readiness/turn gating on commands or menus |
| 3 | Low | Duplicate `Select Model` / `Select Chat Model` commands |
| 4 | Low | Editor context menu is flat (no submenu); diagnostics items always shown |
| 5 | Low | Experimental agent-session commands aren't contributed → unreachable from palette |
| 6 | Low | Autocomplete defaults on, all files, every pause — cost/expectation for a "local-first" product |
| 7 | Polish | Inconsistent command error surfacing |
| 8 | Polish | Walkthrough completion events fire before the action actually happens |
| 9 | Polish | Status-bar "crashed" click silently restarts but says "view status" |
| 10 | Medium | Release packaging currently disables VSCE secret scanning |
| 11 | Medium | Dev install verification only covers default desktop VS Code |
| 12 | Low–Med | No runtime proof command inside VS Code for installed extension state |

---

## 1. Setup ↔ bridge state desync (Medium, onboarding)

`runSetup` opens a terminal running `contenox setup` (`extension.ts:178-194`). The
extension gets no completion signal, and:

- The walkthrough "Configure a local runtime" step completes on
  `onCommand:contenox.runSetup` (`package.json:144-147`) — i.e. the moment the
  terminal is *launched*, before the user has chosen anything.
- The already-running bridge caches its runtime (`runtime/vscodeagent/chat.go:570`
  `ensureRuntime`; reset only via the bridge's own `setConfig` →
  `resetRuntime`, `server.go:589`). Config written by the separate `contenox setup`
  process won't reset a cached runtime, and the status bar stays on `setup` until
  something calls `refreshHealth`.

Net: a first-run user can finish setup in the terminal yet see the extension still
say "setup," with no nudge. **Fix:** on `window.onDidCloseTerminal` for the setup
terminal (or a follow-up "I've finished setup" action), call `bridge.restart()` +
`refreshHealth()`; and point the walkthrough completion at a real readiness signal.

**Status:** fixed in WIP. The setup terminal close event now restarts/refreshes the bridge and
fires an internal walkthrough completion command after the terminal closes.

## 2. Dead context-key plumbing (Low–Med)

`setChatContext` / `setTurnContext` are defined but never called, and
`contenox.connected` / `bridgeHealthy` / `chatVisible` / `turnInProgress` are
consumed by **zero** `when` clauses (`status/contextKeys.ts:8-13`; only
`setBridgeContext` is wired, from `statusBar.ts`). Consequences:

- Command-palette and menu items never disable when the bridge is down/crashed —
  they just fail when invoked.
- No turn-in-progress affordance (e.g., gating a stop/secondary action).

**Fix:** either consume these keys in menu/view `when` clauses (gate editor-context
and palette actions on `contenox.connected`), or delete the unused exports.

**Status:** fixed in WIP. `contenox.turnInProgress`, `contenox.connected`, and
`contenox.hasDiagnostics` are consumed by menus; the unused chat-visible context export was
removed.

## 3. Duplicate model-select commands (Low)

`contenox.selectModel` and `contenox.selectChatModel` both call `selectChatModel`
(`extension.ts:99-104`) and both appear in the palette as "Select Model" and
"Select Chat Model" (`package.json:281-290`, `635-640`). Two identical entries.
**Fix:** drop `contenox.selectModel` (or make it meaningfully different).

**Status:** fixed in WIP. `contenox.selectModel` was removed; `contenox.selectChatModel` remains.

## 4. Editor context menu clutter (Low)

Five Contenox commands are injected flat into `editor/context`
(`package.json:687-711`, groups `contenox@1..5`). Most coding extensions nest under
a single "Contenox ▸" submenu. Also `fixDiagnostics` / `explainDiagnostics` have no
`when` guard there, so they always show and no-op with a toast when there are no
diagnostics (`participant.ts:107-110,122-125`). **Fix:** move to a `submenu` and add
a diagnostics `when` guard (or keep but accept the toast).

**Status:** fixed in WIP. Editor actions now live under a Contenox submenu, are hidden in
untrusted workspaces, and diagnostic actions use `contenox.hasDiagnostics`.

## 5. Experimental agent sessions are unreachable from the palette (Low)

`contenox.openAgentSession` and `contenox.diagnoseAgentSessions` are registered
(`extension.ts:77-78`) but not declared in `contributes.commands` and not in the
palette. So even after enabling `contenox.experimental.nativeAgentSessions`, a user
has no command to launch the feature. **Fix:** contribute the commands (optionally
gated by the setting via `when`), or document the exact entry point.

**Status:** fixed in WIP. Both commands are contributed and visible from the palette; the command
implementation still checks `contenox.experimental.nativeAgentSessions`.

## 6. Autocomplete defaults (Low, expectation-setting)

`autocomplete.enabled` defaults true (`package.json:552-556`), the provider is
registered for `{ pattern: "**" }` (`autocomplete/provider.ts:9-12`), and
`startOnActivation` is true — so from install, every debounced pause
(`settings.ts:52`, 180 ms) in any file fires a bridge completion request. With no
autocomplete model configured this fails silently (warning throttled to 30 s). For a
product positioned as "local-first / reviewable," a default-on completion that may
call a paid cloud model on every pause deserves a deliberate decision or a first-run
opt-in. (The right-side status item toggle mitigates this.)

**Status:** fixed in WIP. Inline autocomplete now defaults off and must be deliberately enabled.

## 7. Inconsistent command error surfacing (Polish)

`showStatus` / `restartRuntime` / autocomplete-test catch errors → toast + reveal
Output (`extension.ts:144-176`). `openSession` / `deleteSession` and the config
selectors `throw` on failure (`extension.ts:223-235`, `selectors.ts:152-159`),
yielding VS Code's generic "command errored" toast without revealing the Contenox
output channel. **Fix:** standardize on catch → friendly toast → reveal Output.

**Status:** fixed in WIP for session/config commands. Open/delete session and config selectors now
catch failures, reveal Contenox output, and show a clear error message.

## 8. Walkthrough completion events fire early (Polish)

Besides the setup step (#1), the `sessions` step completes on
`onView:contenox.sessions` (`package.json:179-181`) — i.e., merely revealing the
view, not using it. Steps get checked off before the user does the thing. **Fix:**
use truer completion signals where available.

**Status:** fixed in WIP. Setup completes after the setup terminal closes; the sessions step
completes on `contenox.openSession`, not `onView:contenox.sessions`.

## 9. Status-bar "crashed" affordance (Polish)

On crash the tooltip says "Click to view status" (`status/statusBar.ts:27-31`), but
the bound `showStatus` actually calls `ensureStarted` first (`extension.ts:146`) —
i.e. it attempts a restart. **Fix:** relabel ("Click to restart") or split
view-vs-restart actions.

**Status:** fixed in WIP. The crashed tooltip now says the click restarts and shows status.

## 10. Release packaging disables VSCE secret scanning (Medium)

`packages/vscode/scripts/package-target.js` currently uses `--allow-package-all-secrets` and
`--allow-package-env-file` to bypass the broken local `vsce` secretlint path. That is acceptable
as a dev unblock only if our own package guard remains strict, but it should not be release
posture.

**Fix:** split dev packaging from release packaging, or pin/fix the `@vscode/vsce`/secretlint
dependency path and re-enable VSCE scanning for release.

**Status:** fixed in WIP for the packaging split. `package-vscode` and
`package-vscode-proposed` now run VSCE without the bypass flags. `package-vscode-dev` and
`package-vscode-proposed-dev` opt into `CONTENOX_VSCODE_SKIP_VSCE_SECRET_SCAN=1` for local dev
installs while the package-clean guard still runs.

## 11. Dev install verification only covers default desktop VS Code (Medium)

`make dev-install-vscode` verifies `~/.vscode/extensions/...` through
`packages/vscode/scripts/assert-installed-dev.js`. It does not cover Insiders, custom
`--extensions-dir`, Flatpak paths, or remote extension hosts. This can still create "installed but
stale UI" confusion outside the default local Code install.

**Fix:** support `VSCODE_CLI`, `VSCODE_EXTENSIONS_DIR`, and `code-insiders`; print the exact
verified extension path every time. Remote extension-host installs should be documented as a
separate manual step if we cannot drive them locally.

**Status:** fixed in WIP for local desktop installs. `make dev-install-vscode` accepts
`VSCODE_CLI` and `VSCODE_EXTENSIONS_DIR`, defaults `code-insiders` to the Insiders extension root,
and the verifier prints the exact installed path plus the reload requirement.

## 12. Missing runtime proof inside VS Code (Low–Med)

The installed extension files can contain the restored `Provider`, `Model`, `Thinking`, and `HITL
Policy` rows while the user-facing panel still shows stale UI until the window reloads. There is
no command inside VS Code that proves which extension path/version/binary the current window is
running.

**Fix:** add `Contenox: Show Runtime Info` that reports `extensionPath`, package
version, bridge binary path, remote name/UI kind, and other build/install facts. Emit the same data
as an activation telemetry event so stale-window bugs can be checked from logs.

**Status:** fixed in WIP. `Contenox: Show Runtime Info` is contributed to the command
palette and prints the loaded extension path/version, bridge binary path, remote/UI kind, telemetry
log path, and installed session-tree marker check. Activation also emits `extension.activated`
with the same fields.

---

## Cross-cutting note: many redundant ways to set the same config

Provider/model/think/HITL can be changed from the Runtime webview
(`config/RuntimeControlsView.ts`), the Sessions tree config nodes
(`chat/SessionTreeProvider.ts:127`), the palette selectors (`config/selectors.ts`),
chat slash commands (`/policy`, `/model`, …), and the CLI. Not a bug, but the
duplicate `selectModel` (#3) plus four+ entry points is a lot of surface to keep
consistent; worth a deliberate IA pass.

## WIP Todos

- Done: add tests for `SessionTreeProvider.getChildren()` proving config rows come before sessions.
- Decide final UX: either keep `Runtime` webview plus fallback rows, or make the Sessions tree the
  only supported runtime-control surface. Duplication is useful for debugging but not ideal
  long-term.
- Done: split dev packaging from release packaging so dev can be pragmatic while release keeps stricter
  VSCE checks.
- Done: make `make dev-install-vscode` print "Reload Window required" plus the verified installed path
  every time.
- Done: add a runtime proof command and activation telemetry for extension path/version/binary.

## What's solid (no action)

- Webview uses CSP + per-render nonce, `enableScripts` only, no local resources.
- Telemetry is local JSONL only; clearly described; user-toggleable.
- Untrusted workspaces: `capabilities.untrustedWorkspaces: limited` with restricted
  configs; autocomplete and chat runtime actions check `isTrusted`.
- Autocomplete has stale-result guards (sequence + `document.version`), document
  size/binary/long-line checks, and throttled failure warnings.
- Spawn errors map ENOENT/EACCES/ENOEXEC to actionable messages with remote/arch
  context (`BridgeProcess.ts:273-293`).
- Sessions view shows relative timestamps, active markers, and message previews.
