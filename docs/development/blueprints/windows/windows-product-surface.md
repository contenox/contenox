# Blueprint: Windows Product Surface and Distribution

> **Status:** initial blueprint. Captures the shift from Linux/CLI-shaped distribution to Windows GUI/product experience.  
> **Owner:** runtime  
> **Companions:** `../windows-development.md` (contributor/build side), `../modeld/release-artifacts.md`, vscode/ docs.

## Problem

The current distribution and first-run story is shaped for Linux CLI users:

- `curl -fsSL https://contenox.com/install.sh | sh`
- `install.sh` explicitly rejects non-Linux/Darwin.
- After "install" the user is expected to open a terminal, run `contenox setup`, `contenox serve`, etc.
- No desktop icon, no Start Menu entry, no "double-click and use" path.
- Local inference (`modeld`) Windows prebuilts exist in the release pipeline but are repeatedly described as "unverified" and require a dedicated Windows worker.

This is a mismatch with the intended Windows audience (gamers, prosumers, developers) who expect something closer to **Docker Desktop**:

- Install the thing.
- Double-click an icon.
- Get a working local agent on their GPU (with a sensible default model).
- Primary interaction surfaces: VS Code (canvas/chat), Beam (the web UI), possibly others.

The CLI remains important (especially for ACP, scripts, editors), but on Windows the *product surface* is the installed experience, not the terminal command.

## Vision

On Windows, Contenox is a GUI-first product:

- User installs once (Store or PowerShell one-liner).
- Gets a desktop icon / Start Menu entry.
- Clicks it → Beam opens (or a launcher starts the local server + UI).
- Local GPU inference "just works" with a default model we can distribute (Gemma-class or similar) via the existing S3-backed modeld store.
- Strong integration with VS Code as a primary usage surface.
- The underlying `contenox` binary and `modeld` daemon power everything, but the user doesn't have to think about them at first.

The goal is "install → agent on my GPU" with minimal terminal friction for the target Windows users.

## Current State (as of mid-2026)

- **CLI binary**: Pure Go (`CGO_ENABLED=0`). Cross-compiles cleanly. `make build-contenox-windows` and the release workflow produce `contenox-windows-amd64.exe`. `contenox update` has Windows-specific replacement logic.
- **local_shell**: Has real Windows support (detects `pwsh` / PowerShell / `cmd.exe` and uses correct invocation flags).
- **VS Code extension**: Already builds and embeds the Windows binary for `win32-x64`. Uses it via a bridge process.
- **Beam**: Functional local web UI served by `contenox serve`. Shows modeld console, providers, chat, approvals, etc. Not packaged or auto-launched as a desktop app today.
- **modeld Windows**: Packaging infrastructure exists (`package-modeld-release-windows`, `scripts/modeld-deps-bundle-windows.sh`, launcher `modeld.cmd`, zip extraction in `modeldinstall`). Repeatedly marked "unverified" in docs. Requires native Windows toolchain for trustworthy DLLs.
- **Distribution**:
  - GitHub Releases only (raw `.exe` + ACP zip).
  - `install.sh` (bash only, hard-rejects Windows).
  - No MSIX, no code signing, no Store presence, no shortcuts.
- **Onboarding**: Terminal `contenox setup` + manual `model pull`. No first-run GUI flow.
- **Website / docs / landing**: Only show the Linux curl command.

Result: A Windows user following the advertised path immediately hits an error, and even a manual download leaves them with a bare executable and no obvious "app" experience.

## Distribution Strategy

**Primary channel: Microsoft Store (MSIX)**

- Users search/install from the Store (or via `winget --source msstore`).
- Microsoft provides signing and hosting.
- Clean "installed app" experience with shortcuts.
- Auto-updates.
- Good for the gamer/prosumer side of the audience who may discover it in the Store.

**Fallback / power-user channel: PowerShell one-liner**

```powershell
iwr -useb https://contenox.com/install.ps1 | iex
```

This is "more than good enough" for the non-Store population (devs, people without Store, automation, immediate latest bits). The script can create the same shortcuts and run initial setup that the Store package provides.

**Direct downloads** (GitHub Releases + website button) remain available but are secondary.

We deprioritize (at least initially) a traditional GUI installer (Inno Setup `.exe` wizard) because the Store + PowerShell combo covers the main needs. A full wizard can be added later if demand appears.

### Why this is realistic

- The Store is no longer "just for consumer apps." Many dev tools ship there.
- winget + the new Store CLI make Store bits accessible from the terminal.
- A well-written PowerShell script gives the same practical outcome as a simple installer for most users.
- We still need to solve the *experience inside the package* (shortcuts, first-run GPU setup, Beam launch).

## Packaging Approach (MSIX)

MSIX packaging is **not developed in Go**. Go produces the executables; the MSIX is a Windows packaging container.

Typical flow:

1. Build on a `windows-latest` GitHub runner:
   - `contenox.exe` (pure Go).
   - `modeld.exe` + runtime DLLs (when we have reliable Windows prebuilts).
   - Beam SPA assets (embedded or copied).
   - Icons, license, etc.
2. Create / validate an `AppxManifest.xml` that declares:
   - Identity, display name, version.
   - Entry points / shortcuts (desktop, Start Menu).
   - Capabilities needed for a local agent tool (file system, execution, etc.).
   - Any protocol or file associations (future).
3. Package with `makeappx`, the MSIX Packaging Tool, `winapp` CLI, or a Windows Application Packaging Project.
4. (For Store) Submit via Partner Center (requires a Microsoft developer account). Store can provide the final signing.

For direct / PowerShell users we can also publish the raw binaries or a minimal archive. The PowerShell script can optionally register an MSIX or just place files + shortcuts.

### What goes into the package for the Windows product vision

- The `contenox` binary (provides `serve`, `setup`, model management, ACP, etc.).
- Beam web assets (so `contenox serve` works out of the box).
- (When ready) modeld Windows artifacts or a post-install step that pulls them.
- Icons and branding from `website/public/`.
- A manifest that creates a "Contenox" entry that launches the Beam experience (or a small launcher that starts the server + browser).

The CLI remains available in PATH for power users and editors.

## Launch and First-Run Experience

Target flow after install (Store or PowerShell):

1. Desktop / Start Menu icon appears.
2. Click → starts the local server (if not running) and opens Beam (or a welcome surface).
3. First-run detects GPU capabilities.
4. Offers / auto-pulls a default local model we curate and distribute via the existing S3 modeld store (e.g. a Gemma-class model).
5. Runs the equivalent of `contenox setup` (or a GUI version of it) to pick provider + model.
6. User is immediately productive in Beam or can open VS Code.

The PowerShell script and the MSIX manifest should both produce the same shortcuts and the same post-install hook behavior.

## Local Inference (modeld) on Windows

This is a critical dependency for the "agent on your GPU" promise.

- The Go-side download/install logic in `runtime/internal/modeldinstall` already understands Windows (`.zip`, `modeld.cmd` launcher).
- The blocker is reliable production of Windows `modeld` prebuilts (llama.cpp + optional OpenVINO + CUDA).
- We need a repeatable Windows worker path (or improved CI) that produces the bundles consumed by `package-modeld-release-windows`.
- Default models we want to ship must be available in the public index so `contenox modeld install` + `model pull` work on first run.

Until this is solid, the Windows product story is incomplete even if the packaging is perfect.

## VS Code and Other Surfaces

- The VS Code extension is already one of the most "Windows native" surfaces we have (it embeds the platform binary).
- On Windows we should treat the extension experience (chat, canvas, autocomplete) as a first-class way to use the installed Contenox.
- Beam remains the local "cockpit" / full UI.
- The underlying engine (chains, HITL, tools, local_shell) must continue to work excellently under PowerShell/cmd.exe.

## Implementation Pieces (initial cut)

1. **Distribution plumbing**
   - Create `https://contenox.com/install.ps1` (and update website/landing to surface Windows options).
   - Add Windows packaging job(s) in `.github/workflows/` that produce MSIX artifacts on tags (or on demand).
   - Set up Microsoft developer account + Partner Center app registration.

2. **MSIX manifest and packaging**
   - Minimal `AppxManifest.xml` declaring the app, shortcuts, and capabilities.
   - CI step that assembles the package (using `windows-latest` + SDK tools or `winapp` / msstore CLI).
   - Test installation on clean Windows images.

3. **Launch experience**
   - Decide on launcher strategy (direct `contenox serve` + browser, or a small native stub).
   - Create desktop/Start Menu shortcut(s) in both the PowerShell script and the MSIX.
   - Post-install hook that can trigger `contenox setup` or a welcome flow.

4. **First-run / model experience**
   - Curate and make available at least one good default local model for Windows GPU users.
   - Improve `contenox setup` (or add a Beam onboarding path) to be friendly after a fresh install.
   - GPU detection + clear messaging when no accelerator is present.

5. **modeld Windows hardening**
   - Make Windows prebuilt production reliable (repeatable worker, tests, smoke).
   - Ensure `contenox modeld install` + model pull works cleanly on Windows.

6. **Website and docs**
   - Prominent Windows install path (Store button + PowerShell one-liner).
   - Quickstart that shows the Windows flow first or in parallel.
   - Update any "unsupported OS" messages.

7. **Signing & trust**
   - Rely on Store signing for the MSIX path.
   - For direct/PowerShell artifacts, consider a code-signing certificate later (not required on day one if we push Store hard).
