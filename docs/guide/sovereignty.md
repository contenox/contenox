---
title: AI Sovereignty & the EU AI Act
description: What sovereignty means operationally with contenox — you choose inference and state infrastructure, oversight is a policy you authored, and local and hosted models can work together.
order: 16
---

# AI Sovereignty & the EU AI Act

Sovereignty over an AI system (in German: **AI-Souveränität**) is not a feature you switch on. It is a set of concrete operational questions: where does inference run, where does state live, who holds the credentials, what can the agent reach, and who decides when a human must intervene. Contenox is built so that every one of those answers belongs to the operator — and each answer is a file, a flag, or a grant you can read, version, and revoke.

That matters most where contenox is typically embedded: tool-heavy orchestration, request-processing and analytics chains, scripts and pipelines — the places where an agent's effects reach real systems and the operator has to account for them.

## What sovereignty means operationally

- **You compose the inference boundary.** Run native inference through [modeld](/docs/integrations/providers/modeld/), connect [Ollama](/docs/integrations/providers/ollama/) or [vLLM](/docs/integrations/providers/openai/), and register hosted backends against your own accounts and regions. Local and hosted models can work side by side; providers are configuration, not architecture.
- **You place the state.** SQLite (`~/.contenox/local.db` by default) is the zero-dependency option. Three environment variables move the store, message bus and key-value state onto PostgreSQL, NATS and Valkey you operate — see [External backends for state](/docs/reference/config/#external-backends-for-state-opt-in). No Contenox account or service is required, and telemetry is opt-in and off by default.
- **Secrets resolve from your environment at request time.** Backends reference credentials by environment-variable name (`--api-key-env`); the value is read when a request is made and never lands in a config file on disk.
- **Agents see what you grant, not what you have.** Every agent-reachable shell gets a [scrubbed, least-privilege environment](/docs/guide/confinement/environment/). Chains expose tools through a [per-invocation allowlist](/docs/guide/declarations/#tools). Sessions run only inside the one [workspace](/docs/reference/contenox-cli/#workspace-authority) the instance was launched with — never a directory a client asked for, and never inside the runtime's own config, database, or policies, which no setting can open. And for foreign agent code, contenox carries a kernel-enforced, fail-closed [sandbox](/docs/guide/confinement/sandbox/) — see the [threat model](/docs/guide/confinement/why/) for why that wall is structural rather than cooperative, and the sandbox guide for exactly what it does and does not confine.

## Mapping to the EU AI Act's oversight themes

The EU AI Act (Regulation (EU) 2024/1689) asks, among other things, that high-risk AI systems be designed for **effective human oversight** — its Article 14 language includes the ability of the natural persons overseeing a system to understand it, to intervene in its operation, and to interrupt it — alongside obligations around transparency, record-keeping (Article 12), and risk management. These are the questions an operator asks anyway before leaving an agent alone with real work; the Act happens to ask the same ones. Contenox does not interpret the Act for you. What it gives you are operator-authored mechanisms that map naturally onto those themes:

| Oversight theme | Contenox mechanism |
|---|---|
| **Human oversight** — a person can intervene in or interrupt the system's operation | [HITL policies](/docs/guide/hitl/) as human-in-the-loop checkpoints: authored allow/approve/deny rules evaluated before any tool call executes, failing closed to approval when nothing matches. The [durable approvals inbox](/docs/reference/contenox-cli/#contenox-approvals) records every question as a durable row before anything waits on it — the run then waits on that row, so answering it from any terminal, or from a phone, releases that same run; and if the process ends first, the row and its checkpoint let it resume exactly once elsewhere. How long it waits before it [resolves unanswered](/docs/guide/hitl/#how-an-unanswered-ask-ends), and that it then resolves to a denial, are written on the envelope's grant rather than fixed by the runtime. A run whose process ended before its ask was answered [says it is suspended](/docs/guide/hitl/#the-life-of-an-ask) rather than ending silently, and the open question is re-presented to a client that reconnects, so a waiting decision stays visible to the person responsible for making it. [Attention bounds](/docs/guide/hitl/#who-may-answer-a-subagent-attention) state who may answer an escalated question: a human by default, an agent only if the envelope says so, and only a bounded number of times. |
| **Traceability and record-keeping** | The audit trail is local and readable: [`contenox state`](/docs/reference/contenox-cli/#contenox-state) inspects the captured execution state of past runs — per-task steps, handlers, transitions, and timings per request. `--trace` emits structured operation telemetry on stderr. Durable asks record who answered — and whether it was a person or an agent. Chains and policies are plain versioned files, so the configuration that produced a run is diffable. |
| **Risk controls** | Authored deny rules and condition operators (path globs, host matching, command blacklists, substitution detection) in the [policy file](/docs/guide/hitl/#policy-file-format). An LLM [moderation gate](/docs/use-cases/moderation-gate/) as an ordinary chain step, on a model you choose. [Compute bounds](#compute-and-attention-bounds) capping a mission's total spend. Per-invocation [tool allowlists](/docs/guide/declarations/#tools) and [scoped workflow credentials](/docs/use-cases/nested-permission-bomb/). `contenox vet` validates chains and envelopes before anything runs them, and warns on fields that read stronger than they are enforced. |
| **Data governance** | Operator-placed state, [environment scrubbing](/docs/guide/confinement/environment/), secrets resolved from env at request time, region-pinned backends ([below](#sovereign-deployment-options)), and the [one workspace](/docs/reference/contenox-cli/#workspace-authority) an instance is launched with, bounding where its sessions — and the missions they dispatch — may operate. |

> **Note:**
> This is not legal advice, and using contenox does not make a deployment compliant with the EU AI Act. Whether the Act's obligations apply to your system, and whether a given configuration satisfies them, depends on what you build and deploy — that assessment is yours and your counsel's. What contenox provides are the operational controls such an assessment can point at: authored, versioned, and inspectable rather than implicit.

## Compute and attention bounds

An envelope — the same HITL policy file that gates tool calls — can also carry a `compute` block that puts a ceiling on a mission's total spend, and an `attention` block that says who may answer the unit's questions:

```json
{
  "default_action": "approve",
  "rules": [],
  "compute": {
    "maxTurns": 40,
    "maxToolCalls": 200,
    "maxTokens": 2000000,
    "modelAllowlist": ["qwen3:8b"],
    "backendAllowlist": ["ollama"],
    "onExhausted": "finish_stuck"
  },
  "attention": { "allowAgentAnswers": false }
}
```

- Every compute bound is a **ceiling and opt-in**: absent or zero means unbounded, and bounds only ever restrict — they never grant.
- `maxTurns` is enforced host-side. `maxToolCalls` is validated but not yet enforced by the shipped hosts. `maxTokens` is best-effort, enforced when the unit reports usage.
- `modelAllowlist` and `backendAllowlist` are enforced at the point where a model is resolved, covering chat, prompt, streaming, and embedding calls. A unit cannot switch itself to a model or backend you did not name — which is how you pin an unattended mission to local inference only.
- Exhaustion is never silent: a mission that crosses a bound finishes as stuck rather than running on. (`onExhausted: "pause_ask"` is not implemented and is rejected at validation — an envelope that sets it fails to load and fails `contenox vet`; use `finish_stuck`.)
- The `attention` block is documented in the [HITL guide](/docs/guide/hitl/#who-may-answer-a-subagent-attention): by default only a human may answer a unit's escalated question; an envelope can hand a bounded number of routine questions to the firing agent, and the durable record always shows who answered.

Unknown fields in a `compute` block fail the policy load rather than silently running the mission unbounded.

## Sovereign deployment options

**Native local inference: modeld.** [modeld](/docs/integrations/providers/modeld/) selects a backend and model for the available hardware, owns model loading and resident sessions, and exposes one transport to the rest of Contenox. `contenox auto` installs and verifies that path. With local state and environment-resolved secrets, no workload needs to leave the machine.

**Existing local serving: Ollama or vLLM.** [Ollama](/docs/integrations/providers/ollama/) and self-hosted [vLLM](/docs/integrations/providers/openai/) remain first-class backends. For higher-throughput serving on your own GPUs, Contenox has a native `vllm` backend type and also speaks to vLLM through its OpenAI-compatible endpoint. These backends can coexist with modeld and hosted providers rather than defining a separate deployment mode.

**EU-region cloud.** When you use hosted models, you can still pin where requests are processed, on your own account and keys:

- [AWS Bedrock — EU regions](/docs/integrations/providers/bedrock/#eu-regions): a `bedrock-runtime.eu-central-1.amazonaws.com` (Frankfurt) or other EU-region URL, with `eu.`-prefixed inference profiles.
- [Vertex AI — EU regions and data residency](/docs/integrations/providers/vertex/#eu-regions-and-data-residency): regional endpoints such as `europe-west4` (Netherlands) or `europe-west3` (Frankfurt) keep ML processing in the pinned region; the global endpoint does not.
- [OpenAI — EU data residency](/docs/integrations/providers/openai/#eu-data-residency): eligible API projects created with Europe as their region, served via `eu.api.openai.com`.

A region-pinned cloud backend is a weaker posture than local inference — the provider's terms and infrastructure are still in the loop — but the account, the region, the keys, and the decision remain yours, and swapping to a local backend later is a configuration change, not a rewrite.

## Human + AI collaboration

Sovereignty is not only about where computation happens — it is about who decides. Contenox treats Human + AI collaboration as an authored artifact: the [HITL policy](/docs/guide/hitl/) you wrote decides which actions run unattended, which pause for a person, and which are denied outright. Because asks are durable, that collaboration survives process boundaries — a question a unit cannot decide alone waits in the [approvals inbox](/docs/reference/contenox-cli/#contenox-approvals) for a human answer rather than dying with the process that raised it, and how long it waits before it [resolves unanswered](/docs/guide/hitl/#how-an-unanswered-ask-ends) is a duration you write on the grant, not a default you inherit. The division of labor between you and the agent is a file you can read, review, and change — not a vendor's default you discovered after the fact. That is what "trustworthy AI" means mechanically here: written rules instead of hidden prompts, budgets instead of hope, traces instead of guesswork.

## Next steps

- [HITL policies](/docs/guide/hitl/) — the policy format, condition operators, presets, and attention bounds.
- [Why contenox confines agents](/docs/guide/confinement/why/) — the threat model behind structural confinement.
- [Least-privilege shell environment](/docs/guide/confinement/environment/) — scrub-and-inject for every agent-reachable shell.
- [The pause is yours to define](/docs/use-cases/authored-approval/) — writing and activating your own approval policy.
- [The nested permission bomb](/docs/use-cases/nested-permission-bomb/) — scoped workflow credentials instead of inherited human access.
