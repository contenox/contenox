# Contenox Roadmap

Contenox is a local runtime for packaged, auditable AI workflows. The valuable
artifact is a versioned chain: prompts, model routing, tools, budgets, branch
logic, retries, pauses, and human approval gates are explicit enough to review
and commit.

## V1 Direction

The V1 release should make the local runtime reliable before expanding product
surfaces again.

Primary surfaces:

- `contenox` CLI for local workflow execution
- `contenox acp` for ACP clients such as Zed, JetBrains, and AionUi
- `contenox vscode-agent --stdio` through the VS Code extension

## Workstreams

### Feature Coverage

- Keep `docs/blueprints/v1-feature-map.md` as the manual release-test inventory.
- Update it whenever commands, providers, tools, policies, ACP behavior, or VS
  Code behavior change.

### VS Code Marketplace

- Publish as `contenox.runtime` under publisher `contenox`.
- Ship platform-specific VSIX packages with one native `bin/contenox` binary.
- Use the VS Code extension as an adapter around the Go runtime, not a separate
  implementation of Contenox.
- Keep marketplace copy focused on local-first ownership and auditable workflow
  execution, not bridge internals.

### Quality Gate

- CI must build the runtime and run fast unit tests.
- CLI help drift checks must pass after command changes.
- VS Code package checks must prove the VSIX contains only intended files.
- Manual release testing should follow the feature map before a stable V1 tag.
