---
title: The pause is yours to define
description: HITL isn't a checkbox. It's a policy file you wrote.
---

# The pause is yours to define

A lot of AI tools advertise "human-in-the-loop." Usually it's a checkbox: turn it on, the tool pauses before every tool call. Turn it off, it doesn't.

That's not how Contenox does it, and the difference is exactly the difference this site is about.

## The pause as a policy you authored

In Contenox, HITL is on by default. But *what* pauses is determined by a policy file — a JSON document you write, version in git, and review in PRs alongside the chains it governs.

The default policy (`~/.contenox/hitl-policy-default.json`) pauses on destructive actions — filesystem writes, `sed`, shell commands, mutating HTTP verbs — and silently allows everything else. But you can author your own:

```json
{
  "default_action": "deny",
  "rules": [
    { "tools": "local_fs",    "tool": "read_file",   "action": "allow" },
    { "tools": "local_fs",    "tool": "list_dir",    "action": "allow" },
    { "tools": "local_fs",    "tool": "write_file",  "action": "approve" },
    { "tools": "local_fs",    "tool": "sed",         "action": "approve" },
    { "tools": "local_shell", "tool": "local_shell", "action": "approve" },
    { "tools": "zendesk",     "tool": "send_reply",  "action": "approve" }
  ]
}
```

Reading files passes silently. Writing files pauses. Shell commands pause. Sending a Zendesk reply pauses. Everything else is denied. You wrote that.

## Why this matters

The "approve everything" model breaks under load. If a chain reads ten files and runs one shell command, you don't want ten approval prompts. You want one — at the side-effect.

The "approve nothing" model breaks under stakes. If a chain might delete a database row, you don't want it to just *try*.

Authored policies are the in-between. The pause goes where the operator who wrote the policy decided it should go. Inside loops where it's safe to read, the agent flows. At the boundary where something irreversible happens, the agent stops and asks.

## Three policies, three postures

Contenox ships with three:

**Default** (`hitl-policy-default.json`). Prompts on writes, `sed`, shell commands, and mutating HTTP verbs. Allows reads and safe HTTP methods. This is what runs out of the box.

**Strict** (`hitl-policy-strict.json`). Deny-by-default. Only explicitly listed tools can run, and those get an approval prompt. For production or compliance-sensitive environments.

**Dev** (`hitl-policy-dev.json`). Allow-all — silent pass-through. For local development when you trust the chain and don't want interruptions.

Switch between them in one command:

```bash
contenox config set hitl-policy-name hitl-policy-strict.json
```

## You own the boundary

The most important thing about authored policies is that the boundary lives in your file. If a security review changes what should pause, you change the policy — not the engine, not a setting screen, not a vendor's roadmap. The gate moved because you moved a JSON key.

That's also why this scales to a team. A reviewer reading your policy file in a PR can see exactly where the pauses are. They don't have to read the engine source or trust a vendor claim. The policy is right there in the artifact, next to the chains it governs.

The chain is the contract. The policy is part of the contract you wrote.
