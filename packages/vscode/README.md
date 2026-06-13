# Contenox for VS Code

Run local, reviewable AI workflows in VS Code.

Use it for codebase chat, source-aware explanations, diagnostics help, commit
drafts, workspace review, inline autocomplete, and tool-assisted workflows that
stay under your control. The extension is the VS Code frontend for the Contenox
runtime: it uses the models and providers you configure, keeps state on your
machine, and asks for approval before crossing policy boundaries.

## Why Contenox

Modern coding agents are useful, but too much AI-assisted work still depends on
fragile prompt habits, hidden context, vendor-specific behavior, and black-box
tool execution.

Contenox takes a different approach: repeated AI work should be defined,
inspected, versioned, and controlled like an engineering artifact. Contenox
Chains make prompts, model routing, tool access, retries, branches, budgets, and
approval gates reviewable.

Contenox is runtime-first, not editor-first: it sits across your editor,
terminal, project state, tools, sessions, and model/provider configuration.

## What You Can Do

- Chat with `@contenox` in VS Code Chat without requiring GitHub Copilot sign-in
  for Contenox-owned requests.
- Ask about selected code or open files with editor context attached.
- Explain and fix diagnostics from the command palette or lightbulb menu.
- Review workspace changes and draft commit messages from the current diff.
- Use inline autocomplete powered by your configured Contenox model/provider.
- Connect local or external models, including local GGUF models and common cloud
  providers.
- Use MCP, OpenAPI, and local tools through Contenox runtime policies.
- Approve or deny gated tool actions before they run.
- Keep sessions, configuration, telemetry, and workflow state local.

## Local-First Ownership

The extension includes the Contenox runtime and starts it locally for the active
workspace. You can use the bundled runtime, point the extension at an existing
Contenox binary, or reuse an existing Contenox data directory.

In VS Code Remote Development, WSL, Dev Containers, SSH, and GitHub Codespaces,
the extension runs as a workspace extension on the remote workspace host. That
means the Contenox runtime also runs on the remote host, not on your local
laptop. Install the Marketplace/VSIX package that matches the remote operating
system and architecture, or set `contenox.binaryPath` to a `contenox` binary
that exists inside that remote environment.

Contenox is designed around:

- local-first usage
- model and provider choice
- auditable execution traces
- human-in-the-loop approval
- explicit tool and policy boundaries
- workflows that can graduate from chat into repeatable Chains

## Getting Started

1. Install the extension.
2. Run `Contenox: Run Setup` from the command palette.
3. Choose or configure a model/provider.
4. Run `Contenox: Open Chat`.
5. Ask `@contenox` about your code, selected text, diagnostics, or workspace
   changes.

For inline suggestions, make sure `contenox.autocomplete.enabled` is enabled and
run `Contenox: Trigger Autocomplete` or `Contenox: Test Autocomplete At Cursor`.

## Remote Development

Contenox declares itself as a VS Code workspace extension so workspace files,
git state, terminals, tools, and the runtime bridge live on the same machine as
the checked-out code.

Remote behavior to expect:

- SSH, WSL, containers, and Codespaces need a Linux-compatible runtime in the
  remote environment.
- `Contenox: Run Setup` opens a terminal in the remote workspace.
- `contenox.dataDir` is resolved in the remote environment when set.
- Local diagnostic telemetry is written where the runtime runs.
- Use `Developer: Show Running Extensions` to confirm Contenox is running in the
  remote extension host.

If the bridge cannot start, run `Contenox: Show Output`; the log includes the
remote name, platform, architecture, runtime path, and the exact spawn error.

## CLI Compatibility

If you already use Contenox from the terminal, the extension can reuse the same
state and configuration. Common settings are available through the CLI:

```sh
contenox config set default-provider mistral
contenox config set default-model codestral-latest
contenox config set default-autocomplete-provider mistral
contenox config set default-autocomplete-model codestral-latest
```

## Links

- Website: https://contenox.com
- Runtime source: https://github.com/contenox/runtime
- License: Apache 2.0
