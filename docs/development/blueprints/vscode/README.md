# VS Code Blueprints

The VS Code extension (`packages/vscode`) embeds the Go runtime and talks to it
over header-framed JSON-RPC stdio (`contenox vscode-agent --stdio`). These docs
cover the extension's permission model, local-model availability, and
Marketplace release process.

| Doc | Status | What it covers |
| --- | --- | --- |
| [acp-permission-bridge.md](acp-permission-bridge.md) | implemented | ACP-shaped permission handling inside the VS Code bridge (HITL approvals via editor UI); landed as `vscodeagent/approval.go` + `session/request_permission` |
| [local-model-availability.md](local-model-availability.md) | decision, implemented | Stable local-model advertisement across modeld restarts: graced `ServeableBackend()` + resolution self-heal |
| [marketplace-release.md](marketplace-release.md) | process | Marketplace publish workflow, `vscode-marketplace` environment, VSIX targets |
