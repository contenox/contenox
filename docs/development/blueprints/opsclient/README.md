# Ops Client Blueprints

Contenox as the operated-system agent: the product split from `modeld` (the
inference node), session-scoped to a host rather than a repo, driven remotely
through the ACP client engine (`../acp/acp-client-engine.md`) and reviewed
through beam as the phone-reachable operator console.

| Doc | Status | What it covers |
| --- | --- | --- |
| [operator-console.md](operator-console.md) | direction | The modeld/contenox product split, session=host, remote topologies (same box, LAN, ssh-stdio, `/acp` WS), beam-as-screen and the phone-usable gate rule, standing ops work as chains, the ops-grade HITL policy gap, and open decisions (remote-owner trust profile, Windows as a target) |

Related: [`../acp/acp-client-engine.md`](../acp/acp-client-engine.md) (the
Go-runtime ACP client capability this surface drives through) and
[`../beam/acp-chat-workspace.md`](../beam/acp-chat-workspace.md) (the
workspace layout beam reuses for ops sessions).
