# Retired R&D

Product lines contenox built, shipped, learned from, and retired. V1 focuses on
one surface: the `contenox` CLI with the Beam terminal UI (`contenox beam`) and
ACP editor integrations. This page is the record; the code and the detailed
design documents live on only in git history.

## Beam web UI

A browser-based chat, fleet, and mission oversight surface embedded in the
runtime and served by `contenox serve`. It proved out chat-on-ACP, inline
permission cards, session workspaces, and multi-agent oversight — ideas that
carried directly into the V1 terminal experience. V1 ships without it because
one interactive surface is enough, and the terminal is where the work already
happens. The name lives on: Beam is now the TUI you get with `contenox beam` —
chat, plan, and shell in one persistent session.

## modeld local inference

An in-house local inference daemon wrapping llama.cpp and OpenVINO, with its
own ownership model, capacity accounting, and release artifacts. It taught us a
lot about honest context windows, VRAM budgeting, and multi-client
coordination — and also that maintaining an inference engine is a product of
its own. V1 delegates local inference to Ollama or vLLM, which do that job
full-time.

## VS Code extension

A full extension with a chat participant, inline autocomplete, runtime
controls, and an ACP permission bridge between the editor and the Go runtime.
The bridge work validated the blocking `session/request_permission` flow that
ACP integrations still use. V1 reaches editors through ACP directly (Zed,
JetBrains, AionUi, OpenClaw), so a bespoke extension is no longer needed.

## HTTP API, apiframework, and the OpenAPI generator

A REST API over the runtime, the `apiframework` package behind it, and an
in-house generator that derived the OpenAPI spec from the Go handlers. It made
the runtime remotely drivable and kept the spec honest against the code. V1 has
no server to describe — the CLI and ACP are the product surface — so the API
and its tooling went with `contenox serve`. Consuming *third-party* OpenAPI
specs as tools is unaffected and remains a core feature.
