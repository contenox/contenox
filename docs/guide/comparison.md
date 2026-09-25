---
title: How contenox compares
description: Direct agentic work across existing systems, models, and interfaces while retaining control of its capabilities and boundaries.
order: 3
---

# How contenox compares

contenox is built for agentic work that reaches beyond one prompt, model or
interface. It carries the same declarations, capabilities, policies and state
from the terminal and editor into scripts and unattended execution.

A terminal, a model loop, and file editing are useful parts of a harness.
contenox also puts API integration in the ordinary configuration surface. You
can give an agent access to an existing service without writing a harness
extension for each operation.

## Existing systems as tools

An MCP server supplies its tools through the protocol. An HTTP service supplies
them through an OpenAPI v3 description. You configure the endpoint,
authentication, and any arguments the runtime must supply rather than letting
the model choose them.

```bash
contenox tools add erp --url https://erp.example.com \
  --spec ~/.contenox/erp-subset.yaml

contenox mcp add inventory --transport stdio \
  --command inventory-mcp
```

These are configuration examples: the service, spec, and MCP executable are
yours to provide. A subset spec can expose just the operations needed for the
job. Agent declarations can carry their own service definitions, so the tools
belong to the workflow that uses them.

That is useful when a job crosses systems: provision machines through Proxmox,
configure a Kubernetes cluster, set up a GitLab pipeline, and record the result
in BookStack or ERPNext. Those services need suitable API descriptions or MCP
servers and credentials with the required access. The integration mechanism
stays the same as the job changes.

## Configuration and extensions

An extensible harness lets you implement capabilities in its host language.
contenox provides a declarative path for services that already have a machine
interface. Both approaches have a place.

| Work | In contenox |
|---|---|
| Connect an existing API | Register its OpenAPI spec or MCP server and configure authentication. |
| Give an agent a role | Write a Markdown declaration with instructions and tool access. |
| Require approval for an operation | Set an envelope rule evaluated at the tool boundary. |
| Repeat a workflow | Run a declared chain from the terminal, CI, or cron. |
| Add behaviour the service does not expose | Implement a tool or adapter; a declaration cannot supply a missing API. |

If you prefer to express orchestration in application code, an SDK or an
extension-oriented harness may fit better. contenox is useful when you want to
configure access to existing systems and reuse it across agents and jobs.

## The chain and the envelope

The chain describes the work. The envelope describes what is permitted:
operations that may proceed, operations that need approval, and operations
that are denied. They can change independently. Most users start with an agent
declaration and a preset; explicit chains are available when the workflow needs
branches, retries, or bounded tool loops.

Approval asks are stored locally. A suspended run can retain its checkpoint
while it waits for an answer. You can inspect and answer asks with
`contenox approvals`, then resume the work.

The model still needs enough capability and context for the task. An API schema
describes how to call a service; it does not establish that a proposed change
is correct. Credentials, tool scope, and approval policy remain the operator's
responsibility.

## Start with one service

- [Remote tools](/docs/integrations/tools/remote/) — OpenAPI specs, authentication, and injected arguments.
- [MCP servers](/docs/integrations/tools/mcp/) — local processes and remote endpoints.
- [Declaring agents](/docs/guide/declarations/) — instructions and the tools an agent may reach.
- [HITL policies](/docs/guide/hitl/) — permissions and approval rules.
- [Writing a chain](/docs/guide/chains/writing-a-chain/) — explicit workflow control.
