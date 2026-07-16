import {
  AcpClient,
  AcpError,
  createAcpClient,
  JSON_RPC_ERROR_CODES,
  textContent,
  workspaceConfigOptionsFromInit,
  type SessionConfigOption,
  type SessionConfigOptionValue,
  type SessionEventHandlers,
  type SessionId,
  type SessionInfo,
  type Transport,
} from '../lib/acp';
import type { AcpSessionAction } from './acpSessionState';
import type { AcpWorkspaceAction } from './acpWorkspaceState';

/**
 * Orchestration for a multi-session ACP workspace: one long-lived connection
 * (with reconnect-on-drop), a `session/list` roster, and exactly one
 * subscribed/"open" session at a time. Kept free of React so it can be driven
 * directly in a test with fake transports/timers (see
 * `acpWorkspaceController.test.ts`) — `useAcpWorkspace.ts` is the only caller
 * in the app proper, supplying `useReducer` dispatchers and re-rendering on
 * the resulting `acpWorkspaceReducer`/`acpSessionReducer` state.
 *
 * Design notes:
 *  - Exactly one `AcpClient.subscribe()` is active at a time (for whichever
 *    session is "open"). Because a subscription always wins routing over a
 *    concurrent `prompt()` call's per-turn handlers (see client.ts's
 *    `handlersFor`), `sendPrompt` below passes NO per-turn handlers — all
 *    streamed content, including permission requests, flows through the
 *    session's standing subscription.
 *  - `newSession()`/`openSession()`/`deleteSession()` are the lazy-creation
 *    and switching primitives (D5): nothing here auto-creates a session on
 *    connect. Callers (the React hook / page) decide when to call
 *    `newSession()` — typically on first prompt submit.
 */

export type WorkspaceDispatch = (action: AcpWorkspaceAction) => void;
export type SessionDispatch = (action: AcpSessionAction) => void;

export interface AcpWorkspaceControllerDeps {
  /**
   * Builds a fresh `Transport` for one connection attempt. Called anew for
   * the initial `connect()` AND for every reconnect attempt — this is what
   * lets a reconnect re-read a possibly-refreshed auth token (see
   * `AcpWorkspaceProvider.tsx`, which closes over `getStoredApiToken()`).
   */
  createTransport: () => Transport;
  /** Wraps a transport into a client. Defaults to `createAcpClient(transport)` with no capability provider. */
  createClient?: (transport: Transport) => AcpClient;
  /** `cwd` sent with `session/new`/`session/load`/`session/resume`. Defaults to `'/'`. */
  cwd?: string;
}

export interface AcpWorkspaceController {
  /** Idempotent: concurrent/repeated calls share the first attempt's in-flight (then settled) promise — "one controller, many consumers" needs only one connection. */
  connect(): Promise<void>;
  /** Pages `session/list` to completion and replaces the roster. No-op while disconnected. */
  refreshSessions(): Promise<void>;
  /** Lazy-creation primitive (D5): creates a session, subscribes to it, and makes it active. Returns the new session id. */
  newSession(): Promise<SessionId>;
  /** Subscribes to `id` BEFORE issuing `session/load` (replay arrives before the response resolves — see client.ts), then closes whichever session was previously open. */
  openSession(id: SessionId): Promise<void>;
  deleteSession(id: SessionId): Promise<void>;
  /**
   * Client-side only: unsubscribes from and (fire-and-forget) closes the
   * currently-open session, resets the session reducer, and clears
   * `activeSessionId` — WITHOUT deleting the session or creating a new one.
   * No-op if no session is open. Intended for the "new session" affordances
   * (header/sidebar/not-found buttons): they call this and navigate to bare
   * `/chat` so the composer comes up empty and the next submit's lazy
   * `newSession()` call (D5) mints a genuinely new session instead of
   * reusing whatever was active.
   */
  clearActiveSession(): void;
  /** No-ops while disposed, disconnected, no session is open, or the OPEN session already has a prompt in flight (an old session's still-settling turn never blocks a newly-opened session). Slash-command text passes through verbatim. */
  sendPrompt(text: string): void;
  /** Resolves the in-flight `session/request_permission`, if any, for the open session. */
  respondPermission(optionId: string): void;
  /** Fire-and-forget `session/cancel` for the open session. */
  cancel(): void;
  setConfigOption(configId: string, value: SessionConfigOptionValue): Promise<void>;
  /**
   * Applies a batch of config options to the currently-open session,
   * sequentially (awaiting each `set_config_option` round trip). Used to flush
   * the empty-chat's staged choices right after `newSession()` so they win over
   * the server's per-session defaults for the very first turn — the turn that
   * fails when the configured default model is broken. No-ops with no open
   * session. On failure it surfaces the error via the session error banner
   * (like a failed prompt) and rejects, so the caller can hold the turn back.
   */
  applyConfigOptions(options: Array<{ configId: string; value: SessionConfigOptionValue }>): Promise<void>;
  /**
   * Manual reconnect: cancels any pending automatic backoff timer and
   * immediately runs the same fresh-transport+initialize+resume path the
   * supervisor uses (see `attemptReconnect`), restarting the attempt counter
   * at 0. Intended for a "Retry connection" button shown once the automatic
   * supervisor has given up (`disconnected`), but safe to call any time the
   * connection isn't `ready` — e.g. to jump a pending backoff wait.
   */
  reconnect(): Promise<void>;
  /** Tears down: further async continuations become no-ops, the reconnect supervisor stops, any pending permission is rejected. */
  dispose(): void;
}

const MAX_RECONNECT_ATTEMPTS = 8; // mirrors useTaskEvents.ts's MAX_RETRIES
const RECONNECT_BASE_DELAY_MS = 1000;
const RECONNECT_MAX_DELAY_MS = 30000;

let idCounter = 0;
/** Monotonic id local to this module — unique per browser tab, which is all a client-side message/turn id needs to be. */
function nextId(prefix: string): string {
  idCounter += 1;
  return `${prefix}-${idCounter}`;
}

function errMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function isAuthRequired(err: unknown): boolean {
  return err instanceof AcpError && err.code === JSON_RPC_ERROR_CODES.authRequired;
}

/**
 * Classifies an `openSession()` failure as "no such session" vs. some other
 * problem. `acpsvc/session.go`'s `LoadSession` wraps an unknown-session
 * lookup as `ErrInvalidParams` (-32602), not the more specific
 * `ErrResourceNotFound` (-32002) — but `session/load`'s only
 * externally-supplied parameter on this call path is `id` (`cwd` is this
 * controller's own constant, always a valid absolute path — see the `cwd`
 * closure variable above), so ANY `invalidParams`/`resourceNotFound` failure
 * reaching this specific call site can only mean "no such session". Anything
 * else (internal error, a non-`AcpError` transport-level failure) is treated
 * as a generic error instead.
 */
function classifySessionOpenFailure(err: unknown): 'not_found' | 'error' {
  if (
    err instanceof AcpError &&
    (err.code === JSON_RPC_ERROR_CODES.resourceNotFound || err.code === JSON_RPC_ERROR_CODES.invalidParams)
  ) {
    return 'not_found';
  }
  return 'error';
}

export function createAcpWorkspaceController(
  deps: AcpWorkspaceControllerDeps,
  workspaceDispatch: WorkspaceDispatch,
  sessionDispatch: SessionDispatch,
): AcpWorkspaceController {
  const cwd = deps.cwd ?? '/';
  const buildClient = deps.createClient ?? ((transport: Transport) => createAcpClient(transport));

  let disposed = false;
  let client: AcpClient | null = null;
  let currentTransport: Transport | null = null;
  let connectPromise: Promise<void> | null = null;

  let activeSessionId: SessionId | null = null;
  let unsubscribeActive: (() => void) | null = null;
  /**
   * Which session has a `session/prompt` call in flight, or null. Tracked BY
   * SESSION (not a single boolean) so that switching away mid-turn — e.g.
   * clearActiveSession() + lazy newSession() — doesn't leave the new
   * session's first prompt silently blocked behind the old session's still-
   * settling turn. Only re-prompting the SAME session while its turn is in
   * flight is a no-op.
   */
  let promptSessionId: SessionId | null = null;
  /** Fallback grouping id for a live turn's chunks that arrive with no server-assigned `messageId`. */
  let currentTurnId = '';

  let permissionResolve: ((optionId: string) => void) | null = null;
  let permissionReject: ((err: Error) => void) | null = null;

  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  // ---------------------------------------------------------------------
  // Shared helpers
  // ---------------------------------------------------------------------

  /** Dispatches `setup_required` if `err` is a `-32000 auth_required` JSON-RPC error; never throws itself. */
  function guardAuthRequired(err: unknown): void {
    if (isAuthRequired(err)) {
      workspaceDispatch({ type: 'setup_required', message: errMessage(err) });
    }
  }

  async function runGuarded<T>(fn: () => Promise<T>): Promise<T> {
    try {
      return await fn();
    } catch (err) {
      guardAuthRequired(err);
      throw err;
    }
  }

  function rejectPendingPermission(): void {
    const reject = permissionReject;
    permissionResolve = null;
    permissionReject = null;
    // Let AcpClient answer the agent's request with outcome "cancelled" (see
    // client.ts's `handlePermissionRequest` catch) instead of hanging.
    reject?.(new Error('acp: pending permission request was superseded'));
  }

  /**
   * Handlers routed to whichever session is currently subscribed. Built once
   * per `subscribe()` call (on open/create/resume/reload) rather than per
   * prompt — the standing subscription always wins over a concurrent
   * `prompt()` call's per-turn handlers, so `sendPrompt` deliberately passes
   * none (see the module doc comment).
   */
  function buildSessionHandlers(sid: SessionId): SessionEventHandlers {
    return {
      onUserMessageChunk: (text, id) => sessionDispatch({ type: 'user_message_chunk', id: id ?? nextId('user'), text }),
      onMessageChunk: (text, id) => sessionDispatch({ type: 'message_chunk', id: id ?? currentTurnId, text }),
      onThoughtChunk: (text, id) => sessionDispatch({ type: 'thought_chunk', id: id ?? currentTurnId, text }),
      onToolCall: event => sessionDispatch({ type: 'tool_call', event }),
      onPlan: entries => sessionDispatch({ type: 'plan', entries }),
      onUsage: usage => sessionDispatch({ type: 'usage', usage }),
      onAvailableCommands: commands => sessionDispatch({ type: 'available_commands', commands }),
      onConfigOptions: configOptions => sessionDispatch({ type: 'config_options', configOptions }),
      onSessionInfo: info =>
        workspaceDispatch({
          type: 'session_upserted',
          session: { sessionId: sid, title: info.title, updatedAt: info.updatedAt },
        }),
      onPermissionRequest: request =>
        new Promise<string>((resolve, reject) => {
          permissionResolve = resolve;
          permissionReject = reject;
          sessionDispatch({ type: 'permission_request', request });
        }),
    };
  }

  // ---------------------------------------------------------------------
  // Connection lifecycle (connect + reconnect supervisor)
  // ---------------------------------------------------------------------

  /** Builds a fresh transport+client and runs `initialize()`; closes the transport on failure. Never mutates controller state — callers decide whether/how to adopt the result. */
  async function establishConnection(): Promise<{
    client: AcpClient;
    transport: Transport;
    agentName: string | null;
    workspaceConfigOptions: SessionConfigOption[];
  }> {
    const transport = deps.createTransport();
    const c = buildClient(transport);
    try {
      const init = await c.initialize();
      return {
        client: c,
        transport,
        agentName: init.agentInfo?.name ?? null,
        // Workspace-level (session-less) config options advertised in the
        // initialize `_meta` — empty for agents that don't speak the extension.
        // Drives the empty-chat controls before any session exists (see
        // AcpChatPage). Refreshed on every reconnect too, since runtime state
        // (available models) may have changed while disconnected.
        workspaceConfigOptions: workspaceConfigOptionsFromInit(init),
      };
    } catch (err) {
      transport.close();
      throw err;
    }
  }

  function adoptClient(c: AcpClient, transport: Transport): void {
    client = c;
    currentTransport = transport;
    transport.onClose(() => handleTransportClose(transport));
  }

  async function refreshSessionsInternal(c: AcpClient): Promise<void> {
    const collected: SessionInfo[] = [];
    let cursor: string | undefined;
    for (;;) {
      const page = await runGuarded(() => c.listSessions(cursor));
      collected.push(...page.sessions);
      if (!page.nextCursor) break;
      cursor = page.nextCursor;
    }
    if (disposed) return;
    workspaceDispatch({ type: 'sessions_replaced', sessions: collected });
  }

  function connect(): Promise<void> {
    if (disposed) return Promise.reject(new Error('acp: workspace controller disposed'));
    if (!connectPromise) {
      connectPromise = doConnect();
    }
    return connectPromise;
  }

  async function doConnect(): Promise<void> {
    workspaceDispatch({ type: 'connecting' });
    try {
      const established = await establishConnection();
      if (disposed) {
        established.transport.close();
        return;
      }
      adoptClient(established.client, established.transport);
      workspaceDispatch({
        type: 'ready',
        agentName: established.agentName,
        workspaceConfigOptions: established.workspaceConfigOptions,
      });
      await refreshSessionsInternal(established.client);
    } catch (err) {
      if (disposed) return;
      guardAuthRequired(err);
      if (!isAuthRequired(err)) {
        workspaceDispatch({ type: 'error', message: errMessage(err) });
      }
    }
  }

  /** Re-binds `activeSessionId` after a reconnect: tries `session/resume` (transcript kept client-side) first, falling back to a full `session/load` replay if resume fails (e.g. the serve process restarted and wiped its in-memory session map). */
  async function restoreActiveSession(c: AcpClient): Promise<void> {
    const sid = activeSessionId;
    if (!sid) return;
    try {
      const result = await c.resumeSession(sid, cwd);
      unsubscribeActive = c.subscribe(sid, buildSessionHandlers(sid));
      // See the equivalent comment in newSession() — session/resume's
      // response carries this session's config options inline too.
      if (result.configOptions) sessionDispatch({ type: 'config_options', configOptions: result.configOptions });
      sessionDispatch({ type: 'connection_resumed' });
      return;
    } catch {
      // Fall through to the full-reload fallback below.
    }
    sessionDispatch({ type: 'session_reset', sessionId: sid });
    // Subscribe BEFORE load: session/load's replay notifications reach the
    // wire before the session/load response resolves (see client.ts /
    // acpsvc/session.go's replayMessages).
    unsubscribeActive = c.subscribe(sid, buildSessionHandlers(sid));
    try {
      const result = await c.loadSession(sid, cwd);
      if (result.configOptions) sessionDispatch({ type: 'config_options', configOptions: result.configOptions });
      sessionDispatch({ type: 'connection_resumed' });
    } catch (err) {
      sessionDispatch({ type: 'prompt_error', message: `failed to restore session after reconnect: ${errMessage(err)}` });
    }
  }

  function scheduleReconnectAttempt(attemptIndex: number): void {
    if (disposed) return;
    const delay = Math.min(RECONNECT_BASE_DELAY_MS * 2 ** attemptIndex, RECONNECT_MAX_DELAY_MS);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      void attemptReconnect(attemptIndex);
    }, delay);
  }

  async function attemptReconnect(attemptIndex: number): Promise<void> {
    if (disposed) return;
    try {
      const established = await establishConnection();
      if (disposed) {
        established.transport.close();
        return;
      }
      adoptClient(established.client, established.transport);
      workspaceDispatch({
        type: 'ready',
        agentName: established.agentName,
        workspaceConfigOptions: established.workspaceConfigOptions,
      });
      await restoreActiveSession(established.client);
      if (disposed) return;
      await refreshSessionsInternal(established.client);
    } catch (err) {
      if (disposed) return;
      if (isAuthRequired(err)) {
        workspaceDispatch({ type: 'setup_required', message: errMessage(err) });
        return; // Terminal: never retried.
      }
      const next = attemptIndex + 1;
      if (next >= MAX_RECONNECT_ATTEMPTS) {
        workspaceDispatch({ type: 'disconnected' });
        return;
      }
      scheduleReconnectAttempt(next);
    }
  }

  function reconnect(): Promise<void> {
    if (disposed) return Promise.reject(new Error('acp: workspace controller disposed'));
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    workspaceDispatch({ type: 'reconnecting' });
    return attemptReconnect(0);
  }

  function handleTransportClose(transport: Transport): void {
    if (disposed) return; // User-initiated (dispose()) — never reconnect.
    if (transport !== currentTransport) return; // Stale listener from a superseded transport.
    workspaceDispatch({ type: 'reconnecting' });
    if (activeSessionId) sessionDispatch({ type: 'connection_lost' });
    scheduleReconnectAttempt(0);
  }

  // ---------------------------------------------------------------------
  // Session roster + switching
  // ---------------------------------------------------------------------

  async function refreshSessions(): Promise<void> {
    if (disposed || !client) return;
    await refreshSessionsInternal(client);
  }

  async function newSession(): Promise<SessionId> {
    if (disposed || !client) throw new Error('acp: workspace controller is not connected');
    const c = client;

    rejectPendingPermission();
    const previousId = activeSessionId;
    const previousUnsubscribe = unsubscribeActive;

    // session/new mints the id server-side, so (unlike session/load) we
    // cannot subscribe until the response carries it.
    const result = await runGuarded(() => c.newSession(cwd)).catch(err => {
      // Auth failures are already surfaced as `setup_required` by
      // guardAuthRequired (inside runGuarded) — that swaps the whole page to
      // SetupRequiredState, so there's no composer left to show an inline
      // error in. Anything else (e.g. a transient RPC failure) needs to
      // reach the UI some other way: reuse the same inline error banner a
      // failed prompt turn shows (`prompt_error`, see acpSessionState.ts)
      // since this failure happens on the current page before any session
      // exists yet to attach it to.
      if (!isAuthRequired(err)) {
        sessionDispatch({ type: 'prompt_error', message: errMessage(err) });
      }
      throw err;
    });
    const sid = result.sessionId;

    sessionDispatch({ type: 'session_reset', sessionId: sid });
    activeSessionId = sid;
    currentTurnId = '';
    unsubscribeActive = c.subscribe(sid, buildSessionHandlers(sid));
    // session/new's response carries the session's initial config options
    // (model/think/token-limit/hitl-policy) inline — unlike everything else
    // in buildSessionHandlers(), this never arrives as a session/update
    // notification on a fresh session, so it has to be applied here rather
    // than relying on onConfigOptions.
    if (result.configOptions) sessionDispatch({ type: 'config_options', configOptions: result.configOptions });

    workspaceDispatch({ type: 'session_upserted', session: { sessionId: sid, cwd } });
    workspaceDispatch({ type: 'active_session_changed', sessionId: sid });
    // A brand-new session is trivially "open" — clears any stale
    // not_found/error left by a previous failed openSession() (e.g. the
    // NotFoundState page's "start new session" action).
    workspaceDispatch({ type: 'session_load_succeeded' });

    if (previousId && previousId !== sid) {
      previousUnsubscribe?.();
      // Fire-and-forget: releasing the old session's connection-local state
      // is bookkeeping, not something the caller should wait on to see the
      // new session — see the interface doc comment on newSession().
      void c.closeSession(previousId).catch(() => {});
    }

    return sid;
  }

  async function openSession(id: SessionId): Promise<void> {
    if (disposed || !client) return;
    if (activeSessionId === id) return;
    const c = client;

    rejectPendingPermission();
    workspaceDispatch({ type: 'session_load_start' });
    const previousId = activeSessionId;
    const previousUnsubscribe = unsubscribeActive;

    activeSessionId = id;
    currentTurnId = '';
    sessionDispatch({ type: 'session_reset', sessionId: id });
    workspaceDispatch({ type: 'active_session_changed', sessionId: id });
    // Subscribe BEFORE load: session/load's replay notifications reach the
    // wire before the session/load response resolves (see client.ts's
    // subscribe() doc comment / acpsvc/session.go's replayMessages, and the
    // wire-fact test in client.test.ts's "session/load replay routing" suite).
    unsubscribeActive = c.subscribe(id, buildSessionHandlers(id));

    try {
      const result = await runGuarded(() => c.loadSession(id, cwd));
      // See the equivalent comment in newSession() — session/load's response
      // carries this session's config options inline, same as session/new's.
      if (result.configOptions) sessionDispatch({ type: 'config_options', configOptions: result.configOptions });
    } catch (err) {
      // The optimistic switch above already tore down `previousId`'s
      // subscription and reset the session reducer to represent `id` — there
      // is no live state left to roll back to. Land in a well-defined "no
      // session open" state instead of leaving activeSessionId pointed at a
      // session that doesn't exist.
      unsubscribeActive?.(); // the failed `id`'s subscription
      previousUnsubscribe?.(); // `previousId`'s subscription, still live until now — see the mirrored cleanup on the success path below
      unsubscribeActive = null;
      activeSessionId = null;
      currentTurnId = '';
      sessionDispatch({ type: 'session_reset', sessionId: null });
      workspaceDispatch({ type: 'active_session_changed', sessionId: null });
      if (previousId) {
        // Fire-and-forget — see the equivalent comment in newSession().
        void c.closeSession(previousId).catch(() => {});
      }
      if (classifySessionOpenFailure(err) === 'not_found') {
        workspaceDispatch({ type: 'session_load_not_found' });
      } else {
        workspaceDispatch({ type: 'session_load_failed', message: errMessage(err) });
      }
      return;
    }

    workspaceDispatch({ type: 'session_load_succeeded' });
    if (previousId && previousId !== id) {
      previousUnsubscribe?.();
      // Fire-and-forget — see the equivalent comment in newSession().
      void c.closeSession(previousId).catch(() => {});
    }
  }

  async function deleteSession(id: SessionId): Promise<void> {
    if (disposed || !client) return;
    const c = client;

    if (id === activeSessionId) {
      rejectPendingPermission();
      unsubscribeActive?.();
      unsubscribeActive = null;
      activeSessionId = null;
      currentTurnId = '';
      sessionDispatch({ type: 'session_reset', sessionId: null });
      workspaceDispatch({ type: 'active_session_changed', sessionId: null });
      // Fire-and-forget — see the equivalent comment in newSession(). The
      // session/delete call below is what actually matters here.
      void c.closeSession(id).catch(() => {});
    }

    await runGuarded(() => c.deleteSession(id));
    workspaceDispatch({ type: 'session_removed', sessionId: id });
  }

  function clearActiveSession(): void {
    if (disposed) return;
    rejectPendingPermission();
    const previousId = activeSessionId;
    const previousUnsubscribe = unsubscribeActive;
    unsubscribeActive = null;
    activeSessionId = null;
    currentTurnId = '';
    sessionDispatch({ type: 'session_reset', sessionId: null });
    workspaceDispatch({ type: 'active_session_changed', sessionId: null });
    if (previousId) {
      previousUnsubscribe?.();
      // Fire-and-forget — see the equivalent comment in newSession(). The
      // session itself is NOT deleted, just released connection-side.
      if (client) void client.closeSession(previousId).catch(() => {});
    }
  }

  // ---------------------------------------------------------------------
  // Turn + permission + config actions on the open session
  // ---------------------------------------------------------------------

  function sendPrompt(text: string): void {
    if (disposed || !client || !activeSessionId || promptSessionId === activeSessionId || !text.trim()) return;
    const c = client;
    const sid = activeSessionId;
    // Fallback grouping key for chunks that arrive without a `messageId` — one
    // assistant message per turn, per the client core's documented contract.
    currentTurnId = nextId('assistant');

    // Slash-command text passes through verbatim — acpsvc/commandrunner.go
    // parses `/name args` itself; this layer never inspects it.
    sessionDispatch({ type: 'user_message_chunk', id: nextId('user'), text });
    sessionDispatch({ type: 'prompt_start' });
    promptSessionId = sid;

    // No per-turn handlers: the session's standing subscription (set up in
    // newSession()/openSession()) already routes every session/update and
    // session/request_permission for sid — see the module doc comment.
    //
    // The settle handlers below are gated on `activeSessionId === sid`, NOT
    // on `disposed`: the terminal dispatch is a pure reducer update whose
    // entire purpose is ending the turn (clearing `isPrompting` and every
    // message's `streaming` flag — see acpSessionState.ts's `endStreaming`),
    // so it must run even after dispose() — but it must NOT run once the
    // session reducer represents a DIFFERENT session (the user switched or
    // cleared mid-turn; `session_reset` already put that state in a clean
    // not-prompting shape), or the old turn's failure banner / stopReason
    // would bleed into the freshly-opened session.
    c.prompt(sid, [textContent(text)])
      .then(({ stopReason }) => {
        if (promptSessionId === sid) promptSessionId = null;
        if (activeSessionId !== sid) return;
        sessionDispatch({ type: 'prompt_end', stopReason });
      })
      .catch((err: unknown) => {
        if (promptSessionId === sid) promptSessionId = null;
        if (!disposed) guardAuthRequired(err);
        if (activeSessionId !== sid) return;
        sessionDispatch({ type: 'prompt_error', message: errMessage(err) });
      });
  }

  function respondPermission(optionId: string): void {
    const resolve = permissionResolve;
    permissionResolve = null;
    permissionReject = null;
    if (!resolve) return;
    sessionDispatch({ type: 'permission_resolved' });
    resolve(optionId);
  }

  function cancel(): void {
    if (!client || !activeSessionId) return;
    client.cancel(activeSessionId);
  }

  async function setConfigOption(configId: string, value: SessionConfigOptionValue): Promise<void> {
    if (disposed || !client || !activeSessionId) return;
    const result = await runGuarded(() => client!.setConfigOption(activeSessionId!, configId, value));
    sessionDispatch({ type: 'config_options', configOptions: result.configOptions });
  }

  async function applyConfigOptions(
    options: Array<{ configId: string; value: SessionConfigOptionValue }>,
  ): Promise<void> {
    if (disposed || !client || !activeSessionId || options.length === 0) return;
    try {
      // Sequential, not parallel: each set_config_option's response carries the
      // full recomputed option set (see acpsvc SetSessionConfigOption), and a
      // later option (e.g. token-limit clamped to the model cap) can depend on
      // an earlier one (model). Applying in order keeps the server's view and
      // the reducer's `session.configOptions` consistent.
      for (const { configId, value } of options) {
        await setConfigOption(configId, value);
      }
    } catch (err) {
      // Non-auth failures (auth already became setup_required via
      // guardAuthRequired inside setConfigOption) surface on the same inline
      // banner a failed prompt uses — the session exists by now, so there's a
      // surface to attach it to. Rethrow so the caller holds the prompt back.
      if (activeSessionId) sessionDispatch({ type: 'prompt_error', message: errMessage(err) });
      throw err;
    }
  }

  // ---------------------------------------------------------------------
  // Teardown
  // ---------------------------------------------------------------------

  function dispose(): void {
    if (disposed) return;
    disposed = true;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    rejectPendingPermission();
    client?.close();
    client = null;
    currentTransport = null;
  }

  return {
    connect,
    refreshSessions,
    newSession,
    openSession,
    deleteSession,
    clearActiveSession,
    sendPrompt,
    respondPermission,
    cancel,
    setConfigOption,
    applyConfigOptions,
    reconnect,
    dispose,
  };
}
