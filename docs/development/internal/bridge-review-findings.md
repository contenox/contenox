# VS Code Bridge — Review Findings

Review of the VS Code extension bridge (`packages/vscode/src/bridge`, `packages/vscode/src/chat`)
and its Go counterpart (`runtime/vscodeagent`), cross-referenced against the ACP reference
implementation (`libacp`, `runtime/acpsvc`) and the
[permission-bridge blueprint](./acp-permission-bridge.md).

Date: 2026-06-14

## Summary

The blueprint's core design is implemented correctly: approvals flow as a blocking reverse
`session/request_permission` request (Go→editor), `runtime/vscodeagent.callClient` faithfully
mirrors `libacp.AgentSideConnection.call` (pending map under mutex, fail-closed on
write/close/ctx), the shared `runtime/approvalflow` builder is used by both `acpsvc` and
`vscodeagent`, and shipped `hitl-policy-default.json` defaults `local_shell` to `approve`
(blueprint item 7 — done). The boundary itself is sound. The items below are the reviewed gaps
and their current WIP status.

| # | Severity | Finding |
|---|----------|---------|
| 1 | Medium (HITL integrity) | Permission requests routed to the wrong turn under concurrency |
| 2 | Low–Med (latent) | Runtime-side abandonment of a pending approval desyncs the editor modal |
| 3 | Low (cleanup) | Legacy `approvalRequested` / `approvalRespond` path is fully dead code |
| 4 | Low (observation) | Permission-pending guard is effectively inert in the synchronous bridge |
| 5 | Low (hardening) | No Content-Length upper bound in either JSON-RPC framer |

---

## Follow-up Findings From Sidebar/Session Review

These are the immediate WIP items found after restoring the Runtime/Sessions sidebar controls.

| # | Severity | Finding |
|---|----------|---------|
| 6 | High | Session reads currently mutate the active session |
| 7 | High | Proposed/native agent-session content has the same active-session side effect |

### 6. Session reads currently mutate active session (High)

`packages/vscode/src/chat/SessionTreeProvider.ts` calls `sessionLoad` just to render preview
children, but `runtime/vscodeagent.chat.sessionLoad` calls `SetActiveID`. Expanding a session in
the tree can silently make that session active. Later `@contenox` turns may then continue in the
wrong Contenox session.

**Fix:** split the bridge API so history reads use a non-mutating `sessionRead`/`sessionGet`, and
reserve `sessionLoad` or `sessionActivate` for explicit user activation. The tree preview must use
the non-mutating path.

**Tests:** add a bridge/runtime test proving `sessionRead` returns messages without changing
`sessionList().sessions[*].isActive`, and that `sessionLoad` still activates explicitly.

**Status:** fixed in WIP. The bridge now has a non-mutating `sessionRead` RPC. The sidebar preview
uses `sessionRead`, while explicit `Open Session` still uses activating `sessionLoad`. Runtime test
coverage verifies that `sessionRead` does not change active state and `sessionLoad` does.

### 7. Proposed/native agent-session content has the same side effect (High)

`packages/vscode/src/agentSessions/provider.ts` loads proposed native session content through
`sessionLoad`. That also marks the session active. Even though proposed native sessions are off by
default, this must be fixed before treating that path as real.

**Fix:** all history/content reads use the new non-mutating API. Explicit user actions that mean
"continue this session" may still call the activating path.

**Status:** fixed in WIP. Proposed/native agent-session content reads now call `sessionRead`.

## 1. Permission requests routed to the wrong turn under concurrency (Medium)

`BridgeClient.handleServerRequest` selects the most-recently-pushed handler and ignores
`params.sessionId`:

```ts
// packages/vscode/src/bridge/BridgeClient.ts:370
const handler = this.permissionHandlers[this.permissionHandlers.length - 1];
```

There is a single shared `BridgeClient` (`BridgeProcess.ts:33`) across the `contenox` panel
participant, the experimental `contenox-agent` session participant (`agentSessions/provider.ts`
routes into the same `ContenoxChatParticipant`), and every chat editor tab. Each in-flight turn
pushes its own handler bound to **its** `toolInvocationToken` + response stream
(`turnRunner.ts:128`).

With two concurrent turns, an approval for turn A's session is dispatched to turn B's handler:

- Renders in the wrong chat, tied to the wrong `toolInvocationToken`.
- Confused-deputy: user approves A's action believing it belongs to B's conversation.
- Or an incorrect fail-closed deny (B's call routed to a token-less agent session).

Contradicts blueprint item 5 ("keep native approval UI tied to the current chat request").

**Reachability:** any two overlapping turns — multiple chat editor tabs (default), or the
experimental agent-sessions participant alongside the panel.

**Fix:** route by session. The Go side already populates `RequestPermissionRequest.SessionID`
reliably (`approval.go` → `sessionIDFromContext` via the requestID→turn map registered in
`chatSend` before either chat/command turn dispatches). Change `pushPermissionRequestHandler`
to take a `sessionId`, and have the dispatcher match `params.sessionId` (fail-closed
`{ outcome: { outcome: "cancelled" } }` on no match).

**Tests:** current coverage exercises only a single handler (`extension.test.ts:80`,
`server_test.go:373`). Add a two-concurrent-turns routing test.

**Status:** fixed in WIP. `BridgeClient.pushPermissionRequestHandler` is now registered per
session id, `session/request_permission` dispatch requires an exact `params.sessionId` match, and
missing handlers fail closed. Extension tests cover multi-handler routing.

## 2. Runtime-side abandonment desyncs the editor modal (Low–Med, latent)

When the Go side gives up on an in-flight `session/request_permission` (its ctx is cancelled),
`callClient` removes the pending entry and returns without telling the editor:

```go
// runtime/vscodeagent/client_requests.go:68-71
select {
case <-ctx.Done():
    s.removeClientRequest(key)
    return ctx.Err()
```

It never sends `$/cancelRequest` to the editor, and the TS side has no inbound `$/cancelRequest`
handler (it only ever *sends* one, `BridgeClient.ts:457`). The native approval modal lingers; a
late "Allow" click is silently swallowed (`handleClientResponsePayload` drops the unknown id)
while the runtime already denied.

**Reachability:** the common case is fine — when the *user* cancels the turn, the chat
`CancellationToken` dismisses `vscode.lm.invokeTool` directly. This only bites when the runtime
abandons independently of that token:
- a HITL `timeoutS`/`onTimeout` policy (`runtime/localtools/hitl.go:137-160` — supported; no
  shipped policy sets it yet), or
- a bridge crash mid-approval (`BridgeProcess.ts:158`).

**Fix:** have `callClient` emit `$/cancelRequest` on ctx-done; have `handleServerRequest` drive a
per-request `CancellationTokenSource` so an inbound cancel dismisses the modal. Worth doing
before any timeout policy ships.

**Status:** fixed in WIP. Runtime-side `callClient` sends `$/cancelRequest` when its context ends.
The TypeScript bridge tracks active server requests and cancels the permission handler token on
inbound `$/cancelRequest`. Tests cover both directions.

## 3. Legacy approval path is fully dead code (Low, cleanup)

The Go side no longer emits `approvalRequested` anywhere, so the old path is unreachable but
still wired on both sides:

- TS: `onApprovalRequested` → `handleApproval` → `client.approvalRespond`
  (`participant.ts:264,376`, `turnRunner.ts:125`, `BridgeClient.ts:207`).
- Go: `approvalRespond` handler + `ApprovalBroker.Respond` (`chat.go:444`, `approval.go:86`).

`ApprovalBroker.Respond` reads a `pending` map nothing ever populates, so it is a permanent
`false`. Blueprint item 3 said to delete this once TS migrated — TS has migrated. Two approval
mechanisms (one inert) is a regression foot-gun.

**Fix:** delete the legacy notification/response path on both sides.

**Status:** fixed in WIP. `approvalRespond`, `approvalRequested`, `ApprovalBroker.Respond`, and the
old TypeScript notification handler path have been removed. Permission decisions now flow only
through blocking `session/request_permission`.

## 4. Permission-pending guard is effectively inert here (Low, observation)

`notifyToolCallGuarded` (`runtime/vscodeagent/events.go:76`) ports `acpsvc`'s `permPending`
idea (`runtime/acpsvc/transport.go:111`), but `acpsvc` relies on **async** event delivery
(channel + `translateEvents` goroutine) while the bridge sink publishes **synchronously** on the
tool goroutine. Given the real emission order — pending fires *before* `markPending`
(`taskexec.go:847` precedes `toolsProvider.Exec`), completed fires *after* `clearPending`
(`approval.go` defer runs when `AskApproval` returns, before `inner.Exec`) — nothing is emitted
inside the guard window, so it suppresses nothing.

Harmless, but the protection the blueprint describes is not actually active in this transport.
(The same sync model makes the check-then-send TOCTOU a non-issue — one goroutine per turn.)

**Status:** fixed in WIP by removal. The VS Code bridge no longer carries the misleading
permission-pending guard. ACP keeps its async transport guard; the synchronous VS Code bridge uses
the blocking permission request as the enforcement boundary.

## 5. No Content-Length upper bound in either framer (Low, hardening)

`runtime/vscodeagent/jsonrpc.go:84` (`make([]byte, contentLength)`) and
`packages/vscode/src/bridge/JsonRpcFramer.ts:27` accept any non-negative length. The peer is
local and trusted, so this is defense-in-depth only; a cap + reject prevents a runaway
allocation from a malformed frame.

**Status:** fixed in WIP. Both framers now reject payloads above 64 MiB.

---

## What's solid (verified, no action needed)

- `callClient` ↔ `libacp.call` shape parity: pending map under mutex, removal on every exit
  path, fail-closed on write error / connection close / ctx cancel.
- Outcome handling is fail-closed everywhere (`cancelled` → deny, unknown outcome → deny,
  no handler → deny).
- ID spaces overlap between directions but are disambiguated by message shape
  (`isRequest`/`isResponse`/`handleClientResponsePayload`), so no misrouting.
- Diff/meta field names line up across the wire (`libacp/tools.go` `oldText`/`newText`,
  `_meta` on `RequestPermissionRequest`/`PermissionToolCall`) and the TS
  `approvalEventFromPermissionRequest` extracts them correctly.
- `hitl-policy-default.json` and `hitl-policy-dev.json` both default `local_shell` to `approve`.

## WIP Todos

- Done: add non-mutating bridge API: `sessionRead`/`sessionGet`, then reserve `sessionLoad` or
  `sessionActivate` for explicit user activation.
- Done: update sidebar session preview and proposed agent-session provider to use the non-mutating read
  API.
- Done: add tests for the active-session behavior: preview/read must not change active session; explicit
  load/activate must.
- Done: harden HITL approval cards further: include regular ACP content as fallback details, not only
  `rawInput`, diff, and meta.
- Done: add approval tests that simulate deny/cancel and verify the bridge returns the reject/cancel
  option and the runtime does not execute.
