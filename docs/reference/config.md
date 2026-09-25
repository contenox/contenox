---
title: Configuration
description: Saved defaults, session choices, agent limits and model capacity.
order: 2
---

# Configuration

Use `contenox config` for everyday defaults and `/settings` for the current
session. Saved defaults live in SQLite; agent definitions, permission rules and
native inference profiles have their own files. A saved default is an input to
resolution, not a claim about a running session.

## Workspaces vs global

Contenox has two layers of state:

- **Global state** — one shared database at `~/.contenox/local.db`. Holds backends, provider configuration, sessions, MCP registrations, and defaults. Shared by every project on your machine.
- **Global runtime files** — `~/.contenox/` also holds `agents.toml` (which carries the `[envelopes.*]` sections), an `agents/` directory for agents you want everywhere, the transpiled envelopes and compiled chains under `~/.contenox/.generated/`, and the shipped chain files under `~/.contenox/system/`.
- **Workspace state** — one `.contenox/` directory per project, containing a `workspace.id` file (a UUID written on `contenox init`), this project's [agent declarations](/docs/guide/declarations/) and `agents.toml`, and any chain or policy files that override a global one by name. Each workspace scopes its own messages and workspace-specific config overrides inside the single global database.

Files resolve by name, workspace first: the workspace `.contenox/`, then `~/.contenox/`, then `~/.contenox/system/`. Copying a shipped chain up out of `system/` is how you take ownership of it.

Running `contenox init` in a project directory creates a `.contenox/` folder with a fresh `workspace.id`, seeds `agents.toml` and `agents/`, and ensures the default runtime files exist under `~/.contenox/`. The same project always resolves to the same workspace regardless of where you invoke `contenox` from, as long as you're inside the directory tree.

Backends and global defaults survive across every workspace. A workspace's sessions and workspace-scoped overrides are invisible to other workspaces.

## Local models

Run `contenox auto` to choose and pull a native model for this machine and open
the TUI. Use `contenox auto --dry-run` to inspect the choice first. See
[native modeld](/docs/integrations/providers/modeld/) for backend installation,
model downloads and hardware requirements. A larger context setting does not
allocate additional GPU memory: modeld reports the capacity it can serve.

## Register cloud or external backends

```bash
# Local Ollama (base URL inferred automatically)
contenox backend add ollama --type ollama

# Ollama Cloud
contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY

# OpenAI (base URL inferred)
contenox backend add openai --type openai --api-key-env OPENAI_API_KEY

# Anthropic (base URL inferred)
contenox backend add anthropic --type anthropic --api-key-env ANTHROPIC_API_KEY

# Google Gemini
contenox backend add gemini --type gemini --api-key-env GEMINI_API_KEY

# AWS Bedrock
contenox backend add bedrock --type bedrock --url https://bedrock-runtime.us-east-1.amazonaws.com

# Self-hosted vLLM or compatible endpoint
contenox backend add myvllm --type vllm --url http://gpu-host:8000

# Vertex AI — --url is required (include project and region)
# Option A: service account JSON (works everywhere)
export VERTEX_SA_JSON=$(cat /path/to/service-account.json)
contenox backend add vertex --type vertex-google \
  --url "https://us-central1-aiplatform.googleapis.com/v1/projects/YOUR_PROJECT_ID/locations/us-central1" \
  --api-key-env VERTEX_SA_JSON

# Option B: Application Default Credentials (CLI only)
gcloud auth application-default login
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
contenox backend add vertex --type vertex-google \
  --url "https://us-central1-aiplatform.googleapis.com/v1/projects/YOUR_PROJECT_ID/locations/us-central1"
```

Backends are **global** — they live in `~/.contenox/local.db` and are visible to every workspace.

## Set persistent defaults

Setting names use dotted namespaces and `snake_case` fields. Numeric names state
the unit where applicable: `window_tokens`, `max_output_tokens`, `max_age_days`.
The model name and its provider are a selection; context and output are separate
budgets, and permission policy selects a set of rules.

```bash
contenox config list
contenox config set inference.context.window_tokens 230000
contenox config set inference.generation.max_output_tokens 8192
contenox config set inference.reasoning.effort high
contenox config set execution.permissions.policy hitl-policy-strict.json
contenox config get inference.context.window_tokens --explain
contenox config get inference.context.window_tokens --json
```

The default listing shows common settings. `config list --all` includes advanced
defaults. `config list --describe` prints each setting's meaning, effect, readers,
environment override and fallback, plus the locations of configuration outside
SQLite. Command help and these descriptions come from the same registry.

`config get` without flags returns the stored default and its scope. `--explain`
and `--json` also resolve environment overrides and built-in defaults. They do
not query a running session or start a model to discover hardware capacity.

Saved inference defaults are global. The chain selection and permission policy
are workspace-scoped and fall back to the global row. Use `--scope global` to
set that fallback explicitly:

```bash
contenox config set execution.permissions.policy hitl-policy-strict.json --scope global
contenox config reset execution.permissions.policy
contenox config reset inference.context.window_tokens
```

Reset removes the override at the selected scope. A workspace reset can expose
a global value; it does not necessarily restore the shipped default. Changes
apply when the relevant reader next loads them; start a new Contenox invocation for saved
inference defaults, and restart the surface for startup settings such as logging.

### Current session

In the TUI or a native ACP session:

```text
/settings
/context 131072
/output 8192
/reasoning high
/model provider/model-name
/permissions hitl-policy-strict.json
/settings inference.context.window_tokens 230000
```

These commands change the current session only. ACP selects expose the same
choices and descriptions; TUI slash completion uses the advertised values.
`/model` accepts an unambiguous bare model name as well as a provider/model pair.
`/think`, `/max-tokens` and `/policy` remain aliases. Unlike older versions,
`/model`, `/provider`, `/max-tokens` and `/policy` no longer save defaults.
Use `contenox config set` explicitly when a choice should apply on future launches.
Session changes are held in memory; reopening a native session uses the launch
and saved defaults again. An external ACP agent owns its own settings.

For context and output, `inherit` removes a session override. `auto` is a choice:

- `/context auto` follows the selected model's reported capacity, still bounded
  by an explicit agent ceiling. It does not mean unlimited. Unknown capacity
  uses a bounded fallback, reported in `/settings`.
- `/output auto` sends no explicit generation cap. The engine reserves one
  eighth of the context window as output headroom.
- `/context inherit` and `/output inherit` return to invocation or saved defaults.

For saved token defaults, `auto` is stored as `0`; `config reset` removes the
stored value. The output setting is tokens **per model call**, not a session
spending cap. A long tool loop can consume many times that amount. Aggregate
mission budgets belong to the permission envelope's compute limits.

### Defaults, overrides and limits

For common inference settings, an explicit invocation flag wins over a supported
environment variable, which wins over the saved default, which wins over the
built-in fallback. The legacy `CONTENOX_DEFAULT_*` environment names remain
supported; `config get <setting> --explain` names the variable for that setting.
A session control then replaces the inherited choice for that session.

Agent and model limits are constraints, not weaker defaults. The effective
history window is the smallest positive value among the chosen window, the
agent chain ceiling and the selected model's reported capacity. `/settings`
shows that result, the chain source and the limits. Output headroom is reserved
within this window; increasing the output cap leaves less room for history.
The backend may further clamp generation to its output capability.

Shipped agents use `[chain] token_limit = 0` to inherit. A positive value is an
additional ceiling. The shipped `max_tokens` template inherits the session
output setting; a literal in a custom chain pins that stage. A declaration's
explicit reasoning effort or pinned model also remains authoritative.

### Existing installations

Old names such as `default-token-limit` and `default-max-tokens` remain aliases
of the same database rows. There is no second store and no value-copy migration.
`config get <setting> --explain` shows the old spelling when needed.

Existing `agents.toml` files and custom chains are preserved. For example, a
saved window of `230000` with an existing agent ceiling of `131072` still yields
`131072`. To make that agent inherit, change the applicable `[chain] token_limit`
to `0` and start a new Contenox invocation. Check both `~/.contenox/agents.toml` and the
workspace `.contenox/agents.toml`, including `[agents.<name>.chain]`. A workspace
or per-agent override can still narrow the global setting. Keep a positive
ceiling when it is deliberate. Generated chain files are outputs; edit their
agent configuration rather than the generated JSON.

### Which envelope a surface runs under

`hitl-policy-name` is the persistent setting. Per run, `--hitl-policy` overrides
it on `beam`, `acp`, `acpx` and `serve`, and each surface resolves in three
steps:

1. **`--hitl-policy <name-or-path>`.** A value carrying a path separator is a
   path and is used **verbatim** — that exact file, and a missing one is an error
   rather than a fallback. Anything else is an envelope name; `strict` and
   `hitl-policy-strict.json` name the same one.
2. **Your own file** — a top-level `hitl-policy-<name>.json` in the workspace
   `.contenox/`, then `~/.contenox/`.
3. **The transpiled envelope**, rendered from `[envelopes.<name>]` in
   [`agents.toml`](/docs/reference/agents-config/#envelopesname) into
   `.generated/hitl-policy-<name>.json` on every run.

```bash
contenox beam --hitl-policy strict                       # by envelope name
contenox beam --hitl-policy ./ops/locked.json            # by path, verbatim
```

Full detail, including what happens when a name resolves to nothing, is in
[Policy resolution order](/docs/guide/hitl/#policy-resolution-order).

Environment overrides are invocation-local and do not rewrite stored values. See the [environment reference](/docs/reference/contenox-cli/#environment-variables).

## Native inference settings

`inference.context.window_tokens` controls the harness history budget. Native
memory allocation, cache precision and device selection belong to modeld and
its backend profiles. Increasing the harness budget cannot raise a physical
capacity limit. A positive explicit window that exceeds the selected model's
reported capacity stops the turn with both values; `auto` follows the reported
capacity. Use `contenox show <model>` to inspect the selected model and
[modeld's reference](/docs/integrations/providers/modeld/) for native settings.

`modeld --min-hot-context=0` disables the hot-context floor, as do zero in the
capacity JSON and environment forms. An omitted setting retains the native
default. `CONTENOX_OPENVINO_DEVICE` overrides a model's `contenox-openvino.json`
`device`; without the environment override, the profile chooses the device.
These settings affect model loading, not an already running conversation's
saved preferences.

## Manage backends

```bash
contenox backend list
contenox backend show openai
contenox backend remove myvllm
```

## Supported providers

| `--type` | Notes                                                                                                     |
| -------- | --------------------------------------------------------------------------------------------------------- |
| `ollama` | Local: run `ollama serve` first. Hosted: use `--url https://ollama.com/api --api-key-env OLLAMA_API_KEY`. |
| `openai` | Use `--api-key-env OPENAI_API_KEY`. Base URL inferred.                                                    |
| `anthropic` | Anthropic Claude (direct API). Use `--api-key-env ANTHROPIC_API_KEY`. Base URL inferred.               |
| `gemini` | Use `--api-key-env GEMINI_API_KEY`. Base URL inferred.                                                    |
| `bedrock` | Amazon Bedrock (Converse API). Requires `--url` (carries the region). Auth: ambient AWS credential chain (env / profile / IAM role), or static keys JSON via `--api-key-env`. |
| `vllm`   | Self-hosted OpenAI-compatible endpoint. Requires `--url`.                                                 |
| `vertex-google` | Vertex AI — Gemini on GCP. Requires `--url` with project and region. Auth: service account JSON via `--api-key-env`, or ADC (no flag needed if `gcloud auth application-default login` is configured). |

## Database location

Contenox uses **one** database at `~/.contenox/local.db` by default. Override with:

- `--db <path>` — use a specific SQLite file (useful for isolated tests or per-environment state)
- `--data-dir <path>` — point at a specific workspace directory (overrides the walk-up discovery)

The walk-up from the current directory only decides **which workspace** you're operating in (by finding a `.contenox/workspace.id` file). The database itself is always the global one unless `--db` is passed.

## External backends for state (opt-in)

By default nothing external is required: the store, the message bus and the key-value cache all live in the one SQLite file above. Three environment variables move them onto servers you run instead. Each is read once, at process start.

| Variable | Moves | Accepted form |
|---|---|---|
| `CONTENOX_POSTGRES_URL` | the store, out of the SQLite file | `postgres://user:pass@host:5432/dbname?sslmode=…`, or a keyword connection string (`host=… user=… dbname=…`) |
| `CONTENOX_NATS_URL` | the message bus, off the database | `nats://host:4222` (also `tls://`, `ws://`, `wss://`); comma-separate a server list |
| `CONTENOX_VALKEY_URL` | the key-value cache, off the database | `valkey://host:6379`, `valkey://user:password@host:6379`, `valkey://:password@host:6379/3?namespace=contenox`, or a bare `host:6379` |

Unset means the SQLite file, unchanged — no migration, no prompt, no difference in behaviour.

Rules worth knowing before you set any of them:

- **A setting that cannot be used stops the process.** A malformed value, or a server that will not accept a connection, is reported by name at startup and the command exits. Contenox never quietly falls back to the local file: asking for Postgres and getting a SQLite file would leave you reading the wrong state.
- **`CONTENOX_POSTGRES_URL` requires the other two.** The database-backed bus and key-value table are written for SQLite and are refused by Postgres, so selecting Postgres without `CONTENOX_NATS_URL` and `CONTENOX_VALKEY_URL` is rejected rather than half-wired.
- **Terminate TLS in front of Valkey.** A `valkeys://` or `rediss://` URL is refused instead of being silently downgraded to a plaintext connection.
- **The schema is applied on connect**, exactly as it is for the SQLite file, so an empty Postgres database is enough to start against.
- **Every process reads the same variables.** Export them where the CLI and any surface you launch will inherit them, or each will resolve its own state.
- **A shared NATS server is shared state.** Subject names are fixed (`mcp.*`, `missionservice.events.*`, and the rest), with nothing in them that identifies a deployment. Two contenox deployments pointed at one NATS server therefore see each other's requests and events. Give each its own server, or its own NATS account.

### Isolating contenox inside a Valkey you already run

Three parts of `CONTENOX_VALKEY_URL` keep the cache out of the way of whatever else uses that server, and all three are honoured or refused — never dropped:

- **The user.** `valkey://appuser:secret@cache:6379` authenticates as `appuser`. A URL with only a password (`valkey://:secret@cache:6379`) authenticates as the server's default user, as before. A user with no password (`valkey://appuser@cache:6379`) is refused rather than sent: unlike NATS, Valkey has nowhere to put a token in the user position.
- **The database index.** The path is the database to `SELECT`: `valkey://cache:6379/3` uses database 3. No path means database 0. A path that is not a database index — `/contenox`, `/-1`, `/3/extra` — is refused at startup rather than quietly becoming 0. A server that will not honour the index (a cluster, or one configured with fewer databases) fails the connection, and the command stops with the variable named.
- **The key namespace.** `?namespace=contenox` prefixes every key contenox writes with `contenox:`, so its `prov:*` and `presence:*` keys become `contenox:prov:*` and `contenox:presence:*` and cannot collide with another tenant's. The prefix is invisible to contenox itself. It also gives you something to write an ACL rule against: `ACL SETUSER appuser on >secret ~contenox:* +@all` confines contenox to its own keys.

A namespace is a literal prefix, so it cannot contain whitespace or the glob characters `*?[]\`. `namespace` is the only query parameter read; any other — `?db=3` included, since the database index belongs in the path — is refused rather than ignored.

Every process that shares the cache must be given the same database index and namespace: they are part of the address, not a per-process preference.

### Checking which backend a process actually uses

[`contenox doctor`](/docs/reference/contenox-cli/#contenox-doctor) grows a **State storage** section as soon as one of the three is set. It names the backend behind each of them and, for a remote one, whether it answered:

```
State storage:
  • store: Postgres (postgres://contenox:xxxxx@db:5432/contenox, from CONTENOX_POSTGRES_URL)
    Status: reachable
  • message bus: NATS (nats://bus:4222, from CONTENOX_NATS_URL)
    Status: reachable
  • key-value cache: SQLite (/home/you/.contenox/local.db)
    Status: local file
```

A Valkey line prints the URL you set rather than just its host — `key-value cache: Valkey (valkey://appuser:xxxxx@cache:6379/3?namespace=contenox, from CONTENOX_VALKEY_URL)` — so the database index and namespace a process is actually using are visible where you check them.

Credentials are masked there, in the URL form and in a keyword connection string. A URL that carries no password loses its whole userinfo instead of the password alone — token auth puts the credential where a username goes, as `nats://<token>@host:4222` does — so a bare `postgres://contenox@db/…` prints as `postgres://xxxxx@db/…` too. With none of the three set the section is absent, which is itself the answer: everything is in the SQLite file. A remote backend that does not answer is named with the variable that selected it, and the command then stops rather than reporting on a runtime it cannot build.

### What opting in gets you

Moving state off the file is worth it when it has to outlive the machine — a container host with no durable disk — or when it belongs in infrastructure you already operate: a Postgres you back up, monitor and can query with your own tools; a NATS server a deployment already runs; a Valkey already in place. Nothing else changes: the same commands, the same schema, the same [event log](/docs/guide/events/), and the same workspace layout on disk.

### What this does not claim

**Selecting shared backends does not make several contenox processes safe to run against one of them.** Nothing here coordinates two runtimes: there is no leader election, no distributed lock, and no fencing token. Each process opens the backends named in its own environment, runs its own background passes, and treats the state it reads as its own. Individual mechanisms do claim a row before working on it, but that is not the same as a deployment designed — or tested — for two processes sharing one backend.

Run one contenox process per set of backends. A multi-process deployment against shared state is not something contenox supports today, and pointing these variables at a shared server does not create it.

Two more things these settings are not:

- **Not a migration.** An existing SQLite file is not copied, read, or converted. Selecting Postgres starts against whatever is in that database — an empty one comes up empty, with no backends and no sessions, and `contenox doctor` will say so.
- **Not a fallback pair.** With a variable set there is one backend, not a preferred one and a spare. If the server is unreachable the command fails instead of continuing on the local file.
