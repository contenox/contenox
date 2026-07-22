# Beam Blueprints

Beam is the browser SPA embedded in the `contenox` binary and served by
`contenox serve` (SPA at `/`, REST API under `/api`, chat over the `/acp`
WebSocket). Beam's job is the supervise–review–intervene loop: the setup
wizard, the admin control plane, the ACP chat client, and the fleet/mission
oversight surfaces.

| Doc | Status | What it covers |
| --- | --- | --- |
| [beam-on-acp.md](beam-on-acp.md) | rule | **The chat re-engineering doctrine.** Why Beam's chat surface is an ACP client of `acpsvc` and nothing else, the reusable-component extraction (Part A), the ACP-native surface and its wire-to-rendering data mapping (Part B), and the migration doctrine, wire-layer rule, capability-provider seam, and session-identity coexistence rule (Part C). |
| [fleet-manager.md](fleet-manager.md) | design record | Beam as fleet manager: manifest, dispatch, envelopes, the ops board, exceptions up / green silence |
| [attention-layer.md](attention-layer.md) | blueprint | The relevance computation over journals, task events, and reports: what operators must attend, ranked |
| [ide-workflows.md](ide-workflows.md) | blueprint | The oversight cockpit arcs: changed-files + diff review and the other IDE workflows that serve supervise–review–intervene (never editing) |
| [component-roadmap.md](component-roadmap.md) | backlog | Plumb-ready component list tiered by whether its data source exists |
| [session-workspace-files.md](session-workspace-files.md) | implemented | One workspace root per session: sandboxed `local_fs`, file tree, @-mentions as ACP resource blocks |
| [shell-sessions.md](shell-sessions.md) | implemented | Persistent per-session PTY shells surfaced to both agent and user |
| [workspace-tabs.md](workspace-tabs.md) | implemented | The in-app tabbed workspace: tab model, surfaces, rails-list/tabs-hold |
| [workspace-canvas.md](workspace-canvas.md) | landing in slices | The side-by-side working area for terminals, files, and diffs inside a chat tab |

[`../v1-feature-map.md`](../v1-feature-map.md) describes how Beam relates to
the V1 product surface.
