# Beam Blueprints

Beam is the browser SPA embedded in the `contenox` binary and served by
`contenox serve` (SPA at `/`, REST API under `/api`). These docs cover its
restoration, current scope, and R&D directions.

| Doc | Status | What it covers |
| --- | --- | --- |
| [beam-on-acp.md](beam-on-acp.md) | rule | **The chat re-engineering doctrine.** Why Beam's chat surface is an ACP client of `acpsvc` and nothing else, the reusable-component extraction (Part A), the ACP-native surface and its wire-to-rendering data mapping (Part B), and the migration doctrine, wire-layer rule, capability-provider seam, and session-identity coexistence rule (Part C). |
| [acp-chat-workspace.md](acp-chat-workspace.md) | primary direction | **The workspace blueprint.** Beam chat re-engineered as an ACP client: the productivity model (run/review/own; governance-surface sovereignty), reusable `chat-kit` + `acp-web-client` packages, the three-zone + command-palette layout, reclaimed assets (Monaco, canvas, visualizer, FileTree, Cmdbar), downward-repair rule, and truth-gated migration. Supersedes and folds in the former sovereign-workspace and chat-canvas blueprints. |
| [http-ui-revival.md](http-ui-revival.md) | executed | Migration plan that restored the HTTP API + Beam UI |
| [local-runtime-cockpit.md](local-runtime-cockpit.md) | direction | Beam as the modeld cockpit: status, capacity/fit diagnostics, KV residency, model variants |
| [remote-connector.md](remote-connector.md) | R&D / vision | Headless `contenox-connector` for controlling a remote engine from a local client |
| [auth.md](auth.md) | R&D / vision | Single-operator password auth for Beam; builds on the loopback + bearer `TOKEN` model |

`docs/blueprints/v1-feature-map.md` describes how Beam relates to the V1
product surface.
