# Blueprints

Design records, decision documents, and R&D directions for the Contenox
runtime. Blueprints capture the *why* behind the implementation; user-facing
how-to docs live one level up in `docs/`.

## Subsystems

| Area | What it covers |
| --- | --- |
| [modeld/](modeld/README.md) | The local inference daemon: ownership model, session boundary, effective context, llama.cpp and OpenVINO backends, release artifacts |
| [acp/](acp/README.md) | Agent Client Protocol surface: contenox as agent (registry submission artifacts, e2e conformance) and as client (the client-side engine, fleet and mission machinery) |
| [vscode/](vscode/README.md) | VS Code extension: permission bridge, local-model availability, Marketplace release |
| [beam/](beam/README.md) | The embedded browser UI: chat-on-ACP doctrine, fleet/mission oversight surfaces, workspace surfaces |
| [providers/](providers/README.md) | Cloud/hosted provider integrations |
| [windows/](windows/README.md) | Windows product surface, distribution (Store + PowerShell), MSIX packaging, GUI-first / desktop agent experience |

## Product

| Doc | Status | What it covers |
| --- | --- | --- |
| [v1-feature-map.md](v1-feature-map.md) | reference | The V1 surface mapped for release testing: boundaries, journeys, per-area risks |
| [local-coding-node-goals.md](local-coding-node-goals.md) | goals | The substrate-neutral "why": what the local coding node must achieve |
| [product-surface-truth-blueprint.md](product-surface-truth-blueprint.md) | rule | Everything surfaced must actually work; the certification stance |
| [tool-hardening.md](tool-hardening.md) | research + staged design | Local tools vs. model diversity: per-model tool surfaces, the ten hardening recommendations, the eval harness |
| [windows/windows-product-surface.md](windows/windows-product-surface.md) | initial | Windows GUI-first shift, Store-primary distribution, PowerShell fallback, launch + local GPU agent experience |
