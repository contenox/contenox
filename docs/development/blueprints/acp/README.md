# ACP Blueprints

Contenox is an ACP hub: `contenox acp` serves the Agent Client Protocol over
stdio upward, for editor and desktop clients (Zed, JetBrains, AionUi, beam);
`contenox acpx` runs the headless / untrusted-driver profile. The same Go
runtime is also an ACP client downward, driving other ACP agents (including
other contenox instances) as a taskengine step or a modelprovider
implementation. These docs cover both directions: getting contenox listed and
installable in ACP clients, and the client-side engine capability that lets
contenox drive agents of its own.

| Doc | Status | What it covers |
| --- | --- | --- |
| [acp-client-engine.md](acp-client-engine.md) | direction | Contenox as an ACP client: the models/tools/agents ladder, ACP-as-taskengine-step and ACP-as-modelprovider, the provider honesty rule, the permission-routing invariant, the shared `libacp` client-core prerequisite, and the agent registry pattern |
| [declared-agents-and-harnesses.md](declared-agents-and-harnesses.md) | direction | Agents and harnesses as declared resources: the trust stance (equip, don't govern), the harness-is-the-client-role contract, beam as fleet manager (one ACP connection per agent, no aggregate agent), headless agents, and the manager → harnesses → headless ramp |
| [zed-registry.md](zed-registry.md) | plan | Listing contenox on the Zed/ACP registry so an install yields a working, model-wired agent |
| [registry-submission/](registry-submission/README.md) | artifacts | The `agent.json` + icon to copy into an `agentclientprotocol/registry` fork, with validation steps |
| [openide-integration.md](openide-integration.md) | research | OpenIDE (IntelliJ Platform) integration via a native plugin over the existing runtime and stdio bridge |

Related: [`../vscode/acp-permission-bridge.md`](../vscode/acp-permission-bridge.md)
(ACP-shaped permissions inside the VS Code bridge), the ACP slash-command
surface documented in `docs/contenox-cli.md`, and
[`../opsclient/operator-console.md`](../opsclient/operator-console.md) (the
operator-console consumer of the client engine, driving remote hosts instead
of editors).
