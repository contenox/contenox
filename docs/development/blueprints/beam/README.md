# Beam Blueprints

Beam is the browser SPA embedded in the `contenox` binary and served by
`contenox serve` (SPA at `/`, REST API under `/api`). These docs cover its
restoration, current scope, and R&D directions.

| Doc | Status | What it covers |
| --- | --- | --- |
| [sovereign-workspace.md](sovereign-workspace.md) | decision blueprint | **Primary product framing:** Beam as UX-sovereign control center (not an IDE fork); brain/glass split, positioning, concrete PR plan, readiness matrix, and truth-gated implementation slices |
| [sovereign-workspace-architecture.md](sovereign-workspace-architecture.md) | engineering boundaries | Component/state ownership rules, event→canvas data flow, and anti-patterns for implementing sovereign-workspace slices |
| [http-ui-revival.md](http-ui-revival.md) | executed | Migration plan that restored the HTTP API + Beam UI |
| [local-runtime-cockpit.md](local-runtime-cockpit.md) | direction | Beam as the modeld cockpit: status, capacity/fit diagnostics, KV residency, model variants |
| [chat-canvas.md](chat-canvas.md) | R&D / vision | Chat/canvas split: renderer-agnostic artifact panel as the second pane |
| [remote-connector.md](remote-connector.md) | R&D / vision | Headless `contenox-connector` for controlling a remote engine from a local client |
| [auth.md](auth.md) | R&D / vision | Single-operator password auth for Beam; builds on the loopback + bearer `TOKEN` model |

`docs/blueprints/v1-feature-map.md` describes how Beam relates to the V1
product surface.
