# Local Runtime

Contenox runs a local runtime process for the active workspace.

Run setup once to choose the model provider and default model that the editor
should use. The extension reuses the same local Contenox configuration as the
CLI.

In VS Code Remote Development, WSL, Dev Containers, SSH, and Codespaces, the
runtime runs on the remote workspace host. Use a VSIX/Marketplace package that
matches that remote host, or set `contenox.binaryPath` to a `contenox` binary
inside the remote environment.

Useful follow-up commands:

- `Contenox: Show Status`
- `Contenox: Show Output`
- `Contenox: Select Provider`
- `Contenox: Select Chat Model`
