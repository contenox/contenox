# Content refresh inventory: the external-ACP-agents wave

A maintainer checklist of every outward-facing surface that goes stale or
missing once contenox treats **external ACP agents as first-class**. The wave
landed: read-only REST `GET /api/agents` (+ `/agents/by-name/{name}`,
`/agents/{id}`, `runtime/internal/agentregistryapi/routes.go`); the
`contenox agent check <name>` verb that drives a live turn and prints the reply,
advertised commands, and MCP-forward report (`runtime/contenoxcli/agent_cmd.go`);
per-agent `mcp_servers` allowlist forwarding (`ExternalACPConfig.McpServers`,
`runtime/runtimetypes/agents.go`); beam's agent picker + per-session agent
attribution, proxied agent slash-command menus, and **inline permission cards
replacing the old modal approval gate** (`packages/beam/src/components/AgentPicker.tsx`,
`packages/beam/src/pages/chat/components/PermissionCard.tsx`, deleted
`PermissionGate.tsx` / `ApprovalViewTab.tsx`); and the `make acp-host-e2e`
harness (`runtime/agenthost/e2e_*_test.go`).

Priorities: **P0** = user-facing content is now *misleading* (shows or describes
UI/commands that no longer exist, or a shipped command has no reference at all);
**P1** = a shipped capability is *undocumented / unshown* (incomplete);
**P2** = polish, cleanup, or an optional new selling point.

**Status (2026-07-21, post-shoot):** every row in §1, §2, AND §3 is DONE. The
prose pass landed first (CLI-reference `agent` section, beam.md picker +
inline-card rewrite, the `external-acp-agents` integration guide, README
section, blueprint status note, TODO prune, landing points EN+DE), then the
media was shot per `recording-shot-list.md`: `beam-demo.webm` re-recorded
(agent-picker → inline-card → agent-view story, ~33 s, + new
`beam-video-cover.png`), `agent-permission-card.png` / `agent-picker.png` /
`agent-slash-menu.png` / `agent-check.gif` shot and embedded (beam.md, the
agents integration guide, the CLI reference), `beam-new-chat.png` re-shot (both
embeds), `beam-agent-view.png` verified (picker not in frame — kept), and the
orphans deleted (`beam-approval-gate.png`, `beam-chat.png`, `beam-modeld.png`).
All `TODO(recapture)` markers are removed.

The website owns no doc content: every `/docs/**` page renders markdown straight
from this repo's `docs/` tree (`website/src/content.config.ts`,
`website/README.md`). Editing a doc here *is* editing the site. The auto-generated
OpenAPI (`/openapi.json`) and its `/docs` UI (`runtime/internal/openapidocs/handlers.go`)
already reflect the new routes — no hand-edit needed.

## 1. CLI reference and prose docs

| File | What's stale / missing | What it should say | Prio |
|---|---|---|---|
| `docs/reference/contenox-cli.md` | The entire `contenox agent` command family (`search`/`add`/`list`/`show`/`check`/`edit`/`remove`/`enable`/`disable`) has **no section at all** — the reference jumps `tools` → `init`. `agent check` and the `mcp_servers` config field are the newest holes. The blueprint even deep-links here for "the `contenox agent` CLI." | Add a `### contenox agent` section documenting the whole family; give `check <name> [prompt...]` its own subsection (drives one live `initialize → session/new → prompt` turn via `runtime/agenthost`, streams the reply, prints advertised `/commands`, forwards the agent's `mcp_servers`, `--timeout` flag) and document the `mcp_servers` allowlist field on the agent config. | **P0** |
| `docs/guide/beam.md` | "Tour of the window" omits the sidebar **agent picker** ("New chat with an agent") and chatting with a registered external agent + per-session `Agent: {name}` attribution. The **approval-gate** section (`beam.md:156-171`) describes the retired modal gate and its "maximize to a tab" restore behavior; approvals are now inline transcript cards (`PermissionCard`, warning-styled, `role="group"`) that persist in the transcript and where click-outside no longer denies. Proxied agent slash-command menu and agent-advertised toolbar config options are unmentioned. | Add an "Chat with a registered agent" subsection (sidebar picker → staged agent → attribution). Rewrite the approval-gate section for the inline card (no modal, no maximize-to-tab, click-outside is inert). Mention the composer command-suggestion menu. | **P1** |
| `docs/integrations/` (new file, e.g. `docs/integrations/agents/external-acp-agents.md`) | No how-to exists for registering and driving a foreign ACP agent. `integrations/` has `editors/`, `providers/`, `tools/` but nothing for **agents you host**. | New guide: `contenox agent add <name> -- <cmd>` (and registry form), `agent check` to verify, per-agent `mcp_servers` forwarding + its consent boundary (named-server-by-named-server, no wildcard, contenox-side auth never forwarded — mirrors `docs/development/acp-client.md`), and that these agents then appear in beam's picker and over `GET /api/agents`. | **P1** |
| `README.md` | "Editor Integration" / "Connect Your Stack" cover MCP, tools, backends, and contenox-as-agent, but not the new inverse: contenox can now **register and drive external ACP agents** (`contenox agent add`/`check`) and chat with them from beam. | Add a short "Host external agents" note or bullet linking the new integration guide. | **P1** |
| `docs/development/acp-client.md` | Already updated this wave — accurately frames `agent check` as the user-facing twin of `make acp-host-e2e`, MCP forwarding, and the command surface. | Keep. Use as the source of truth when writing the reference/integration prose above. | keep |
| `docs/development/blueprints/acp/agent-servers-and-client-e2e.md` | Status reads "direction," but the harness (`make acp-host-e2e`) and `agent check` have landed. Its deep-link to `contenox-cli.md` for the `agent` CLI resolves only once row 1 is done. | Optional: note the harness is built; the link resolves after the CLI reference is fixed. | **P2** |
| `TODO.md` | The beam-chat gap list predates this wave; some items (external-agent chat, per-session agent attribution) are now shipped. | Prune completed items; keep genuinely open ones. | **P2** |
| `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `SUPPORT.md` | Reviewed — no agent/beam/ACP content that this wave invalidates. | No change. | — |

There is no root `CHANGELOG*` (the only CHANGELOG references are demo fixtures
inside `scripts/demos/` and `record-beam.mjs`); nothing to update there.

## 2. Website landing (hand-authored, not sourced from `docs/`)

| File | What's stale / missing | What it should say | Prio |
|---|---|---|---|
| `website/src/pages/index.astro` + `website/src/components/Landing.astro` | The `beamPoints` / caps sell chains + HITL + agent-view but never mention beam can now **drive any registered ACP agent** (picker, per-session attribution). Nothing false, but a new capability is unsurfaced. | Optional: add a beam point or capability line ("Bring your own agent — register any ACP agent and drive it, gated, from the same surface"). | **P2** |
| `website/src/pages/de/index.astro` | German mirror of the above — must stay in lockstep with the English `LandingStrings`. | If the English landing gains an agent line, translate it here. | **P2** |

The landing's hero video and HITL gif are covered in §3.

## 3. Demo media — tapes, screenshots, videos

House recording standards (`scripts/demos/RECORDING.md`): seed a **meaningful
sidebar** before capture, beam-led flows, **no trash sessions**, fast cloud model
on camera, secrets scrubbed. Optimized assets live flat under `website/public/`;
`website/dist/` is build output (regenerated, never hand-edited).

### Terminal tapes — all keep (CLI HITL/flows unaffected by this wave)

| Asset | Source tape | Embedded in | Verdict |
|---|---|---|---|
| `hero.gif` | `scripts/demos/hero.tape` | `docs/reference/contenox-cli.md:5` | keep |
| `quickstart.gif` | `scripts/demos/quickstart.tape` | `docs/guide/quickstart.md:74` | keep |
| `install.gif` | `scripts/demos/install.tape` | `docs/guide/quickstart.md:28` | keep |
| `chain-blocked.gif` | `scripts/demos/chain-blocked.tape` | `docs/guide/first-chain.md:205`, `docs/development/chains.md:21` | keep |
| `hitl-approve.gif` | `scripts/demos/hitl-approve.tape` | `website/src/components/Landing.astro:120`, `docs/guide/hitl.md:20` | keep (terminal HITL, not beam) |
| `modeld-console.gif` | (beam capture) | `docs/guide/quickstart.md:64` | keep (modeld console unaffected) |
| `aionui-custom-agent.png` | (editor capture) | `docs/integrations/editors/aionui.md:20` | keep |

### Beam screenshots / video — the stale surface

| Asset | Embedded in | What's stale | Verdict |
|---|---|---|---|
| `beam-demo.webm` (+ `beam-video-cover.png` poster) | `website/src/components/Landing.astro:72-73` (homepage hero), `docs/guide/beam.md:19` | The 30-second hero loop ends on the **old modal approval gate**; the whole flow predates the agent picker and inline card. Most prominent asset on the site. | **retire + re-record** — **P0** |
| `beam-approval-gate.png` | *(embed removed — a commented-out `agent-permission-card.png` placeholder sits in `docs/guide/beam.md`'s approval section)* | Showed the retired modal gate; no longer shown anywhere. The PNG is an orphan in `website/public/` until its replacement is shot. | shoot the replacement, then delete the orphan — **P1** |
| `beam-new-chat.png` | `docs/guide/beam.md:124`, `docs/guide/first-chain.md:57` | Empty new-chat with per-session controls, but no **staged-agent picker** (now on the empty surface) and no sidebar "New chat with an agent." | **update** — **P1** |
| `beam-agent-view.png` | `docs/guide/beam.md:145` | Agent-view overlay itself is unchanged, but the sidebar chrome now carries the agent-picker chevron. | verify; re-shoot only if the picker is visible in-frame — **P2** |
| `beam-login.png` | `docs/guide/beam.md:61` | Login page unaffected. | keep |
| `beam-providers.png` | `docs/guide/quickstart.md:116` | Providers page unaffected. | keep |
| `beam-chat.png`, `beam-modeld.png` | *not referenced by any doc or landing* | Orphaned in `website/public/`; `beam-chat.png` likely shows pre-wave chat chrome (old assistant/robot icon). | retire the orphans, or leave unused — **P2** |

### Recording infrastructure

| File | What's stale | What to do | Prio |
|---|---|---|---|
| `scripts/demos/record-beam.mjs` | ~~stale dialog selector~~ **DONE** — waits on the `role="group"` card, clicks Allow, and the story opens with the agent-picker beat. | Run as-is for the hero re-record. | done |
| `scripts/demos/RECORDING.md` | ~~predated the inline card~~ **DONE** — documents the modal→inline-card selector change and the agent-picker/external-agent flows. | Keep. | done |

### Proposed NEW media (per house standards: seeded sidebar, beam-led, real turns)

| New asset | Surface | Shows | Prio |
|---|---|---|---|
| `agent-picker` GIF/PNG (beam) | new `docs/integrations/agents/*` + `docs/guide/beam.md` | Seeded sidebar → "New chat with an agent" chevron → `AgentPicker` (native contenox at top + registered agents) → pick → empty chat shows `Agent: {name}` → send. | **P1** |
| `agent-permission-card` PNG (beam) | `docs/guide/beam.md` (replaces `beam-approval-gate.png`) | A real turn against a registered agent (e.g. claude) hitting a gated action → inline `PermissionCard` in the transcript with Allow/Deny; click-outside no longer denies. | **P1** |
| `agent-check` GIF — tape AUTHORED (`scripts/demos/agent-check.tape`), GIF unshot | new `docs/integrations/agents/*` + CLI reference `agent check` subsection | `contenox agent check <name>` streaming a reply, `Turn completed (agent … stopReason=end_turn)`, `Agent advertises N command(s): …`, and the `Forwarding MCP servers:` line. Seed a `demo-agent` per the tape header, then `vhs agent-check.tape && ./mkgif.sh agent-check`. | **P1** |
| `agent-slash-menu` PNG (beam) | `docs/guide/beam.md` | The proxied agent-advertised `/commands` menu in the composer. | **P2** |

## 4. OpenAPI and auto-served docs

| Surface | State | Action |
|---|---|---|
| `runtime/internal/openapidocs/openapi.json` | Already regenerated this wave (+`/agents`, `/agents/by-name/{name}`, `/agents/{id}`, `ExternalACPConfig.mcp_servers`). | none — keep; do not hand-edit (regenerate via the spec pipeline if the shape changes) |
| `GET /docs` UI + `GET /openapi.json` (`runtime/internal/openapidocs/handlers.go`) | Serves the spec above live — the new agent routes appear automatically. | none — optionally smoke-check the rendered `/docs` shows the agent paths |
| Prose REST reference | contenox has no hand-written REST reference page; `GET /api/agents` is documented only by the OpenAPI spec. | acceptable; optionally cross-link the spec from the new agents integration guide |
