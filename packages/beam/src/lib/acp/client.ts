/**
 * `AcpClient`: a JSON-RPC 2.0 client for the Agent Client Protocol, speaking
 * over a pluggable `Transport` (see transport.ts). It knows the ACP method
 * names and wire shapes (see types.ts) and nothing else — no React, no
 * contenox types, no assumption about which agent is on the other end.
 *
 * ## Stage 2.5: built on `@agentclientprotocol/sdk`, with the dispatch loop
 * staying local — here is the exact split and why.
 *
 * This client leans on the official `@agentclientprotocol/sdk` as a
 * COMPILE-TIME dependency only (devDependency, pinned exactly — the
 * `experimental/*` entry points involved are not semver-covered). Importing
 * any SDK *runtime* code pulls its entire Zod-built schema surface into the
 * bundle (+25 kB gzip measured on this route's chunk, with no narrower
 * subpath available), so all imports from it are `import type`. What the SDK
 * provides here:
 *  - Wire-shape *types* — types.ts re-exports the SDK's generated schema
 *    types wherever they are byte-identical to libacp's subset (see that
 *    file's header), and `AcpError` is typed against the SDK's
 *    `ErrorResponse`. Drift between this client and the official schema
 *    surfaces at typecheck time, at zero runtime cost.
 *  - The framing itself: `Transport`/`WebSocketTransport` (transport.ts) send
 *    exactly one JSON-RPC message per WebSocket TEXT frame, which is what the
 *    SDK's own `experimental/ws-client` (`createWebSocketStream`) does too —
 *    verified live against `runtime/contenoxcli/acp_ws.go` using the SDK's
 *    `client()` app talking `initialize`/`session/new`/`session/list` over
 *    that entry point (see the Stage 2.5 verification script; not part of
 *    this module because production code never needs to construct its own
 *    WebSocket — `WebSocketTransport` already owns that).
 *
 * What stays hand-rolled, and why — this was NOT a stylistic choice, it is
 * forced by two constraints verified against the SDK's actual (not just
 * documented) behavior:
 *
 *  1. **Synchronous send-on-call.** `acpWorkspaceController.ts` (Stage 2,
 *     unchanged here) and its test suite depend on `transport.send(...)`
 *     happening synchronously inside `prompt()`/`initialize()`/etc. — e.g.
 *     `acpWorkspaceController.test.ts`'s "sends session/prompt..." test reads
 *     `transport.lastSent()` immediately after calling `sendPrompt()`, with no
 *     `await` in between. The SDK's `Connection.sendMessage()` (reached via
 *     `ClientApp`/`client()`, or the deprecated `ClientSideConnection` — both
 *     wrap the same `Connection`) queues every write through
 *     `this.writeQueue.then(...)`, and separately its public `Stream` contract
 *     (`{ readable, writable }`, also what `createWebSocketStream` returns) is
 *     built on WHATWG `WritableStream`, whose sink callback never runs
 *     synchronously even on an empty, non-backpressured queue (verified with a
 *     standalone repro: a synchronous `WritableStream` sink still only fires
 *     after a microtask tick). Routing outbound sends through either would
 *     silently break that test's synchronicity assumption.
 *  2. **libacp's real traffic is more lenient than the SDK's generated
 *     schema.** The SDK's `ClientApp`/`client()` validates every built-in
 *     method's params against its generated Zod schema before your handler
 *     ever sees them — including a session-update router that is *always*
 *     registered internally (there's no way to opt a built-in method out of
 *     it, even by passing your own parser to `onRequest`/`onNotification`).
 *     That schema marks fields libacp does not always send as required — e.g.
 *     `tool_call.title` — and rejects them with `invalidParams`, silently
 *     dropping the notification instead of routing it. Verified with a
 *     standalone repro feeding the SDK's `client()` app a `tool_call` update
 *     with no `title` (a shape this client's own tests, and real
 *     `tool_call_update` traffic, both send): the registered handler never
 *     fires; the SDK logs a validation error instead.
 *
 * Given both, the actual message loop below — id correlation, the
 * subscribe()-wins-over-per-prompt-handlers routing, session/update
 * dispatch, and JSON-RPC error construction — stays exactly what it was: a
 * thin, fully-owned engine matching libacp's real (narrower, more lenient)
 * wire contract. That is the same conclusion the original hand-rolled-only
 * version reached, now with reproducible evidence instead of a prediction.
 *
 * Revisit triggers for a full engine swap (re-evaluate, don't assume): the
 * SDK exports its low-level `Connection`, OR its built-in method validation
 * becomes tolerant of libacp's actual traffic, OR the remote-transport RFD
 * stabilizes and `experimental/ws-client` graduates.
 */
import type { ErrorResponse } from '@agentclientprotocol/sdk';
import type { AcpCapabilityProvider } from './clientFactory';
import type { Transport } from './transport';
import {
  ACP_PROTOCOL_VERSION,
  JSON_RPC_ERROR_CODES,
  type AvailableCommand,
  type ClientCapabilities,
  type ContentBlock,
  type Implementation,
  type InitializeResponse,
  type ListSessionsResponse,
  type LoadSessionResponse,
  type McpServer,
  type NewSessionResponse,
  type PlanEntry,
  type RequestPermissionRequest,
  type ResumeSessionResponse,
  type SessionConfigOption,
  type SessionConfigOptionValue,
  type SessionId,
  type SessionNotification,
  type SessionUpdate,
  type SetSessionConfigOptionResponse,
  type StopReason,
  type ToolCallContent,
  type ToolCallLocation,
  type ToolCallStatus,
  type ToolKind,
  type UsageCost,
} from './types';

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 envelope — private to this module. `AcpClient` mints its own
// numeric ids and correlates them itself (see the header comment for why this
// isn't the SDK's `Connection`); these are just the wire shapes it reads/writes.
// ---------------------------------------------------------------------------

type JsonRpcId = number | string | null;

interface JsonRpcRequestFrame {
  jsonrpc: '2.0';
  id: JsonRpcId;
  method: string;
  params?: unknown;
}

interface JsonRpcNotificationFrame {
  jsonrpc: '2.0';
  method: string;
  params?: unknown;
}

interface JsonRpcResponseFrame {
  jsonrpc: '2.0';
  id: JsonRpcId;
  result?: unknown;
  error?: ErrorResponse;
}

/**
 * A JSON-RPC error surfaced back to a caller as a thrown `Error`. `errorObject`
 * is typed as the SDK's `ErrorResponse` — the same `{code, message, data?}`
 * shape as before, now sourced from `@agentclientprotocol/sdk` rather than a
 * locally-declared duplicate (see this module's header comment).
 */
export class AcpError extends Error {
  readonly code: number;
  readonly data?: unknown;

  constructor(errorObject: ErrorResponse) {
    super(errorObject.message);
    this.name = 'AcpError';
    this.code = errorObject.code;
    this.data = errorObject.data;
  }
}

/** Normalized view of `tool_call` / `tool_call_update`, passed to `onToolCall`. */
export interface ToolCallEvent {
  /** Which update variant produced this event — a card is created on `tool_call`, advanced on `tool_call_update`. */
  updateKind: 'tool_call' | 'tool_call_update';
  toolCallId: string;
  title?: string;
  kind?: ToolKind;
  status?: ToolCallStatus;
  content?: ToolCallContent[];
  locations?: ToolCallLocation[];
  rawInput?: unknown;
  rawOutput?: unknown;
}

export interface UsageEvent {
  used: number;
  size: number;
  cost?: UsageCost;
}

/** Normalized view of `session_info_update`, passed to `onSessionInfo`. */
export interface SessionInfoEvent {
  title?: string;
  updatedAt?: string;
}

/**
 * Handlers for `session/update` notifications and `session/request_permission`
 * requests. All are optional; an update kind with no handler is silently
 * dropped (see the routing table in client.ts's `routeSessionUpdate`).
 *
 * Used both as the per-turn handlers passed to `prompt()` and as the
 * session-scoped handlers passed to `subscribe()` — see `subscribe()`'s doc
 * comment for how the two interact when both are registered for the same
 * session at once.
 */
export interface SessionEventHandlers {
  onMessageChunk?: (text: string, messageId?: string) => void;
  onThoughtChunk?: (text: string, messageId?: string) => void;
  /** `user_message_chunk` — mainly seen during `session/load` history replay. */
  onUserMessageChunk?: (text: string, messageId?: string) => void;
  onToolCall?: (event: ToolCallEvent) => void;
  onPlan?: (entries: PlanEntry[]) => void;
  onUsage?: (usage: UsageEvent) => void;
  onAvailableCommands?: (commands: AvailableCommand[]) => void;
  /** `config_option_update` — sent after a slash command changes session config. */
  onConfigOptions?: (configOptions: SessionConfigOption[]) => void;
  /** `session_info_update` — sent after a prompt turn resolves. */
  onSessionInfo?: (info: SessionInfoEvent) => void;
  /**
   * Answers the server's `session/request_permission`. Resolve with the
   * `optionId` of the chosen `PermissionOption`. If the returned promise
   * rejects (e.g. the caller tore down without a choice), the client responds
   * with outcome `"cancelled"` so the agent is never left hanging.
   */
  onPermissionRequest?: (request: RequestPermissionRequest) => Promise<string>;
}

/** @deprecated Renamed to {@link SessionEventHandlers}; kept as an alias for source compatibility. */
export type PromptHandlers = SessionEventHandlers;

interface PendingCall {
  resolve: (value: unknown) => void;
  reject: (err: Error) => void;
}

const UNSUPPORTED_CLIENT_METHODS = new Set([
  'fs/read_text_file',
  'fs/write_text_file',
  'terminal/create',
  'terminal/output',
  'terminal/wait_for_exit',
  'terminal/kill',
  'terminal/release',
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

/** Options accepted by the `AcpClient` constructor (and `createAcpClient`, see clientFactory.ts). */
export interface AcpClientOptions {
  /** Supplies terminal/fs handling and extra `initialize()` capabilities. See clientFactory.ts. */
  capabilities?: AcpCapabilityProvider;
}

export class AcpClient {
  private readonly transport: Transport;
  private nextId = 1;
  private readonly pending = new Map<number, PendingCall>();
  private readonly activePrompts = new Map<SessionId, SessionEventHandlers>();
  /**
   * Session-scoped subscriptions registered via `subscribe()`. Unlike
   * `activePrompts`, entries here live independent of any in-flight
   * `prompt()` call — this is what lets a caller observe out-of-turn traffic
   * (session/load replay, the post-session/new banner + available commands,
   * config_option_update after a slash command, post-turn session_info_update
   * — see acpsvc/session.go, commands.go, prompt.go for where the runtime
   * emits these).
   */
  private readonly subscriptions = new Map<SessionId, SessionEventHandlers>();
  private readonly capabilityProvider?: AcpCapabilityProvider;
  private closed = false;

  constructor(transport: Transport, opts: AcpClientOptions = {}) {
    this.transport = transport;
    this.capabilityProvider = opts.capabilities;
    this.transport.onMessage((text) => this.handleMessage(text));
    this.transport.onClose((err) => this.handleClose(err));
  }

  // -------------------------------------------------------------------------
  // JSON-RPC plumbing
  // -------------------------------------------------------------------------

  private handleClose(err?: Error): void {
    this.closed = true;
    const failure = err ?? new Error('acp: transport closed');
    for (const call of this.pending.values()) {
      call.reject(failure);
    }
    this.pending.clear();
    this.activePrompts.clear();
    this.subscriptions.clear();
    this.capabilityProvider?.dispose?.();
  }

  private handleMessage(text: string): void {
    let msg: unknown;
    try {
      msg = JSON.parse(text);
    } catch {
      return; // Malformed frame: nothing we can correlate it to.
    }
    if (!isRecord(msg)) return;

    const hasMethod = typeof msg.method === 'string';
    const hasId = Object.prototype.hasOwnProperty.call(msg, 'id') && msg.id !== undefined;

    if (hasMethod && hasId) {
      void this.handleIncomingRequest(msg as unknown as JsonRpcRequestFrame);
    } else if (hasMethod) {
      this.handleNotification(msg as unknown as JsonRpcNotificationFrame);
    } else if (hasId) {
      this.handleResponse(msg as unknown as JsonRpcResponseFrame);
    }
  }

  private handleResponse(resp: JsonRpcResponseFrame): void {
    if (typeof resp.id !== 'number') return; // We only ever mint numeric ids.
    const call = this.pending.get(resp.id);
    if (!call) return;
    this.pending.delete(resp.id);
    if (resp.error) {
      call.reject(new AcpError(resp.error));
    } else {
      call.resolve(resp.result);
    }
  }

  private call<T>(method: string, params?: unknown): Promise<T> {
    if (this.closed) {
      return Promise.reject(new Error('acp: client is closed'));
    }
    const id = this.nextId++;
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (value: unknown) => void, reject });
      const frame: JsonRpcRequestFrame = { jsonrpc: '2.0', id, method, params };
      try {
        this.transport.send(JSON.stringify(frame));
      } catch (err) {
        this.pending.delete(id);
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    });
  }

  private notify(method: string, params?: unknown): void {
    const frame: JsonRpcNotificationFrame = { jsonrpc: '2.0', method, params };
    this.transport.send(JSON.stringify(frame));
  }

  private respondResult(id: JsonRpcId, result: unknown): void {
    const frame: JsonRpcResponseFrame = { jsonrpc: '2.0', id, result };
    this.transport.send(JSON.stringify(frame));
  }

  private respondError(id: JsonRpcId, code: number, message: string): void {
    const frame: JsonRpcResponseFrame = { jsonrpc: '2.0', id, error: { code, message } };
    this.transport.send(JSON.stringify(frame));
  }

  // -------------------------------------------------------------------------
  // Server -> client requests: session/request_permission, fs/*, terminal/*
  // -------------------------------------------------------------------------

  private async handleIncomingRequest(req: JsonRpcRequestFrame): Promise<void> {
    if (req.method === 'session/request_permission') {
      await this.handlePermissionRequest(req);
      return;
    }
    if (UNSUPPORTED_CLIENT_METHODS.has(req.method)) {
      if (this.capabilityProvider) {
        try {
          const result = await this.capabilityProvider.handleRequest(req.method, req.params);
          this.respondResult(req.id, result);
          return;
        } catch {
          // The provider declined (or threw) — fall through to the same
          // refusal a client with no provider would send, rather than
          // leaking internal error detail or hanging the agent.
        }
      }
      // With no capability provider (or one that declined), this client never
      // advertises fs/terminal capabilities in initialize(), so a conformant
      // agent will not call these — but answer instead of hanging if one does
      // anyway.
      this.respondError(
        req.id,
        JSON_RPC_ERROR_CODES.methodNotFound,
        `not supported by this client: ${req.method}`,
      );
      return;
    }
    this.respondError(req.id, JSON_RPC_ERROR_CODES.methodNotFound, `method not found: ${req.method}`);
  }

  private async handlePermissionRequest(req: JsonRpcRequestFrame): Promise<void> {
    const params = req.params as RequestPermissionRequest;
    const handlers = params && this.handlersFor(params.sessionId);
    if (!handlers?.onPermissionRequest) {
      this.respondError(
        req.id,
        JSON_RPC_ERROR_CODES.internalError,
        `no permission handler registered for session ${params?.sessionId ?? '<unknown>'}`,
      );
      return;
    }
    try {
      const optionId = await handlers.onPermissionRequest(params);
      this.respondResult(req.id, { outcome: { outcome: 'selected', optionId } });
    } catch {
      // The caller's handler failed or never resolved a choice (e.g. the
      // dialog was torn down); tell the agent the request was cancelled
      // rather than leaving it hanging or sending a raw RPC error.
      this.respondResult(req.id, { outcome: { outcome: 'cancelled' } });
    }
  }

  // -------------------------------------------------------------------------
  // Notifications: session/update
  // -------------------------------------------------------------------------

  private handleNotification(n: JsonRpcNotificationFrame): void {
    if (n.method !== 'session/update') return;
    const params = n.params as SessionNotification;
    if (!params) return;
    const handlers = this.handlersFor(params.sessionId);
    if (!handlers) return;
    this.routeSessionUpdate(params.update, handlers);
  }

  /**
   * Resolves which handler set receives `session/update` notifications and
   * `session/request_permission` requests for `sessionId`. A `subscribe()`
   * subscription, if one is active, ALWAYS wins over the current `prompt()`
   * call's per-turn handlers for that session — the two are never both
   * invoked for the same event (see `subscribe()`'s doc comment). This is
   * what lets a subscription observe traffic whether or not a prompt is in
   * flight, without double-delivering anything to a concurrent prompt().
   */
  private handlersFor(sessionId: SessionId): SessionEventHandlers | undefined {
    return this.subscriptions.get(sessionId) ?? this.activePrompts.get(sessionId);
  }

  /**
   * Handler -> sessionUpdate-kind routing table:
   *   user_message_chunk         -> onUserMessageChunk
   *   agent_message_chunk        -> onMessageChunk
   *   agent_thought_chunk        -> onThoughtChunk
   *   tool_call / tool_call_update -> onToolCall
   *   plan                       -> onPlan
   *   usage_update                -> onUsage
   *   available_commands_update  -> onAvailableCommands
   *   config_option_update       -> onConfigOptions
   *   session_info_update        -> onSessionInfo
   *   current_mode_update        -> no handler in this slice; dropped
   */
  private routeSessionUpdate(update: SessionUpdate, handlers: SessionEventHandlers): void {
    switch (update.sessionUpdate) {
      case 'user_message_chunk':
        handlers.onUserMessageChunk?.(update.content.text ?? '', update.messageId);
        return;
      case 'agent_message_chunk':
        handlers.onMessageChunk?.(update.content.text ?? '', update.messageId);
        return;
      case 'agent_thought_chunk':
        handlers.onThoughtChunk?.(update.content.text ?? '', update.messageId);
        return;
      case 'tool_call':
      case 'tool_call_update':
        handlers.onToolCall?.({
          updateKind: update.sessionUpdate,
          toolCallId: update.toolCallId,
          title: update.title,
          kind: update.kind,
          status: update.status,
          content: update.content,
          locations: update.locations,
          rawInput: update.rawInput,
          rawOutput: update.rawOutput,
        });
        return;
      case 'plan':
        handlers.onPlan?.(update.entries);
        return;
      case 'usage_update':
        handlers.onUsage?.({ used: update.used, size: update.size, cost: update.cost });
        return;
      case 'available_commands_update':
        handlers.onAvailableCommands?.(update.availableCommands);
        return;
      case 'config_option_update':
        handlers.onConfigOptions?.(update.configOptions);
        return;
      case 'session_info_update':
        handlers.onSessionInfo?.({ title: update.title, updatedAt: update.updatedAt });
        return;
      case 'current_mode_update':
        return;
    }
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  /**
   * `clientCapabilities` is merged with the registered capability provider's
   * `capabilities()` (if any) before being sent — explicit values passed here
   * take precedence over the provider's per top-level key (`fs`, `terminal`,
   * `session`, `auth`). With no provider, this is a no-op merge and the
   * request is byte-identical to passing `clientCapabilities` straight
   * through.
   */
  async initialize(
    clientCapabilities: ClientCapabilities = {},
    clientInfo?: Implementation,
  ): Promise<InitializeResponse> {
    const providerCapabilities = this.capabilityProvider?.capabilities() ?? {};
    return this.call<InitializeResponse>('initialize', {
      protocolVersion: ACP_PROTOCOL_VERSION,
      clientCapabilities: { ...providerCapabilities, ...clientCapabilities },
      clientInfo,
    });
  }

  async newSession(cwd: string, mcpServers: McpServer[] = []): Promise<NewSessionResponse> {
    return this.call<NewSessionResponse>('session/new', { cwd, mcpServers });
  }

  async loadSession(
    sessionId: SessionId,
    cwd: string,
    mcpServers: McpServer[] = [],
  ): Promise<LoadSessionResponse> {
    return this.call<LoadSessionResponse>('session/load', { sessionId, cwd, mcpServers });
  }

  async resumeSession(
    sessionId: SessionId,
    cwd: string,
    mcpServers?: McpServer[],
  ): Promise<ResumeSessionResponse> {
    return this.call<ResumeSessionResponse>('session/resume', { sessionId, cwd, mcpServers });
  }

  async deleteSession(sessionId: SessionId): Promise<void> {
    await this.call<Record<string, never>>('session/delete', { sessionId });
  }

  async closeSession(sessionId: SessionId): Promise<void> {
    await this.call<Record<string, never>>('session/close', { sessionId });
  }

  async listSessions(cursor?: string): Promise<ListSessionsResponse> {
    return this.call<ListSessionsResponse>('session/list', cursor ? { cursor } : {});
  }

  async setConfigOption(
    sessionId: SessionId,
    configId: string,
    value: SessionConfigOptionValue,
  ): Promise<SetSessionConfigOptionResponse> {
    const isBool = typeof value === 'boolean';
    return this.call<SetSessionConfigOptionResponse>('session/set_config_option', {
      sessionId,
      configId,
      type: isBool ? 'boolean' : undefined,
      value,
    });
  }

  /**
   * Runs one prompt turn. Registers `handlers` for the duration of the call so
   * `session/update` notifications and `session/request_permission` requests
   * for `sessionId` route to them, then sends `session/prompt` and resolves
   * once the agent returns a `stopReason` (or rejects on a JSON-RPC error).
   *
   * If a `subscribe()` subscription is active for `sessionId`, it takes
   * priority over these per-turn handlers for the duration of the call — see
   * `handlersFor()`. `handlers` still take effect the moment the subscription
   * is torn down (its unsubscribe function called) while the prompt is still
   * in flight.
   */
  async prompt(
    sessionId: SessionId,
    blocks: ContentBlock[],
    handlers: SessionEventHandlers = {},
  ): Promise<{ stopReason: StopReason }> {
    this.activePrompts.set(sessionId, handlers);
    try {
      return await this.call<{ stopReason: StopReason }>('session/prompt', {
        sessionId,
        prompt: blocks,
      });
    } finally {
      this.activePrompts.delete(sessionId);
    }
  }

  /**
   * Subscribes to `session/update` notifications and `session/request_permission`
   * requests for `sessionId`, independent of any `prompt()` call. This is how a
   * caller observes traffic the runtime emits outside a prompt turn: the
   * `session/load` history replay, the post-`session/new`/`session/load` banner
   * + `available_commands_update` + initial `usage_update`, `config_option_update`
   * after a slash command, and the post-turn `session_info_update` (see
   * acpsvc/session.go, acpsvc/commands.go, acpsvc/prompt.go in the runtime for
   * exactly where each of these is emitted).
   *
   * Returns an unsubscribe function. Calling it removes this subscription only
   * if it is still the active one for `sessionId` (a no-op if a newer
   * `subscribe()` call, or another unsubscribe, already replaced/removed it),
   * so it is always safe to call more than once.
   *
   * Routing precedence while this subscription is active: it receives ALL
   * `session/update` / `session/request_permission` traffic for `sessionId`,
   * including traffic that would otherwise go to a concurrent `prompt()`
   * call's per-turn handlers — the subscription wins and the same event is
   * never delivered to both (see `handlersFor()`).
   */
  subscribe(sessionId: SessionId, handlers: SessionEventHandlers): () => void {
    this.subscriptions.set(sessionId, handlers);
    return () => {
      if (this.subscriptions.get(sessionId) === handlers) {
        this.subscriptions.delete(sessionId);
      }
    };
  }

  /** Sends `session/cancel` — a notification, not a request; fire-and-forget. */
  cancel(sessionId: SessionId): void {
    this.notify('session/cancel', { sessionId });
  }

  /** Closes the underlying transport. */
  close(): void {
    this.transport.close();
  }
}
