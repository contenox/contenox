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

## Design Philosophy: Engine over Interface

Contenox is not an interactive Agentic IDE like Cursor or Cline, and it intentionally lacks a rich Terminal User Interface (TUI). 

It is designed to be a piece of **programmable infrastructure**:
1. **UNIX Composability:** It uses standard commands, flags, and `stdin`/`stdout` so you can pipe it into `jq`, `grep`, or other tools.
2. **Headless Automation:** By storing state across runs in SQLite, it is built to run autonomously in CI/CD pipelines, cron jobs, or git hooks where no human is watching the screen.
3. **Fire and Forget:** You kick off a plan, go get a coffee, and come back. If it fails, the state is safe on disk, and you just re-run the failed step.

## Beyond the CLI

Under the hood, Contenox is powered by the Contenox Runtime API, a standalone execution engine. If you need to embed these workflows into an application instead of running them from your terminal, the Runtime API can be self-hosted as a scalable REST backend.

## Next steps

- [Quickstart](/guide/quickstart) — install Contenox and run your first chain in 5 minutes
- [Core Concepts](/guide/concepts) — chains, tasks, hooks, transitions explained
- [Chains reference](/chains/) — build your own chains from scratch
