# Windows Development

This guide covers working on the Contenox runtime when your daily driver is Windows, or when you use a Windows machine specifically as a build worker to produce Windows-native `modeld` artifacts.

The codebase is primarily developed on Linux. Windows support exists for:

- The pure-Go `contenox` CLI and most runtime packages.
- The VS Code extension (embeds a Windows `contenox.exe`).
- Producing official `windows-amd64` `modeld` release artifacts (llama.cpp + optional OpenVINO) via a native toolchain.

## Recommended paths

| Path | Best for | Limitations |
|------|----------|-------------|
| **WSL2 (Ubuntu/Debian recommended)** | Day-to-day Go development, CLI work, running tests, UI/Beam packaging, VS Code extension builds | Produces Linux modeld artifacts only. Use a real Windows box or VM when you need to emit Windows DLL bundles. |
| **Native Windows (PowerShell/cmd + Git Bash/MSYS2)** | Verifying Windows-specific behavior (paths, `local_shell`, PowerShell/cmd.exe execution), acting as a `modeld` Windows dependency producer | Tooling for native C/C++ (llama.cpp, OpenVINO) is more involved than on Linux. `make` is often unavailable or partial. |
| **Remote SSH / VS Code Remote** | Offloading heavy work (gopls, builds) to a Linux box while editing on Windows; or driving a Windows worker headlessly | See the SSH section below for bundle production. |

For most contributors: install WSL2 and do the normal Linux flow inside it (see [CONTRIBUTING.md](../CONTRIBUTING.md) and `docs/development/modeld-source-build.md`). Drop to a native Windows checkout only when you need Windows-specific verification or when you are the designated Windows modeld builder.

## WSL2 setup (fast path)

1. Enable WSL2 and install a distro (Ubuntu 22.04/24.04 works well).
2. Inside WSL, follow the Linux prerequisites and build instructions from `CONTRIBUTING.md`.
3. Your Windows files are at `/mnt/c/...`. Clone the repo inside WSL for best performance on Go modules and builds.
4. The VS Code "WSL" extension lets you develop inside the WSL filesystem while using Windows editors/tools.

This path gives you nearly identical behavior to a native Linux dev box for everything except producing Windows `modeld` native bundles.

## Native Windows prerequisites

- **Git for Windows** (brings Git Bash). Configure `core.autocrlf=input` (or `false`) to keep the repo LF-only:
  ```powershell
  git config --global core.autocrlf input
  ```
- Go 1.25+ (official MSI).
- Node.js 22+ (for `packages/ui`, `packages/beam`, `packages/vscode`).
- Make is helpful but not required for everything. You can get it via:
  - Chocolatey: `choco install make`
  - Or just invoke the individual `go build` / `npm` commands.
- For `modeld` Windows native work (see below):
  - MinGW-w64 (via MSYS2 is easiest) **or** Visual Studio Build Tools + Clang/lld.
  - CMake.
  - For OpenVINO-inclusive bundles: matching OpenVINO SDK + GenAI sources built for your toolchain.
- (Optional but recommended) Windows OpenSSH server so you can drive the box from your Linux dev machine.

Build the pure-Go CLI any time:

```powershell
go build -o bin\contenox.exe .\cmd\contenox
```

Or use the cross target (works from any OS):

```bash
make build-contenox-windows
```

## Building and testing on Windows

- Unit tests: `go test -short ./...` (or `make test-unit` if make is present).
- Many system tests assume Unix shells; expect differences in `local_shell`, path handling, and process execution. Run real Windows verification with `cmd.exe` / PowerShell.
- The VS Code extension packages a platform-specific `contenox`:
  - On Windows it embeds `bin/contenox-windows-amd64.exe` (or the native `.exe`).
- Local model inference still requires a separately built `modeld.exe` + its DLLs (see the modeld section).

## Working with modeld on Windows

`contenox` talks to `modeld` (the native daemon) for local llama.cpp / OpenVINO inference. On Windows the daemon and its libraries are `.exe` + `.dll`.

- You normally consume prebuilt Windows bundles produced by a Windows worker (see below).
- For local development of the Go side you can run against a Linux `modeld` (different machine) or build everything natively on the Windows box.

See `docs/development/modeld-source-build.md` for general modeld usage and `docs/integrations/providers/modeld.md` for the user view.

## Producing Windows modeld dependency bundles ("that script")

Windows release artifacts are produced in two stages:

1. A Windows machine builds the native dependencies (llama.cpp runtime DLLs, optional OpenVINO pieces) and runs the bundler to create a content-addressed dependency bundle.
2. A machine with `make` + store credentials (usually Linux) consumes the bundle, assembles the final `modeld-<ver>-windows-amd64.zip`, and publishes it.

The key script on the Windows side is:

```
scripts/modeld-deps-bundle-windows.sh
```

It is a bash script. Run it from Git Bash, MSYS2 bash, or any bash that has the repo on its filesystem. It does **not** compile llama.cpp or OpenVINO — it only packages already-built native outputs into the layout + `manifest.json` + `bundle.env` + fingerprint that the release system understands.

The Makefile target that calls it (on a make-capable host) is:

```bash
make bundle-modeld-deps-windows
```

It simply exports a set of `PLATFORM`, `LLAMA_REF`, `LLAMA_RUNTIME`, `OPENVINO_*`, ... variables and invokes the script. On a Windows box without `make` you set the same variables and run the script directly.

See also:

- `CONTRIBUTING.md` section "Cross-platform release packaging"
- `docs/development/modeld-release-runbook.md`
- `scripts/modeld-deps-bundle-windows.sh` (the script itself documents the expected inputs)
- `mk/llama-flags.mk` and `Makefile.llamacpp-direct` (for how the llama runtime is produced)

### Building the bundle on a Windows worker via SSH (just the building part)

This is the narrow "via ssh" flow for producing the Windows dep bundle when your primary machine is Linux.

**Assumptions on the Windows worker**

- It has a working MinGW or MSVC-based toolchain + CMake that can produce the llama.cpp DLLs (and optional OpenVINO pieces) for the desired variant (cpu / cuda).
- The llama.cpp reference and built runtime directories are already populated from a previous native build step on this box (the heavy CMake work). The bundle script only packages them.
- Git Bash (or MSYS2 bash) is on PATH and the repo is cloned at a matching commit.
- OpenSSH server is enabled (or you use another remote execution method).

**Steps (focused on the bundle production)**

1. From your Linux dev machine, SSH in:

   ```bash
   ssh windows-builder
   ```

2. On the Windows worker (Git Bash / MSYS2 session):

   ```bash
   cd /c/Users/builder/src/github.com/contenox/enterprise/runtime

   # The llama runtime (DLLs + stamp) and optional OpenVINO layout must already exist
   # from prior native toolchain work on this machine. Example layout expectations
   # are enforced by the script (see common_dll / llama_dll lookups and OpenVINO checks).

   # Direct invocation mirroring what `make bundle-modeld-deps-windows` does.
   # LLAMA_CPP_COMMIT must match the commit used for the rest of the release.
   # Provide the OpenVINO_* vars (and non-empty paths) only when including OpenVINO.
   PLATFORM=windows-amd64 \
   OUT=bin/modeld-deps \
   LLAMA_REF="$PWD/.build/llama-ref" \
   LLAMA_RUNTIME="$PWD/bin/llama-runtime-windows" \
   LLAMA_CPP_COMMIT="0123456789abcdef0123456789abcdef01234567" \
   OPENVINO_PKG="" \
   GENAI_SRC="" \
   GENAI_PKG="" \
   TOKENIZERS_LIB="" \
   OPENVINO_GENAI_VERSION="" \
   bash scripts/modeld-deps-bundle-windows.sh
   ```

   The script will print the created bundle name and directory, e.g.:

   ```
   modeld-deps-bundle-windows: bundle dir -> bin/modeld-deps/modeld-deps-windows-amd64-cpu
   modeld-deps-bundle-windows: fingerprint -> ...
   modeld-deps-windows-amd64-cpu
   ```

3. Copy the produced bundle directory back to your primary machine. Example from the Linux side:

   ```bash
   mkdir -p bin/modeld-deps
   scp -r 'windows-builder:~/src/.../runtime/bin/modeld-deps/modeld-deps-windows-*' bin/modeld-deps/
   ```

4. On the Linux machine that has store credentials and `make`:

   ```bash
   make push-modeld-deps
   ```

   (The push step deduplicates by fingerprint and updates the index as needed.)

**Notes**

- The produced bundle records the runtime ABI (`dl-v1` for MinGW-style, `dl-v1-msvc` for the MSVC toolchain). Consumers must request a matching `MODELD_EXPECT_RUNTIME_ABI` when pulling.
- For MSVC builds you must also include the VC++ redistributables (`msvcp140.dll`, `vcruntime140*.dll`, `vcomp140.dll`) in the final package (normally handled by `MODELD_MSVC_REDIST_DIR` during the Linux-side `package-modeld-release-windows` step, or manually when hand-rolling on the Windows box).
- Keep the worker's llama.cpp build inputs in sync with the pinned commit used by the rest of the release (`LLAMA_CPP_COMMIT` from the build environment / `mk/llama-flags.mk`).
- You only need to run the bundle step on the Windows worker. The final `modeld.exe` packaging + smoke + archive + `push-modeld-release` happens from the make-capable host using the pulled bundle.

If you are also hand-assembling the final package directly on the Windows box (no `make`), replicate the steps performed by the `package-modeld-release-windows` target (CGO build of `modeld.exe` against the libs from a bundle dir, write `modeld.cmd` launcher, copy runtime libs, run `scripts/modeld-package-release.sh`) and copy the resulting archive + sidecars back for publishing.

## Common Windows gotchas

- **Shell execution**: `local_shell` and similar tools behave differently under `cmd.exe` vs PowerShell vs Git Bash. Test real Windows scenarios.
- **Paths**: Forward and backslashes are usually tolerated, but be careful with CGO include/lib paths and any code that shells out.
- **Line endings**: The repository uses LF. Let Git handle conversion on checkout.
- **Case sensitivity**: Windows is case-preserving but not sensitive; avoid relying on case-only differences in filenames.
- **Antivirus / real-time protection**: Can interfere with large native builds or many small files during `go build`.
- **CGO cross from Linux**: Possible for some Windows packaging steps with the right mingw cross environment, but native Windows toolchains are required for trustworthy llama.cpp / OpenVINO DLLs.

## See also

- [CONTRIBUTING.md](../CONTRIBUTING.md) — main local development instructions
- `docs/development/modeld-source-build.md` — modeld build/packaging details
- `docs/development/modeld-release-runbook.md` — release process and platform matrix
- `scripts/modeld-deps-bundle-windows.sh` — the bundler script and its required environment variables
- `Makefile` (search for `windows`, `MODELD_WINDOWS_TOOLCHAIN`, `bundle-modeld-deps-windows`)
- `mk/llama-flags.mk` and `Makefile.llamacpp-direct`
