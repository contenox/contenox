---
title: HITL Policies
description: Control which tool calls require human approval using named policy presets.
---

# HITL Policies

Human-in-the-loop (HITL) lets you intercept tool calls before they execute and decide — approve, block, or let them pass automatically — based on a named policy file.

## How it works

HITL is **on by default**. When the engine runs a tool call, it evaluates the active policy and takes one of three actions:

- **approve** — pause and prompt the user; execution continues only after explicit approval
- **allow** — pass through silently (no prompt)
- **deny** — reject the call immediately without prompting

In the **CLI**, approval prompts appear inline in the terminal (TTY):

![A destructive rm command stops at a human approval gate before it runs](/hitl-approve.gif)

To disable HITL entirely, pass `--auto`:

```bash
# HITL on (default) — the engine pauses before destructive tool calls:
contenox chat --shell "refactor main.go"

# HITL off — autonomous mode, no approval prompts:
contenox chat --shell --auto "refactor main.go"
contenox run  --auto --chain ./my-chain.json "do the thing"
```

> [!WARNING]
> `--auto` disables all approval prompts. Use only in trusted environments or non-interactive scripts.

## Policy file format

A policy is a JSON file with an optional `default_action` and a list of `rules`:

```json
{
  "default_action": "deny",
  "rules": [
    { "tools": "local_fs",    "tool": "write_file",  "action": "approve" },
    { "tools": "local_fs",    "tool": "sed",         "action": "approve" },
    { "tools": "local_shell", "tool": "local_shell", "action": "approve" },
    { "tools": "webtools",    "tool": "web_post",    "action": "approve" },
    { "tools": "webtools",    "tool": "web_put",     "action": "approve" },
    { "tools": "webtools",    "tool": "web_patch",   "action": "approve" },
    { "tools": "webtools",    "tool": "web_delete",  "action": "approve" }
  ]
}
```

| Field | Type | Description |
|---|---|---|
| `default_action` | `"allow"` \| `"deny"` | Action for tool calls that match no rule. Defaults to `"allow"` if omitted. |
| `rules[].tools` | string | Tools name (`local_fs`, `local_shell`, a remote tool name, …) |
| `rules[].tool` | string | Tool name within that tool (`write_file`, `sed`, `local_shell`, …) |
| `rules[].action` | `"approve"` \| `"allow"` \| `"deny"` | What to do when this rule matches |

Rules are evaluated top-to-bottom; the first match wins.

## Built-in presets

Contenox ships three policy presets. They are written to `.contenox/` by `contenox init`.

| Name | Behaviour |
|---|---|
| `hitl-policy-default.json` | Prompts for filesystem writes, `sed`, shell commands, and the mutating webtools verbs (`web_post`, `web_put`, `web_patch`, `web_delete`); allows everything else (including `web_get` / `web_head`) |
| `hitl-policy-strict.json` | Deny-by-default; only the rules listed are prompted |
| `hitl-policy-dev.json` | `default_action: allow` — silent pass-through; useful for local development |

## Selecting the active policy

```bash
contenox config set hitl-policy-name hitl-policy-strict.json
contenox config get hitl-policy-name   # verify
```

This writes to the KV store and takes effect immediately — no restart required. The setting applies to all subsequent invocations in the same workspace.

## Policy resolution order

When HITL is enabled and a tool call needs evaluation, the engine resolves the policy as follows:

1. Read the `hitl-policy-name` key from the KV store.
2. If set, load that file from the workspace `.contenox/` directory, falling back to `~/.contenox/`.
3. If the key is empty or the file is missing, fall back to `hitl-policy-default.json`.
4. If that file is also missing, use a built-in allow-all policy (equivalent to `hitl-policy-dev.json`).
