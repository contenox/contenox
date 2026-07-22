import {
  AcpClient,
  AcpError,
  agentMeta,
  createAcpClient,
  JSON_RPC_ERROR_CODES,
  workspaceConfigOptionsFromInit,
  type PromptCapabilities,
  type SessionConfigOption,
  type SessionConfigOptionValue,
  type SessionEventHandlers,
  type SessionId,
  type SessionInfo,
  type Transport,
} from '../lib/acp';
import { adoptMeta, type AdoptRef } from '../lib/adoptMeta';
import type { PendingImageAttachment } from '../pages/chat/lib/imageAttachments';
import { promptBlocksFromDraft, type WorkspaceFileRef } from '../pages/chat/lib/mentions';
import type { AcpSessionAction } from './acpSessionState';
import { EMPTY_SESSION_KEY, type AcpSessionsAction, type AcpWorkspaceAction } from './acpWorkspaceState';

/**
 * Orchestration for a multi-session ACP workspace: one long-lived connection
 * (with reconnect-on-drop), a `session/list` roster, and any number of
 * concurrently-subscribed ("open") sessions — each with its own live-state
 * slice, plus a single "focused" session the single-view UI renders. Kept
 * free of React so it can be driven directly in a test with fake
 * transports/timers (see `acpWorkspaceController.test.ts`) —
 * `useAcpWorkspace.ts` is the only caller in the app proper, supplying
 * `useReducer` dispatchers and re-rendering on the resulting
 * `acpWorkspaceReducer`/`acpSessionsReducer` state.
 *
 * Multiplexing (workspace-tabs Slice 1): the wire already namespaces every
 * `session/update` by sessionId (see client.ts's `subscriptions` map /
 * `handlersFor`), so N standing `client.subscribe()` subscriptions coexist
 * over ONE connection. This controller holds one entry per open session in
 * `openSessions` (its subscription + per-session turn/permission bookkeeping)
 * and dispatches each session's traffic into ITS OWN slice via
 * `sessionDispatch(sid, …)`. A background (non-focused) session therefore
 * keeps accumulating its updates — a reply that began while focused keeps
 * streaming after the user switches away.
 *
 * Design notes:
 *  - A subscription always wins routing over a concurrent `prompt()` call's
 *    per-turn handlers (see client.ts's `handlersFor`), so `sendPrompt` below
 *    passes NO per-turn handlers — all streamed content, including permission
 *    requests, flows through the session's standing subscription.
 *  - `newSession()`/`openSession()`/`deleteSession()` are the lazy-creation
 *    and single-view switching primitives (D5): nothing here auto-creates a
 *    session on connect. Callers (the React hook / page) decide when to call
 *    `newSession()` — typically on first prompt submit. They keep the
 *    pre-multiplexing "one open session at a time" behavior (each closes the
 *    previously-focused session) by delegating to the multi-session
 *    primitives `openSessionTab()`/`closeSessionTab()`/`focusSession()`, which
 *    Slice 2 (tab UI) uses directly to hold several sessions open at once.
 */

export type WorkspaceDispatch = (action: AcpWorkspaceAction) => void;
/** Dispatches into the multiplexed sessions store (one slice per open session) — see `acpSessionsReducer`. */
export type SessionsDispatch = (action: AcpSessionsAction) => void;

export interface AcpWorkspaceControllerDeps {
  /**
   * Builds a fresh `Transport` for one connection attempt. Called anew for
   * the initial `connect()` AND for every reconnect attempt, so a reconnect
   * re-opens the WebSocket with the current same-origin auth cookie (see
   * `AcpWorkspaceProvider.tsx`'s `buildAcpWsUrl`).
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
  /** Lazy-creation primitive (D5): creates a session, subscribes to it, focuses it, and closes whichever session was previously focused. Returns the new session id. `cwd` overrides the default workspace root for this session (the root the user picked before the first prompt); falls back to the controller's default cwd. `agentName`, when set, binds the session to a registered external agent via the `session/new` `_meta` extension (see `AGENT_META_KEY`) — the runtime spawns/drives that agent instead of the native chain; the response `_meta` echo is threaded into the roster for attribution. */
  newSession(cwd?: string, agentName?: string | null): Promise<SessionId>;
  /**
   * ADOPT an already-running instance + downstream session (a fleet dispatch)
   * into a NEW upstream chat session, via the `session/new` `contenox.adopt`
   * `_meta` extension (see `adoptMeta.ts` / acpsvc/adopt.go). Subscribes to and
   * focuses the resulting session and threads the response `_meta` (which
   * carries the `controller` verdict) into the roster so the chat surface can
   * label it. Unlike `newSession`, adoption is ADDITIVE: it does NOT tear down
   * the previously-focused session, so opening a running unit to watch it never
   * closes a chat the operator already had open. Returns the new upstream
   * session id. `cwd` governs only the upstream session's own bookkeeping — the
   * downstream cwd was fixed at dispatch and cannot be re-rooted.
   */
  adoptSession(ref: AdoptRef, cwd?: string): Promise<SessionId>;
  /** Single-view switch: opens `id` as a tab (see `openSessionTab`) and closes whichever session was previously focused, preserving the pre-multiplexing "one open session at a time" behavior. No-op if `id` is already focused. */
  openSession(id: SessionId): Promise<void>;
  /**
   * Multi-session primitive (Slice 2): subscribes to `id` BEFORE issuing
   * `session/load` (replay arrives before the response resolves — see
   * client.ts) and focuses it, WITHOUT closing any other open session. Opening
   * an already-open session just focuses it (dedup by identity). Several
   * sessions can be open and live at once; each keeps its own slice.
   */
  openSessionTab(id: SessionId): Promise<void>;
  /**
   * Multi-session primitive (Slice 2): unsubscribes from and (fire-and-forget)
   * closes `id` — `session/close`, NOT `session/delete`, so the session stays
   * in the roster — and drops its slice. If `id` was focused, focus moves to
   * another still-open session (or the empty view). No-op if `id` is not open.
   * Leaves every other open session live.
   */
  closeSessionTab(id: SessionId): void;
  /** Multi-session primitive (Slice 2): re-points the focused/rendered session to `id` without any wire traffic. No-op if `id` is not open. */
  focusSession(id: SessionId): void;
  /**
   * Multi-session primitive (Slice 2): re-points focus to the empty/new-chat
   * surface (activeSessionId -> null) WITHOUT closing any open session — unlike
   * `clearActiveSession`, which tears the focused session down. This is what
   * lets a fresh "new session" tab coexist with already-open session tabs: the
   * next lazy `newSession()` then mints a genuinely new session (its `previousId`
   * is null, so it closes nothing). No wire traffic.
   */
  focusEmptyTab(): void;
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
  /**
   * No-ops while disposed, disconnected, no session is open, or the OPEN session
   * already has a prompt in flight (an old session's still-settling turn never
   * blocks a newly-opened session). Slash-command text passes through verbatim.
   * `mentions` are workspace files referenced with `@` in the composer; each is
   * sent as a `resource_link` content block alongside the text — reference only,
   * the agent reads the file through its tools (never an embedded/attached copy).
   * `images` are composer attachments; each becomes an `image` content block
   * (base64 + mime type) after the text/mention blocks, and is echoed into the
   * transcript on the local user message immediately.
   */
  sendPrompt(text: string, mentions?: WorkspaceFileRef[], images?: PendingImageAttachment[]): void;
  /**
   * `!` passthrough: runs one user line in the open session's persistent shell
   * without an LLM turn (no prompt, no tokens). Output streams to the terminal
   * panel via the standing subscription; a compact card is added to the
   * transcript to record the line. No-op with no open session. Rejects if shell
   * sessions are disabled on the server.
   */
  runTerminal(command: string): Promise<void>;
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

/**
 * Safety-net timeout for a prompt turn that dies with NO terminal event —
 * no `stopReason`, no error, and no transport close (a silent stall: the
 * server stopped mid-turn, or the stream ended without a result). The
 * watchdog is ARMED on prompt start and RE-armed on every live activity
 * (chunks, tool calls, usage, terminal output), so a slow-but-alive turn keeps
 * resetting it and only true silence for this whole window trips it. Paused
 * while a permission request is pending (the turn is legitimately blocked on
 * the user). Generous on purpose: local inference can be slow, and a false
 * positive is worse than a late one. Exported so tests can drive it.
 */
export const PROMPT_STALL_TIMEOUT_MS = 120_000;
/**
 * Surfaced (as a normal `prompt_error`, i.e. the failed-turn transcript card +
 * recovery banner) when the stall watchdog trips. English like every other raw
 * runtime error string — the UI localizes only the wrapper/headline.
 */
const STALLED_TURN_MESSAGE = 'The agent stopped responding before the turn completed (no result received).';

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

/**
 * Per-open-session controller bookkeeping. One entry lives in `openSessions`
 * for each concurrently-subscribed session; the entry is what makes the
 * multiplexing correct — `promptInFlight`, `currentTurnId` and the pending
 * permission resolver are PER SESSION, so a background session's in-flight
 * turn or awaiting permission never collides with the focused one's.
 */
interface OpenSessionEntry {
  /** Tears down this session's standing `client.subscribe()` subscription. */
  unsubscribe: () => void;
  /** Fallback grouping id for THIS session's live turn chunks that arrive with no server-assigned `messageId`. */
  currentTurnId: string;
  /** True while this session has a `session/prompt` call in flight — re-prompting the same in-flight session is a no-op, but other sessions may prompt concurrently. */
  promptInFlight: boolean;
  /** Resolver/rejector for THIS session's in-flight `session/request_permission`, if any. */
  permissionResolve: ((optionId: string) => void) | null;
  permissionReject: ((err: Error) => void) | null;
  /** Dead-turn watchdog timer for THIS session's in-flight turn (see `PROMPT_STALL_TIMEOUT_MS`); null when disarmed. */
  stallTimer: ReturnType<typeof setTimeout> | null;
  /**
   * True once THIS turn has reached a terminal outcome — a real settle
   * (`prompt_end`/`prompt_error`) OR the stall watchdog firing. Whichever
   * happens first sets it, so the other becomes a no-op: a late real settle
   * after a stall (or vice-versa) never double-surfaces the turn.
   */
  turnSettled: boolean;
}

export function createAcpWorkspaceController(
  deps: AcpWorkspaceControllerDeps,
  workspaceDispatch: WorkspaceDispatch,
  sessionsDispatch: SessionsDispatch,
): AcpWorkspaceController {
  const cwd = deps.cwd ?? '/';
  const buildClient = deps.createClient ?? ((transport: Transport) => createAcpClient(transport));

  let disposed = false;
  let client: AcpClient | null = null;
  let currentTransport: Transport | null = null;
  let connectPromise: Promise<void> | null = null;

  /**
   * Every currently-subscribed session, keyed by id. Replaces the old single
   * `activeSessionId`/`unsubscribeActive` pair: N sessions can be open and
   * live at once, each with its own subscription and per-session bookkeeping
   * (see `OpenSessionEntry`).
   */
  const openSessions = new Map<SessionId, OpenSessionEntry>();
  /** The session the single-view UI renders (its slice is what `selectFocusedSession` returns); null = the empty-chat view. Always one of `openSessions`' keys, or null. */
  let focusedSessionId: SessionId | null = null;

  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  // ---------------------------------------------------------------------
  // Shared helpers
  // ---------------------------------------------------------------------

  /** Maps a (possibly null, i.e. empty-chat) session id to its slice storage key. */
  function keyOf(sid: SessionId | null): string {
    return sid ?? EMPTY_SESSION_KEY;
  }

  /** Routes a single-session action into `sid`'s slice (or the empty-chat slice when `sid` is null). */
  function sessionDispatch(sid: SessionId | null, action: AcpSessionAction): void {
    sessionsDispatch({ type: 'session_dispatch', key: keyOf(sid), action });
  }

  /** Drops `sid`'s slice from the multiplexed store. */
  function removeSlice(sid: SessionId): void {
    sessionsDispatch({ type: 'session_closed', key: keyOf(sid) });
  }

  /** Re-points the focused/rendered session, keeping the internal pointer, the sessions store's `focusedKey`, and `workspace.activeSessionId` in lockstep. */
  function setFocus(sid: SessionId | null): void {
    focusedSessionId = sid;
    sessionsDispatch({ type: 'session_focused', key: keyOf(sid) });
    workspaceDispatch({ type: 'active_session_changed', sessionId: sid });
  }

  /** Creates a fresh entry for `sid` and opens its standing subscription against `c`. Overwrites any prior entry (e.g. a stale one after reconnect). */
  function trackSubscription(sid: SessionId, c: AcpClient): OpenSessionEntry {
    const entry: OpenSessionEntry = {
      unsubscribe: () => {},
      currentTurnId: '',
      promptInFlight: false,
      permissionResolve: null,
      permissionReject: null,
      stallTimer: null,
      turnSettled: true,
    };
    openSessions.set(sid, entry);
    entry.unsubscribe = c.subscribe(sid, buildSessionHandlers(sid));
    return entry;
  }

  /** Unsubscribes, drops `sid`'s slice, rejects its pending permission, and fire-and-forget `session/close`s it. Leaves every other open session untouched. */
  function teardownSession(sid: SessionId, c: AcpClient | null): void {
    rejectPendingPermission(sid);
    const entry = openSessions.get(sid);
    clearStallTimer(entry);
    entry?.unsubscribe();
    openSessions.delete(sid);
    removeSlice(sid);
    // Fire-and-forget: releasing the session's connection-local state is
    // bookkeeping, not something a caller waits on (see newSession's history).
    if (c) void c.closeSession(sid).catch(() => {});
  }

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

  /** Rejects `sid`'s pending permission (if any) so AcpClient answers the agent with outcome "cancelled" (see client.ts's `handlePermissionRequest` catch) instead of hanging. */
  function rejectPendingPermission(sid: SessionId): void {
    const entry = openSessions.get(sid);
    if (!entry) return;
    const reject = entry.permissionReject;
    entry.permissionResolve = null;
    entry.permissionReject = null;
    reject?.(new Error('acp: pending permission request was superseded'));
  }

  function rejectAllPendingPermissions(): void {
    for (const sid of openSessions.keys()) rejectPendingPermission(sid);
  }

  // ---------------------------------------------------------------------
  // Dead-turn watchdog: a turn must never end in a silently-dead state.
  // ---------------------------------------------------------------------

  function clearStallTimer(entry: OpenSessionEntry | undefined): void {
    if (entry?.stallTimer) {
      clearTimeout(entry.stallTimer);
      entry.stallTimer = null;
    }
  }

  /**
   * (Re)arms `sid`'s dead-turn watchdog while its turn is in flight. If the
   * window elapses with no activity and no settle, the turn is surfaced as a
   * failure (`prompt_error`) rather than left hanging with a stuck typing
   * indicator — covering the "stream ended without a stopReason / server went
   * silent" case that no transport `close` ever signals. No-op once the turn
   * has settled or is no longer in flight.
   */
  function armStallTimer(sid: SessionId): void {
    const entry = openSessions.get(sid);
    if (!entry || !entry.promptInFlight || entry.turnSettled) return;
    clearStallTimer(entry);
    entry.stallTimer = setTimeout(() => {
      const e = openSessions.get(sid);
      if (!e || e.turnSettled || !e.promptInFlight) return;
      e.stallTimer = null;
      e.turnSettled = true;
      e.promptInFlight = false;
      sessionDispatch(sid, { type: 'prompt_error', message: STALLED_TURN_MESSAGE });
    }, PROMPT_STALL_TIMEOUT_MS);
  }

  /** Any live traffic for `sid` proves the turn is alive — reset its watchdog. */
  function noteActivity(sid: SessionId): void {
    const entry = openSessions.get(sid);
    if (entry?.promptInFlight && !entry.turnSettled && entry.stallTimer) armStallTimer(sid);
  }

  /**
   * Handlers routed to `sid`'s slice. Built once per `subscribe()` call (on
   * open/create/resume/reload) rather than per prompt — the standing
   * subscription always wins over a concurrent `prompt()` call's per-turn
   * handlers, so `sendPrompt` deliberately passes none (see the module doc
   * comment). Every dispatch targets `sid`'s OWN slice (not one global one),
   * which is what lets several subscribed sessions accumulate concurrently.
   */
  function buildSessionHandlers(sid: SessionId): SessionEventHandlers {
    // Read the fallback turn id off the live entry each time (not captured):
    // it advances per turn via `sendPrompt`.
    const turnId = () => openSessions.get(sid)?.currentTurnId ?? '';
    // Every streamed update proves the in-flight turn is still alive, so it
    // resets the dead-turn watchdog (see armStallTimer/noteActivity).
    const active = (fn: () => void) => {
      noteActivity(sid);
      fn();
    };
    return {
      onUserMessageChunk: (text, id, image) =>
        active(() => sessionDispatch(sid, { type: 'user_message_chunk', id: id ?? nextId('user'), text, image })),
      onMessageChunk: (text, id, image) =>
        active(() => sessionDispatch(sid, { type: 'message_chunk', id: id ?? turnId(), text, image })),
      onThoughtChunk: (text, id) => active(() => sessionDispatch(sid, { type: 'thought_chunk', id: id ?? turnId(), text })),
      onToolCall: event => active(() => sessionDispatch(sid, { type: 'tool_call', event })),
      onPlan: entries => active(() => sessionDispatch(sid, { type: 'plan', entries })),
      onUsage: usage => active(() => sessionDispatch(sid, { type: 'usage', usage })),
      onTerminalOutput: payload => active(() => sessionDispatch(sid, { type: 'terminal_output', payload })),
      onAvailableCommands: commands => sessionDispatch(sid, { type: 'available_commands', commands }),
      onConfigOptions: configOptions => sessionDispatch(sid, { type: 'config_options', configOptions }),
      onSessionInfo: info =>
        workspaceDispatch({
          type: 'session_upserted',
          session: { sessionId: sid, title: info.title, updatedAt: info.updatedAt },
        }),
      onPermissionRequest: request =>
        new Promise<string>((resolve, reject) => {
          const entry = openSessions.get(sid);
          if (entry) {
            entry.permissionResolve = resolve;
            entry.permissionReject = reject;
            // Pause the watchdog: the turn is legitimately blocked on the user,
            // which can take arbitrarily long — respondPermission re-arms it.
            clearStallTimer(entry);
          }
          sessionDispatch(sid, { type: 'permission_request', request });
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
    promptCapabilities: PromptCapabilities;
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
        // What a session/prompt may carry (image/audio/…) — gates the
        // composer's attach affordances. `{}` when the agent advertises none.
        promptCapabilities: init.agentCapabilities?.promptCapabilities ?? {},
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
      // An empty roster comes back as `sessions: null` on the wire (Go marshals a
      // nil slice as null), so guard the spread rather than throwing "not
      // iterable" on a fresh workspace with no sessions yet.
      collected.push(...(page.sessions ?? []));
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
        promptCapabilities: established.promptCapabilities,
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

  /** Re-binds ONE open session after a reconnect: tries `session/resume` (transcript kept client-side) first, falling back to a full `session/load` replay if resume fails (e.g. the serve process restarted and wiped its in-memory session map). */
  async function restoreOneSession(sid: SessionId, c: AcpClient): Promise<void> {
    try {
      const result = await c.resumeSession(sid, cwd);
      trackSubscription(sid, c);
      // See the equivalent comment in newSession() — session/resume's
      // response carries this session's config options inline too.
      if (result.configOptions) sessionDispatch(sid, { type: 'config_options', configOptions: result.configOptions });
      sessionDispatch(sid, { type: 'connection_resumed' });
      return;
    } catch {
      // Fall through to the full-reload fallback below.
    }
    sessionDispatch(sid, { type: 'session_reset', sessionId: sid });
    // Subscribe BEFORE load: session/load's replay notifications reach the
    // wire before the session/load response resolves (see client.ts /
    // acpsvc/session.go's replayMessages).
    trackSubscription(sid, c);
    try {
      const result = await c.loadSession(sid, cwd);
      if (result.configOptions) sessionDispatch(sid, { type: 'config_options', configOptions: result.configOptions });
      sessionDispatch(sid, { type: 'connection_resumed' });
    } catch (err) {
      sessionDispatch(sid, { type: 'prompt_error', message: `failed to restore session after reconnect: ${errMessage(err)}` });
    }
  }

  /** Re-binds EVERY open session after a reconnect — not just the focused one — so background tabs keep streaming across a drop (workspace-tabs Slice 1). */
  async function restoreOpenSessions(c: AcpClient): Promise<void> {
    // Snapshot ids first: `trackSubscription` rewrites `openSessions` entries
    // as we go, and `disposed` may flip mid-loop.
    for (const sid of [...openSessions.keys()]) {
      if (disposed) return;
      await restoreOneSession(sid, c);
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
        promptCapabilities: established.promptCapabilities,
      });
      await restoreOpenSessions(established.client);
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
    // Every open session's live updates are now stale — flag them all. A drop
    // is owned by the connection_lost UX (the reconnecting banner), NOT the
    // failed-turn surface: settle any in-flight turn here and disarm its
    // watchdog, so the client's imminent pending-call rejection ("transport
    // closed") doesn't ALSO fire a prompt_error card/banner for what is really
    // a transient reconnect.
    for (const sid of openSessions.keys()) {
      const entry = openSessions.get(sid);
      if (entry) {
        clearStallTimer(entry);
        entry.turnSettled = true;
        entry.promptInFlight = false;
      }
      sessionDispatch(sid, { type: 'connection_lost' });
    }
    scheduleReconnectAttempt(0);
  }

  // ---------------------------------------------------------------------
  // Session roster + switching
  // ---------------------------------------------------------------------

  async function refreshSessions(): Promise<void> {
    if (disposed || !client) return;
    await refreshSessionsInternal(client);
  }

  /** Re-points focus to the empty-chat view: resets the empty slice and focuses it (activeSessionId -> null). */
  function focusEmpty(): void {
    sessionDispatch(null, { type: 'session_reset', sessionId: null });
    setFocus(null);
  }

  async function newSession(overrideCwd?: string, agentName?: string | null): Promise<SessionId> {
    if (disposed || !client) throw new Error('acp: workspace controller is not connected');
    const c = client;
    // The workspace root the user picked on the empty chat becomes this
    // session's cwd; absent a pick, the controller's default cwd is used.
    const sessionCwd = overrideCwd && overrideCwd.trim() !== '' ? overrideCwd : cwd;

    // A staged external agent binds the session via the session/new `_meta`
    // extension; a null/blank agentName means the native chain (no `_meta`).
    const meta = agentName && agentName.trim() !== '' ? agentMeta(agentName.trim()) : undefined;

    const previousId = focusedSessionId;
    if (previousId) rejectPendingPermission(previousId);

    // session/new mints the id server-side, so (unlike session/load) we
    // cannot subscribe until the response carries it.
    const result = await runGuarded(() => c.newSession(sessionCwd, [], meta)).catch(err => {
      // Auth failures are already surfaced as `setup_required` by
      // guardAuthRequired (inside runGuarded) — that swaps the whole page to
      // SetupRequiredState, so there's no composer left to show an inline
      // error in. Anything else (e.g. a transient RPC failure) needs to
      // reach the UI some other way: reuse the same inline error banner a
      // failed prompt turn shows (`prompt_error`, see acpSessionState.ts).
      // The session doesn't exist yet, so this lands in the currently-focused
      // slice — the empty-chat slice when creating from a bare `/chat`.
      if (!isAuthRequired(err)) {
        sessionDispatch(focusedSessionId, { type: 'prompt_error', message: errMessage(err) });
      }
      throw err;
    });
    const sid = result.sessionId;

    sessionDispatch(sid, { type: 'session_reset', sessionId: sid });
    trackSubscription(sid, c);
    // session/new's response carries the session's initial config options
    // (model/think/token-limit/hitl-policy) inline — unlike everything else
    // in buildSessionHandlers(), this never arrives as a session/update
    // notification on a fresh session, so it has to be applied here rather
    // than relying on onConfigOptions.
    if (result.configOptions) sessionDispatch(sid, { type: 'config_options', configOptions: result.configOptions });

    // Thread the response `_meta` echo (external agent attribution — see
    // AGENT_META_KEY) into the roster so the sidebar row + transcript label can
    // read it right away, matching what a later session/list would carry.
    workspaceDispatch({ type: 'session_upserted', session: { sessionId: sid, cwd: sessionCwd, _meta: result._meta } });
    setFocus(sid);
    // A brand-new session is trivially "open" — clears any stale
    // not_found/error left by a previous failed openSession() (e.g. the
    // NotFoundState page's "start new session" action).
    workspaceDispatch({ type: 'session_load_succeeded' });

    // Single-view semantics: creating a session closes the previously-focused
    // one, so only the new session stays open (Slice 2 opens tabs additively
    // via openSessionTab instead).
    if (previousId && previousId !== sid) teardownSession(previousId, c);

    return sid;
  }

  /**
   * Adopt an already-running instance+session into a new upstream session. The
   * body mirrors newSession's "register a freshly-minted session" tail, with two
   * deliberate differences: the `session/new` carries the `contenox.adopt`
   * `_meta` (so the runtime attaches a viewer to the running dispatch instead of
   * spawning a fresh agent), and it is ADDITIVE — no teardown of the
   * previously-focused session, since watching a running unit must not close a
   * chat the operator already had open (the tab UI holds several sessions live
   * at once). The response `_meta` echo carries the `controller` verdict; it is
   * threaded into the roster via session_upserted so the chat header can label
   * "Übernommen"/"Beobachten" with no second round trip.
   */
  async function adoptSession(ref: AdoptRef, overrideCwd?: string): Promise<SessionId> {
    if (disposed || !client) throw new Error('acp: workspace controller is not connected');
    const c = client;
    const sessionCwd = overrideCwd && overrideCwd.trim() !== '' ? overrideCwd : cwd;
    const meta = adoptMeta(ref.instanceId, ref.sessionId);

    const result = await runGuarded(() => c.newSession(sessionCwd, [], meta)).catch(err => {
      // Surface a non-auth failure (e.g. the instance is no longer running, or an
      // old serve rejected the adopt) on the same inline banner a failed prompt
      // uses, in whatever slice is currently focused (the empty-chat slice when
      // adopting from a bare /chat). Auth failures already became setup_required.
      if (!isAuthRequired(err)) {
        sessionDispatch(focusedSessionId, { type: 'prompt_error', message: errMessage(err) });
      }
      throw err;
    });
    const sid = result.sessionId;

    sessionDispatch(sid, { type: 'session_reset', sessionId: sid });
    trackSubscription(sid, c);
    if (result.configOptions) sessionDispatch(sid, { type: 'config_options', configOptions: result.configOptions });
    // Thread the response `_meta` echo (contenox.agent + contenox.adopt outcome)
    // into the roster so the header can read the controller verdict + agent name.
    workspaceDispatch({ type: 'session_upserted', session: { sessionId: sid, cwd: sessionCwd, _meta: result._meta } });
    setFocus(sid);
    workspaceDispatch({ type: 'session_load_succeeded' });
    // Additive: leave every previously-open session untouched (no teardown).
    return sid;
  }

  /**
   * Multi-session primitive: subscribe to `id` (BEFORE `session/load` — replay
   * arrives before the response resolves) and focus it, WITHOUT closing any
   * other open session. Opening an already-open session just focuses it.
   */
  async function openSessionTab(id: SessionId): Promise<void> {
    if (disposed || !client) return;
    const c = client;

    if (openSessions.has(id)) {
      // Already subscribed and live — dedup by identity, just render it.
      setFocus(id);
      workspaceDispatch({ type: 'session_load_succeeded' });
      return;
    }

    workspaceDispatch({ type: 'session_load_start' });
    setFocus(id);
    sessionDispatch(id, { type: 'session_reset', sessionId: id });
    // Subscribe BEFORE load: session/load's replay notifications reach the
    // wire before the session/load response resolves (see client.ts's
    // subscribe() doc comment / acpsvc/session.go's replayMessages, and the
    // wire-fact test in client.test.ts's "session/load replay routing" suite).
    trackSubscription(id, c);

    try {
      const result = await runGuarded(() => c.loadSession(id, cwd));
      // See the equivalent comment in newSession() — session/load's response
      // carries this session's config options inline, same as session/new's.
      if (result.configOptions) sessionDispatch(id, { type: 'config_options', configOptions: result.configOptions });
    } catch (err) {
      // Roll this tab back — leave every OTHER open session untouched. If it
      // was focused (it is, we just focused it), fall back to the empty view
      // rather than leaving focus on a session that failed to load.
      teardownSession(id, c);
      if (focusedSessionId === id) focusEmpty();
      if (classifySessionOpenFailure(err) === 'not_found') {
        workspaceDispatch({ type: 'session_load_not_found' });
      } else {
        workspaceDispatch({ type: 'session_load_failed', message: errMessage(err) });
      }
      return;
    }

    workspaceDispatch({ type: 'session_load_succeeded' });
  }

  async function openSession(id: SessionId): Promise<void> {
    if (disposed || !client) return;
    if (focusedSessionId === id) return;
    const c = client;
    // Single-view switch: open `id` as a tab, then close whichever session was
    // focused before — preserving the pre-multiplexing "one open session at a
    // time" behavior. On a failed load, `openSessionTab` has already rolled
    // back to the empty view; closing the previous session matches the old
    // behavior (no session left open after a failed switch).
    const previousId = focusedSessionId;
    await openSessionTab(id);
    if (previousId && previousId !== id) teardownSession(previousId, c);
  }

  function closeSessionTab(id: SessionId): void {
    if (disposed || !openSessions.has(id)) return;
    const wasFocused = focusedSessionId === id;
    teardownSession(id, client);
    if (wasFocused) {
      // Move focus to another still-open session (arbitrary — Slice 2's tab UI
      // owns which neighbor to pick), or the empty view if none remain.
      const next = openSessions.keys().next();
      if (next.done) focusEmpty();
      else setFocus(next.value);
    }
  }

  function focusSession(id: SessionId): void {
    if (disposed || !openSessions.has(id)) return;
    setFocus(id);
  }

  function focusEmptyTab(): void {
    if (disposed) return;
    // Additive: focus the empty surface, leaving every open session subscribed
    // and live (unlike clearActiveSession, which tears the focused one down).
    focusEmpty();
  }

  async function deleteSession(id: SessionId): Promise<void> {
    if (disposed || !client) return;
    const c = client;

    if (openSessions.has(id)) {
      const wasFocused = focusedSessionId === id;
      // Tears down subscription + slice + fire-and-forget session/close. The
      // session/delete call below is what actually removes it server-side.
      teardownSession(id, c);
      if (wasFocused) focusEmpty();
    }

    await runGuarded(() => c.deleteSession(id));
    workspaceDispatch({ type: 'session_removed', sessionId: id });
  }

  function clearActiveSession(): void {
    if (disposed) return;
    const previousId = focusedSessionId;
    if (!previousId) return; // No-op: no session is open.
    // The session itself is NOT deleted, just released connection-side.
    teardownSession(previousId, client);
    focusEmpty();
  }

  // ---------------------------------------------------------------------
  // Turn + permission + config actions on the open session
  // ---------------------------------------------------------------------

  function sendPrompt(text: string, mentions: WorkspaceFileRef[] = [], images: PendingImageAttachment[] = []): void {
    if (disposed || !client || !focusedSessionId || !text.trim()) return;
    const sid = focusedSessionId;
    const entry = openSessions.get(sid);
    // Re-prompting the SAME session while its turn is in flight is a no-op —
    // but per-session tracking means a DIFFERENT session may prompt
    // concurrently (its own turn streams into its own slice).
    if (!entry || entry.promptInFlight) return;
    const c = client;
    // Fallback grouping key for chunks that arrive without a `messageId` — one
    // assistant message per turn, per the client core's documented contract.
    entry.currentTurnId = nextId('assistant');

    // text block + one resource_link per @-mention (reference only — the agent
    // reads via its tools; see promptBlocksFromDraft) + one image block per
    // composer attachment (the ONE embedded content kind — a pasted screenshot
    // has no workspace path to reference).
    const blocks = promptBlocksFromDraft(text, mentions, images);

    // Slash-command text passes through verbatim — acpsvc/commandrunner.go
    // parses `/name args` itself; this layer never inspects it.
    // Local echo: the text chunk plus one image chunk per attachment, all under
    // ONE message id — the sent user message shows its images immediately, in
    // the same shape a session/load replay would rebuild.
    const echoId = nextId('user');
    sessionDispatch(sid, { type: 'user_message_chunk', id: echoId, text });
    for (const image of images) {
      sessionDispatch(sid, {
        type: 'user_message_chunk',
        id: echoId,
        text: '',
        image: { data: image.data, mimeType: image.mimeType },
      });
    }
    sessionDispatch(sid, { type: 'prompt_start' });
    entry.promptInFlight = true;
    entry.turnSettled = false;
    // Arm the dead-turn watchdog so a turn that dies with no terminal event
    // (no stopReason, no error, no transport close) can't leave the UI stuck.
    armStallTimer(sid);

    // No per-turn handlers: the session's standing subscription (set up in
    // newSession()/openSessionTab()) already routes every session/update and
    // session/request_permission for sid — see the module doc comment.
    //
    // The settle handlers below dispatch into `sid`'s OWN slice, so — unlike
    // the pre-multiplexing single-reducer design — they can never bleed into
    // another session's state; a background turn settling correctly updates
    // its own (possibly non-focused) slice. They are gated only on the session
    // still being OPEN (`openSessions.has(sid)`), NOT on `disposed`: the
    // terminal dispatch clears `isPrompting`/`streaming` (see
    // acpSessionState.ts's `endStreaming`) so it must run even after dispose()
    // to avoid a stuck typing indicator — but it must NOT recreate a slice for
    // a session that was closed/cleared mid-turn (its old turn's failure/
    // stopReason must simply be dropped).
    c.prompt(sid, blocks)
      .then(({ stopReason }) => {
        const e = openSessions.get(sid);
        if (!e) return; // Session closed mid-turn — drop the settle.
        clearStallTimer(e);
        if (e.turnSettled) return; // The watchdog already surfaced this turn as stalled.
        e.turnSettled = true;
        e.promptInFlight = false;
        sessionDispatch(sid, { type: 'prompt_end', stopReason });
      })
      .catch((err: unknown) => {
        const e = openSessions.get(sid);
        clearStallTimer(e);
        if (e) e.promptInFlight = false;
        if (!disposed) guardAuthRequired(err);
        if (!e || e.turnSettled) return; // Closed mid-turn, or already surfaced by the watchdog.
        e.turnSettled = true;
        sessionDispatch(sid, { type: 'prompt_error', message: errMessage(err) });
      });
  }

  async function runTerminal(command: string): Promise<void> {
    if (disposed || !client || !focusedSessionId) return;
    const sid = focusedSessionId;
    const res = await runGuarded(() => client!.runTerminal(sid, command));
    // Record the line in the transcript unless the session was closed while the
    // call was in flight — the live stream already reached the panel via the
    // standing subscription.
    if (openSessions.has(sid)) {
      sessionDispatch(sid, { type: 'terminal_card', id: nextId('term'), command, output: res.output ?? '' });
    }
  }

  function respondPermission(optionId: string): void {
    if (!focusedSessionId) return;
    const entry = openSessions.get(focusedSessionId);
    const resolve = entry?.permissionResolve;
    if (entry) {
      entry.permissionResolve = null;
      entry.permissionReject = null;
    }
    if (!resolve) return;
    sessionDispatch(focusedSessionId, { type: 'permission_resolved' });
    resolve(optionId);
    // The turn is unblocked and will resume streaming — re-arm the watchdog
    // that onPermissionRequest paused.
    armStallTimer(focusedSessionId);
  }

  function cancel(): void {
    if (!client || !focusedSessionId) return;
    client.cancel(focusedSessionId);
  }

  async function setConfigOption(configId: string, value: SessionConfigOptionValue): Promise<void> {
    if (disposed || !client || !focusedSessionId) return;
    const sid = focusedSessionId;
    const result = await runGuarded(() => client!.setConfigOption(sid, configId, value));
    sessionDispatch(sid, { type: 'config_options', configOptions: result.configOptions });
  }

  async function applyConfigOptions(
    options: Array<{ configId: string; value: SessionConfigOptionValue }>,
  ): Promise<void> {
    if (disposed || !client || !focusedSessionId || options.length === 0) return;
    const sid = focusedSessionId;
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
      if (openSessions.has(sid)) sessionDispatch(sid, { type: 'prompt_error', message: errMessage(err) });
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
    rejectAllPendingPermissions();
    // Disarm every dead-turn watchdog so none fires after teardown; the
    // in-flight prompt's own settle handler still clears state (BUG 4a).
    for (const entry of openSessions.values()) clearStallTimer(entry);
    // Deliberately leaves `openSessions` populated: an in-flight prompt's
    // settle handler still needs its entry to dispatch prompt_end (see
    // sendPrompt / BUG 4a) so the typing indicator can't get stuck.
    client?.close();
    client = null;
    currentTransport = null;
  }

  return {
    connect,
    refreshSessions,
    newSession,
    adoptSession,
    openSession,
    openSessionTab,
    closeSessionTab,
    focusSession,
    focusEmptyTab,
    deleteSession,
    clearActiveSession,
    sendPrompt,
    runTerminal,
    respondPermission,
    cancel,
    setConfigOption,
    applyConfigOptions,
    reconnect,
    dispose,
  };
}
