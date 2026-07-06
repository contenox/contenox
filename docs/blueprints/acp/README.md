# ACP Blueprints

`contenox acp` serves the Agent Client Protocol over stdio for editor and
desktop clients (Zed, JetBrains, AionUi); `contenox acpx` runs the headless /
untrusted-driver profile. These docs cover getting contenox listed and
installable in ACP clients and related editor-integration paths.

| Doc | Status | What it covers |
| --- | --- | --- |
| [zed-registry.md](zed-registry.md) | plan | Listing contenox on the Zed/ACP registry so an install yields a working, model-wired agent |
| [registry-submission/](registry-submission/README.md) | artifacts | The `agent.json` + icon to copy into an `agentclientprotocol/registry` fork, with validation steps |
| [openide-integration.md](openide-integration.md) | research | OpenIDE (IntelliJ Platform) integration via a native plugin over the existing runtime and stdio bridge |

Related: [`../vscode/acp-permission-bridge.md`](../vscode/acp-permission-bridge.md)
(ACP-shaped permissions inside the VS Code bridge) and the ACP slash-command
surface documented in `docs/contenox-cli.md`.
