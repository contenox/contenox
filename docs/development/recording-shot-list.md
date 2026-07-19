# Recording shot list: the external-ACP-agents wave

The concrete, take-by-take plan for the media the external-ACP-agents wave needs
— the follow-through to the audit in
[content-refresh-inventory.md](content-refresh-inventory.md) §3. It says *exactly*
what to shoot, with what tool, in what order, and where each asset embeds. The
reusable how-to (tools, gotchas, the model-swap ritual, the selectors) lives in
[`scripts/demos/RECORDING.md`](../../scripts/demos/RECORDING.md); this file does
not repeat it, it schedules against it.

House style: real turns only, a **meaningful seeded sidebar** (no trash
sessions), secrets scrubbed, and a fast cloud model on camera. Every take below
names its output filename and embed target so nothing is shot blind.

## Shared pre-flight (every Beam take)

Do this once before any Beam capture, then leave it running for the whole
session:

1. **Model-swap ritual** — set a fast, quiet cloud model and restore your real
   default afterward. Verbatim commands in
   [`RECORDING.md` §"Before any recording"](../../scripts/demos/RECORDING.md).
   In short: `contenox config set default-provider vertex-google` /
   `default-model gemini-3-flash-preview` / `default-think off`; restore after.
2. **Seed a meaningful sidebar** — a handful of believable, triaged sessions
   (RevOps/HubSpot tool-call runs, a repo-chore session), so a new agent session
   sits among real neighbours. No husks, no "test test".
3. **Register the demo agent** — the agent takes need one enabled external agent.
   The sidebar's agent chevron is hidden until one exists:
   ```bash
   contenox agent add claude-acp                  # or: add demo-agent -- <cmd>
   contenox agent check claude-acp "say hello"    # confirm it answers first
   ```
4. **Reset fixtures** between takes (`git checkout <file>`), verify frames after
   (Read renders images), and confirm no token/key is on camera.

Drive Beam with Playwright at **1440×900** (`record-beam.mjs`, or the Playwright
MCP browser). Terminal takes use **vhs** (`scripts/demos/*.tape` + `mkgif.sh`).

---

## Take 1 — Hero loop (re-record)

The homepage hero and the beam-guide video. The current cut ends on the retired
modal approval gate and predates the agent picker + inline card.

- **Tool:** Playwright video (`scripts/demos/record-beam.mjs`, retargeted — see
  Selectors). ~30 s, encode to webm (`libvpx-vp9 -crf 40`).
- **Output:** `beam-demo.webm` (+ poster frame `beam-video-cover.png`), flat under
  `website/public/`.
- **Embed targets:** `website/src/components/Landing.astro` (homepage hero
  `<video>`) and `docs/guide/beam.md` (top-of-page `<video>`, currently behind a
  `TODO(recapture)` comment — remove it once the new cut lands).
- **Seed:** meaningful sidebar + the `demo-project` workspace on the allowlist
  (`contenox serve <demo-dir>`), plus the registered agent from pre-flight.
- **Steps:**
  1. Opening pan: open the sessions drawer (seeded sidebar on camera ~2 s), close it.
  2. **New chat with an agent** chevron → pick the registered agent from the
     `AgentPicker` (Contenox (default) at top). *(This is the new beat vs. the old
     cut.)* The empty chat stages it and reads "Say hello — you are talking to
     {name}, live".
  3. Send a prompt that reads then writes a file (e.g. *"Read TODO.md and add a
     0.2.0 entry to CHANGELOG.md for the completed items."*).
  4. Let the tool-call cards render; the gated write raises the **inline
     permission card** — hold ~3 s on the diff, then **Allow**.
  5. Closer: open the workspace **Files** panel and toggle **Agent view** so the
     per-file policy verdicts are the final frame. Grab `cover-frame.png` here for
     the poster.
- **Selectors / timing:** wait on the card via `getByRole('group', { name: /permission required/i })`, **not** a dialog role, and click **Allow** (no
  click-outside/`y` shortcut). Full selector snippet in
  [`RECORDING.md` §2](../../scripts/demos/RECORDING.md). `record-beam.mjs:94` still
  waits on `[role="dialog"]` and presses `y` — retarget both first.

---

## Take 2 — Permission card (replaces `beam-approval-gate.png`)

A still of the inline permission card against a *registered agent's* gated action.

- **Tool:** Playwright screenshot (Playwright MCP), 1440×900.
- **Output:** `agent-permission-card.png`, flat under `website/public/`.
- **Embed target:** `docs/guide/beam.md` "The approval gate" section — a
  commented-out `<!-- ![…](/agent-permission-card.png) -->` placeholder is already
  there; uncomment it and delete the sibling `TODO(recapture)` and the old
  `beam-approval-gate.png` reference. Retire `beam-approval-gate.png`.
- **Seed:** the registered agent + `demo-project` workspace; a HITL policy that
  gates writes (so the card actually raises).
- **Steps:**
  1. Start a chat against the registered agent (sidebar chevron → pick it).
  2. Prompt it into a single gated file write (a `CHANGELOG.md`/`TODO.md` edit
     reads well — a real diff, not a destructive command).
  3. When the inline card appears in the transcript, screenshot with the card and
     its **diff** in frame, buttons visible (**Allow** / **Deny**, and their
     *always* variants if the policy offers them).
- **Selectors / timing:** the card is `role="group"`, `aria-label` "Permission
  required". Frame it inside the transcript (it is *inline*, not a modal) so the
  screenshot visibly differs from the old floating gate.

---

## Take 3 — Agent picker

Shows the sidebar picker and the staged-agent empty chat.

- **Tool:** Playwright — a short GIF (screenshot sequence stitched with ffmpeg) is
  ideal; a single PNG of the open dropdown is the cheap fallback.
- **Output:** `agent-picker.gif` (or `agent-picker.png`), flat under
  `website/public/`.
- **Embed targets:** `docs/integrations/agents/external-acp-agents.md` ("Drive it
  from Beam") and `docs/guide/beam.md` ("Chat with a registered agent") — add the
  embed in both once shot.
- **Seed:** ≥1 enabled registered agent (ideally two, so the list reads as a
  roster) + meaningful sidebar.
- **Steps:**
  1. Sidebar in view. Click the chevron beside **New session** (`aria-label`
     "New chat with an agent").
  2. Hold on the open `AgentPicker`: **Contenox (default)** at the top, registered
     agents below.
  3. Pick a registered agent → the empty chat shows the staged greeting; the new
     session row carries `Agent: {name}`.
  4. (GIF) type the first line of a prompt to show attribution persisting, then stop.
- **Selectors / timing:** chevron `aria-label` `acp_sidebar.new_session_with_agent`;
  the picker is a `Dropdown` (`AgentPicker.tsx`) anchored to the rail's right edge.
  Keep the panel un-clipped — capture at ≥1440 wide.

---

## Take 4 — `agent check` (terminal)

The CLI verification turn: streamed reply, stop reason, advertised commands,
forwarded MCP servers.

- **Tool:** **vhs**. New tape `scripts/demos/agent-check.tape`; render with
  `mkgif.sh` (`vhs agent-check.tape && ./mkgif.sh agent-check`).
- **Output:** `agent-check.gif`, flat under `website/public/`.
- **Embed targets:** `docs/reference/contenox-cli.md` (`contenox agent check`
  subsection) and `docs/integrations/agents/external-acp-agents.md` ("Verify it
  with `agent check`") — add the embed in both.
- **Seed:** an agent whose config declares an `mcp_servers` allowlist (so the
  `Forwarding MCP servers:` line appears) and that advertises ≥1 slash command
  (so the `Agent advertises N command(s):` line appears). Register it off-camera.
- **Steps (tape):**
  1. `Set Theme "Catppuccin Mocha"`, `Set FontSize 18`, fixed Width/Height/Padding
     to match the other tapes.
  2. `Type "contenox agent check demo-agent \"summarize this repo in one line\""`
     → `Enter`.
  3. `Wait+Screen` on a loose regex of the reply, then let these lines land:
     `Forwarding MCP servers: …`, the streamed reply, `Turn completed (agent …
     stopReason=end_turn).`, `Agent advertises N command(s): …`.
  4. Short hold, done.
- **Selectors / timing:** vhs gotchas apply (no `&&`/`$()` inside `Type`; relative
  `Output` path; loose `Wait+Screen` regex) — see
  [`RECORDING.md` §1](../../scripts/demos/RECORDING.md). Keep the GIF < ~3 MB.
  Scrub any real path/credential from the run command on camera.

---

## Take 5 — Agent slash-command menu

The proxied agent-advertised `/commands` in the composer.

- **Tool:** Playwright screenshot, 1440×900.
- **Output:** `agent-slash-menu.png`, flat under `website/public/`.
- **Embed target:** `docs/guide/beam.md` ("Composer shortcuts") — add the embed.
- **Seed:** a session against a registered agent that advertises commands (the
  same agent as Take 4). The command menu is populated from the agent's
  `available_commands_update`, so drive one turn first if the list arrives lazily.
- **Steps:**
  1. In a live agent session, click the composer and type `/`.
  2. When the **command suggestions** menu opens (`SlashCommandMenu`, label
     `acp_chat.slash_menu_label` "Command suggestions"), screenshot it with a few
     agent-advertised commands (e.g. `/compact`, `/review`) visible.
- **Selectors / timing:** the menu is anchored above the composer; capture with
  the composer and one highlighted entry in frame.

---

## Take 6 — New-chat surface (update `beam-new-chat.png`)

Refresh the empty new-chat still so it shows the staged-agent picker beat.

- **Tool:** Playwright screenshot, 1440×900.
- **Output:** `beam-new-chat.png` (overwrite), flat under `website/public/`.
- **Embed targets:** `docs/guide/beam.md` ("Per-session controls", behind a
  `TODO(recapture)` comment — remove it once reshot) and `docs/guide/first-chain.md`.
- **Seed:** `demo-project` workspace on the allowlist + ≥1 registered agent (so
  the sidebar chevron is present in the sidebar chrome).
- **Steps:**
  1. Open a fresh empty chat with the sidebar visible (chevron in frame).
  2. Bind the `demo-project` workspace so the per-session **Model / HITL Policy /
     Think / Token Limit / Workspace** controls show above the empty chat.
  3. Screenshot the empty surface with the sidebar (including the "New chat with
     an agent" chevron) in frame.
- **Selectors / timing:** keep the framing close to the existing shot so the swap
  is drop-in; the only new element is the sidebar chevron.

---

## After the shoot

- Optimize assets and place them **flat** under `website/public/` (never edit
  `website/dist/` — it is build output).
- Enable the embeds and remove the matching `TODO(recapture)` comments in
  `docs/guide/beam.md`; retire the orphaned `beam-approval-gate.png` (and the
  unused `beam-chat.png` / `beam-modeld.png` if you are cleaning house).
- Restore the real `contenox config` defaults (model-swap ritual) and
  `git checkout` any demo fixtures.
