/**
 * `AcpClient`: a JSON-RPC 2.0 client for the Agent Client Protocol, speaking
 * over a pluggable `Transport` (see transport.ts). It knows the ACP method
 * names and wire shapes (see types.ts) and nothing else — no React, no
 * contenox types, no assumption about which agent is on the other end.
 *
 * Why hand-rolled instead of the official `@agentclientprotocol/sdk`: see the
 * decision note in the accompanying report. Short version — the official SDK
 * is real and its `experimental/ws-client` entry point does work in a browser,
 * but its ergonomic API (`ClientSideConnection` / `client()` app builder) is
 * built around a much broader, still-changing surface (NES suggestions,
 * providers, multi-connection SSE routing) that libacp does not implement, its
 * primary constructor is marked deprecated in favor of that broader app
 * framework, and it pulls in a new peer dependency (zod) for no benefit here:
 * the wire-level JSON-RPC framing/dispatch this task specifies (raw text
 * frames, explicit id correlation, a handler-object per prompt() call) is not
 * something the SDK saves us from writing — its Stream abstraction operates on
 * parsed objects, not text frames, so wrapping it would mean building this
 * same routing layer again on top of an adapter. A thin, fully-owned client
 * matching libacp's actual (narrower) method set is simpler and has a smaller
 * trust boundary.
 */
import type { Transport } from './transport';
import {
  ACP_PROTOCOL_VERSION,
  JSON_RPC_ERROR_CODES,
  type AvailableCommand,
  type ClientCapabilities,
  type ContentBlock,
  type Implementation,
  type InitializeResponse,
  type JsonRpcErrorObject,
  type JsonRpcId,
  type JsonRpcNotification,
  type JsonRpcRequest,
  type JsonRpcResponse,
  type ListSessionsResponse,
  type LoadSessionResponse,
  type McpServer,
  type NewSessionResponse,
  type PlanEntry,
  type RequestPermissionRequest,
  type ResumeSessionResponse,
  type SessionConfigOptionValue,
  type SessionId,
  type SessionNotification,
  type SessionUpdate,
  type SetSessionConfigOptionResponse,
  type StopReason,
  type TokenUsage,
  type ToolCallContent,
  type ToolCallLocation,
  type ToolCallStatus,
  type ToolKind,
  type UsageCost,
} from './types';

/** A JSON-RPC error surfaced back to a caller as a thrown `Error`. */
export class AcpError extends Error {
  readonly code: number;
  readonly data?: unknown;

  constructor(errorObject: JsonRpcErrorObject) {
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

/**
 * Handlers for one `prompt()` turn. All are optional; an update kind with no
 * handler is silently dropped (see the routing table in client.ts's
 * `routeSessionUpdate`).
 */
export interface PromptHandlers {
  onMessageChunk?: (text: string, messageId?: string) => void;
  onThoughtChunk?: (text: string, messageId?: string) => void;
  onToolCall?: (event: ToolCallEvent) => void;
  onPlan?: (entries: PlanEntry[]) => void;
  onUsage?: (usage: UsageEvent) => void;
  onAvailableCommands?: (commands: AvailableCommand[]) => void;
  /**
   * Answers the server's `session/request_permission`. Resolve with the
   * `optionId` of the chosen `PermissionOption`. If the returned promise
   * rejects (e.g. the caller tore down without a choice), the client responds
   * with outcome `"cancelled"` so the agent is never left hanging.
   */
  onPermissionRequest?: (request: RequestPermissionRequest) => Promise<string>;
}

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

export class AcpClient {
  private readonly transport: Transport;
  private nextId = 1;
  private readonly pending = new Map<number, PendingCall>();
  private readonly activePrompts = new Map<SessionId, PromptHandlers>();
  private closed = false;

  constructor(transport: Transport) {
    this.transport = transport;
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
      void this.handleIncomingRequest(msg as unknown as JsonRpcRequest);
    } else if (hasMethod) {
      this.handleNotification(msg as unknown as JsonRpcNotification);
    } else if (hasId) {
      this.handleResponse(msg as unknown as JsonRpcResponse);
    }
  }

  private handleResponse(resp: JsonRpcResponse): void {
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
      const frame: JsonRpcRequest = { jsonrpc: '2.0', id, method, params };
      try {
        this.transport.send(JSON.stringify(frame));
      } catch (err) {
        this.pending.delete(id);
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    });
  }

  private notify(method: string, params?: unknown): void {
    const frame: JsonRpcNotification = { jsonrpc: '2.0', method, params };
    this.transport.send(JSON.stringify(frame));
  }

  private respondResult(id: JsonRpcId, result: unknown): void {
    const frame: JsonRpcResponse = { jsonrpc: '2.0', id, result };
    this.transport.send(JSON.stringify(frame));
  }

  private respondError(id: JsonRpcId, code: number, message: string): void {
    const frame: JsonRpcResponse = { jsonrpc: '2.0', id, error: { code, message } };
    this.transport.send(JSON.stringify(frame));
  }

  // -------------------------------------------------------------------------
  // Server -> client requests: session/request_permission, fs/*, terminal/*
  // -------------------------------------------------------------------------

  private async handleIncomingRequest(req: JsonRpcRequest): Promise<void> {
    if (req.method === 'session/request_permission') {
      await this.handlePermissionRequest(req);
      return;
    }
    if (UNSUPPORTED_CLIENT_METHODS.has(req.method)) {
      // This client never advertises fs/terminal capabilities in initialize(),
      // so a conformant agent will not call these — but answer instead of
      // hanging if one does anyway.
      this.respondError(
        req.id,
        JSON_RPC_ERROR_CODES.methodNotFound,
        `not supported by this client: ${req.method}`,
      );
      return;
    }
    this.respondError(req.id, JSON_RPC_ERROR_CODES.methodNotFound, `method not found: ${req.method}`);
  }

  private async handlePermissionRequest(req: JsonRpcRequest): Promise<void> {
    const params = req.params as RequestPermissionRequest;
    const handlers = params && this.activePrompts.get(params.sessionId);
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

  private handleNotification(n: JsonRpcNotification): void {
    if (n.method !== 'session/update') return;
    const params = n.params as SessionNotification;
    if (!params) return;
    const handlers = this.activePrompts.get(params.sessionId);
    if (!handlers) return;
    this.routeSessionUpdate(params.update, handlers);
  }

  /**
   * Handler -> sessionUpdate-kind routing table:
   *   agent_message_chunk        -> onMessageChunk
   *   agent_thought_chunk        -> onThoughtChunk
   *   tool_call / tool_call_update -> onToolCall
   *   plan                       -> onPlan
   *   usage_update                -> onUsage
   *   available_commands_update  -> onAvailableCommands
   *   user_message_chunk, current_mode_update, config_option_update,
   *   session_info_update        -> no handler in this slice; dropped
   */
  private routeSessionUpdate(update: SessionUpdate, handlers: PromptHandlers): void {
    switch (update.sessionUpdate) {
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
      case 'user_message_chunk':
      case 'current_mode_update':
      case 'config_option_update':
      case 'session_info_update':
        return;
    }
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  async initialize(
    clientCapabilities: ClientCapabilities = {},
    clientInfo?: Implementation,
  ): Promise<InitializeResponse> {
    return this.call<InitializeResponse>('initialize', {
      protocolVersion: ACP_PROTOCOL_VERSION,
      clientCapabilities,
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
   */
  async prompt(
    sessionId: SessionId,
    blocks: ContentBlock[],
    handlers: PromptHandlers = {},
  ): Promise<{ stopReason: StopReason; usage?: TokenUsage }> {
    this.activePrompts.set(sessionId, handlers);
    try {
      return await this.call<{ stopReason: StopReason; usage?: TokenUsage }>('session/prompt', {
        sessionId,
        prompt: blocks,
      });
    } finally {
      this.activePrompts.delete(sessionId);
    }
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
