---
title: The nested permission bomb
description: Why inheriting human permissions is a privilege escalation vulnerability, and how to author actual AI boundaries.
---

# The nested permission bomb

If you're rolling out an AI workflow or assistant inside your company, there is a trap waiting for you. It usually starts with the path of least resistance:

> *"The assistant works for the user, so it gets the user's permissions."*

Because an assistant sits locally on your machine or right in your editor, this feels natural. But what looks like a seamless developer experience is actually an authorization anti-pattern. Nested permissions are authorization models where an AI system inherits the full runtime authority of the invoking human. Reuse the identity you already have, inherit the scope, ship the feature.

It is a bomb for the future. Excessive agency and over-privileged automation are named risks in both the NIST AI Risk Management Framework and the ACSC's secure-AI guidance. Here is what breaks the moment you nest your rights into an AI workflow:

* **Prompt injection meets production.** You can merge to main. You can `terraform apply`. You can drop tables. Cloning your full keyring into a local workflow means malicious instructions hidden in a random README are one tool call away from production. The workflow doesn't need your full keyring. It needs a small, scoped one.
* **Your audit trail becomes fiction.** Once the AI acts as you, your logs say *"Tom ran this query at 3am."* Did Tom run it? Did the workflow? You can't tell. SOC 2, SOX, and anything that cares about attribution is broken by default.
* **You've accidentally created recursive delegation.** A local context window triggers a code generation task, which triggers a test execution, which pushes a commit. If each step runs silently with your local shell's full rights, you've built an unbounded delegation loop with no permission boundary.

### AI workflows should not inherit human access

When we frame these systems as direct extensions of ourselves, we default to giving them our exact access profile. But some automated workflows need rights *no human on the team should have*.

If Finance wants a chat interface that can query the data warehouse to answer revenue questions, the right answer is: **"the workflow has read access; the team does not."**

Nested permissions force the exact opposite: you have to grant a human the access first just so the workflow can inherit it.

At Contenox, we built the engine around the belief that least privilege requires explicit, authored boundaries. AI workflows should have explicitly authored capability boundaries, not inherited human supertokens. You don't hand over your keys. You write the policy.

### Separate identities via hidden injection

To fix the audit log problem, the workflow needs its own context. Its credentials should also be short-lived and independently revocable.

When you register a remote tool or MCP server in Contenox, you don't use your personal Bearer token. You bind a service-level credential to the tool and use `--inject` to pass hidden parameters directly into every call.

```bash
contenox tools add warehouse-api \
  --url https://api.internal.com \
  --header "Authorization: Bearer $WORKFLOW_SCOPED_TOKEN" \
  --inject "caller_id=workflow-finance-01" \
  --inject "invoked_by=tom"
```

The model never sees these parameters — they are hidden from the tool schema before it reaches the LLM, so the model can't hallucinate or tamper with them. The Contenox engine merges them back in at execution time. Now your logs tell the truth, and the workflow has its own least-privilege identity.

### Narrow tool surfaces via sub-specs

When you connect a workflow to an external system, you rarely want to give it the entire API. If a workflow's job is to extract leads and draft CRM records, it shouldn't possess a `delete_contact` tool just because the vendor offers it.

Instead of generic API wrappers, Contenox encourages you to drop in a hand-curated OpenAPI sub-spec via `--spec`. By explicitly defining a narrow subset of the API, you restrict the workflow's capabilities at the schema level. Capabilities outside the exposed schema are unreachable by the runtime.

### HITL isn't a checkbox. It's a policy file.

If a workflow runs locally on your machine, it technically has access to your filesystem. But capability does not equal permission.

In Contenox, Human-In-The-Loop (HITL) isn't an opaque "on/off" switch that nags you for every trivial action. It is a programmable safety net. You define your permission boundaries using a strict, version-controlled JSON policy.

```json
{
  "default_action": "approve",
  "rules": [
    {"tools": "local_fs", "tool": "*", "action": "deny", "when": [{"key": "path", "op": "glob", "value": "**/{.ssh,.gnupg,.aws,.azure,.kube,.config/gcloud}/**"}]},
    {"tools": "local_fs", "tool": "*", "action": "deny", "when": [{"key": "path", "op": "glob", "value": "**/{.password-store,.local/share/keyrings,Library/Keychains,.config/1Password}/**"}]},
    {"tools": "local_fs", "tool": "*", "action": "deny", "when": [{"key": "path", "op": "glob", "value": "**/{.bash_history,.zsh_history,.netrc,.npmrc}"}]},

    {"tools": "local_shell", "tool": "local_shell", "action": "deny"},
    {"tools": "webtools", "tool": "web_post", "action": "deny"},

    {"tools": "local_fs", "tool": "write_file", "action": "approve"},
    {"tools": "local_fs", "tool": "sed", "action": "approve"},

    {"tools": "local_fs", "tool": "read_file", "action": "allow"},
    {"tools": "local_fs", "tool": "list_dir", "action": "allow"}
  ]
}
```

*You authored this.* This policy blocks access to credentials and shell history, requires approval for writes, and allows harmless reads without interruption.

The safety rails are explicitly defined and version-controlled in your git repository, not hidden behind a binary we shipped.

### Stop shipping privilege escalation features

AI workflows can be incredibly powerful. Treating them as an unbounded extension of a human user is a trap.

Your workflow doesn't need your full keyring. How it behaves, which tools it can access, and when it needs to pause for human approval should be a policy you author, version in git, and review in PRs — the same artifact engineering already reviews; now your AI safety boundaries live there too.

The identity, the scope, and the approval boundary should be authored explicitly.

Not inherited accidentally.
