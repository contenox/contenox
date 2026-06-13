import * as vscode from "vscode";
import { diagnoseAgentSessions, openAgentSession, registerAgentSessions } from "./agentSessions/provider";
import { registerAutocomplete, testAutocompleteAtCursor } from "./autocomplete/provider";
import { AutocompleteStatus } from "./autocomplete/status";
import { bridgeCommandArgs, BridgeProcess } from "./bridge/BridgeProcess";
import { ContenoxChatParticipant } from "./chat/participant";
import { SessionDocumentProvider, sessionDocumentScheme } from "./chat/SessionDocumentProvider";
import { SessionTreeProvider } from "./chat/SessionTreeProvider";
import { registerDiagnosticCodeActions } from "./codeActions/diagnostics";
import { registerApprovalTool } from "./approval/nativeTool";
import { RuntimeControlsViewProvider } from "./config/RuntimeControlsView";
import {
  selectAutocompleteModel,
  selectAutocompleteProvider,
  selectChatModel,
  selectHitlPolicy,
  selectProvider,
  selectThinkLevel,
} from "./config/selectors";
import { readBridgeSettings } from "./config/settings";
import { DiffStore, OpenDiffArgs, StoredDiff } from "./editor/diffStore";
import { registerLanguageModelProvider, testLanguageModelProvider } from "./lm/provider";
import { ContenoxOutput } from "./logging/output";
import { TelemetryLogger } from "./logging/telemetry";
import { MCPServerProviderRegistration, registerMCPServerProvider, showMCPServers } from "./mcp/provider";
import { ContenoxStatusBar } from "./status/statusBar";

export function activate(context: vscode.ExtensionContext): void {
  const output = new ContenoxOutput();
  const telemetry = new TelemetryLogger(extensionVersion(context));
  const status = new ContenoxStatusBar();
  const autocompleteStatus = new AutocompleteStatus();
  const bridge = new BridgeProcess(output, status, extensionVersion(context), context.extensionUri, telemetry);
  const sessions = new SessionTreeProvider(bridge);
  const runtimeControls = new RuntimeControlsViewProvider(bridge, telemetry, () => {
    sessions.refresh();
  });
  const sessionDocuments = new SessionDocumentProvider(telemetry);
  const diffStore = new DiffStore(telemetry);
  const chat = new ContenoxChatParticipant(context, bridge, diffStore, output, telemetry, () => sessions.refresh());
  const mcpProvider = registerMCPServerProvider(bridge, telemetry);

  context.subscriptions.push(
    output,
    telemetry,
    status,
    autocompleteStatus,
    bridge,
    chat,
    diffStore,
    sessions,
    runtimeControls,
    sessionDocuments,
    vscode.workspace.registerTextDocumentContentProvider("contenox-diff", diffStore),
    vscode.workspace.registerTextDocumentContentProvider(sessionDocumentScheme(), sessionDocuments),
    registerLanguageModelProvider(bridge, output, telemetry),
    registerAgentSessions(context, bridge, chat, telemetry, () => sessions.refresh()),
    mcpProvider,
    registerAutocomplete(bridge, output, telemetry),
    registerDiagnosticCodeActions(telemetry),
    registerApprovalTool(telemetry),
  );
  context.subscriptions.push(vscode.window.registerWebviewViewProvider("contenox.controls", runtimeControls));
  context.subscriptions.push(vscode.window.registerTreeDataProvider("contenox.sessions", sessions));
  context.subscriptions.push(vscode.workspace.onDidChangeConfiguration((event) => {
    if (
      event.affectsConfiguration("contenox.autocomplete") ||
      event.affectsConfiguration("contenox.autocompleteProvider") ||
      event.affectsConfiguration("contenox.autocompleteModel")
    ) {
      autocompleteStatus.update();
    }
  }));
  context.subscriptions.push(
    vscode.commands.registerCommand("contenox.openChat", () => chat.openChat()),
    vscode.commands.registerCommand("contenox.openWalkthrough", () => openWalkthrough(telemetry)),
    vscode.commands.registerCommand("contenox.openAgentSession", () => openAgentSession(telemetry)),
    vscode.commands.registerCommand("contenox.diagnoseAgentSessions", () => diagnoseAgentSessions(context, telemetry)),
    vscode.commands.registerCommand("contenox.askSelection", () => chat.askSelection()),
    vscode.commands.registerCommand("contenox.fixSelection", () => chat.fixSelection()),
    vscode.commands.registerCommand("contenox.addSelectionToChat", () => chat.addSelectionToChat()),
    vscode.commands.registerCommand("contenox.fixDiagnostics", (diagnostics?: readonly vscode.Diagnostic[]) => chat.fixDiagnostics(diagnostics)),
    vscode.commands.registerCommand("contenox.explainDiagnostics", (diagnostics?: readonly vscode.Diagnostic[]) =>
      chat.explainDiagnostics(diagnostics),
    ),
    vscode.commands.registerCommand("contenox.reviewChanges", () => chat.reviewChanges()),
    vscode.commands.registerCommand("contenox.draftCommitMessage", () => chat.draftCommitMessage()),
    vscode.commands.registerCommand("contenox.refreshSessions", () => sessions.refresh()),
    vscode.commands.registerCommand("contenox.openSession", (arg?: unknown) =>
      openSession(bridge, chat, sessionDocuments, sessions, telemetry, arg),
    ),
    vscode.commands.registerCommand("contenox.deleteSession", (arg?: unknown) => deleteSession(bridge, chat, sessions, telemetry, arg)),
    vscode.commands.registerCommand("contenox.showStatus", () => showStatus(bridge, output, telemetry)),
    vscode.commands.registerCommand("contenox.restartBridge", () => restartBridge(bridge, output, telemetry)),
    vscode.commands.registerCommand("contenox.runSetup", () => runSetup(bridge, telemetry)),
    vscode.commands.registerCommand("contenox.selectProvider", () =>
      runConfigSelector(() => selectProvider(bridge), sessions, runtimeControls),
    ),
    vscode.commands.registerCommand("contenox.selectModel", () =>
      runConfigSelector(() => selectChatModel(bridge), sessions, runtimeControls),
    ),
    vscode.commands.registerCommand("contenox.selectChatModel", () =>
      runConfigSelector(() => selectChatModel(bridge), sessions, runtimeControls),
    ),
    vscode.commands.registerCommand("contenox.selectAutocompleteProvider", () => selectAutocompleteProvider(bridge)),
    vscode.commands.registerCommand("contenox.selectAutocompleteModel", () => selectAutocompleteModel(bridge)),
    vscode.commands.registerCommand("contenox.selectHitlPolicy", () =>
      runConfigSelector(() => selectHitlPolicy(bridge), sessions, runtimeControls),
    ),
    vscode.commands.registerCommand("contenox.selectThinkLevel", () =>
      runConfigSelector(() => selectThinkLevel(bridge), sessions, runtimeControls),
    ),
    vscode.commands.registerCommand("contenox.triggerAutocomplete", () => vscode.commands.executeCommand("editor.action.inlineSuggest.trigger")),
    vscode.commands.registerCommand("contenox.testAutocompleteAtCursor", () => testAutocompleteAtCursor(bridge, output, telemetry)),
    vscode.commands.registerCommand("contenox.enableAutocomplete", () => setAutocompleteEnabled(true, autocompleteStatus)),
    vscode.commands.registerCommand("contenox.disableAutocomplete", () => setAutocompleteEnabled(false, autocompleteStatus)),
    vscode.commands.registerCommand("contenox.toggleAutocomplete", () => toggleAutocomplete(autocompleteStatus)),
    vscode.commands.registerCommand("contenox.acceptAutocomplete", () => output.info("Contenox autocomplete accepted")),
    vscode.commands.registerCommand("contenox.showOutput", () => output.show()),
    vscode.commands.registerCommand("contenox.showTelemetryLog", () => telemetry.show()),
    vscode.commands.registerCommand("contenox.clearTelemetryLog", () => telemetry.clear()),
    vscode.commands.registerCommand("contenox.testLanguageModelProvider", () => testLanguageModelProvider(output, telemetry)),
    vscode.commands.registerCommand("contenox.showMCPServers", () => showMCPServers(bridge, output, telemetry)),
    vscode.commands.registerCommand("contenox.refreshMCPServers", () => refreshMCPServers(mcpProvider)),
    vscode.commands.registerCommand("contenox.openToolDiff", (arg?: OpenDiffArgs | StoredDiff) => openToolDiff(diffStore, arg)),
  );

  if (readBridgeSettings().startOnActivation) {
    void bridge.ensureStarted().catch((error) => {
      output.warn(errorMessage(error));
    });
  }
}

function openWalkthrough(telemetry: TelemetryLogger): Thenable<unknown> {
  telemetry.event("command.open_walkthrough");
  return vscode.commands.executeCommand("workbench.action.openWalkthrough", "contenox.runtime#getStarted", false);
}

export function deactivate(): void {
  // VS Code disposes extension subscriptions after deactivate returns.
}

async function showStatus(bridge: BridgeProcess, output: ContenoxOutput, telemetry: TelemetryLogger): Promise<void> {
  try {
    await bridge.ensureStarted();
    const health = await bridge.refreshHealth();
    const provider = health.defaultProvider || "no provider";
    const model = health.defaultModel || "no model";
    telemetry.event("command.show_status", {
      status: health.status,
      configured: health.configured,
      provider,
      model,
    });
    vscode.window.showInformationMessage(`Contenox ${health.status}: ${provider} / ${model}`);
  } catch (error) {
    telemetry.error("command.show_status.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

async function restartBridge(bridge: BridgeProcess, output: ContenoxOutput, telemetry: TelemetryLogger): Promise<void> {
  try {
    telemetry.event("command.restart_bridge");
    const state = await bridge.restart();
    const provider = state.health.defaultProvider || "no provider";
    const model = state.health.defaultModel || "no model";
    vscode.window.showInformationMessage(`Contenox bridge restarted: ${provider} / ${model}`);
  } catch (error) {
    telemetry.error("command.restart_bridge.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

function runSetup(bridge: BridgeProcess, telemetry: TelemetryLogger): void {
  const settings = readBridgeSettings();
  const binary = bridge.commandBinaryPath();
  const args = bridgeCommandArgs(settings.dataDir, "setup");
  telemetry.event("command.run_setup", {
    binary,
    args,
    cwd: bridge.commandCwd(),
    dataDir: settings.dataDir,
  });
  const terminal = vscode.window.createTerminal({
    name: "Contenox Setup",
    cwd: bridge.commandCwd(),
  });
  terminal.show();
  terminal.sendText([shellQuote(binary), ...args.map(shellQuote)].join(" "));
}

async function toggleAutocomplete(status: AutocompleteStatus): Promise<void> {
  const enabled = !vscode.workspace.getConfiguration("contenox").get<boolean>("autocomplete.enabled", true);
  await setAutocompleteEnabled(enabled, status);
}

async function setAutocompleteEnabled(enabled: boolean, status: AutocompleteStatus): Promise<void> {
  await vscode.workspace
    .getConfiguration("contenox")
    .update("autocomplete.enabled", enabled, vscode.ConfigurationTarget.Workspace);
  status.update();
  vscode.window.showInformationMessage(`Contenox autocomplete ${enabled ? "enabled" : "disabled"}`);
}

async function openSession(
  bridge: BridgeProcess,
  chat: ContenoxChatParticipant,
  sessionDocuments: SessionDocumentProvider,
  sessions: SessionTreeProvider,
  telemetry: TelemetryLogger,
  arg: unknown,
): Promise<void> {
  const sessionId = sessionIdFromArg(arg);
  if (!sessionId) {
    await chat.openSession();
    return;
  }
  telemetry.event("command.open_session", { sessionId });
  const state = await bridge.ensureStarted();
  if (!state.initialize.capabilities.sessionList) {
    throw new Error("Bridge does not support session loading");
  }
  const client = bridge.currentClient;
  if (!client) {
    throw new Error("Bridge client is not available");
  }
  const result = await client.sessionLoad({ sessionId });
  chat.setActiveSession(result.session.id);
  await sessionDocuments.open(result);
  sessions.refresh();
  vscode.window.showInformationMessage(`Loaded Contenox session: ${result.session.name || result.session.id}`);
}

async function runConfigSelector(
  action: () => Promise<string | undefined>,
  sessions: SessionTreeProvider,
  runtimeControls: RuntimeControlsViewProvider,
): Promise<void> {
  const selected = await action();
  if (selected !== undefined) {
    sessions.refresh();
    await runtimeControls.refresh();
  }
}

async function deleteSession(
  bridge: BridgeProcess,
  chat: ContenoxChatParticipant,
  sessions: SessionTreeProvider,
  telemetry: TelemetryLogger,
  arg: unknown,
): Promise<void> {
  const sessionId = sessionIdFromArg(arg);
  if (!sessionId) {
    return;
  }
  const choice = await vscode.window.showWarningMessage("Delete this Contenox session?", { modal: true }, "Delete");
  if (choice !== "Delete") {
    return;
  }
  telemetry.event("command.delete_session", { sessionId });
  const state = await bridge.ensureStarted();
  if (!state.initialize.capabilities.sessionList) {
    throw new Error("Bridge does not support session deletion");
  }
  const client = bridge.currentClient;
  if (!client) {
    throw new Error("Bridge client is not available");
  }
  await client.sessionDelete({ sessionId });
  chat.clearActiveSession(sessionId);
  sessions.refresh();
}

async function openToolDiff(diffStore: DiffStore, arg: OpenDiffArgs | StoredDiff | undefined): Promise<void> {
  if (!arg) {
    vscode.window.showWarningMessage("No Contenox diff is available to open.");
    return;
  }
  await diffStore.open(arg);
}

function refreshMCPServers(provider: MCPServerProviderRegistration): void {
  provider.refresh();
  vscode.window.showInformationMessage("Contenox MCP server definitions refreshed.");
}

function sessionIdFromArg(arg: unknown): string | undefined {
  if (typeof arg === "string") {
    return arg;
  }
  if (arg && typeof arg === "object") {
    const maybe = arg as { session?: { id?: unknown }; id?: unknown };
    if (typeof maybe.session?.id === "string") {
      return maybe.session.id;
    }
    if (typeof maybe.id === "string") {
      return maybe.id;
    }
  }
  return undefined;
}

function extensionVersion(context: vscode.ExtensionContext): string {
  const version = (context.extension.packageJSON as { version?: unknown }).version;
  return typeof version === "string" ? version : "0.0.0";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function shellQuote(value: string): string {
  if (/^[A-Za-z0-9_./:=@%+-]+$/.test(value)) {
    return value;
  }
  return `'${value.replace(/'/g, "'\\''")}'`;
}
