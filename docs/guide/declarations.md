---
title: Declaring agents
description: An agent is one Markdown file with a YAML frontmatter header. Where declarations live, what the frontmatter says, how a directory of them becomes a workflow, and what lives in agents.toml instead.
order: 13
---

# Declaring agents

An agent is one file:

```markdown
---
name: reviewer
description: Reviews a file for correctness problems
tools: Read, Glob, Grep
---

You are a code reviewer. Read the file you are asked about, then list the
problems you can point at in what you actually read.
```

The frontmatter says how to run it, the body becomes its system prompt. Drop it
in and the next run picks it up.

If you have written agents for Claude Code, this is the same file.

## Where declarations live

```
.contenox/
  agents/
    reviewer.md      one agent
    triage.md        another
  agents.toml        what a declaration cannot say
```

`~/.contenox/agents/` works the same way for agents you want everywhere.

`.claude/agents/` and `.agents/agents/` in your project are read where they are.
Those agents are prefixed with the tool they came from (`reviewer` becomes
`claude-code-reviewer`); your own keep their name.

## The frontmatter

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | the agent's identity |
| `description` | yes | when to reach for it |
| `tools` | no | the toolsets it may call. **Omitted inherits every tool**, the same as `"*"` — name them to narrow it |
| `disallowedTools` | no | individual tools hidden from it, by name, whatever the grant above admits |
| `model` | no | routing stays on your configured default unless you pin it |
| `posture` | no | the envelope this agent runs under: `read_only`, `ask_always` or `auto_edit`. Prefer it over `permissionMode` in a declaration you write here — it says what you mean without importing another tool's vocabulary |
| `permissionMode` | no | the Claude Code spelling of the same thing, for a declaration imported from there: `acceptEdits` auto-accepts file writes; otherwise writes and shell ask you first. `posture` wins if both appear |
| `effort` | no | reasoning effort: `low`, `medium`, `high`, `xhigh` |
| `maxTurns` | no | tightens how much one run may spend |
| `mcpServers` | no | MCP servers this agent may reach, or ones it brings itself — see [Tools an agent brings with it](#tools-an-agent-brings-with-it) |
| `remoteTools` | no | OpenAPI services this agent brings itself |

The body is the system prompt and expands the usual macros — `{{tools}}`,
`{{host:os}}`, `{{var:…}}`, `{{date}}` — plus `{{skills}}`.

## What an agent can reach

`tools` is not the whole answer, and reading it as if it were is the one mistake
worth warning about. An agent's reach is the sum of three things:

1. **`tools`** — the hosted toolsets. The vocabulary is small: `"*"` means every
   connected toolset, `!name` removes one, a bare name grants exactly that
   toolset, and an empty list grants nothing. Omitting the field is the same as
   `"*"`. A name like `native-git` or `decl-…` is a namespace so a server you
   connect cannot collide with a toolset contenox hosts — it is **not** a hidden
   exclusion.
2. **`mcpServers`** — granted by naming the server. You do **not** also list it
   under `tools`; declaring it is the grant.
3. **`remoteTools`** — the same, for an OpenAPI service the agent brings.

So an agent with `tools: Read, Bash` and `mcpServers: [github]` can reach Read,
Bash **and** every tool that GitHub server serves. For the complete roster a
session actually holds, ask the runtime rather than reading the file:
`contenox doctor` prints it with each tool's origin, and `/doctor` inside a
session prints the live one.

Admission is by **toolset**: a name resolves through [`[tools]`](/docs/reference/agents-config/#tools)
to a `toolset.tool`, and the agent is granted that toolset.

| You write | The agent gets |
|---|---|
| no `tools:` line | every connected toolset — the declaration inherits |
| `tools: "*"` | the same thing, said out loud |
| `tools: Read, Grep` | exactly the toolsets those names resolve to |
| `disallowedTools: Bash` | the above, minus the individual tool `Bash` resolves to |

> **Quote the star.** This is YAML, where a bare `*` opens an alias, so
> `tools: *` fails to parse the whole file. Write `tools: "*"` or
> `tools: ["*"]`.

The two halves work at different grains, deliberately: `tools:` admits whole
toolsets, and `disallowedTools:` hides individual tools out of what was admitted.

Tool names resolved out of the box: `Read`, `Write`, `Edit`, `Bash`,
`PowerShell`, `Glob`, `Grep`, `WebFetch`. An unknown name is dropped and
reported; the agent runs with the rest. A declaration where no tool resolves
fails.

Naming a tool is not permitting it: what happens when the call is actually made
is the [envelope's](/docs/guide/hitl/) decision.

## Skills: procedures for repeated work

A skill is a Markdown file describing how to do a recurring job — which tools to
call, in what order, what to show the human, where to file the result. Put them
beside your agents:

```
.contenox/
  agents/
    office.md
  skills/
    timesheet.md          one procedure
    release/SKILL.md      or a folder, when it ships reference files
```

Pull the inventory into an agent with `{{skills}}`; only the one-line
description costs context, and the agent reads the file when a request matches.
The macro expands when the chain is generated, not per request, so the prompt
stays a stable cache prefix. Frontmatter is optional — a bare Markdown file takes
its name from the filename and its description from the first line.

Skills are read relative to the project, so they live in the workspace
`.contenox/skills/`; one in `~/.contenox/skills/` is not listed, because the
agent's file tool is rooted at the project and refuses absolute paths.

**Skill or agent?** A skill loads into the agent you are already talking to and
keeps the conversation's context; an agent is a separate actor with its own
session, envelope and dispatch. Reach for a skill when the procedure should come
up mid-task, and an agent when it is a job you start.

## Tools an agent brings with it

Most agents use tools you connected once and share across all of them. An agent
that needs something of its own can carry it in its declaration instead.

A list under `mcpServers` is a **grant** — this agent may reach these
[MCP servers](/docs/integrations/tools/mcp/), and nothing else new:

```yaml
mcpServers: [github, linear]
```

A **mapping** defines servers rather than naming them, and `remoteTools` does the
same for any [OpenAPI service](/docs/integrations/tools/remote/):

```yaml
---
name: researcher
description: Researches a question against internal sources
mcpServers:
  filesystem:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
  linear:
    type: http                       # or sse
    url: https://mcp.linear.app/mcp
    authEnvKey: LINEAR_TOKEN         # the variable's name, never its value
remoteTools:
  billing:
    url: https://internal.example.com
    spec: https://internal.example.com/openapi.json
---
```

These are registered **scoped to this agent**: two agents may each bring a
`filesystem` without colliding, and deleting the declaration retires what it
brought. They show up in `contenox mcp list` and `contenox tools list` under an
`OWNER` of `declaration`. Anything you registered yourself is never touched, and
editing a `declaration`-owned row by hand does not stick — the next discovery
pass writes it back from the file.

**Credentials stay out of the file.** A declaration is committed to source
control, so it may not carry a literal credential. Name the environment variable
instead — `authEnvKey: LINEAR_TOKEN` is accepted, `authToken: sk-live-…` is
refused with the file and field named. For a server that needs an interactive
login, register it once with `contenox mcp add` and `contenox mcp auth`, then
**name** it from the declaration.

**A declaration can arrive with a `git clone`** — `.claude/agents/` is read out
of your workspace, and a stdio `command` starts that process when the agent is
registered. Treat a declaration the way you treat a `Makefile`, and read one
before you run an agent from a repository you do not control.

## Branching: the directory is the chain

One declaration is one loop: a turn, its tools, and a bounded second attempt.
When a request needs *different* loops — changing code is not the same job as
reviewing it — you make directories:

```
agents/
  triage/
    agent.md          the classifier: which branch handles this?
    code/
      agent.md        one loop
      recovery.md     its second attempt (optional)
    docs/
      agent.md        another loop
    failure.md        what it says when every branch has given up
```

`contenox` reads that as **one agent** called `triage`. The `agent.md` beside the
subdirectories becomes a router; each subdirectory becomes a branch; every leaf
is the ordinary five-task loop a single declaration already emits. Nesting works
without any further idea — a branch that itself branches is just a directory with
children.

**The label is the directory name.** The router does not list its branches; they
are the directory names, and contenox appends them to your classifier prompt with
each branch's `description`, so the model is told exactly which answers are valid.
In a hand-written chain the prompt names its labels in prose while the transitions
match the same strings by equality, and nothing keeps the two in step. Here there
is one string, so there is nothing to drift.

**The default is required.** A classifier answering something unmapped is
ordinary, so `default:` says where that goes, and it is refused if it names no
branch. A router with exactly one branch needs no `default:`. If the classifier
itself fails, the request also takes the default branch.

**`recovery.md` is present or absent, never a flag.** A recovery prompt is written
for an agent that has already failed once, so it is a file; a branch that should
simply give up omits it.

**`failure.md`** sits at the root of a tree: one per tree, because there is one
report, and which branch was running is already in the transcript.

**Budget left is a macro, not a number.** A recovery prompt can say
`{{rounds_used}} of {{main_rounds}}`; contenox resolves those when it emits the
chain, from this leaf's own loop and from `agents.toml`, so a prompt cannot
promise a budget the chain does not enforce.

The classifier runs on `router_model` / `router_provider`, which default to your
ordinary model: choosing a lane is a one-word answer.

## What a declaration cannot say

Context budgets, retries, loop bounds and shell allowlists live in
[`agents.toml`](/docs/reference/agents-config/) beside your declarations. A value
there applies to every agent unless you nest it under `[agents.<name>]`, and keys
you leave out keep the value from the layer below — so a per-agent section is
only the difference, never a restatement.

```toml
# .contenox/agents.toml
[chain]
token_limit = 131072

[agents.reviewer.chain]
token_limit = 32768
```

A section naming an agent that does not exist is reported rather than ignored, so
a typo does not read as a knob that does nothing.

## Checking before anything runs

[`contenox vet`](/docs/reference/contenox-cli/) validates what a declaration
became — handler signatures, dataflow, rule shapes, transitions that can never
fire:

```bash
contenox vet
```

## Where to go next

- [The chain and the envelope](/docs/guide/comparison/) — what a declaration
  compiles into, and why they are two files
- [`agents.toml`](/docs/reference/agents-config/) — every knob a declaration
  cannot reach
- [Writing a chain by hand](/docs/guide/chains/writing-a-chain/) — for the cases
  a declaration cannot express
- [Human in the loop](/docs/guide/hitl/) — the envelope in full
