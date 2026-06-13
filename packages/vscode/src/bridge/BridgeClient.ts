import { Readable, Writable } from "node:stream";
import { CancellationToken, Disposable } from "vscode";
import { JsonRpcFramer } from "./JsonRpcFramer";
import {
  ConfigSnapshot,
  ApprovalRequestedEvent,
  ApprovalRespondParams,
  ApprovalRespondResult,
  AutocompleteParams,
  AutocompleteResult,
  ChatCancelParams,
  ChatCancelResult,
  ChatDeltaEvent,
  ChatLifecycleEvent,
  ChatSendParams,
  ChatSendResult,
  HealthResult,
  HitlDecisionEvent,
  InitializeParams,
  InitializeResult,
  JsonRpcError,
  JsonRpcID,
  JsonRpcNotification,
  JsonRpcRequest,
  JsonRpcResponse,
  ListCommandsResult,
  ListHitlPoliciesResult,
  ListMCPServersResult,
  ListModelsParams,
  ListModelsResult,
  ListProvidersResult,
  RequestPermissionParams,
  RequestPermissionResponse,
  SessionCreateParams,
  SessionDeleteParams,
  SessionDeleteResult,
  SessionListResult,
  SessionLoadParams,
  SessionResult,
  SetConfigParams,
  ToolCallEvent,
} from "./protocol";
import { ContenoxOutput } from "../logging/output";
import { TelemetryLogger } from "../logging/telemetry";

interface PendingRequest<T> {
  method: string;
  startedAt: number;
  timer: NodeJS.Timeout;
  cancellation?: Disposable;
  resolve: (value: T) => void;
  reject: (reason: Error) => void;
}

export type PermissionRequestHandler = (params: RequestPermissionParams) => Promise<RequestPermissionResponse>;

export class BridgeRpcError extends Error {
  public constructor(
    message: string,
    public readonly code: number,
    public readonly data?: unknown,
  ) {
    super(message);
  }
}

export class BridgeClient implements Disposable {
  private nextID = 1;
  private disposed = false;
  private readonly pending = new Map<JsonRpcID, PendingRequest<unknown>>();
  private readonly ignoredResponses = new Set<JsonRpcID>();
  private readonly listeners = new Map<string, Set<(params: unknown) => void>>();
  private readonly permissionHandlers: PermissionRequestHandler[] = [];
  private readonly framer: JsonRpcFramer;

  public constructor(
    stdin: Writable,
    stdout: Readable,
    private readonly output: ContenoxOutput,
    private readonly requestTimeoutMs: number,
    private readonly logProtocol: boolean,
    private readonly telemetry: TelemetryLogger,
  ) {
    this.framer = new JsonRpcFramer(
      stdin,
      (message) => this.handleMessage(message),
      (error) => this.output.error(`Bridge protocol parse error: ${error.message}`),
    );
    stdout.on("data", (chunk: Buffer) => this.framer.accept(chunk));
  }

  public initialize(params: InitializeParams): Promise<InitializeResult> {
    return this.request<InitializeResult>("initialize", params);
  }

  public health(): Promise<HealthResult> {
    return this.request<HealthResult>("health");
  }

  public getConfig(): Promise<ConfigSnapshot> {
    return this.request<ConfigSnapshot>("getConfig");
  }

  public setConfig(params: SetConfigParams): Promise<ConfigSnapshot> {
    return this.request<ConfigSnapshot>("setConfig", params);
  }

  public listProviders(): Promise<ListProvidersResult> {
    return this.request<ListProvidersResult>("listProviders");
  }

  public listModels(params?: ListModelsParams): Promise<ListModelsResult> {
    return this.request<ListModelsResult>("listModels", params);
  }

  public listHitlPolicies(): Promise<ListHitlPoliciesResult> {
    return this.request<ListHitlPoliciesResult>("listHitlPolicies");
  }

  public listCommands(): Promise<ListCommandsResult> {
    return this.request<ListCommandsResult>("listCommands");
  }

  public listMCPServers(): Promise<ListMCPServersResult> {
    return this.request<ListMCPServersResult>("listMCPServers");
  }

  public sessionCreate(params?: SessionCreateParams): Promise<SessionResult> {
    return this.request<SessionResult>("sessionCreate", params);
  }

  public sessionList(): Promise<SessionListResult> {
    return this.request<SessionListResult>("sessionList");
  }

  public sessionLoad(params: SessionLoadParams): Promise<SessionResult> {
    return this.request<SessionResult>("sessionLoad", params);
  }

  public sessionDelete(params: SessionDeleteParams): Promise<SessionDeleteResult> {
    return this.request<SessionDeleteResult>("sessionDelete", params);
  }

  public chatSend(params: ChatSendParams): Promise<ChatSendResult> {
    return this.request<ChatSendResult>("chatSend", params);
  }

  public chatCancel(params: ChatCancelParams): Promise<ChatCancelResult> {
    return this.request<ChatCancelResult>("chatCancel", params);
  }

  public approvalRespond(params: ApprovalRespondParams): Promise<ApprovalRespondResult> {
    return this.request<ApprovalRespondResult>("approvalRespond", params);
  }

  public autocomplete(params: AutocompleteParams, token?: CancellationToken): Promise<AutocompleteResult> {
    return this.request<AutocompleteResult>("autocomplete", params, Math.max(this.requestTimeoutMs, 25000), token);
  }

  public shutdown(): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>("shutdown", undefined, 3000);
  }

  public onNotification<T>(method: string, listener: (params: T) => void): Disposable {
    const listeners = this.listeners.get(method) ?? new Set<(params: unknown) => void>();
    const wrapped = listener as (params: unknown) => void;
    listeners.add(wrapped);
    this.listeners.set(method, listeners);
    return {
      dispose: () => {
        listeners.delete(wrapped);
        if (listeners.size === 0) {
          this.listeners.delete(method);
        }
      },
    };
  }

  public onChatDelta(listener: (params: ChatDeltaEvent) => void): Disposable {
    return this.onNotification("chatDelta", listener);
  }

  public onChatStarted(listener: (params: ChatLifecycleEvent) => void): Disposable {
    return this.onNotification("chatStarted", listener);
  }

  public onChatCompleted(listener: (params: ChatLifecycleEvent) => void): Disposable {
    return this.onNotification("chatCompleted", listener);
  }

  public onChatFailed(listener: (params: ChatLifecycleEvent) => void): Disposable {
    return this.onNotification("chatFailed", listener);
  }

  public onChatCancelled(listener: (params: ChatLifecycleEvent) => void): Disposable {
    return this.onNotification("chatCancelled", listener);
  }

  public onToolCall(listener: (params: ToolCallEvent) => void): Disposable {
    return this.onNotification("toolCall", listener);
  }

  public onHitlDecision(listener: (params: HitlDecisionEvent) => void): Disposable {
    return this.onNotification("hitlDecision", listener);
  }

  public onApprovalRequested(listener: (params: ApprovalRequestedEvent) => void): Disposable {
    return this.onNotification("approvalRequested", listener);
  }

  public pushPermissionRequestHandler(handler: PermissionRequestHandler): Disposable {
    this.permissionHandlers.push(handler);
    return {
      dispose: () => {
        const index = this.permissionHandlers.lastIndexOf(handler);
        if (index >= 0) {
          this.permissionHandlers.splice(index, 1);
        }
      },
    };
  }

  public onConfigChanged(listener: (params: ConfigSnapshot) => void): Disposable {
    return this.onNotification("configChanged", listener);
  }

  public request<T>(method: string, params?: unknown, timeoutMs = this.requestTimeoutMs, token?: CancellationToken): Promise<T> {
    if (this.disposed) {
      return Promise.reject(new Error("Contenox bridge client is disposed"));
    }
    if (token?.isCancellationRequested) {
      return Promise.reject(new Error(`Contenox bridge request cancelled before send: ${method}`));
    }

    const id = this.nextID++;
    const request: JsonRpcRequest = {
      jsonrpc: "2.0",
      id,
      method,
    };
    if (params !== undefined) {
      request.params = params;
    }

    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        const pending = this.pending.get(id);
        if (pending) {
          pending.cancellation?.dispose();
        }
        this.pending.delete(id);
        this.ignoredResponses.add(id);
        this.sendCancel(id);
        this.telemetry.warn("bridge.rpc.timeout", {
          id,
          method,
          durationMs: Date.now() - startedAt,
        });
        reject(new Error(`Contenox bridge request timed out: ${method}`));
      }, timeoutMs);
      const cancellation = token?.onCancellationRequested(() => {
        const pending = this.pending.get(id);
        if (!pending) {
          return;
        }
        clearTimeout(pending.timer);
        pending.cancellation?.dispose();
        this.pending.delete(id);
        this.ignoredResponses.add(id);
        this.sendCancel(id);
        this.logCancellation(id, method, Date.now() - pending.startedAt);
        reject(new Error(`Contenox bridge request cancelled: ${method}`));
      });

      const startedAt = Date.now();
      this.pending.set(id, {
        method,
        startedAt,
        timer,
        cancellation,
        resolve: resolve as (value: unknown) => void,
        reject,
      });

      this.logRequest(method, id);
      this.telemetry.event("bridge.rpc.start", {
        id,
        method,
        timeoutMs,
      });
      this.framer.send(request);
    });
  }

  public dispose(): void {
    this.disposed = true;
    for (const [id, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.cancellation?.dispose();
      this.sendCancel(id);
      this.telemetry.warn("bridge.rpc.closed", {
        id,
        method: pending.method,
        durationMs: Date.now() - pending.startedAt,
      });
      pending.reject(new Error(`Contenox bridge closed before ${pending.method} completed`));
      this.pending.delete(id);
    }
  }

  private handleMessage(message: unknown): void {
    if (isNotification(message)) {
      this.logNotification(message.method, message.params);
      this.emitNotification(message.method, message.params);
      return;
    }
    if (isRequest(message)) {
      void this.handleServerRequest(message);
      return;
    }
    if (!isResponse(message)) {
      this.output.warn("Ignoring bridge message without a JSON-RPC response shape");
      return;
    }
    const id = message.id ?? null;
    const pending = this.pending.get(id);
    if (!pending) {
      if (this.ignoredResponses.delete(id)) {
        return;
      }
      this.telemetry.warn("bridge.rpc.unmatched_response", { id });
      this.output.warn(`Ignoring bridge response with no pending request: ${String(id)}`);
      return;
    }
    this.pending.delete(id);
    clearTimeout(pending.timer);
    pending.cancellation?.dispose();

    if (message.error) {
      this.logResponse(pending.method, id, message.error);
      this.telemetry.error("bridge.rpc.error", message.error.message, {
        id,
        method: pending.method,
        code: message.error.code,
        durationMs: Date.now() - pending.startedAt,
      });
      pending.reject(new BridgeRpcError(message.error.message, message.error.code, message.error.data));
      return;
    }

    this.logResponse(pending.method, id);
    this.telemetry.event("bridge.rpc.ok", {
      id,
      method: pending.method,
      durationMs: Date.now() - pending.startedAt,
    });
    pending.resolve(message.result);
  }

  private async handleServerRequest(message: JsonRpcRequest): Promise<void> {
    this.logServerRequest(message.method, message.id);
    this.telemetry.event("bridge.server_request.start", {
      id: message.id,
      method: message.method,
    });
    try {
      switch (message.method) {
        case "session/request_permission": {
          const params = parseRequestPermissionParams(message.params);
          const handler = this.permissionHandlers[this.permissionHandlers.length - 1];
          const result = handler ? await handler(params) : cancelledPermissionResponse();
          if (!handler) {
            this.telemetry.warn("bridge.permission.no_handler", {
              id: message.id,
              sessionId: params.sessionId,
              toolCallId: params.toolCall.toolCallId,
            });
          }
          this.sendResult(message.id, result);
          this.telemetry.event("bridge.server_request.ok", {
            id: message.id,
            method: message.method,
            outcome: result.outcome.outcome,
            optionId: "optionId" in result.outcome ? result.outcome.optionId : undefined,
          });
          return;
        }
        default:
          this.sendError(message.id, -32601, `method not found: ${message.method}`);
          this.telemetry.warn("bridge.server_request.unknown", { id: message.id, method: message.method });
          return;
      }
    } catch (error) {
      this.sendError(message.id, -32603, error instanceof Error ? error.message : String(error));
      this.telemetry.error("bridge.server_request.error", error, {
        id: message.id,
        method: message.method,
      });
    }
  }

  private logCancellation(id: JsonRpcID, method: string, durationMs: number): void {
    const fields = { id, method, durationMs };
    if (method === "autocomplete") {
      this.telemetry.event("bridge.rpc.cancelled.expected", fields);
      return;
    }
    this.telemetry.warn("bridge.rpc.cancelled", fields);
  }

  private logRequest(method: string, id: JsonRpcID): void {
    if (this.logProtocol) {
      this.output.protocol(`--> ${String(id)} ${method}`);
    }
  }

  private logResponse(method: string, id: JsonRpcID, error?: JsonRpcError): void {
    if (!this.logProtocol) {
      return;
    }
    if (error) {
      this.output.protocol(`<-- ${String(id)} ${method} error ${error.code}: ${error.message}`);
    } else {
      this.output.protocol(`<-- ${String(id)} ${method} ok`);
    }
  }

  private logServerRequest(method: string, id: JsonRpcID): void {
    if (this.logProtocol) {
      this.output.protocol(`<-- ${String(id)} ${method}`);
    }
  }

  private logNotification(method: string, params?: unknown): void {
    if (this.logProtocol) {
      this.output.protocol(`<-- notification ${method}`);
    }
    this.telemetry.event("bridge.notification", { method });
    if (method === "hitlDecision" && isHitlDecisionEvent(params)) {
      this.telemetry.event("hitl.decision", {
        sessionId: params.sessionId,
        turnId: params.turnId,
        toolsName: params.toolsName,
        toolName: params.toolName,
        action: params.action,
        reason: params.reason,
        policyName: params.policyName,
        policyPath: params.policyPath,
        argsSummary: params.argsSummary,
        matchedRule: params.matchedRule,
        timeoutS: params.timeoutS,
        approvalRequested: params.approvalRequested,
      });
    }
  }

  private sendCancel(id: JsonRpcID): void {
    this.logRequest("$/cancelRequest", id);
    this.framer.send({
      jsonrpc: "2.0",
      method: "$/cancelRequest",
      params: { id },
    });
  }

  private sendResult(id: JsonRpcID, result: unknown): void {
    if (this.logProtocol) {
      this.output.protocol(`--> ${String(id)} result`);
    }
    this.framer.send({
      jsonrpc: "2.0",
      id,
      result,
    });
  }

  private sendError(id: JsonRpcID, code: number, message: string, data?: unknown): void {
    if (this.logProtocol) {
      this.output.protocol(`--> ${String(id)} error ${code}: ${message}`);
    }
    this.framer.send({
      jsonrpc: "2.0",
      id,
      error: { code, message, data },
    });
  }

  private emitNotification(method: string, params: unknown): void {
    const listeners = this.listeners.get(method);
    if (!listeners) {
      return;
    }
    for (const listener of listeners) {
      try {
        listener(params);
      } catch (error) {
        this.output.error(`Contenox notification handler failed for ${method}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
  }
}

function isNotification(value: unknown): value is JsonRpcNotification {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as JsonRpcNotification;
  return candidate.jsonrpc === "2.0" && typeof candidate.method === "string" && !("id" in candidate);
}

function isRequest(value: unknown): value is JsonRpcRequest {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as JsonRpcRequest;
  return candidate.jsonrpc === "2.0" && typeof candidate.method === "string" && "id" in candidate;
}

function isResponse(value: unknown): value is JsonRpcResponse {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as JsonRpcResponse;
  return candidate.jsonrpc === "2.0" && ("result" in candidate || "error" in candidate);
}

function isHitlDecisionEvent(value: unknown): value is HitlDecisionEvent {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<HitlDecisionEvent>;
  return (
    typeof candidate.sessionId === "string" &&
    typeof candidate.turnId === "string" &&
    typeof candidate.action === "string" &&
    typeof candidate.approvalRequested === "boolean"
  );
}

function parseRequestPermissionParams(value: unknown): RequestPermissionParams {
  if (!value || typeof value !== "object") {
    throw new Error("permission request params must be an object");
  }
  const candidate = value as Partial<RequestPermissionParams>;
  if (typeof candidate.sessionId !== "string") {
    throw new Error("permission request sessionId is required");
  }
  if (!candidate.toolCall || typeof candidate.toolCall !== "object") {
    throw new Error("permission request toolCall is required");
  }
  if (typeof candidate.toolCall.toolCallId !== "string") {
    throw new Error("permission request toolCall.toolCallId is required");
  }
  if (!Array.isArray(candidate.options)) {
    throw new Error("permission request options are required");
  }
  return candidate as RequestPermissionParams;
}

function cancelledPermissionResponse(): RequestPermissionResponse {
  return { outcome: { outcome: "cancelled" } };
}
