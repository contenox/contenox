---
title: What contenox is
description: Direct agentic work from the terminal, an editor, or automation across models and systems under boundaries you control.
order: 2
---

# What contenox is

**AI agents, under your command.** `contenox beam` is the first-party terminal:
a session at your own keyboard, with your filesystem and shell. `contenox acp`
serves the same work over stdio to Zed, JetBrains, AionUi, OpenClaw and anything
else that speaks the Agent Client Protocol. `contenox run` carries it into
scripts, CI and unattended execution. No Contenox account is required.

Four things are in the box, and they are the product:

- **The client.** `beam`, in-tree, so the front door is never somebody else's
  product. Bare `contenox` on a terminal opens it.
- **The declaration.** An agent is a Markdown file with a YAML frontmatter
  header, in `.contenox/agents/`. A directory of them is a workflow. The
  declarations you already keep for Claude Code, Copilot, Cursor, OpenCode or
  Antigravity are read where they are.
- **The compile step.** Each declaration becomes a **chain** (what happens) and
  an **envelope** (what is permitted) — JSON, schema-validated, yours to read,
  neither yours to maintain.
- **The tool boundary.** contenox ships no tools. Your editor or terminal client
  supplies `fs/*` and `terminal/*`; you attach MCP servers and OpenAPI services;
  every call crosses one boundary before it runs.

SQLite provides the zero-dependency default for state. Deployments that need
operated infrastructure can use PostgreSQL for the store, NATS for messaging
and Valkey for key-value state. Native inference runs through modeld; Ollama,
vLLM and hosted providers can be used alongside it. Telemetry remains opt-in.

## Why I built it

I wanted to hand an agent a job and walk away — and the thing that stopped me was
never whether the model was good enough. It was that I had no way to say *this
directory is untouchable* and know it would hold when the model decided a cleanup
was in order.

So the answer had to be a file I wrote, that I could read back, and that was
enforced by something other than the model's good intentions. Once that file
existed the rest followed: the same file names what may not proceed without me,
and the things that start work are declared rather than discovered.

There is a second reason, and it is not smaller. I did not want the record of how
my work behaves to live in someone else's dashboard — a product that can pivot,
exit, leak, or turn my usage into their next feature. It runs on my machine. That
is a choice about who holds the evidence, not a deployment preference.

## What it is typically used for

- **Working in a repository** — review a diff, plan a change, make it, run the
  tests, fix what broke, all in `beam` or in the editor you already use.
- **Repeatable jobs** — a review pass, a release-notes draft, a dependency
  sweep — declared once and fired with `contenox run` from a shell, CI or cron.
- **Reaching systems the model should not hold keys for** — an internal API
  wrapped as a tool with its sensitive arguments filled in by config.
- **Work that must stop for a person** — anything that sends, deploys or
  deletes, held at the tool boundary until someone answers.
- **Local inference** where nothing may leave the network, or a hosted model on
  your own key and your own region — a config change, not a rewrite.

## What it is not

- **Not a chat product.** There is no thread to keep warm and nothing that wants
  you back tomorrow.
- **Not a dashboard.** No screen summarises what an agent did; if you want to
  know, you read the captured execution state on your own disk.
- **Not a hosted service.** No Contenox service sits in the execution path.
  External calls go only to the model backends and tools you configured.
- **Not an editor plugin.** ACP is a protocol the editor speaks; the terminal
  client needs no editor at all.
- **Not a framework or an SDK.** You run it; you do not import it.
- **Not autonomous.** It does exactly what you declared and stops where you said
  stop. That is the feature.

## Where to go next

- [Declaring agents](/docs/guide/declarations/) — the file, the frontmatter, and
  what `agents.toml` supplies that a declaration cannot.
- [Human-in-the-loop policies](/docs/guide/hitl/) — the envelope in full.
- [How contenox compares](/docs/guide/comparison/) — API integration, workflows, and execution policy.
- [Envelope JSON Schema](/schema/hitl-policy-v1.schema.json) and
  [chain JSON Schema](/schema/task-chain.schema.json) — the formats, generated
  from the code that loads them.
- [Sovereignty](/docs/guide/sovereignty/) — local inference, EU regions, and what
  the EU AI Act asks of whoever deploys.
