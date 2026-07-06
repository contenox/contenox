---
title: Multi-provider fallback as authored resilience
description: The vendor doesn't choose your failure mode. You do.
---

# Multi-provider fallback as authored resilience

When a model call fails — rate limit, timeout, provider outage — something has to happen. Most tools pick a behavior for you: retry, error, fallback to a default. Contenox makes you pick.

## The shape

```json
{
  "id": "summarise",
  "handler": "chat_completion",
  "system_instruction": "Summarise the input in two sentences.",
  "execute_config": {
    "models": ["qwen2.5:7b", "gpt-4o-mini", "gemini-2.0-flash"],
    "providers": ["ollama", "openai", "gemini"],
    "retry_policy": {
      "max_attempts": 3,
      "initial_backoff": "1s",
      "max_backoff": "10s",
      "jitter": 0.25,
      "rate_limit_min_wait": "10s"
    }
  },
  "transition": {
    "on_failure": "summarise_locally",
    "branches": [{ "operator": "default", "goto": "end" }]
  }
}
```

That single task encodes a four-level resilience policy:

1. Try `qwen2.5:7b` on Ollama. If it transient-fails, retry with backoff and jitter (3 attempts).
2. If retries exhaust on Ollama, fall over to `gpt-4o-mini` on OpenAI. Retry there.
3. If OpenAI also exhausts, try `gemini-2.0-flash` on Gemini.
4. If all three fail, route to the `summarise_locally` task — which might use a smaller embedded model, or just truncate the input. Whatever you wrote.

## Why authoring this matters

A vendor-supplied "fallback" forces a single shape. Maybe it always retries the same model. Maybe it falls back to a hardcoded default. Maybe it doesn't fall back at all and just throws. You don't get to look at the code, so you don't know which.

Here you wrote the policy. If your CI step has to finish in 30 seconds, you write tighter backoff and fewer attempts. If your overnight batch job can wait, you write longer backoff and more providers. The same engine runs both because the policy is in the chain file, not in the engine.

## The on_failure escape hatch

`on_failure` is the part most people miss. The retry/fallback policy handles transient errors *within* a task. `on_failure` handles the case where the task itself is unrecoverable — and routes to a different task, not just a re-attempt. That gives you authored degradation: when the network is down, fall back to something local. When the local thing fails too, write a polite "sorry, try later" message. The whole chain is the safety net.

## What's on your side

Three choices, all yours:

- **The order of providers.** You picked which one is primary. The vendor didn't.
- **The retry shape.** Number of attempts, backoff curve, jitter, the special rate-limit pause.
- **The terminal behavior.** What happens when everything's exhausted? You wrote that task.

This is what authored resilience looks like. Three keys, your decisions.
