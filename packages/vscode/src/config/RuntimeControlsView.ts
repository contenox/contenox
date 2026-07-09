import * as vscode from "vscode";
import { BridgeProcess } from "../bridge/BridgeProcess";
import { ConfigSnapshot, HitlPolicyInfo, ModelInfo, ProviderInfo } from "../bridge/protocol";
import { TelemetryLogger } from "../logging/telemetry";

type RuntimeControlMessage =
  | { type: "refresh" }
  | { type: "setProvider"; value: string }
  | { type: "setModel"; value: string }
  | { type: "setThink"; value: string }
  | { type: "setHitl"; value: string };

interface RuntimeControlsState {
  config: ConfigSnapshot;
  providers: ProviderInfo[];
  models: ModelInfo[];
  policies: HitlPolicyInfo[];
}

const thinkLevels = ["auto", "off", "minimal", "low", "medium", "high", "xhigh"];

export class RuntimeControlsViewProvider implements vscode.WebviewViewProvider, vscode.Disposable {
  private view: vscode.WebviewView | undefined;

  public constructor(
    private readonly bridge: BridgeProcess,
    private readonly telemetry: TelemetryLogger,
    private readonly onChanged: () => void,
  ) {}

  public resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = { enableScripts: true };
    view.webview.html = this.renderShell(view.webview);
    view.webview.onDidReceiveMessage((message: RuntimeControlMessage) => {
      void this.handleMessage(message);
    });
    void this.refresh();
  }

  public async refresh(): Promise<void> {
    const view = this.view;
    if (!view) {
      return;
    }
    try {
      const state = await this.loadState();
      await view.webview.postMessage({ type: "state", state });
      this.telemetry.event("runtime_controls.refreshed", {
        providers: state.providers.length,
        models: state.models.length,
        policies: state.policies.length,
      });
    } catch (error) {
      await view.webview.postMessage({ type: "error", message: errorMessage(error) });
      this.telemetry.warn("runtime_controls.refresh.failed", { error: errorMessage(error) });
    }
  }

  public dispose(): void {
    this.view = undefined;
  }

  private async handleMessage(message: RuntimeControlMessage): Promise<void> {
    if (message.type === "refresh") {
      await this.refresh();
      return;
    }

    const client = await requireClient(this.bridge);
    switch (message.type) {
      case "setProvider":
        await client.setConfig({ defaultProvider: message.value });
        break;
      case "setModel":
        await client.setConfig({ defaultModel: message.value });
        break;
      case "setThink":
        await client.setConfig({ defaultThink: message.value });
        break;
      case "setHitl":
        await client.setConfig({ hitlPolicyName: message.value });
        break;
    }

    await this.bridge.refreshHealth();
    this.onChanged();
    await this.refresh();
  }

  private async loadState(): Promise<RuntimeControlsState> {
    const client = await requireClient(this.bridge);
    const config = await client.getConfig();
    const [providers, models, policies] = await Promise.all([
      client.listProviders(),
      client.listModels(config.defaultProvider ? { provider: config.defaultProvider } : undefined),
      client.listHitlPolicies(),
    ]);
    return {
      config,
      providers: providers.providers,
      models: models.models,
      policies: policies.policyFiles?.length
        ? policies.policyFiles
        : policies.policies.map((name) => ({
            name,
            active: name === policies.activePolicyName,
          })),
    };
  }

  private renderShell(webview: vscode.Webview): string {
    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 12px;
      color: var(--vscode-foreground);
      font: var(--vscode-font-weight) var(--vscode-font-size) var(--vscode-font-family);
      background: var(--vscode-sideBar-background, transparent);
    }
    h1 {
      margin: 0 0 12px;
      font-size: 11px;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: var(--vscode-descriptionForeground);
    }
    fieldset {
      margin: 0;
      padding: 0;
      border: 0;
    }
    .row {
      display: grid;
      gap: 4px;
      margin-bottom: 12px;
    }
    label {
      color: var(--vscode-descriptionForeground);
      font-size: 11px;
      font-weight: 500;
      letter-spacing: 0.03em;
      text-transform: uppercase;
    }
    select, button {
      width: 100%;
      min-height: 28px;
      font: inherit;
      color: var(--vscode-dropdown-foreground);
      background: var(--vscode-dropdown-background);
      border: 1px solid var(--vscode-dropdown-border);
      border-radius: 4px;
    }
    select:focus-visible, button:focus-visible {
      outline: 1px solid var(--vscode-focusBorder);
      outline-offset: 1px;
    }
    select:disabled, button:disabled {
      opacity: 0.55;
      cursor: not-allowed;
    }
    button {
      margin-top: 4px;
      color: var(--vscode-button-foreground);
      background: var(--vscode-button-secondaryBackground);
      border-color: var(--vscode-button-border, transparent);
      cursor: pointer;
    }
    button:hover:not(:disabled) {
      background: var(--vscode-button-secondaryHoverBackground);
    }
    .status {
      margin: 10px 0 0;
      color: var(--vscode-descriptionForeground);
      font-size: 11px;
      line-height: 1.4;
      min-height: 15px;
    }
    body.busy select,
    body.busy button:not(#refresh) {
      pointer-events: none;
      opacity: 0.7;
    }
  </style>
</head>
<body>
  <h1>Runtime</h1>
  <fieldset id="controls">
  <div class="row">
    <label for="provider">Provider</label>
    <select id="provider"></select>
  </div>
  <div class="row">
    <label for="model">Model</label>
    <select id="model"></select>
  </div>
  <div class="row">
    <label for="think">Thinking</label>
    <select id="think"></select>
  </div>
  <div class="row">
    <label for="hitl">HITL Policy</label>
    <select id="hitl"></select>
  </div>
  </fieldset>
  <button id="refresh" type="button">Refresh runtime</button>
  <p id="status" class="status">Loading…</p>
  <script nonce="${nonce}">
    const vscode = acquireVsCodeApi();
    const provider = document.getElementById("provider");
    const model = document.getElementById("model");
    const think = document.getElementById("think");
    const hitl = document.getElementById("hitl");
    const status = document.getElementById("status");

    function setBusy(busy) {
      document.body.classList.toggle("busy", busy);
      status.textContent = busy ? "Applying runtime settings…" : "Runtime settings are applied immediately.";
    }

    provider.addEventListener("change", () => { setBusy(true); vscode.postMessage({ type: "setProvider", value: provider.value }); });
    model.addEventListener("change", () => { setBusy(true); vscode.postMessage({ type: "setModel", value: model.value }); });
    think.addEventListener("change", () => { setBusy(true); vscode.postMessage({ type: "setThink", value: think.value }); });
    hitl.addEventListener("change", () => { setBusy(true); vscode.postMessage({ type: "setHitl", value: hitl.value }); });
    document.getElementById("refresh").addEventListener("click", () => vscode.postMessage({ type: "refresh" }));

    window.addEventListener("message", (event) => {
      if (event.data.type === "state") {
        render(event.data.state);
      } else if (event.data.type === "error") {
        status.textContent = event.data.message || "Runtime unavailable";
      }
    });

    function render(state) {
      setOptions(provider, state.providers.map((item) => ({
        value: item.provider,
        label: item.configured ? item.provider : item.provider + " (not configured)"
      })), state.config.defaultProvider || "");
      setOptions(model, state.models.map((item) => ({
        value: item.name,
        label: item.displayName || item.name
      })), state.config.defaultModel || "");
      setOptions(think, ${JSON.stringify(thinkLevels)}.map((level) => ({ value: level, label: level })), state.config.defaultThink || "auto");
      setOptions(hitl, state.policies.map((item) => ({
        value: item.name,
        label: item.active ? item.name + " (active)" : item.name
      })), state.config.hitlPolicyName || "");
      setBusy(false);
    }

    function setOptions(select, options, selectedValue) {
      select.replaceChildren();
      if (!options.length) {
        const option = document.createElement("option");
        option.value = "";
        option.textContent = "not available";
        select.appendChild(option);
        select.disabled = true;
        return;
      }
      select.disabled = false;
      let hasSelected = false;
      for (const item of options) {
        const option = document.createElement("option");
        option.value = item.value;
        option.textContent = item.label;
        option.selected = item.value === selectedValue;
        hasSelected ||= option.selected;
        select.appendChild(option);
      }
      if (selectedValue && !hasSelected) {
        const option = document.createElement("option");
        option.value = selectedValue;
        option.textContent = selectedValue + " (current)";
        option.selected = true;
        select.prepend(option);
      }
    }
  </script>
</body>
</html>`;
  }
}

async function requireClient(bridge: BridgeProcess) {
  await bridge.ensureStarted();
  const client = bridge.currentClient;
  if (!client) {
    throw new Error("Contenox runtime connection is not available");
  }
  return client;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
