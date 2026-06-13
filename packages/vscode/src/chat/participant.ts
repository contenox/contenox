import * as vscode from "vscode";
import { BridgeClient } from "../bridge/BridgeClient";
import { BridgeProcess } from "../bridge/BridgeProcess";
import {
  ApprovalRequestedEvent,
  ChatDeltaEvent,
  ChatLifecycleEvent,
  EditorContextAttachment,
  RequestPermissionParams,
  RequestPermissionResponse,
  ToolCallContent,
  ToolCallEvent,
} from "../bridge/protocol";
import { collectEditorContext, contextSummary } from "../editor/context";
import { DiffStore } from "../editor/diffStore";
import { collectGitChangeContext } from "../editor/gitContext";
import { ContenoxOutput } from "../logging/output";
import { TelemetryLogger } from "../logging/telemetry";
import { requestNativeApproval } from "../approval/nativeTool";
import { sessionTitleFromInput } from "./sessionTitle";
import { ChatTurnRunner, TurnResult } from "./turnRunner";

interface PendingContext {
  context: EditorContextAttachment[];
  expiresAt: number;
}

const pendingContextTtlMs = 10 * 60 * 1000;

export class ContenoxChatParticipant implements vscode.Disposable {
  private readonly disposables: vscode.Disposable[] = [];
  private readonly turns: ChatTurnRunner;
  public readonly participant: vscode.ChatParticipant;
  private sessionId: string | undefined;
  private pendingContext: PendingContext | undefined;

  public constructor(
    private readonly context: vscode.ExtensionContext,
    bridge: BridgeProcess,
    private readonly diffStore: DiffStore,
    private readonly output: ContenoxOutput,
    private readonly telemetry: TelemetryLogger,
    private readonly onSessionsChanged: () => void,
  ) {
    this.turns = new ChatTurnRunner(bridge, output, telemetry);
    this.participant = vscode.chat.createChatParticipant("contenox", (request, chatContext, response, token) =>
      this.handleRequest(request, chatContext, response, token, "native-chat"),
    );
    this.participant.iconPath = vscode.Uri.joinPath(this.context.extensionUri, "media", "contenox-icon.png");
    this.disposables.push(this.participant);
    this.telemetry.event("chat.participant.registered", { id: this.participant.id });
  }

  public async openChat(): Promise<void> {
    await this.openNativeChat("@contenox ", false);
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
    this.setPendingContext(editorContext);
    await this.openNativeChat("@contenox /explain", true);
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
    this.setPendingContext(editorContext);
    await this.openNativeChat("@contenox /fix", true);
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
    this.setPendingContext(editorContext);
    await this.openNativeChat("@contenox /explain ", false);
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
    this.setPendingContext(editorContext);
    await this.openNativeChat("@contenox /fix", true);
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
    this.setPendingContext(editorContext);
    await this.openNativeChat("@contenox /explain the active diagnostics", true);
  }

  public async reviewChanges(): Promise<void> {
    const gitContext = await collectGitChangeContext();
    if (gitContext.length === 0) {
      vscode.window.showInformationMessage("No git changes are available to review.");
      return;
    }
    this.setPendingContext(gitContext);
    await this.openNativeChat("@contenox /review", true);
  }

  public async draftCommitMessage(): Promise<void> {
    const gitContext = await collectGitChangeContext();
    if (gitContext.length === 0) {
      vscode.window.showInformationMessage("No git changes are available for a commit message.");
      return;
    }
    this.setPendingContext(gitContext);
    await this.openNativeChat("@contenox /commit", true);
  }

  public async openSession(sessionId?: string): Promise<void> {
    this.setActiveSession(sessionId);
    await this.openChat();
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

  public handleAgentSessionRequest(
    sessionId: string | undefined,
    request: vscode.ChatRequest,
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
  ): Promise<vscode.ChatResult> {
    return this.handleRequest(request, undefined, response, token, "agent-session", sessionId);
  }

  public dispose(): void {
    for (const disposable of this.disposables.splice(0)) {
      disposable.dispose();
    }
  }

  private async handleRequest(
    request: vscode.ChatRequest,
    _chatContext: vscode.ChatContext | undefined,
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
    source: "native-chat" | "agent-session",
    sessionIdOverride?: string,
  ): Promise<vscode.ChatResult> {
    const startedAt = Date.now();
    const command = request.command ?? "";
    const prompt = request.prompt.trim();
    const pendingContext = this.takePendingContext();
    const referenceContext = await contextFromReferences(request.references, this.telemetry);
    const explicitContext = mergeEditorContext([...(pendingContext ?? []), ...referenceContext]);
    const editorContext = explicitContext.length > 0 ? explicitContext : await this.contextForRequest(command);
    const input = inputForRequest(command, prompt);
    pushEditorReferences(response, editorContext);
    this.telemetry.event("chat.request.start", {
      source,
      command,
      promptChars: prompt.length,
      context: contextSummary(editorContext),
      referenceCount: request.references.length,
      referenceContext: contextSummary(referenceContext),
      sessionId: sessionIdOverride,
    });

    try {
      const result = await this.sendAndStream(input, editorContext, response, token, request.toolInvocationToken, sessionIdOverride);
      this.telemetry.event("chat.request.end", {
        source,
        command,
        failed: result.failed,
        durationMs: Date.now() - startedAt,
        sessionId: result.event.sessionId,
        turnId: result.event.turnId,
        stopReason: result.event.stopReason,
      });
      if (result.failed) {
        return { errorDetails: { message: result.event.error || "Contenox request failed" } };
      }
      return { metadata: { sessionId: result.event.sessionId, turnId: result.event.turnId } };
    } catch (error) {
      this.telemetry.error("chat.request.error", error, {
        source,
        command,
        durationMs: Date.now() - startedAt,
      });
      this.output.error(`Contenox chat request failed: ${errorMessage(error)}`);
      response.markdown(`Contenox error: ${errorMessage(error)}`);
      return { errorDetails: { message: errorMessage(error) } };
    }
  }

  private async sendAndStream(
    input: string,
    editorContext: EditorContextAttachment[],
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
    toolInvocationToken: vscode.ChatParticipantToolToken | undefined,
    sessionIdOverride?: string,
  ) {
    try {
      const runOptions = {
        sessionId: sessionIdOverride ?? this.sessionId,
        input,
        context: editorContext,
        token,
        reuseExistingSession: sessionIdOverride ? false : true,
        createSessionName: sessionTitleFromInput(input),
      };
      const handlers = {
        onStarted: () => response.progress("Contenox started"),
        onDelta: (event: ChatDeltaEvent) => {
          if (event.thinking) {
            response.progress("Thinking...");
          }
          if (event.content) {
            response.markdown(event.content);
          }
        },
        onToolCall: (event: ToolCallEvent) => this.renderToolCall(event, response),
        onApprovalRequested: (client: BridgeClient, event: ApprovalRequestedEvent) => {
          void this.handleApproval(client, event, response, toolInvocationToken, token);
        },
        onPermissionRequested: (_client: BridgeClient, event: RequestPermissionParams) =>
          this.handlePermissionRequest(event, response, toolInvocationToken, token),
        onCompletedWithoutDelta: (_event: ChatLifecycleEvent, content: string | undefined) => {
          if (content) {
            response.markdown(content);
          }
        },
      };
      let result: TurnResult;
      try {
        result = await this.turns.run(runOptions, handlers);
      } catch (error) {
        if (!sessionIdOverride && runOptions.sessionId && isSessionNotFound(error)) {
          this.telemetry.warn("chat.session.stale_cleared", {
            sessionId: runOptions.sessionId,
            error: errorMessage(error),
          });
          this.sessionId = undefined;
          result = await this.turns.run(
            {
              ...runOptions,
              sessionId: undefined,
              reuseExistingSession: true,
            },
            handlers,
          );
        } else {
          throw error;
        }
      }
      if (!sessionIdOverride) {
        this.sessionId = result.event.sessionId;
      }
      if (result.failed) {
        if (result.event.error) {
          response.markdown(`Contenox failed: ${result.event.error}`);
        } else {
          response.markdown("Contenox request cancelled.");
        }
      }
      return result;
    } finally {
      this.onSessionsChanged();
    }
  }

  private async handlePermissionRequest(
    event: RequestPermissionParams,
    response: vscode.ChatResponseStream,
    toolInvocationToken: vscode.ChatParticipantToolToken | undefined,
    token: vscode.CancellationToken,
  ): Promise<RequestPermissionResponse> {
    const approval = approvalEventFromPermissionRequest(event);
    response.progress(`Approval required: ${approval.title}`);
    this.telemetry.event("chat.approval.requested", {
      approvalId: approval.approvalId,
      toolName: approval.toolName,
      title: approval.title,
      optionCount: approval.options.length,
    });
    renderApprovalSummary(approval, response);
    if (!toolInvocationToken) {
      this.telemetry.warn("chat.approval.no_tool_invocation_token", {
        approvalId: approval.approvalId,
        toolName: approval.toolName,
      });
      response.progress(`Denied: ${approval.title}`);
      return { outcome: { outcome: "cancelled" } };
    }

    const approved = await requestNativeApproval(approval, toolInvocationToken, token, this.telemetry);
    const option = selectedPermissionOption(event, approved);
    if (!option) {
      this.telemetry.warn("chat.approval.no_matching_option", {
        approvalId: approval.approvalId,
        approved,
      });
      response.progress(`Denied: ${approval.title}`);
      return { outcome: { outcome: "cancelled" } };
    }

    this.telemetry.event("chat.approval.responded", {
      approvalId: approval.approvalId,
      optionId: option.optionId,
      approved,
    });
    response.progress(`${approved ? "Approved" : "Denied"}: ${approval.title}`);
    return {
      outcome: {
        outcome: "selected",
        optionId: option.optionId,
      },
    };
  }

  private async contextForRequest(command: string): Promise<EditorContextAttachment[]> {
    switch (command) {
      case "fix":
        return collectEditorContext({ includeSelection: true, includeActiveFile: true, includeDiagnostics: true });
      case "explain":
        return collectEditorContext({ includeSelection: true, includeActiveFile: false, includeDiagnostics: false });
      case "review":
      case "commit":
        return collectGitChangeContext();
      default:
        return [];
    }
  }

  private async handleApproval(
    client: BridgeClient,
    event: ApprovalRequestedEvent,
    response: vscode.ChatResponseStream,
    toolInvocationToken: vscode.ChatParticipantToolToken | undefined,
    token: vscode.CancellationToken,
  ): Promise<void> {
    response.progress(`Approval required: ${event.title}`);
    this.telemetry.event("chat.approval.requested", {
      approvalId: event.approvalId,
      toolName: event.toolName,
      title: event.title,
      optionCount: event.options.length,
    });
    renderApprovalSummary(event, response);
    const approved = await requestNativeApproval(event, toolInvocationToken, token, this.telemetry);
    const option = approved
      ? event.options.find((candidate) => candidate.id === "allow" || candidate.kind.startsWith("approve")) ?? event.options[0]
      : event.options.find((candidate) => candidate.id === "deny" || candidate.kind.startsWith("reject")) ??
        event.options[event.options.length - 1];
    await client.approvalRespond({
      approvalId: event.approvalId,
      optionId: option?.id ?? (approved ? "allow" : "deny"),
      approved,
    });
    this.telemetry.event("chat.approval.responded", {
      approvalId: event.approvalId,
      optionId: option?.id ?? (approved ? "allow" : "deny"),
      approved,
    });
    response.progress(`${approved ? "Approved" : "Denied"}: ${event.title}`);
  }

  private renderToolCall(event: ToolCallEvent, response: vscode.ChatResponseStream): void {
    response.progress(toolProgress(event));
    const diff = this.diffStore.registerToolDiff(event);
    if (!diff) {
      return;
    }
    this.telemetry.event("chat.tool.diff", {
      sessionId: event.sessionId,
      turnId: event.turnId,
      toolName: event.toolName,
      status: event.status,
      diffId: diff.id,
      hasFile: Boolean(diff.fileUri),
    });
    if (diff.fileUri) {
      response.reference(diff.fileUri);
    }
    response.button({
      command: "contenox.openToolDiff",
      title: "Open Diff",
      arguments: [diff],
    });
  }

  private async openNativeChat(query: string, submit: boolean): Promise<void> {
    this.telemetry.event("chat.open_native", { submit, queryChars: query.length });
    try {
      await vscode.commands.executeCommand("workbench.action.chat.open", {
        query,
        isPartialQuery: !submit,
        mode: "ask",
      });
    } catch (error) {
      this.telemetry.error("chat.open_native.failed", error, { submit });
      await vscode.env.clipboard.writeText(query);
      vscode.window.showInformationMessage("Open VS Code Chat and paste the Contenox prompt. It has been copied to the clipboard.");
    }
  }

  private setPendingContext(context: EditorContextAttachment[]): void {
    this.pendingContext = {
      context,
      expiresAt: Date.now() + pendingContextTtlMs,
    };
    this.telemetry.event("chat.pending_context.set", { context: contextSummary(context) });
  }

  private takePendingContext(): EditorContextAttachment[] | undefined {
    const pending = this.pendingContext;
    this.pendingContext = undefined;
    if (!pending || pending.expiresAt < Date.now()) {
      return undefined;
    }
    this.telemetry.event("chat.pending_context.used", { context: contextSummary(pending.context) });
    return pending.context;
  }
}

export function approvalEventFromPermissionRequest(event: RequestPermissionParams): ApprovalRequestedEvent {
	const toolCall = event.toolCall;
	const meta = { ...(event._meta ?? {}), ...(toolCall._meta ?? {}) };
	const diff = firstDiffContent(toolCall.content);
	const args = objectRecord(toolCall.rawInput);
	const rawDiffOld = stringValue(meta.diffOld) ?? stringValue(diff?.oldText);
	const rawDiffNew = stringValue(meta.diffNew) ?? stringValue(diff?.newText);
	const hasContentDiff = isNonBlank(rawDiffOld) || isNonBlank(rawDiffNew);
	return {
		approvalId: toolCall.toolCallId,
		toolsName: stringValue(meta.toolsName),
		toolName: stringValue(meta.toolName),
		title: nonBlankString(toolCall.title) ?? toolCall.toolCallId,
		policyName: stringValue(meta.policyName),
		policyPath: stringValue(meta.policyPath),
		args,
		diff: nonBlankString(meta.diff),
		diffOld: hasContentDiff ? rawDiffOld ?? "" : undefined,
		diffNew: hasContentDiff ? rawDiffNew ?? "" : undefined,
		options: event.options.map((option) => ({
			id: option.optionId,
			label: option.name,
      kind: option.kind,
    })),
  };
}

function selectedPermissionOption(event: RequestPermissionParams, approved: boolean) {
  if (approved) {
    return event.options.find((candidate) => candidate.optionId === "allow" || candidate.kind.startsWith("allow")) ?? event.options[0];
  }
  return (
    event.options.find((candidate) => candidate.optionId === "deny" || candidate.kind.startsWith("reject")) ??
    event.options[event.options.length - 1]
  );
}

function firstDiffContent(content: readonly ToolCallContent[] | undefined): ToolCallContent | undefined {
	return content?.find((entry) => entry.type === "diff" && (isNonBlank(entry.oldText) || isNonBlank(entry.newText)));
}

function objectRecord(value: unknown): Record<string, unknown> | undefined {
	if (typeof value === "string") {
		try {
			const parsed = JSON.parse(value) as unknown;
			return objectRecord(parsed);
		} catch {
			return undefined;
		}
	}
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return undefined;
	}
	return value as Record<string, unknown>;
}

function stringValue(value: unknown): string | undefined {
	return typeof value === "string" ? value : undefined;
}

function nonBlankString(value: unknown): string | undefined {
	const str = stringValue(value);
	return isNonBlank(str) ? str : undefined;
}

function isNonBlank(value: unknown): value is string {
	return typeof value === "string" && value.trim().length > 0;
}

function renderApprovalSummary(event: ApprovalRequestedEvent, response: vscode.ChatResponseStream): void {
  response.markdown(approvalSummaryMarkdown(event));
}

function approvalSummaryMarkdown(event: ApprovalRequestedEvent): string {
  const sections = [`**Approval required:** \`${escapeInlineCode(event.title)}\``];
  if (event.policyName || event.policyPath) {
    const policy = event.policyName ?? "active HITL policy";
    sections.push(`**Policy:** \`${escapeInlineCode(policy)}\`${event.policyPath ? `\n\n\`${escapeInlineCode(event.policyPath)}\`` : ""}`);
  }
  if (event.args && Object.keys(event.args).length > 0) {
    sections.push(`**Input**\n\n${codeBlock(JSON.stringify(event.args, null, 2), "json", 4000)}`);
  }
  if (event.diff) {
    sections.push(`**Proposed change**\n\n${codeBlock(event.diff, "diff", 12000)}`);
  } else if (event.diffOld || event.diffNew) {
    sections.push(`**Current content**\n\n${codeBlock(event.diffOld ?? "", "", 6000)}`);
    sections.push(`**Proposed content**\n\n${codeBlock(event.diffNew ?? "", "", 6000)}`);
  }
  return `${sections.join("\n\n")}\n\n`;
}

function codeBlock(value: string, language: string, maxChars: number): string {
  return `\`\`\`${language}\n${truncateForChat(value, maxChars).replace(/```/g, "``\\`")}\n\`\`\``;
}

function escapeInlineCode(value: string): string {
  return value.replace(/[`\\]/g, "\\$&");
}

function truncateForChat(value: string, maxChars: number): string {
  if (value.length <= maxChars) {
    return value;
  }
  return `${value.slice(0, maxChars)}\n... truncated ...`;
}

async function contextFromReferences(
  references: readonly vscode.ChatPromptReference[],
  telemetry: TelemetryLogger,
): Promise<EditorContextAttachment[]> {
  if (references.length === 0) {
    return [];
  }
  const out: EditorContextAttachment[] = [];
  for (const reference of references) {
    try {
      const item = await contextFromReference(reference);
      if (item) {
        out.push(item);
      }
    } catch (error) {
      telemetry.warn("chat.reference_context.skipped", {
        id: reference.id,
        valueType: referenceValueType(reference.value),
        error: errorMessage(error),
      });
    }
  }
  telemetry.event("chat.reference_context.collected", {
    referenceCount: references.length,
    context: contextSummary(out),
  });
  return out;
}

async function contextFromReference(reference: vscode.ChatPromptReference): Promise<EditorContextAttachment | undefined> {
  const location = locationFromReferenceValue(reference.value);
  if (location) {
    const document = await vscode.workspace.openTextDocument(location.uri);
    const hasRange = !location.range.isEmpty;
    const content = hasRange ? document.getText(location.range) : document.getText();
    if (stringsIsBlank(content)) {
      return undefined;
    }
    return {
      kind: hasRange ? "selection" : "active_file",
      uri: document.uri.toString(),
      languageId: document.languageId,
      content,
    };
  }

  const uri = uriFromReferenceValue(reference.value);
  if (uri) {
    const document = await vscode.workspace.openTextDocument(uri);
    const content = document.getText();
    if (stringsIsBlank(content)) {
      return undefined;
    }
    return {
      kind: "active_file",
      uri: document.uri.toString(),
      languageId: document.languageId,
      content,
    };
  }

  if (typeof reference.value === "string" && !stringsIsBlank(reference.value)) {
    return {
      kind: "reference",
      content: reference.value,
    };
  }

  if (typeof reference.modelDescription === "string" && !stringsIsBlank(reference.modelDescription)) {
    return {
      kind: "reference",
      content: reference.modelDescription,
    };
  }

  return undefined;
}

function locationFromReferenceValue(value: unknown): vscode.Location | undefined {
  if (value instanceof vscode.Location) {
    return value;
  }
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const maybe = value as { uri?: unknown; range?: unknown };
  const uri = uriFromReferenceValue(maybe.uri);
  const range = rangeFromUnknown(maybe.range);
  if (!uri || !range) {
    return undefined;
  }
  return new vscode.Location(uri, range);
}

function uriFromReferenceValue(value: unknown): vscode.Uri | undefined {
  if (value instanceof vscode.Uri) {
    return value;
  }
  if (typeof value === "string") {
    try {
      return vscode.Uri.parse(value);
    } catch {
      return undefined;
    }
  }
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const maybe = value as { scheme?: unknown; path?: unknown; fsPath?: unknown; external?: unknown; toString?: unknown };
  if (typeof maybe.scheme === "string" && typeof maybe.path === "string") {
    try {
      return vscode.Uri.from({
        scheme: maybe.scheme,
        path: maybe.path,
      });
    } catch {
      return undefined;
    }
  }
  if (typeof maybe.fsPath === "string") {
    return vscode.Uri.file(maybe.fsPath);
  }
  if (typeof maybe.external === "string") {
    try {
      return vscode.Uri.parse(maybe.external);
    } catch {
      return undefined;
    }
  }
  return undefined;
}

function rangeFromUnknown(value: unknown): vscode.Range | undefined {
  if (value instanceof vscode.Range) {
    return value;
  }
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const maybe = value as { start?: unknown; end?: unknown };
  const start = positionFromUnknown(maybe.start);
  const end = positionFromUnknown(maybe.end);
  if (!start || !end) {
    return undefined;
  }
  return new vscode.Range(start, end);
}

function positionFromUnknown(value: unknown): vscode.Position | undefined {
  if (value instanceof vscode.Position) {
    return value;
  }
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const maybe = value as { line?: unknown; character?: unknown };
  if (typeof maybe.line !== "number" || typeof maybe.character !== "number") {
    return undefined;
  }
  return new vscode.Position(maybe.line, maybe.character);
}

function mergeEditorContext(context: EditorContextAttachment[]): EditorContextAttachment[] {
  const seen = new Set<string>();
  const out: EditorContextAttachment[] = [];
  for (const item of context) {
    if (stringsIsBlank(item.content)) {
      continue;
    }
    const key = `${item.kind}\x00${item.uri ?? ""}\x00${item.languageId ?? ""}\x00${item.content}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(item);
  }
  return out;
}

function referenceValueType(value: unknown): string {
  if (value instanceof vscode.Location) {
    return "Location";
  }
  if (value instanceof vscode.Uri) {
    return "Uri";
  }
  if (value === null) {
    return "null";
  }
  if (Array.isArray(value)) {
    return "array";
  }
  return typeof value;
}

function stringsIsBlank(value: string): boolean {
  return value.trim().length === 0;
}

function inputForRequest(command: string, prompt: string): string {
  switch (command) {
    case "fix":
      return prompt ? `Fix this diagnostic: ${prompt}` : "Fix the diagnostics in the active file.";
    case "explain":
      return prompt ? `Explain this code: ${prompt}` : "Explain the selected code.";
    case "compact":
    case "doctor":
    case "help":
    case "policy":
    case "websearch":
      return `/${command}${prompt ? ` ${prompt}` : ""}`;
    case "review":
      return prompt ? `Review the current git changes with this focus: ${prompt}` : "Review the current git changes.";
    case "commit":
      return prompt ? `Draft a commit message for the current git changes with this focus: ${prompt}` : "Draft a commit message for the current git changes.";
    default:
      return prompt || "/help";
  }
}

function toolProgress(event: ToolCallEvent): string {
  const title = event.title || event.toolName || event.taskId || "tool";
  if (event.status === "failed") {
    return `Tool failed: ${title}`;
  }
  if (event.status === "completed") {
    return `Tool completed: ${title}`;
  }
  return `Tool ${event.status}: ${title}`;
}

function pushEditorReferences(response: vscode.ChatResponseStream, context: readonly EditorContextAttachment[]): void {
  const seen = new Set<string>();
  for (const item of context) {
    if (!item.uri || seen.has(item.uri)) {
      continue;
    }
    seen.add(item.uri);
    try {
      response.reference(vscode.Uri.parse(item.uri));
    } catch {
      // Editor context references are best-effort metadata for the chat UI.
    }
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function isSessionNotFound(error: unknown): boolean {
  return /^session ".+" not found$/.test(errorMessage(error));
}
