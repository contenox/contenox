import * as vscode from "vscode";
import { BridgeProcess } from "../bridge/BridgeProcess";
import {
  EditorContextAttachment,
  RequestPermissionParams,
  RequestPermissionResponse,
  SessionInfo,
  SessionMessage,
} from "../bridge/protocol";
import { collectEditorContext, contextSummary } from "../editor/context";
import { DiffStore } from "../editor/diffStore";
import { collectGitChangeContext } from "../editor/gitContext";
import { ContenoxOutput } from "../logging/output";
import { TelemetryLogger } from "../logging/telemetry";
import { ChatTurnRunner } from "./turnRunner";
import { approvalEventFromPermissionRequest, selectedPermissionOption } from "./permissionOutcome";
import {
  ChatHostToWebviewMessage,
  ChatWebviewToHostMessage,
  WireMessage,
  WireRuntimeSummary,
  WireSession,
  WireSessionResponse,
} from "./webviewProtocol";

interface PendingContext {
  context: EditorContextAttachment[];
  expiresAt: number;
}

interface PendingApproval {
  event: RequestPermissionParams;
  resolve: (response: RequestPermissionResponse) => void;
}

const pendingContextTtlMs = 10 * 60 * 1000;

export class ChatWebviewViewProvider implements vscode.WebviewViewProvider, vscode.Disposable {
  private view: vscode.WebviewView | undefined;
  private readonly turns: ChatTurnRunner;
  private sessionId: string | undefined;
  private pendingContext: PendingContext | undefined;
  private readonly queued: ChatHostToWebviewMessage[] = [];
  private readonly activeTurns = new Map<string, vscode.CancellationTokenSource>();
  private readonly pendingApprovals = new Map<string, PendingApproval>();
  private lastContextUsed: number | undefined;
  private lastContextSize: number | undefined;
  private contextUsageSub: vscode.Disposable | undefined;

  public constructor(
    private readonly bridge: BridgeProcess,
    private readonly diffStore: DiffStore,
    private readonly extensionUri: vscode.Uri,
    output: ContenoxOutput,
    private readonly telemetry: TelemetryLogger,
    private readonly onSessionsChanged: () => void,
  ) {
    this.turns = new ChatTurnRunner(bridge, output, telemetry);
  }

  public resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = {
      enableScripts: true,
      localResourceRoots: [vscode.Uri.joinPath(this.extensionUri, "media")],
    };
    view.webview.html = this.renderShell(view.webview);
    view.webview.onDidReceiveMessage((message: ChatWebviewToHostMessage) => {
      void this.handleMessage(message);
    });
  }

  public dispose(): void {
    this.view = undefined;
    for (const tokenSource of this.activeTurns.values()) {
      tokenSource.cancel();
      tokenSource.dispose();
    }
    this.activeTurns.clear();
    this.contextUsageSub?.dispose();
  }

  private ensureContextUsageSubscription() {
    if (this.contextUsageSub) return;
    const client = this.bridge.currentClient;
    if (client) {
      this.contextUsageSub = client.onContextUsage((ev) => {
        this.lastContextUsed = ev.used;
        this.lastContextSize = ev.size;
        void this.pushRuntimeSummary();
      });
    }
  }

  public reveal(): void {
    void vscode.commands.executeCommand("contenox.chat.focus");
  }

  public async openChat(): Promise<void> {
    this.reveal();
  }

  public async refreshRuntimeSummary(): Promise<void> {
    await this.pushRuntimeSummary();
  }

  public async showRuntimeSettingsPicker(): Promise<void> {
    await this.handleOpenRuntimeSettings();
  }

  public async openSession(sessionId?: string): Promise<void> {
    this.setActiveSession(sessionId);
    this.reveal();
    if (sessionId) {
      void this.postToWebview({ type: "selectSession", id: sessionId });
    }
  }

  public setActiveSession(sessionId?: string): void {
    if (sessionId) {
      this.sessionId = sessionId;
    }
  }

  public clearActiveSession(sessionId?: string): void {
    if (!sessionId || this.sessionId === sessionId) {
      this.sessionId = undefined;
    }
  }

  public async askSelection(): Promise<void> {
    const editorContext = await collectEditorContext({
      includeSelection: true,
      includeActiveFile: false,
      includeDiagnostics: false,
    });
    if (!editorContext.some((item) => item.kind === "selection")) {
      vscode.window.showInformationMessage("No editor selection is active.");
      return;
    }
    await this.runQuickAction(editorContext, "Explain the selected code.", true);
  }

  public async fixSelection(): Promise<void> {
    const editorContext = await collectEditorContext({
      includeSelection: true,
      includeActiveFile: true,
      includeDiagnostics: false,
    });
    if (!editorContext.some((item) => item.kind === "selection")) {
      vscode.window.showInformationMessage("No editor selection is active.");
      return;
    }
    await this.runQuickAction(editorContext, "Fix the diagnostics in the active file.", true);
  }

  public async addSelectionToChat(): Promise<void> {
    const editorContext = await collectEditorContext({
      includeSelection: true,
      includeActiveFile: false,
      includeDiagnostics: false,
    });
    if (!editorContext.some((item) => item.kind === "selection")) {
      vscode.window.showInformationMessage("No editor selection is active.");
      return;
    }
    await this.runQuickAction(editorContext, "", false);
  }

  public async fixDiagnostics(diagnostics?: readonly vscode.Diagnostic[]): Promise<void> {
    const editorContext = await collectEditorContext({
      includeSelection: true,
      includeActiveFile: true,
      includeDiagnostics: true,
      diagnostics,
    });
    if (!editorContext.some((item) => item.kind === "diagnostics")) {
      vscode.window.showInformationMessage("No diagnostics are available for the active file.");
      return;
    }
    await this.runQuickAction(editorContext, "Fix the diagnostics in the active file.", true);
  }

  public async explainDiagnostics(diagnostics?: readonly vscode.Diagnostic[]): Promise<void> {
    const editorContext = await collectEditorContext({
      includeSelection: true,
      includeActiveFile: true,
      includeDiagnostics: true,
      diagnostics,
    });
    if (!editorContext.some((item) => item.kind === "diagnostics")) {
      vscode.window.showInformationMessage("No diagnostics are available for the active file.");
      return;
    }
    await this.runQuickAction(editorContext, "Explain the active diagnostics.", true);
  }

  public async reviewChanges(): Promise<void> {
    const gitContext = await collectGitChangeContext();
    if (gitContext.length === 0) {
      vscode.window.showInformationMessage("No git changes are available to review.");
      return;
    }
    await this.runQuickAction(gitContext, "Review the current git changes.", true);
  }

  public async draftCommitMessage(): Promise<void> {
    const gitContext = await collectGitChangeContext();
    if (gitContext.length === 0) {
      vscode.window.showInformationMessage("No git changes are available for a commit message.");
      return;
    }
    await this.runQuickAction(gitContext, "Draft a commit message for the current git changes.", true);
  }

  private async runQuickAction(
    context: EditorContextAttachment[],
    content: string,
    submit: boolean,
  ): Promise<void> {
    this.setPendingContext(context);
    this.reveal();
    await this.postToWebview({
      type: "composerAction",
      nonce: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
      content,
      submit,
    });
  }

  private setPendingContext(context: EditorContextAttachment[]): void {
    this.pendingContext = { context, expiresAt: Date.now() + pendingContextTtlMs };
    this.telemetry.event("chat.pending_context.set", { context: contextSummary(context) });
  }

  private takePendingContext(): EditorContextAttachment[] {
    const pending = this.pendingContext;
    this.pendingContext = undefined;
    if (!pending || pending.expiresAt < Date.now()) {
      return [];
    }
    this.telemetry.event("chat.pending_context.used", { context: contextSummary(pending.context) });
    return pending.context;
  }

  private async handleMessage(message: ChatWebviewToHostMessage): Promise<void> {
    switch (message.type) {
      case "ready":
        for (const queued of this.queued.splice(0)) {
          void this.postToWebview(queued);
        }
        void this.pushRuntimeSummary();
        return;
      case "getRuntimeSummary":
        return this.handleGetRuntimeSummary(message.requestId);
      case "openRuntimeSettings":
        return this.handleOpenRuntimeSettings();
      case "listSessions":
        return this.handleListSessions(message.requestId);
      case "createSession":
        return this.handleCreateSession(message.requestId, message.title);
      case "getSession":
        return this.handleGetSession(message.requestId, message.id);
      case "renameSession":
        return this.postResult(message.requestId, false, "Renaming sessions is not supported");
      case "deleteSession":
        return this.handleDeleteSession(message.requestId, message.id);
      case "sendMessage":
        return this.handleSendMessage(message.requestId, message.id, message.content);
      case "cancelTurn":
        this.activeTurns.get(message.id)?.cancel();
        return;
      case "listTools":
        return this.postResult(message.requestId, true, []);
      case "approvalResponse":
        return this.handleApprovalResponse(message.requestId, message.optionId);
      case "openDiff":
        await this.diffStore.open({
          title: message.call.title ?? "Contenox Diff",
          before: message.call.diff?.before,
          after: message.call.diff?.after,
          filePath: message.call.diff?.path,
        });
        return;
      case "confirmDelete":
        return this.handleConfirmDelete(message.requestId, message.title);
      case "promptRename":
        return this.handlePromptRename(message.requestId, message.title);
    }
  }

  private async handleConfirmDelete(requestId: string, title: string): Promise<void> {
    const choice = await vscode.window.showWarningMessage(
      `Delete "${title}"?`,
      { modal: true },
      "Delete",
    );
    this.postResult(requestId, true, choice === "Delete");
  }

  private async handleGetRuntimeSummary(requestId: string): Promise<void> {
    try {
      const summary = await this.loadRuntimeSummary();
      this.postResult(requestId, true, summary);
    } catch (error) {
      this.postResult(requestId, false, errorMessage(error));
    }
  }

  private async handleOpenRuntimeSettings(): Promise<void> {
    const summary: WireRuntimeSummary = await this.loadRuntimeSummary().catch(() => ({
      connected: false,
    }));
    const provider = summary.provider || "not set";
    const model = summary.model || "not set";
    const choice = await vscode.window.showQuickPick(
      [
        {
          label: "Provider",
          description: provider,
          command: "contenox.selectProvider",
        },
        {
          label: "Model",
          description: model,
          command: "contenox.selectChatModel",
        },
        {
          label: "Thinking level",
          description: summary.think || "auto",
          command: "contenox.selectThinkLevel",
        },
        {
          label: "HITL policy",
          description: summary.hitlPolicy || "default",
          command: "contenox.selectHitlPolicy",
        },
        {
          label: "Open full Runtime panel",
          description: "Detailed configuration in sidebar",
          focusSettings: true,
        },
      ],
      {
        title: "Contenox runtime",
        placeHolder: "Choose a runtime setting to change",
      },
    );
    if (!choice) {
      return;
    }
    if ("focusSettings" in choice && choice.focusSettings) {
      await vscode.commands.executeCommand("contenox.controls.focus");
      return;
    }
    if (!("command" in choice) || !choice.command) {
      return;
    }
    await vscode.commands.executeCommand(choice.command);
    await this.pushRuntimeSummary();
  }

  private async pushRuntimeSummary(): Promise<void> {
    this.ensureContextUsageSubscription();
    try {
      const summary = await this.loadRuntimeSummary();
      await this.postToWebview({ type: "runtimeConfig", summary });
    } catch {
      await this.postToWebview({
        type: "runtimeConfig",
        summary: { connected: false },
      });
    }
  }

  private async loadRuntimeSummary(): Promise<WireRuntimeSummary> {
    const state = await this.bridge.ensureStarted();
    const client = this.bridge.currentClient;
    if (!client) {
      throw new Error("Contenox runtime connection is not available");
    }
    const config = await client.getConfig();
    this.ensureContextUsageSubscription();
    let contextSize: number | undefined;
    try {
      // list without provider to ensure we get observed models which carry contextLength
      const ml = await client.listModels(undefined);
      const m = ml.models.find((mm) => mm.name === config.defaultModel);
      if (m) {
        contextSize = (m.contextLength && m.contextLength > 0) ? m.contextLength : 4096; // fallback
      }
    } catch {}
    const used = this.lastContextUsed;
    const size = this.lastContextSize ?? contextSize;
    return {
      provider: config.defaultProvider,
      model: config.defaultModel,
      think: config.defaultThink,
      hitlPolicy: config.hitlPolicyName,
      connected: state.health.status === "ok",
      contextUsed: used,
      contextSize: size,
    };
  }

  private async handlePromptRename(requestId: string, currentTitle: string): Promise<void> {
    const title = await vscode.window.showInputBox({
      title: "Rename session",
      value: currentTitle,
      prompt: "Enter a new session name",
      validateInput: (value) => (value.trim() ? undefined : "Session name cannot be empty"),
    });
    this.postResult(requestId, true, title?.trim() || undefined);
  }

  private async handleListSessions(requestId: string): Promise<void> {
    try {
      const client = await this.requireClient();
      const result = await client.sessionList();
      this.postResult(requestId, true, result.sessions.map(toWireSession));
    } catch (error) {
      this.postResult(requestId, false, errorMessage(error));
    }
  }

  private async handleCreateSession(requestId: string, title: string): Promise<void> {
    try {
      const client = await this.requireClient();
      const result = await client.sessionCreate({ name: title });
      this.onSessionsChanged();
      this.postResult(requestId, true, toWireSessionResponse(result.session, result.messages));
    } catch (error) {
      this.postResult(requestId, false, errorMessage(error));
    }
  }

  private async handleGetSession(requestId: string, id: string): Promise<void> {
    try {
      const client = await this.requireClient();
      const result = await client.sessionLoad({ sessionId: id });
      this.setActiveSession(result.session.id);
      this.postResult(requestId, true, toWireSessionResponse(result.session, result.messages));
    } catch (error) {
      this.postResult(requestId, false, errorMessage(error));
    }
  }

  private async handleDeleteSession(requestId: string, id: string): Promise<void> {
    try {
      const client = await this.requireClient();
      await client.sessionDelete({ sessionId: id });
      this.clearActiveSession(id);
      this.onSessionsChanged();
      this.postResult(requestId, true, undefined);
    } catch (error) {
      this.postResult(requestId, false, errorMessage(error));
    }
  }

  private async handleSendMessage(requestId: string, sessionId: string, content: string): Promise<void> {
    const tokenSource = new vscode.CancellationTokenSource();
    this.activeTurns.set(sessionId, tokenSource);
    const context = this.takePendingContext();
    try {
      const result = await this.turns.run(
        { input: content, context, sessionId, token: tokenSource.token },
        {
          onDelta: (event) => {
            void this.postToWebview({
              type: "delta",
              requestId,
              content: event.content,
              thinking: event.thinking,
            });
          },
          onToolCall: (event) => {
            const diff = this.diffStore.registerToolDiff(event);
            void this.postToWebview({
              type: "toolCall",
              requestId,
              call: {
                id: event.toolCallId ?? event.taskId ?? `${event.turnId}-${event.title ?? "tool"}`,
                title: event.title,
                status: event.status,
                toolName: event.toolName,
                output: event.output,
                error: event.error,
                diff: diff
                  ? { path: event.diffPath, before: event.diffOld, after: event.diffNew }
                  : undefined,
              },
            });
          },
          onPermissionRequested: (_client, event, token) =>
            new Promise<RequestPermissionResponse>((resolve) => {
              this.pendingApprovals.set(requestId, { event, resolve });
              const approval = approvalEventFromPermissionRequest(event);
              void this.postToWebview({
                type: "approvalRequest",
                requestId,
                request: {
                  approvalId: approval.approvalId,
                  title: approval.title,
                  toolName: approval.toolName,
                  details: approval.details,
                  diff:
                    approval.diffOld || approval.diffNew
                      ? { before: approval.diffOld, after: approval.diffNew }
                      : undefined,
                  options: approval.options,
                },
              });
              token.onCancellationRequested(() => {
                if (this.pendingApprovals.delete(requestId)) {
                  resolve({ outcome: { outcome: "cancelled" } });
                }
              });
            }),
        },
      );

      if (result.failed) {
        this.postResult(requestId, false, result.event.error ?? "Contenox request failed");
        return;
      }
      this.postResult(requestId, true, {
        messages: (result.event.messages ?? []).map((message) => toWireMessage(message, sessionId)),
      } satisfies WireSessionResponse);
      this.onSessionsChanged();
      void this.pushRuntimeSummary();
    } catch (error) {
      this.postResult(requestId, false, errorMessage(error));
    } finally {
      this.activeTurns.delete(sessionId);
      tokenSource.dispose();
    }
  }

  private handleApprovalResponse(requestId: string, optionId: string | undefined): void {
    const pending = this.pendingApprovals.get(requestId);
    if (!pending) {
      return;
    }
    this.pendingApprovals.delete(requestId);
    const option = selectedPermissionOption(pending.event, optionId);
    pending.resolve({
      outcome: option ? { outcome: "selected", optionId: option.optionId } : { outcome: "cancelled" },
    });
  }

  private postResult(requestId: string, ok: boolean, value: unknown): void {
    void this.postToWebview(
      ok
        ? { type: "result", requestId, ok: true, value }
        : { type: "result", requestId, ok: false, error: String(value) },
    );
  }

  private async postToWebview(message: ChatHostToWebviewMessage): Promise<void> {
    if (!this.view) {
      this.queued.push(message);
      return;
    }
    await this.view.webview.postMessage(message);
  }

  private async requireClient() {
    const state = await this.bridge.ensureStarted();
    if (!state.initialize.capabilities.chat) {
      throw new Error("This Contenox runtime does not support chat");
    }
    const client = this.bridge.currentClient;
    if (!client) {
      throw new Error("Contenox runtime connection is not available");
    }
    return client;
  }

  private renderShell(webview: vscode.Webview): string {
    const scriptUri = webview.asWebviewUri(
      vscode.Uri.joinPath(this.extensionUri, "media", "chat", "webview.js"),
    );
    const styleUri = webview.asWebviewUri(
      vscode.Uri.joinPath(this.extensionUri, "media", "chat", "webview.css"),
    );
    return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${webview.cspSource} data:; style-src ${webview.cspSource} 'unsafe-inline'; font-src ${webview.cspSource}; script-src ${webview.cspSource};">
  <link rel="stylesheet" href="${styleUri.toString()}">
</head>
<body class="vscode-chat-shell">
  <div id="root"></div>
  <script src="${scriptUri.toString()}"></script>
</body>
</html>`;
  }
}

function toWireSession(info: SessionInfo): WireSession {
  return {
    id: info.id,
    title: info.name ?? info.id,
    createdAt: info.updatedAt ?? new Date().toISOString(),
    updatedAt: info.updatedAt ?? new Date().toISOString(),
    lastMessageAt: info.updatedAt,
  };
}

function toWireSessionResponse(session: SessionInfo, messages: SessionMessage[]): WireSessionResponse {
  return {
    session: toWireSession(session),
    messages: messages.map((message) => toWireMessage(message, session.id)),
  };
}

function toWireMessage(message: SessionMessage, sessionId: string): WireMessage {
  return {
    id: message.id ?? `${sessionId}-${message.timestamp ?? Math.random().toString(16).slice(2)}`,
    sessionId,
    role: isWireRole(message.role) ? message.role : "assistant",
    content: message.content ?? "",
    createdAt: message.timestamp ?? new Date().toISOString(),
  };
}

function isWireRole(role: string): role is "system" | "user" | "assistant" | "tool" {
  return role === "system" || role === "user" || role === "assistant" || role === "tool";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
