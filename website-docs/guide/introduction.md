# Introduction

Contenox is a local AI agent CLI that runs deterministic, observable workflows as explicit state machines.

Instead of wiring prompts together with ad-hoc Python glue, you define your AI behaviour as a **task chain** — a JSON graph of typed tasks, transitions, and tool calls. Every step is inspectable, replayable, and testable.

## How it works

```
User input
    │
    ▼
┌─────────────────────┐
│   Task Chain (JSON) │  ← you define this
│  task → task → …   │
└─────────────────────┘
    │
    ▼
Model (Ollama / OpenAI / vLLM / Gemini)
    │
    ├─ tool call? → Hook (local shell, remote API)
    │                    │
    └─ text reply ←──────┘
```

Each task has a **handler** (what it does), an optional **LLM config** (which model, which hooks), and a **transition** (where to go next). The chain engine drives the loop — the model doesn't.

## Beyond the CLI

Under the hood, Contenox is powered by the Contenox Runtime API, a standalone execution engine. If you need to embed these workflows into an application instead of running them from your terminal, the Runtime API can be self-hosted as a scalable REST backend.

## Next steps

- [Quickstart](/guide/quickstart) — install Contenox and run your first chain in 5 minutes
- [Core Concepts](/guide/concepts) — chains, tasks, hooks, transitions explained
- [Chains reference](/chains/) — build your own chains from scratch
