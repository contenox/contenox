import * as vscode from "vscode";
import { BridgeProcess } from "../bridge/BridgeProcess";
import { SessionInfo, SessionMessage, SessionResult } from "../bridge/protocol";
import { ContenoxChatParticipant } from "../chat/participant";
import { sessionTitleFromChatInput } from "../chat/sessionTitle";
import { TelemetryLogger } from "../logging/telemetry";

const sessionType = "contenox-agent";
const participantId = "contenox";
const maxHistoryMessages = 80;

type ProposedChat = typeof vscode.chat & {
  createChatSessionItemController?: (
    chatSessionType: string,
    refreshHandler: (token: vscode.CancellationToken) => Thenable<void>,
  ) => ProposedChatSessionItemController;
  registerChatSessionItemProvider?: (chatSessionType: string, provider: ProposedChatSessionItemProvider) => vscode.Disposable;
  registerChatSessionContentProvider?: (
    scheme: string,
    provider: ProposedChatSessionContentProvider,
    defaultChatParticipant: vscode.ChatParticipant,
    capabilities?: ProposedChatSessionCapabilities,
  ) => vscode.Disposable;
};

interface ProposedChatSessionItem {
  readonly resource: vscode.Uri;
  label: string;
  iconPath?: vscode.IconPath;
  description?: string | vscode.MarkdownString;
  badge?: string | vscode.MarkdownString;
  tooltip?: string | vscode.MarkdownString;
  metadata?: { readonly [key: string]: unknown };
}

interface ProposedChatSessionItemCollection {
  replace(items: readonly ProposedChatSessionItem[]): void;
  add(item: ProposedChatSessionItem): void;
}

interface ProposedChatSessionItemController extends vscode.Disposable {
  readonly items: ProposedChatSessionItemCollection;
  createChatSessionItem(resource: vscode.Uri, label: string): ProposedChatSessionItem;
}

interface ProposedChatSessionItemProvider {
  readonly onDidChangeChatSessionItems: vscode.Event<void>;
  readonly onDidCommitChatSessionItem: vscode.Event<{ original: ProposedChatSessionItem; modified: ProposedChatSessionItem }>;
  provideChatSessionItems(token: vscode.CancellationToken): vscode.ProviderResult<ProposedChatSessionItem[]>;
}

interface ProposedChatSessionContentProvider {
  provideChatSessionContent(
    resource: vscode.Uri,
    token: vscode.CancellationToken,
    context: { readonly inputState?: ProposedChatSessionInputState },
  ): Thenable<ProposedChatSession> | ProposedChatSession;
}

interface ProposedChatSession {
  readonly title?: string;
  readonly history: ReadonlyArray<unknown>;
  readonly requestHandler: vscode.ChatRequestHandler | undefined;
}

interface ProposedChatContext extends vscode.ChatContext {
  readonly chatSessionContext?: {
    readonly chatSessionResource?: vscode.Uri | ProposedUriShape;
    readonly chatSessionType?: string;
    readonly isUntitled?: boolean;
  };
}

interface ProposedUriShape {
  readonly scheme?: string;
  readonly authority?: string;
  readonly path?: string;
  readonly query?: string;
  readonly fragment?: string;
}

interface ProposedChatSessionCapabilities {
  supportsInterruptions?: boolean;
}

interface ProposedChatSessionInputState {
  readonly sessionResource?: vscode.Uri;
  groups: readonly ProposedChatSessionProviderOptionGroup[];
}

interface ProposedChatSessionProviderOptionGroup {
  readonly id: string;
  readonly name: string;
  readonly items: readonly ProposedChatSessionProviderOptionItem[];
  readonly selected?: ProposedChatSessionProviderOptionItem;
}

interface ProposedChatSessionProviderOptionItem {
  readonly id: string;
  readonly name: string;
  readonly description?: string;
  readonly default?: boolean;
  readonly locked?: boolean;
  readonly icon?: vscode.ThemeIcon;
}

interface DynamicVscodeApi {
  ChatRequestTurn?: new (...args: unknown[]) => unknown;
  ChatRequestTurn2?: new (...args: unknown[]) => unknown;
  ChatResponseTurn?: new (...args: unknown[]) => unknown;
  ChatResponseTurn2?: new (...args: unknown[]) => unknown;
}

export function registerAgentSessions(
  context: vscode.ExtensionContext,
  bridge: BridgeProcess,
  chatParticipant: ContenoxChatParticipant,
  telemetry: TelemetryLogger,
  onSessionsChanged: () => void,
): vscode.Disposable {
  if (!nativeAgentSessionsEnabled()) {
    telemetry.event("agent_sessions.skipped", { reason: "disabled_by_setting" });
    return disposableFrom([]);
  }

  if (!declaresChatSessionsProposal(context)) {
    telemetry.event("agent_sessions.skipped", { reason: "proposal_not_declared" });
    return disposableFrom([]);
  }

  const chatApi = vscode.chat as ProposedChat;
  if (typeof chatApi.registerChatSessionContentProvider !== "function") {
    telemetry.warn("agent_sessions.skipped", { reason: "api_unavailable", vscodeVersion: vscode.version });
    return disposableFrom([]);
  }

  const disposables: vscode.Disposable[] = [];

  try {
    const provider = new ContenoxAgentSessionProvider(context, bridge, chatParticipant, telemetry, onSessionsChanged);
    disposables.push(provider);

    const sessionAgent = vscode.chat.createChatParticipant(sessionType, (request, chatContext, response, token) =>
      provider.handleParticipantRequest(request, chatContext, response, token),
    );
    sessionAgent.iconPath = vscode.Uri.joinPath(context.extensionUri, "media", "contenox-icon.png");
    disposables.push(sessionAgent);
    telemetry.event("agent_sessions.participant.registered", { id: sessionAgent.id });

    disposables.push(
      chatApi.registerChatSessionContentProvider(
        sessionType,
        provider,
        chatParticipant.participant,
        { supportsInterruptions: true },
      ),
    );
    telemetry.event("agent_sessions.content.registered", { type: sessionType });

    if (typeof chatApi.createChatSessionItemController === "function") {
      const controller = chatApi.createChatSessionItemController(sessionType, (token) => provider.refreshController(token));
      provider.attachController(controller);
      disposables.push(controller);
      telemetry.event("agent_sessions.controller.registered", { type: sessionType, mode: "controller" });
    } else if (typeof chatApi.registerChatSessionItemProvider === "function") {
      disposables.push(chatApi.registerChatSessionItemProvider(sessionType, provider.legacyItemProvider()));
      telemetry.event("agent_sessions.controller.registered", { type: sessionType, mode: "legacy_provider" });
    } else {
      telemetry.warn("agent_sessions.items.skipped", { reason: "item_api_unavailable" });
    }
  } catch (error) {
    telemetry.error("agent_sessions.register.failed", error);
  }

  return disposableFrom(disposables);
}

export async function openAgentSession(telemetry: TelemetryLogger): Promise<void> {
  telemetry.event("agent_sessions.open.command");
  if (!nativeAgentSessionsEnabled()) {
    vscode.window.showInformationMessage(
      "Native Contenox agent sessions are experimental. Enable contenox.experimental.nativeAgentSessions to use them.",
    );
    return;
  }
  for (const command of [
    `workbench.action.chat.openNewSessionSidebar.${sessionType}`,
    `workbench.action.chat.openNewSessionEditor.${sessionType}`,
  ]) {
    try {
      await vscode.commands.executeCommand(command);
      telemetry.event("agent_sessions.open.ok", { command });
      return;
    } catch (error) {
      telemetry.warn("agent_sessions.open.failed", { command, error: errorMessage(error) });
    }
  }

  vscode.window.showWarningMessage(
    "Contenox native agent sessions require the proposed VS Code build path. Run make dev-install-vscode-proposed and launch VS Code with --enable-proposed-api contenox.runtime.",
  );
}

export async function diagnoseAgentSessions(context: vscode.ExtensionContext, telemetry: TelemetryLogger): Promise<void> {
  const chatApi = vscode.chat as ProposedChat;
  const commands = await vscode.commands.getCommands(true);
  const report = {
    declared: declaresChatSessionsProposal(context),
    sessionType,
    contentProviderApi: typeof chatApi.registerChatSessionContentProvider === "function",
    controllerApi: typeof chatApi.createChatSessionItemController === "function",
    itemProviderApi: typeof chatApi.registerChatSessionItemProvider === "function",
    openSidebarCommand: commands.includes(`workbench.action.chat.openNewSessionSidebar.${sessionType}`),
    openEditorCommand: commands.includes(`workbench.action.chat.openNewSessionEditor.${sessionType}`),
    openWithPromptCommand: commands.includes(`workbench.action.chat.openSessionWithPrompt.${sessionType}`),
    vscodeVersion: vscode.version,
  };
  telemetry.event("agent_sessions.diagnose", report);
  vscode.window.showInformationMessage(`Contenox native agent session diagnostics: ${JSON.stringify(report)}`);
}

class ContenoxAgentSessionProvider implements ProposedChatSessionContentProvider, vscode.Disposable {
  private readonly itemChangeEmitter = new vscode.EventEmitter<void>();
  private readonly commitEmitter = new vscode.EventEmitter<{ original: ProposedChatSessionItem; modified: ProposedChatSessionItem }>();
  private readonly resourceSessionIds = new Map<string, string>();
  private readonly staleResources = new Set<string>();
  private controller: ProposedChatSessionItemController | undefined;

  public constructor(
    private readonly context: vscode.ExtensionContext,
    private readonly bridge: BridgeProcess,
    private readonly chatParticipant: ContenoxChatParticipant,
    private readonly telemetry: TelemetryLogger,
    private readonly onSessionsChanged: () => void,
  ) {}

  public attachController(controller: ProposedChatSessionItemController): void {
    this.controller = controller;
    this.telemetry.event("agent_sessions.controller.attached", {
      type: sessionType,
      extensible: Object.isExtensible(controller),
    });
    const source = new vscode.CancellationTokenSource();
    void this.refreshController(source.token).finally(() => source.dispose());
  }

  public legacyItemProvider(): ProposedChatSessionItemProvider {
    return {
      onDidChangeChatSessionItems: this.itemChangeEmitter.event,
      onDidCommitChatSessionItem: this.commitEmitter.event,
      provideChatSessionItems: (token) => this.legacyItems(token),
    };
  }

  public async refreshController(token: vscode.CancellationToken): Promise<void> {
    const controller = this.controller;
    if (!controller) {
      return;
    }
    const sessions = await this.listSessions(token);
    const items = sessions.map((session) => this.itemFromSession(session, controller));
    controller.items.replace(items);
    this.telemetry.event("agent_sessions.items.refreshed", { count: items.length, mode: "controller" });
  }

  public async provideChatSessionContent(
    resource: vscode.Uri,
    token: vscode.CancellationToken,
    _context: { readonly inputState?: ProposedChatSessionInputState },
  ): Promise<ProposedChatSession> {
    const sessionId = this.sessionIdForResource(resource);
    let loaded: SessionResult | undefined;
    if (sessionId) {
      try {
        loaded = await this.loadSession(sessionId, token);
      } catch (error) {
        if (!isSessionNotFound(error)) {
          throw error;
        }
        this.markResourceStale(resource, sessionId, error);
      }
    }
    return {
      title: loaded?.session.name || (sessionId ? "Deleted Contenox session" : "Contenox"),
      history: loaded ? messagesToChatTurns(loaded.messages, this.telemetry) : [],
      requestHandler: (request, _chatContext, response, requestToken) =>
        this.handleSessionRequest(resource, request, response, requestToken),
    };
  }

  public handleParticipantRequest(
    request: vscode.ChatRequest,
    chatContext: vscode.ChatContext,
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
  ): Promise<vscode.ChatResult> {
    const resource = sessionResourceFromChatContext(chatContext);
    if (!resource) {
      this.telemetry.warn("agent_sessions.request.no_resource");
      return this.chatParticipant.handleAgentSessionRequest(undefined, request, response, token);
    }
    return this.handleSessionRequest(resource, request, response, token);
  }

  public dispose(): void {
    this.itemChangeEmitter.dispose();
    this.commitEmitter.dispose();
  }

  private async handleSessionRequest(
    resource: vscode.Uri,
    request: vscode.ChatRequest,
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
  ): Promise<vscode.ChatResult> {
    const sessionId = await this.ensureSessionForResource(resource, request, token);
    return this.chatParticipant.handleAgentSessionRequest(sessionId, request, response, token);
  }

  private async ensureSessionForResource(
    resource: vscode.Uri,
    request: vscode.ChatRequest,
    token: vscode.CancellationToken,
  ): Promise<string> {
    const existing = this.sessionIdForResource(resource);
    if (existing) {
      if (await this.sessionExists(existing, token)) {
        return existing;
      }
      this.markResourceStale(resource, existing, new Error(`session "${existing}" not found`));
    }
    if (token.isCancellationRequested) {
      throw new vscode.CancellationError();
    }

    const client = await this.client(token);
    const created = await client.sessionCreate({ name: sessionTitleFromChatInput(request.command, request.prompt) });
    this.resourceSessionIds.set(resource.toString(), created.session.id);
    this.staleResources.delete(resource.toString());

    if (this.controller) {
      const item = this.itemFromSession(created.session, this.controller);
      this.controller.items.add(item);
    }

    this.itemChangeEmitter.fire();
    this.onSessionsChanged();
    this.telemetry.event("agent_sessions.session.bound", {
      resource: resource.toString(),
      sessionId: created.session.id,
      promptChars: request.prompt.length,
    });
    return created.session.id;
  }

  private async legacyItems(token: vscode.CancellationToken): Promise<ProposedChatSessionItem[]> {
    const sessions = await this.listSessions(token);
    const items = sessions.map((session) => this.itemFromSession(session));
    this.telemetry.event("agent_sessions.items.refreshed", { count: items.length, mode: "legacy_provider" });
    return items;
  }

  private async listSessions(token: vscode.CancellationToken): Promise<SessionInfo[]> {
    const client = await this.client(token);
    const result = await client.sessionList();
    return result.sessions;
  }

  private async loadSession(sessionId: string, token: vscode.CancellationToken) {
    const client = await this.client(token);
    return client.sessionRead({ sessionId });
  }

  private async sessionExists(sessionId: string, token: vscode.CancellationToken): Promise<boolean> {
    try {
      await this.loadSession(sessionId, token);
      return true;
    } catch (error) {
      if (isSessionNotFound(error)) {
        return false;
      }
      throw error;
    }
  }

  private async client(token: vscode.CancellationToken) {
    if (token.isCancellationRequested) {
      throw new vscode.CancellationError();
    }
    const state = await this.bridge.ensureStarted();
    if (!state.initialize.capabilities.sessionList) {
      throw new Error("This Contenox runtime does not support session lists");
    }
    const client = this.bridge.currentClient;
    if (!client) {
      throw new Error("Contenox runtime connection is not available");
    }
    return client;
  }

  private itemFromSession(session: SessionInfo, controller?: ProposedChatSessionItemController): ProposedChatSessionItem {
    const label = session.name || session.id;
    const resource = sessionResource(session.id);
    const item = controller?.createChatSessionItem(resource, label) ?? { resource, label };
    item.label = label;
    item.iconPath = vscode.Uri.joinPath(this.context.extensionUri, "media", "contenox-icon.png");
    item.description = session.isActive ? "active" : `${session.messageCount} messages`;
    item.tooltip = `${label}\n${session.messageCount} messages`;
    item.metadata = { sessionId: session.id };
    return item;
  }

  private sessionIdForResource(resource: vscode.Uri): string | undefined {
    const key = resource.toString();
    return this.resourceSessionIds.get(key) ?? (this.staleResources.has(key) ? undefined : sessionIdFromResource(resource));
  }

  private markResourceStale(resource: vscode.Uri, sessionId: string, error: unknown): void {
    const key = resource.toString();
    this.staleResources.add(key);
    this.resourceSessionIds.delete(key);
    this.telemetry.warn("agent_sessions.session.stale", {
      resource: key,
      sessionId,
      error: errorMessage(error),
    });
  }
}

function messagesToChatTurns(messages: readonly SessionMessage[], telemetry: TelemetryLogger): unknown[] {
  const turns: unknown[] = [];
  const recent = messages.slice(Math.max(0, messages.length - maxHistoryMessages));
  for (const message of recent) {
    if (!message.content?.trim()) {
      continue;
    }
    if (message.role === "user") {
      const turn = requestTurn(message.content, telemetry);
      if (turn) {
        turns.push(turn);
      }
      continue;
    }
    if (message.role === "assistant") {
      const turn = responseTurn(message.content, telemetry);
      if (turn) {
        turns.push(turn);
      }
    }
  }
  return turns;
}

function requestTurn(prompt: string, telemetry: TelemetryLogger): unknown | undefined {
  const api = vscode as unknown as DynamicVscodeApi;
  const Constructor = api.ChatRequestTurn2 ?? api.ChatRequestTurn;
  if (!Constructor) {
    telemetry.warn("agent_sessions.history.request_turn_unavailable");
    return undefined;
  }
  try {
    return new Constructor(prompt, undefined, [], participantId, []);
  } catch (error) {
    telemetry.warn("agent_sessions.history.request_turn_failed", { error: errorMessage(error) });
    return undefined;
  }
}

function responseTurn(content: string, telemetry: TelemetryLogger): unknown | undefined {
  const api = vscode as unknown as DynamicVscodeApi;
  const Constructor = api.ChatResponseTurn2 ?? api.ChatResponseTurn;
  if (!Constructor) {
    telemetry.warn("agent_sessions.history.response_turn_unavailable");
    return undefined;
  }
  try {
    return new Constructor([new vscode.ChatResponseMarkdownPart(content)], {}, participantId);
  } catch (error) {
    telemetry.warn("agent_sessions.history.response_turn_failed", { error: errorMessage(error) });
    return undefined;
  }
}

function sessionResource(sessionId: string): vscode.Uri {
  return vscode.Uri.from({
    scheme: sessionType,
    path: `/session/${encodeURIComponent(sessionId)}`,
  });
}

function sessionIdFromResource(resource: vscode.Uri): string | undefined {
  if (resource.scheme !== sessionType) {
    return undefined;
  }
  const match = /^\/session\/(.+)$/.exec(resource.path);
  if (!match) {
    return undefined;
  }
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return match[1];
  }
}

function sessionResourceFromChatContext(context: vscode.ChatContext): vscode.Uri | undefined {
  const proposed = (context as ProposedChatContext).chatSessionContext;
  if (proposed?.chatSessionType && proposed.chatSessionType !== sessionType) {
    return undefined;
  }
  return uriFromUnknown(proposed?.chatSessionResource);
}

function uriFromUnknown(value: unknown): vscode.Uri | undefined {
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const shape = value as ProposedUriShape;
  if (typeof shape.scheme !== "string") {
    return undefined;
  }
  return vscode.Uri.from({
    scheme: shape.scheme,
    authority: shape.authority ?? "",
    path: shape.path ?? "",
    query: shape.query ?? "",
    fragment: shape.fragment ?? "",
  });
}

function declaresChatSessionsProposal(context: vscode.ExtensionContext): boolean {
  const proposals = (context.extension.packageJSON as { enabledApiProposals?: unknown }).enabledApiProposals;
  return Array.isArray(proposals) && proposals.includes("chatSessionsProvider");
}

function nativeAgentSessionsEnabled(): boolean {
  return vscode.workspace.getConfiguration("contenox").get<boolean>("experimental.nativeAgentSessions", false);
}

function isSessionNotFound(error: unknown): boolean {
  return /^session ".+" not found$/.test(errorMessage(error));
}

function disposableFrom(disposables: vscode.Disposable[]): vscode.Disposable {
  return {
    dispose: () => {
      for (const disposable of disposables.reverse()) {
        disposable.dispose();
      }
    },
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
