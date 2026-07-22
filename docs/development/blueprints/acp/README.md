# ACP Blueprints

Contenox is an ACP hub: `contenox acp` serves the Agent Client Protocol over
stdio upward, for editor and desktop clients (Zed, JetBrains, AionUi, beam);
`contenox acpx` runs the headless / untrusted-driver profile. The same Go
runtime is also an ACP client downward, driving other ACP agents (including
other contenox instances) as declared external agents and fleet units. These
docs cover both directions, plus the fleet/mission machinery built on them.

| Doc | Status | What it covers |
| --- | --- | --- |
| [acp-client-engine.md](acp-client-engine.md) | direction | Contenox as an ACP client: the models/tools/agents ladder, ACP-as-taskengine-step and ACP-as-modelprovider, the provider honesty rule, the permission-routing invariant, the shared `libacp` client-core prerequisite, and the agent registry pattern |
| [agent-servers-and-client-e2e.md](agent-servers-and-client-e2e.md) | landed | How the client-host role is verified end-to-end: the self-hosting loopback, the conformance harnesses, `contenox agent check` |
| [fleet-consolidation.md](fleet-consolidation.md) | executed record | Fleet and mission mode: dispatch, the supervision edge, reports, the attention inbox, session adoption — the slices behind `fleetservice`/`missionservice` |
| [mission-plans.md](mission-plans.md) | building | The plan engine as a resident planner: plan revisions, step progress, prompt surface |
| [envelope-compute-bounds.md](envelope-compute-bounds.md) | building | The mission envelope as a unit's TOTAL boundary: compute bounds (turns, tool calls, tokens) alongside HITL action bounds |
| [registry-submission/](registry-submission/README.md) | artifacts | The `agent.json` + icon to copy into an `agentclientprotocol/registry` fork, with validation steps |

Related: [`../vscode/acp-permission-bridge.md`](../vscode/acp-permission-bridge.md)
(ACP-shaped permissions inside the VS Code bridge) and the ACP slash-command
surface documented in `docs/reference/contenox-cli.md`.
