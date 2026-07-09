import * as fs from "node:fs";
import * as vscode from "vscode";
import {
  registerAutocomplete,
  testAutocompleteAtCursor,
} from "./autocomplete/provider";
import { AutocompleteStatus } from "./autocomplete/status";
import { bridgeCommandArgs, BridgeProcess } from "./bridge/BridgeProcess";
import { ChatWebviewViewProvider } from "./chat/ChatWebviewViewProvider";
import {
  SessionDocumentProvider,
  sessionDocumentScheme,
} from "./chat/SessionDocumentProvider";
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
import {
  registerLanguageModelProvider,
  testLanguageModelProvider,
} from "./lm/provider";
import { ContenoxOutput } from "./logging/output";
import { TelemetryLogger } from "./logging/telemetry";
import {
  MCPServerProviderRegistration,
  registerMCPServerProvider,
  showMCPServers,
} from "./mcp/provider";
import { setDiagnosticsContext } from "./status/contextKeys";
import { ContenoxStatusBar } from "./status/statusBar";

export function activate(context: vscode.ExtensionContext): void {
  const output = new ContenoxOutput();
  const telemetry = new TelemetryLogger(extensionVersion(context));
  const status = new ContenoxStatusBar();
  const autocompleteStatus = new AutocompleteStatus();
  const bridge = new BridgeProcess(
    output,
    status,
    extensionVersion(context),
    context.extensionUri,
    telemetry,
  );
  const sessions = new SessionTreeProvider(bridge);
  const sessionDocuments = new SessionDocumentProvider(telemetry);
  const diffStore = new DiffStore(telemetry);
  let chatWebview!: ChatWebviewViewProvider;
  const onWorkspaceDataChanged = () => {
    sessions.refresh();
    void chatWebview.refreshRuntimeSummary();
  };
  chatWebview = new ChatWebviewViewProvider(
    bridge,
    diffStore,
    context.extensionUri,
    output,
    telemetry,
    onWorkspaceDataChanged,
  );
  const runtimeControls = new RuntimeControlsViewProvider(bridge, telemetry, onWorkspaceDataChanged);
  const mcpProvider = registerMCPServerProvider(bridge, telemetry);
  telemetry.event(
    "extension.activated",
    collectExtensionRuntimeInfo(context, bridge, telemetry),
  );

  context.subscriptions.push(
    output,
    telemetry,
    status,
    autocompleteStatus,
    bridge,
    chatWebview,
    diffStore,
    sessions,
    runtimeControls,
    sessionDocuments,
    vscode.workspace.registerTextDocumentContentProvider(
      "contenox-diff",
      diffStore,
    ),
    vscode.workspace.registerTextDocumentContentProvider(
      sessionDocumentScheme(),
      sessionDocuments,
    ),
    registerLanguageModelProvider(bridge, output, telemetry),
    mcpProvider,
    registerAutocomplete(bridge, output, telemetry),
    registerDiagnosticCodeActions(telemetry),
    registerApprovalTool(telemetry),
  );
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider(
      "contenox.controls",
      runtimeControls,
    ),
  );
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("contenox.chat", chatWebview),
  );
  context.subscriptions.push(
    vscode.window.registerTreeDataProvider("contenox.sessions", sessions),
  );
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (
        event.affectsConfiguration("contenox.autocomplete") ||
        event.affectsConfiguration("contenox.autocompleteProvider") ||
        event.affectsConfiguration("contenox.autocompleteModel")
      ) {
        autocompleteStatus.update();
      }
    }),
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("contenox.openChat", () =>
      chatWebview.openChat(),
    ),
    // Opens the quick runtime settings picker (also triggered from the chat header chip).
    vscode.commands.registerCommand("contenox.openRuntimeSettings", () =>
      chatWebview.showRuntimeSettingsPicker(),
    ),
    vscode.commands.registerCommand("contenox.openWalkthrough", () =>
      openWalkthrough(telemetry),
    ),
    vscode.commands.registerCommand(
      "contenox.internal.setupComplete",
      () => undefined,
    ),
    vscode.commands.registerCommand("contenox.askSelection", () =>
      chatWebview.askSelection(),
    ),
    vscode.commands.registerCommand("contenox.fixSelection", () =>
      chatWebview.fixSelection(),
    ),
    vscode.commands.registerCommand("contenox.addSelectionToChat", () =>
      chatWebview.addSelectionToChat(),
    ),
    vscode.commands.registerCommand(
      "contenox.fixDiagnostics",
      (diagnostics?: readonly vscode.Diagnostic[]) =>
        chatWebview.fixDiagnostics(diagnostics),
    ),
    vscode.commands.registerCommand(
      "contenox.explainDiagnostics",
      (diagnostics?: readonly vscode.Diagnostic[]) =>
        chatWebview.explainDiagnostics(diagnostics),
    ),
    vscode.commands.registerCommand("contenox.reviewChanges", () =>
      chatWebview.reviewChanges(),
    ),
    vscode.commands.registerCommand("contenox.draftCommitMessage", () =>
      chatWebview.draftCommitMessage(),
    ),
    vscode.commands.registerCommand("contenox.refreshSessions", () =>
      sessions.refresh(),
    ),
    vscode.commands.registerCommand("contenox.openSession", (arg?: unknown) =>
      openSession(bridge, chatWebview, sessions, output, telemetry, arg),
    ),
    vscode.commands.registerCommand("contenox.openSessionTranscript", (arg?: unknown) =>
      openSessionTranscript(bridge, sessionDocuments, output, telemetry, arg),
    ),
    vscode.commands.registerCommand("contenox.deleteSession", (arg?: unknown) =>
      deleteSession(bridge, chatWebview, sessions, output, telemetry, arg),
    ),
    vscode.commands.registerCommand("contenox.showStatus", () =>
      showStatus(bridge, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.showExtensionRuntimeInfo", () =>
      showExtensionRuntimeInfo(context, bridge, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.restartRuntime", () =>
      restartRuntime(bridge, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.restartBridge", () =>
      restartRuntime(bridge, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.runSetup", () =>
      runSetup(bridge, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.selectProvider", () =>
      runConfigSelector("select_provider", () => selectProvider(bridge), sessions, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.selectChatModel", () =>
      runConfigSelector("select_chat_model", () => selectChatModel(bridge), sessions, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.selectAutocompleteProvider", () =>
      selectAutocompleteProvider(bridge),
    ),
    vscode.commands.registerCommand("contenox.selectAutocompleteModel", () =>
      selectAutocompleteModel(bridge),
    ),
    vscode.commands.registerCommand("contenox.selectHitlPolicy", () =>
      runConfigSelector("select_hitl_policy", () => selectHitlPolicy(bridge), sessions, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.selectThinkLevel", () =>
      runConfigSelector("select_think_level", () => selectThinkLevel(bridge), sessions, chatWebview, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.triggerAutocomplete", () =>
      vscode.commands.executeCommand("editor.action.inlineSuggest.trigger"),
    ),
    vscode.commands.registerCommand("contenox.testAutocompleteAtCursor", () =>
      testAutocompleteAtCursor(bridge, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.enableAutocomplete", () =>
      setAutocompleteEnabled(true, autocompleteStatus),
    ),
    vscode.commands.registerCommand("contenox.disableAutocomplete", () =>
      setAutocompleteEnabled(false, autocompleteStatus),
    ),
    vscode.commands.registerCommand("contenox.toggleAutocomplete", () =>
      toggleAutocomplete(autocompleteStatus),
    ),
    vscode.commands.registerCommand("contenox.acceptAutocomplete", () =>
      output.info("Contenox autocomplete accepted"),
    ),
    vscode.commands.registerCommand("contenox.showOutput", () => output.show()),
    vscode.commands.registerCommand("contenox.showTelemetryLog", () =>
      telemetry.show(),
    ),
    vscode.commands.registerCommand("contenox.clearTelemetryLog", () =>
      telemetry.clear(),
    ),
    vscode.commands.registerCommand("contenox.testLanguageModelProvider", () =>
      testLanguageModelProvider(output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.showMCPServers", () =>
      showMCPServers(bridge, output, telemetry),
    ),
    vscode.commands.registerCommand("contenox.refreshMCPServers", () =>
      refreshMCPServers(mcpProvider),
    ),
    vscode.commands.registerCommand(
      "contenox.openToolDiff",
      (arg?: OpenDiffArgs | StoredDiff) => openToolDiff(diffStore, arg),
    ),
  );
  updateDiagnosticsContext();
  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor(() => updateDiagnosticsContext()),
    vscode.languages.onDidChangeDiagnostics((event) => {
      const active = vscode.window.activeTextEditor?.document.uri;
      if (
        !active ||
        event.uris.some((uri) => uri.toString() === active.toString())
      ) {
        updateDiagnosticsContext();
      }
    }),
  );

  if (readBridgeSettings().startOnActivation) {
    void bridge
      .ensureStarted()
      .then(() => chatWebview.refreshRuntimeSummary())
      .catch((error) => {
        output.warn(errorMessage(error));
      });
  }
}

function updateDiagnosticsContext(): void {
  const active = vscode.window.activeTextEditor?.document.uri;
  const hasDiagnostics = Boolean(
    active && vscode.languages.getDiagnostics(active).length > 0,
  );
  void setDiagnosticsContext(hasDiagnostics);
}

function openWalkthrough(telemetry: TelemetryLogger): Thenable<unknown> {
  telemetry.event("command.open_walkthrough");
  return vscode.commands.executeCommand(
    "workbench.action.openWalkthrough",
    "contenox.contenox-runtime#getStarted",
    false,
  );
}

export function deactivate(): void {
  // VS Code disposes extension subscriptions after deactivate returns.
}

async function showStatus(
  bridge: BridgeProcess,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
): Promise<void> {
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
    vscode.window.showInformationMessage(
      `Contenox ${health.status}: ${provider} / ${model}`,
    );
  } catch (error) {
    telemetry.error("command.show_status.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

async function showExtensionRuntimeInfo(
  context: vscode.ExtensionContext,
  bridge: BridgeProcess,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
): Promise<void> {
  const info = collectExtensionRuntimeInfo(context, bridge, telemetry);
  telemetry.event("command.show_extension_runtime_info", info);
  output.info(`Contenox runtime info:\n${JSON.stringify(info, null, 2)}`);
  output.show();
  vscode.window.showInformationMessage(
    `Contenox ${info.extensionVersion} loaded from ${info.extensionPath}. Full details are in the Contenox output.`,
  );
}

async function restartRuntime(
  bridge: BridgeProcess,
  chatWebview: ChatWebviewViewProvider,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
): Promise<void> {
  try {
    telemetry.event("command.restart_runtime");
    const state = await bridge.restart();
    await chatWebview.refreshRuntimeSummary();
    const provider = state.health.defaultProvider || "no provider";
    const model = state.health.defaultModel || "no model";
    vscode.window.showInformationMessage(
      `Contenox runtime restarted: ${provider} / ${model}`,
    );
  } catch (error) {
    telemetry.error("command.restart_runtime.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

function runSetup(
  bridge: BridgeProcess,
  chatWebview: ChatWebviewViewProvider,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
): void {
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
  const closeSubscription = vscode.window.onDidCloseTerminal((closed) => {
    if (closed !== terminal) {
      return;
    }
    closeSubscription.dispose();
    telemetry.event("command.run_setup.terminal_closed", {
      exitCode: terminal.exitStatus?.code,
      reason: terminal.exitStatus?.reason,
    });
    void vscode.commands.executeCommand("contenox.internal.setupComplete");
    void bridge
      .restart()
      .then(async (state) => {
        await chatWebview.refreshRuntimeSummary();
        vscode.window.showInformationMessage(
          `Contenox setup finished. Runtime refreshed: ${state.health.defaultProvider || "no provider"} / ${state.health.defaultModel || "no model"}`,
        );
      })
      .catch((error) => {
        telemetry.error("command.run_setup.refresh_failed", error);
        output.show();
        vscode.window.showErrorMessage(
          `Contenox setup finished, but runtime refresh failed: ${errorMessage(error)}`,
        );
      });
  });
  terminal.show();
  terminal.sendText([shellQuote(binary), ...args.map(shellQuote)].join(" "));
}

async function toggleAutocomplete(status: AutocompleteStatus): Promise<void> {
  const enabled = !vscode.workspace
    .getConfiguration("contenox")
    .get<boolean>("autocomplete.enabled", false);
  await setAutocompleteEnabled(enabled, status);
}

async function setAutocompleteEnabled(
  enabled: boolean,
  status: AutocompleteStatus,
): Promise<void> {
  await vscode.workspace
    .getConfiguration("contenox")
    .update(
      "autocomplete.enabled",
      enabled,
      vscode.ConfigurationTarget.Workspace,
    );
  status.update();
  vscode.window.showInformationMessage(
    `Contenox autocomplete ${enabled ? "enabled" : "disabled"}`,
  );
}

async function openSession(
  bridge: BridgeProcess,
  chatWebview: ChatWebviewViewProvider,
  sessions: SessionTreeProvider,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
  arg: unknown,
): Promise<void> {
  try {
    const sessionId = sessionIdFromArg(arg);
    if (!sessionId) {
      await chatWebview.openChat();
      return;
    }
    telemetry.event("command.open_session", { sessionId });
    const state = await bridge.ensureStarted();
    if (!state.initialize.capabilities.sessionList) {
      throw new Error("This Contenox runtime does not support session loading");
    }
    const client = bridge.currentClient;
    if (!client) {
      throw new Error("Contenox runtime connection is not available");
    }
    const result = await client.sessionLoad({ sessionId });
    await chatWebview.openSession(result.session.id);
    sessions.refresh();
  } catch (error) {
    telemetry.error("command.open_session.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

async function openSessionTranscript(
  bridge: BridgeProcess,
  sessionDocuments: SessionDocumentProvider,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
  arg: unknown,
): Promise<void> {
  try {
    const sessionId = sessionIdFromArg(arg);
    if (!sessionId) {
      vscode.window.showWarningMessage("No Contenox session is selected for a transcript.");
      return;
    }
    telemetry.event("command.open_session_transcript", { sessionId });
    const state = await bridge.ensureStarted();
    if (!state.initialize.capabilities.sessionList) {
      throw new Error("This Contenox runtime does not support session loading");
    }
    const client = bridge.currentClient;
    if (!client) {
      throw new Error("Contenox runtime connection is not available");
    }
    const result = await client.sessionLoad({ sessionId });
    await sessionDocuments.open(result);
  } catch (error) {
    telemetry.error("command.open_session_transcript.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

async function runConfigSelector(
  name: string,
  action: () => Promise<string | undefined>,
  sessions: SessionTreeProvider,
  chatWebview: ChatWebviewViewProvider,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
): Promise<void> {
  try {
    const selected = await action();
    if (selected !== undefined) {
      sessions.refresh();
      await chatWebview.refreshRuntimeSummary();
    }
  } catch (error) {
    telemetry.error(`command.${name}.failed`, error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

async function deleteSession(
  bridge: BridgeProcess,
  chatWebview: ChatWebviewViewProvider,
  sessions: SessionTreeProvider,
  output: ContenoxOutput,
  telemetry: TelemetryLogger,
  arg: unknown,
): Promise<void> {
  try {
    const sessionId = sessionIdFromArg(arg);
    if (!sessionId) {
      return;
    }
    const choice = await vscode.window.showWarningMessage(
      "Delete this Contenox session?",
      { modal: true },
      "Delete",
    );
    if (choice !== "Delete") {
      return;
    }
    telemetry.event("command.delete_session", { sessionId });
    const state = await bridge.ensureStarted();
    if (!state.initialize.capabilities.sessionList) {
      throw new Error(
        "This Contenox runtime does not support session deletion",
      );
    }
    const client = bridge.currentClient;
    if (!client) {
      throw new Error("Contenox runtime connection is not available");
    }
    await client.sessionDelete({ sessionId });
    chatWebview.clearActiveSession(sessionId);
    sessions.refresh();
  } catch (error) {
    telemetry.error("command.delete_session.failed", error);
    output.show();
    vscode.window.showErrorMessage(errorMessage(error));
  }
}

async function openToolDiff(
  diffStore: DiffStore,
  arg: OpenDiffArgs | StoredDiff | undefined,
): Promise<void> {
  if (!arg) {
    vscode.window.showWarningMessage("No Contenox diff is available to open.");
    return;
  }
  await diffStore.open(arg);
}

function refreshMCPServers(provider: MCPServerProviderRegistration): void {
  provider.refresh();
  vscode.window.showInformationMessage(
    "Contenox MCP server definitions refreshed.",
  );
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
  const version = (context.extension.packageJSON as { version?: unknown })
    .version;
  return typeof version === "string" ? version : "0.0.0";
}

function collectExtensionRuntimeInfo(
  context: vscode.ExtensionContext,
  bridge: BridgeProcess,
  telemetry: TelemetryLogger,
): ExtensionRuntimeInfo {
  const sessionTreeProviderPath = vscode.Uri.joinPath(
    context.extensionUri,
    "dist",
    "chat",
    "SessionTreeProvider.js",
  ).fsPath;
  const markers = inspectSessionTreeMarkers(sessionTreeProviderPath);
  return {
    extensionId: context.extension.id,
    extensionVersion: extensionVersion(context),
    extensionPath: context.extensionUri.fsPath,
    extensionMode: extensionModeName(context.extensionMode),
    vscodeVersion: vscode.version,
    uiKind: uiKindName(vscode.env.uiKind),
    remoteName: vscode.env.remoteName || "local",
    workspaceTrusted: vscode.workspace.isTrusted,
    runtimeBinaryPath: bridge.commandBinaryPath(),
    runtimeCwd: bridge.commandCwd() || "",
    telemetryLogPath: telemetry.logPath(),
    sessionTreeProviderPath,
    sessionTreeConfigRowsPresent: markers.present,
    sessionTreeMissingMarkers: markers.missing,
    sessionTreeInspectionError: markers.error,
  };
}

interface ExtensionRuntimeInfo extends Record<string, unknown> {
  extensionId: string;
  extensionVersion: string;
  extensionPath: string;
  extensionMode: string;
  vscodeVersion: string;
  uiKind: string;
  remoteName: string;
  workspaceTrusted: boolean;
  runtimeBinaryPath: string;
  runtimeCwd: string;
  telemetryLogPath: string;
  sessionTreeProviderPath: string;
  sessionTreeConfigRowsPresent: boolean;
  sessionTreeMissingMarkers: string[];
  sessionTreeInspectionError?: string;
}

function inspectSessionTreeMarkers(file: string): {
  present: boolean;
  missing: string[];
  error?: string;
} {
  const markers = ["contenoxSession", "sessionList", "openSession"];
  try {
    const content = fs.readFileSync(file, "utf8");
    const missing = markers.filter((marker) => !content.includes(marker));
    return { present: missing.length === 0, missing };
  } catch (error) {
    return { present: false, missing: markers, error: errorMessage(error) };
  }
}

function extensionModeName(mode: vscode.ExtensionMode): string {
  switch (mode) {
    case vscode.ExtensionMode.Development:
      return "development";
    case vscode.ExtensionMode.Test:
      return "test";
    case vscode.ExtensionMode.Production:
      return "production";
    default:
      return String(mode);
  }
}

function uiKindName(kind: vscode.UIKind): string {
  switch (kind) {
    case vscode.UIKind.Desktop:
      return "desktop";
    case vscode.UIKind.Web:
      return "web";
    default:
      return String(kind);
  }
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
