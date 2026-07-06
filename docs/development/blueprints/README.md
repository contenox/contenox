# Blueprints

Design records, decision documents, and R&D directions for the Contenox
runtime. Blueprints capture the *why* behind the implementation; user-facing
how-to docs live one level up in `docs/`.

## Subsystems

| Area | What it covers |
| --- | --- |
| [modeld/](modeld/README.md) | The local inference daemon: ownership model, session boundary, effective context, llama.cpp and OpenVINO backends, release artifacts |
| [acp/](acp/README.md) | Agent Client Protocol surface: registry listing, submission artifacts, editor integrations |
| [vscode/](vscode/README.md) | VS Code extension: permission bridge, review/UX findings, Marketplace release |
| [beam/](beam/README.md) | The embedded browser UI: restoration, cockpit direction, R&D visions |
| [providers/](providers/README.md) | Cloud/hosted provider integrations and context-caching strategy |

## Product

| Doc | Status | What it covers |
| --- | --- | --- |
| [v1-feature-map.md](v1-feature-map.md) | reference | The V1 surface mapped for release testing: boundaries, journeys, per-area risks |
| [local-coding-node-goals.md](local-coding-node-goals.md) | goals | The substrate-neutral "why": what the local coding node must achieve |
| [product-surface-truth-blueprint.md](product-surface-truth-blueprint.md) | rule | Everything surfaced must actually work; the certification stance |
| [product-surface-audit-20260702.md](product-surface-audit-20260702.md) | working doc | The full-E2E audit driving the niche-down decision |
| [chat-modes-context.md](chat-modes-context.md) | R&D / vision | Chat modes and context injection for editor surfaces |
| [ci-release-hardening.md](ci-release-hardening.md) | plan | Promotion-based release workflow hardening |

## Historical records

| Doc | What it decided |
| --- | --- |
| [local-mode-spec.md](local-mode-spec.md) | The original local-mode plan (SQLite, in-memory bus, estimate tokenizer) that became the `contenox` CLI |
| [remaining-work.md](remaining-work.md) | Roadmap notes from the modeld refactor |
