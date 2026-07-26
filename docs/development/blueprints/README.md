# Blueprints

Design records, decision documents, and R&D directions for the Contenox
runtime. Blueprints capture the *why* behind the implementation; user-facing
how-to docs live one level up in `docs/`.

## Active subsystems

| Area | What it covers |
| --- | --- |
| [acp/](acp/README.md) | Agent Client Protocol surface: contenox as agent (registry submission artifacts, e2e conformance) and as client (the client-side engine, fleet and mission machinery) |
| [providers/](providers/README.md) | Cloud/hosted provider integrations |
| [windows/](windows/README.md) | Windows product surface: terminal-first CLI distribution |

## Product

| Doc | Status | What it covers |
| --- | --- | --- |
| [v1-feature-map.md](v1-feature-map.md) | reference | The V1 surface mapped for release testing: boundaries, journeys, per-area risks |
| [local-coding-node-goals.md](local-coding-node-goals.md) | goals | The substrate-neutral "why": what the local coding node must achieve |
| [product-surface-truth-blueprint.md](product-surface-truth-blueprint.md) | rule | Everything surfaced must actually work; the certification stance |
| [tool-hardening.md](tool-hardening.md) | research + staged design | Local tools vs. model diversity: per-model tool surfaces, the ten hardening recommendations, the eval harness |

## Past R&D

| Area | What it covers |
| --- | --- |
| [retired/](retired/README.md) | Retired R&D: the Beam web UI, the modeld local inference daemon, the VS Code extension, and the HTTP API surface — what was built, what was learned, and why V1 ships without them |
