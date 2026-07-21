# Recording Contenox demos — playbook

A reusable prompt/checklist for recording marketing demos (terminal GIFs, Beam web
UI, VS Code extension). Written from hard-won experience — follow it and skip the
flailing. Everything here runs on this Linux/X11 box; adjust paths as needed.

## Pick the tool by surface

| Demo surface | Tool | Output |
|---|---|---|
| CLI / terminal (`contenox …`, setup, install) | **vhs** (`.tape` scripts) | GIF (reproducible) |
| Beam web UI (`contenox serve`) | **Playwright** (screenshots) or CDP | PNG / webm |
| VS Code extension (chat webview, autocomplete) | **VS Code + CDP** (`--remote-debugging-port`) + Playwright `connectOverCDP` | PNG, or CDP-screenshot-stitched webm |
| Any real desktop window as smooth video | **ffmpeg x11grab** — last resort, fragile (see gotcha) | mp4/webm |

Prefer scripted/deterministic capture (vhs, CDP screenshots) over `x11grab`.

## Before any recording

1. **Fast, clean model on camera.** Set a snappy cloud model so responses are quick
   and reliable; restore afterward:
   ```bash
   contenox config set default-provider vertex-google
   contenox config set default-model    gemini-3-flash-preview
   contenox config set default-think     off      # crisp output, no <think> noise
   # …record…
   # restore your real default:
   contenox config set default-provider llama
   contenox config set default-model    gemma4-e4b
   contenox config set default-think     high
   ```
   Local `modeld`/`gemma4-e4b` is too slow for smooth takes and its slot can get
   VRAM-starved by a browser during capture.
2. **Reset any demo fixture files** between takes: `git checkout <file>`.
3. **Verify every finished asset**: view frames (Read renders images). Check timing,
   that output is real (no failed steps on camera), and **no API keys/tokens/secrets
   in any frame**.

---

## 1. Terminal GIFs (vhs)

Tapes live in `scripts/demos/*.tape`; `scripts/demos/mkgif.sh` renders MP4→GIF and
collapses idle/thinking time. Render: `cd scripts/demos && vhs foo.tape`, then
`./mkgif.sh foo`.

**vhs gotchas (each one cost real time):**
- **Terminal stops scrolling after a curl progress bar.** Long downloads leave vhs's
  virtual terminal wedged. Insert a `clear` (via a `Type "clear"`/`Enter`) between a
  download phase and the next command.
- **The parser rejects escaped quotes and absolute paths** in `Output`/`Screenshot`
  and chokes on `$(…)`/`&&` inside `Type "…"`. For anything non-trivial, put the shell
  in a **sourced helper script** (see `install-demo-env.sh`) and `Type "source ….sh && clear"`.
- Use **relative** output paths (`Output out/foo.mp4`), not absolute.
- `Wait+Screen /regex/` waits for on-screen text — keep the regex loose (`/q to quit/`
  not the full line) or it times out.
- Keep a consistent look: `Set Theme "Catppuccin Mocha"`, `Set FontSize 18`, fixed
  `Width`/`Height`/`Padding` across tapes.
- Size gate: GIFs < ~3 MB (mkgif.sh handles palette + fps).

Deps: `vhs` (`~/go/bin`), `ttyd`, `ffmpeg` — all installed.

---

## 2. Beam / web UI (Playwright screenshots)

```bash
contenox serve            # SPA at http://127.0.0.1:32123, REST under /api
```
Drive with the Playwright MCP browser (navigate/click/screenshot) at 1440×900. For a
walkthrough video, capture a screenshot sequence and stitch with ffmpeg, or record a
short real session and encode to webm (`libvpx-vp9 -crf 40`).

- Triaged/showable chat sessions already exist (RevOps/HubSpot tool-call runs, etc.).
- `beam serve` embeds a prebuilt SPA; if it 404s, `make build-ui` first.

**Approval gate: modal → inline card (selector change, cost real time).** The old
approval gate was a modal (`[role="dialog"], [role="alertdialog"]`). It is now an
**inline `PermissionCard`** in the transcript (`packages/beam/.../PermissionCard.tsx`):
its wrapper is **`role="group"`** with `aria-label` "Permission required"
(`i18n` key `acp_chat.permission_card_title`; German "Berechtigung erforderlich").
A capture that waits on a dialog role now stalls forever. Wait on the card and click
its option button instead:

```js
const card = page.getByRole('group', { name: /permission required|berechtigung erforderlich/i });
await card.waitFor({ state: 'visible', timeout: 60000 });
await card.getByRole('button', { name: /allow|erlauben/i }).first().click(); // no click-outside/Escape shortcut — the buttons are the only path
```

`record-beam.mjs` is already retargeted: it waits on the `role="group"` card and
clicks **Allow** (no dialog wait, no `y`), and its story opens with the
agent-picker beat — ready for the hero re-record per
`docs/development/recording-shot-list.md`.

**Agent-picker + external-agent chat flow.** To capture chatting with a registered
external agent, seed one first (fast cloud creds still apply — see step 1 above):

```bash
contenox agent add claude-acp                 # registry form (or: add <name> -- <cmd>)
contenox agent check claude-acp "say hello"   # confirm it answers before recording
```

Then, in Beam: the sessions sidebar shows a chevron next to **New session** with
`aria-label` "New chat with an agent" (`acp_sidebar.new_session_with_agent`). Click
it to open the `AgentPicker` dropdown — **Contenox (default)** at the top, registered
agents below. Pick one; the empty chat stages it ("Say hello — you are talking to
{name}, live") and the session row is labelled `Agent: {name}` after the first
prompt. The chevron is hidden when no enabled external agent is registered, so seed
the agent *before* opening Beam.

- Seed a **meaningful sidebar** first (real triaged sessions, no husks) — the agent
  session should sit among believable neighbours, per the no-trash-sessions rule.
- For the permission-card shot, drive the registered agent into a gated write so the
  inline card renders against a *foreign* agent's action.

---

## 3. VS Code extension (CDP)

The chat is now a **webview** (DOM) — fully drivable via CDP, unlike the old native
chat. Standard Marketplace build works in plain `code` (no proposed API needed).

**Install the current build, launch isolated, drive it:**
```bash
make dev-install-vscode            # builds + installs the VSIX into ~/.vscode/extensions
S=/tmp/rec                          # scratch
rm -rf $S/profile; mkdir -p $S/profile
# NVIDIA offload prevents the compositor freezes we hit; isolated --user-data-dir
# keeps your real editor untouched. NOTE: plain `code`, no --enable-proposed-api.
setsid env __NV_PRIME_RENDER_OFFLOAD=1 __GLX_VENDOR_LIBRARY_NAME=nvidia \
  code --user-data-dir "$S/profile" --remote-debugging-port=9223 \
       --new-window /path/to/demo-workspace >/dev/null 2>&1 & disown
```
Then connect with `playwright-core`:
```js
const b = await chromium.connectOverCDP("http://127.0.0.1:9223");
const wb = b.contexts()[0].pages().find(p => p.url().startsWith("vscode-file://"));
```
- Command palette: press `F1`, type a command, `Enter`. Open the chat with
  **`Contenox: Open Chat`** (the `contenox.chat` sidebar webview).
- The webview renders in a nested `vscode-webview://` iframe. `wb.frames()` exposes it,
  but OOPIF enumeration is **flaky** — retry in a loop, and fall back to a full-window
  screenshot to judge the result.
- **Layout for a demo**: split editor (code left, chat right) via `View: Move Editor
  into Right Group`; close the Copilot sidebar with `View: Close Auxiliary Bar`.

**VS Code gotchas (each cost real time):**
- **Never use `Chat: Focus Chat Input`** — that targets VS Code's *default Copilot*
  chat, which triggers a **"Signing in to GitHub…" browser popup** (which then ruins an
  x11grab). Drive the **Contenox webview input directly**.
- **Free port 9223 before relaunching** — `pkill` the old `code` by profile/port and
  confirm CDP returns `000`, or a stale window keeps serving the port and you drive the
  wrong (polluted) instance. Kill by PID (`ps aux | grep <profile> | awk '{print $2}' | xargs kill -9`).
- Use a **fresh** `--user-data-dir` per session; old profiles accumulate a stuck
  GitHub-signin / Copilot chat state.
- The extension bundles its own `bin/contenox`; to test a runtime fix, either
  `make dev-install-vscode` (rebuilds the bundled binary) or set `contenox.binaryPath`
  + `contenox.dataDir` in the workspace `.vscode/settings.json`.

---

## Screen video (ffmpeg x11grab) — avoid unless necessary

```bash
# window at 240,90, 1440x900 on DISPLAY :1
ffmpeg -y -f x11grab -framerate 30 -video_size 1440x900 -i :1.0+240,90 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p out.mp4
# stop cleanly: kill -INT <pid>
```
**It captures by screen coordinates, not by window** — anything that pops over the
region (a GitHub OAuth browser, a notification) gets recorded instead. A stray Brave
OAuth window wrecked a full take this way. Prefer CDP screenshot-stitching (immune to
overlays) for VS Code/Beam; only use x11grab when you need true-motion video and can
guarantee a clean desktop with no popups.

---

## Cleanup

- Restore `contenox config` to the real default (see step 1).
- `git checkout` any demo fixture files you edited.
- Kill demo VS Code / `contenox serve` processes; free port 9223.
- Keep `.tape` sources committed (reproducible); optimized GIFs/PNGs go under
  `website/public/` (flat), heavy videos to S3 or `website/public/`.
