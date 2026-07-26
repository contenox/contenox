---
title: Least-privilege shell environment
description: Give an agent exactly the environment its task needs — not your whole .env — by stripping the runtime's secrets and injecting only what you choose.
---

# Least-privilege shell environment

An agent that can run a shell can read its environment — and by default that environment is the contenox process's own, with every variable it was started with. The usual way an agent gets a value it needs, say a `DATABASE_URL`, is to read your `.env`; but reading `.env` for one variable hands it the `STRIPE_SECRET_KEY` two lines below, too.

Contenox inverts that. It gives each spawned shell — the `local_shell` tool, the `shell_session` / `!` PTY, the interactive terminal — exactly the environment a task needs, in two moves:

- **scrub** — strip serve's own credentials out of the shell, so there is nothing to leak;
- **inject** — add back only the variables you choose, with the values you set.

The agent gets its `DATABASE_URL`, does the work, and your other secrets were never in the room. This is the environment slice of least privilege: deny by default, grant what the job needs.

## How it works

Scrubbing is **on by default** for agent-reachable shells. When contenox spawns one, it builds the environment in two layers:

1. The **scrub** filters the contenox process's environment down to a policy you choose. The default, `deny-secrets`, keeps the toolchain's variables but drops the control plane and the common credential shapes.
2. The **injection** overlays your own variables on top, so an injected value always wins — and applies even when the scrub is `off`.

The scrub policy is set per surface:

| Surface | Who drives it | Scrub variable | Default |
|---|---|---|---|
| `local_shell` and the `!` / `shell_session` PTY | the agent | `SANDBOX_SHELL_SCRUB` | `deny-secrets` |
| the interactive terminal panel | the operator, typing directly | `SANDBOX_TERMINAL_SCRUB` | `off` |

Agent-reachable shells scrub by default because the agent is untrusted. The terminal panel is the operator's own shell, so it defaults to `off`; set it to `deny-secrets` or `strict` when you want the same guarantees there.

## Scrub: deny by default

Each scrub variable takes one of three modes:

| Mode | What passes through |
|---|---|
| `off` | The full environment — no scrubbing. |
| `deny-secrets` | Everything **except** the control plane (`CONTENOX_*`), the common credential shapes, and anything in `SANDBOX_ENV_DENY`. |
| `strict` | **Only** a safe base set plus anything in `SANDBOX_ENV_ALLOW`; everything else is absent. |

**`deny-secrets`** is the lowest-breakage posture and the default for agent shells: a toolchain keeps the environment it expects, while these are stripped —

```
CONTENOX_*     *_TOKEN     *_KEY     *_SECRET
*_PASSWORD     *_PASSWD    *_CREDENTIALS
```

**`strict`** hands the shell only the safe base set —

```
PATH   TERM   COLORTERM   TZ   LANG   LANGUAGE   LC_*
TMPDIR   USER   LOGNAME   SHELL
```

— plus whatever you name in `SANDBOX_ENV_ALLOW`. In `strict` the only denies are the control plane and your explicit `SANDBOX_ENV_DENY`, so you can even re-permit a specific inherited credential by naming it. (In `deny-secrets` the credential-shape denies always win, so to pass a specific inherited secret switch to `strict` — or inject the value with `shell-env`, below.)

`SANDBOX_ENV_ALLOW` and `SANDBOX_ENV_DENY` are comma- or whitespace-separated lists of names or globs. A glob is a single leading or trailing `*`: `LC_*` (prefix), `*_TOKEN` (suffix); matching is case-sensitive.

```bash
# Agent shells scrubbed of secrets (the default); lock down the operator terminal too:
SANDBOX_TERMINAL_SCRUB=deny-secrets contenox beam

# Hand agent shells only a hand-picked environment:
SANDBOX_SHELL_SCRUB=strict SANDBOX_ENV_ALLOW="GOCACHE,CARGO_HOME,HTTP_PROXY" contenox beam
```

> [!NOTE]
> Whenever a scrub is active, `CONTENOX_*` — the control plane's own variables — is **always** dropped, and `HOME` is left as the operator's real home. In `off` mode nothing is scrubbed.

## Inject: grant what the task needs

`SANDBOX_ENV_ALLOW` *passes through* a variable that is already in the process's environment. To give a shell a variable that is **not** in the environment — or to set one to a value you choose — inject it directly. Injected variables are global (every spawned shell), stored as plain configuration, and read live, so an edit applies to the next shell without a restart.

```bash
contenox shell-env set DATABASE_URL=postgres://localhost/app HTTP_PROXY=http://proxy:3128
contenox shell-env list
contenox shell-env unset HTTP_PROXY
```

Injected values are layered on top of the scrub, so they always win and apply even when the scrub mode is `off`. They are plain config, **not** a place for secrets.

> [!NOTE]
> `SANDBOX_ENV_ALLOW` and `shell-env` are different tools. `SANDBOX_ENV_ALLOW` *passes through* a variable the process already has; `shell-env` *sets* a variable to a value you choose, whether or not the process has it.

## Verify before you trust it

Preview exactly which variable names a shell would inherit from the contenox process's environment under the current scrub —

```bash
contenox sandbox env            # the agent-shell policy
contenox sandbox env --terminal # the interactive-terminal policy
```

— and list what you inject on top:

```bash
contenox shell-env list
```

`sandbox env` is a dry run against the live environment (names only, values withheld), so you can confirm the scrub strips what you expect before relying on it.

## How it relates to the agent sandbox

This is the **environment** slice of a larger least-privilege architecture: an agent should reach only what its task needs, and nothing else. The [agent-sandbox blueprint](/docs/development/blueprints/acp/agent-sandbox/) describes the rest of "the wall" — making the filesystem, network, and process tree of a spawned agent absent by construction too, so it cannot read your `.env` off disk any more than from the environment. Environment scrubbing and injection are the part of that wall you can use today; the filesystem and network slices land with the sandbox.
