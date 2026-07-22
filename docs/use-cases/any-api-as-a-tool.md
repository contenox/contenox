---
title: Any API, a tool you authored
description: Contenox turns any HTTP API into a scoped, credential-hidden, policy-governed tool — so an assistant, even an untrusted one, reaches exactly the slice you authored and nothing more.
---

# Any API, a tool you authored

An assistant is only as safe as the tools you hand it, and most setups hand it broad ones: a shell, a whole vendor SDK, an API key sitting in the prompt. Contenox takes the opposite position. You take an API and author a *narrow, governed* tool out of it. That authored boundary is the artifact — and it's the reason to put Contenox between a chat assistant and your systems, beyond containment.

## The mechanism

One command turns an HTTP API into a tool:

```bash
contenox tools add crm \
  --url https://api.vendor.com \
  --header "Authorization: Bearer $CRM_TOKEN" \
  --inject "caller_id=assistant-01" \
  --spec ./crm-readonly.json
```

- `--header` / `--inject` — the credential and fixed parameters are bound at the engine, server-side. The model never sees them; it can't exfiltrate or tamper with what it can't read.
- `--spec` — a hand-curated OpenAPI subset. If the vendor offers `delete_contact` and you didn't put it in the spec, it does not exist for the assistant. Capability is narrowed at the schema, not by asking nicely in a system prompt.

You didn't grant the assistant "the CRM." You authored "read these three endpoints, as this caller, with a token it can't see."

## The mechanism is the same; the approval differs by trust

This is what makes a chat-driven assistant genuinely useful *without* the nested-permission-bomb — but Contenox is explicit that *how a call is permitted* depends on who's driving, instead of pretending one model fits both:

- **Device owner (`acp`, Zed/JetBrains):** a human is in the loop. Calls route through interactive HITL — allow, deny, or approve per call.
- **Untrusted driver (`acpx`, OpenClaw/Telegram):** there is no one on that channel to approve anything, so there is no approval prompt. The operator authors which API tools are `allow` in `hitl-policy-acpx.json`; everything else is denied.

## The rule you cannot skip

`acpx` is `default_action: deny`. Registering an API as a tool does **not** make it callable. Until you add an explicit `allow` rule for that exact tool to `hitl-policy-acpx.json`, the assistant is silently refused it — no prompt, no error. That is the boundary working as designed: an untrusted driver reaches only the specific, vetted tools you deliberately allowed, never something merely registered.

That is the whole thesis in one operational fact: capability is opt-in, per tool, by you — version-controlled in a policy file you review like code. Not inherited, not prompted-around, not default-on.

## Where to next

- [The nested permission bomb](/docs/use-cases/nested-permission-bomb/) — why inherited human access is the anti-pattern this replaces.
- [Use from OpenClaw](/docs/integrations/editors/openclaw/) — the untrusted-driver profile, wired end to end.
- [HITL policies](/docs/guide/hitl/) — the authored allow/deny file itself.
- [Remote tools](/docs/integrations/tools/remote/) — registering an API as a tool, in full.
