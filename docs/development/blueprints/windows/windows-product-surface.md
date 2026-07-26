# Blueprint: Windows Product Surface and Distribution

> **Status:** current. Windows is a terminal-first product, same as Linux and macOS.
> **Owner:** runtime
> **Companion:** `../../windows-development.md` (contributor/build side).

## Product surface

On Windows, Contenox is the same product as everywhere else: the `contenox`
CLI, the Beam terminal UI (`contenox beam`), and ACP editor integrations. There
is no GUI app, no Store package, and no installed desktop experience to
maintain — the terminal is the product.

## Current state

- **CLI binary**: Pure Go (`CGO_ENABLED=0`). Cross-compiles cleanly.
  `make build-contenox-windows` and the release workflow produce
  `contenox-windows-amd64.exe`. `contenox update` has Windows-specific
  replacement logic.
- **local_shell**: Real Windows support — detects `pwsh` / PowerShell /
  `cmd.exe` and uses the correct invocation flags.
- **Local inference**: Ollama on Windows, or a reachable vLLM endpoint. No
  in-house inference runtime to package.
- **Distribution**: GitHub Releases (raw `.exe`); `install.sh` covers
  Linux/macOS only.

## Direction

1. **PowerShell install path**: publish `https://contenox.com/install.ps1`
   (`iwr -useb https://contenox.com/install.ps1 | iex`) that downloads the
   release binary, puts it on PATH, and points the user at `contenox setup`.
   Surface it on the website next to the Unix one-liner.
2. **Terminal experience**: `contenox beam` must behave well in Windows
   Terminal and the legacy console — raw mode, resize, colors, and clean
   teardown. This is the Windows "launch experience".
3. **First run**: `contenox setup` on Windows should offer Ollama detection
   and cloud providers, exactly like Unix.
4. **Signing**: a code-signing certificate for the release `.exe` is worth
   considering once SmartScreen friction becomes a reported problem; not
   required on day one.

Store/MSIX packaging and a GUI-first launcher were explored and retired with
the web UI; see [retired R&D](../retired/README.md).
