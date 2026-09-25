---
title: Model gateway
description: Serve the models you already pay for to machines and people you choose, metered per caller with allowances you state — without handing over a provider key.
order: 18
---

# Model gateway

The gateway exposes configured local and hosted models through one HTTP endpoint,
with client keys, model permissions and usage allowances. Provider credentials
stay on the gateway host.

- [**Day one: configure and serve**](/docs/guide/tutorials/gateway-local/) — install,
  register an upstream, create a key, complete a request and verify revocation.
- [**Day two: operate**](/docs/guide/gateway-operations/) — service startup,
  monitoring, credential rotation, backup/restore, upgrades and troubleshooting.
- [**Serve a team**](/docs/guide/tutorials/gateway-team/) — expose the verified
  endpoint through HTTPS and give each client its own access and allowances.

Run `contenox gateway serve` against the database containing the backends you
want to expose. The gateway serves discovered models and holds no separate model
inventory. Operator key and usage commands must select that same database.

## Client protocols

Both protocol surfaces use the same bearer credentials, model permissions,
routing and usage accounting:

| Client protocol | Base URL on the default listener | Supported endpoints |
| --- | --- | --- |
| Ollama | `http://127.0.0.1:11435` | `/api/tags`, `/api/show`, `/api/chat`, `/api/generate`, `/api/embed`, `/api/version` |
| OpenAI-compatible | `http://127.0.0.1:11435/v1` | `/models`, `/chat/completions`, `/embeddings` |

Chat supports streaming and function-tool messages when the selected upstream
supports them. The OpenAI-compatible surface accepts a subset of chat-completions
options. Unsupported options such as multiple choices, logprobs, nonzero
frequency/presence penalties, stop sequences, constrained `response_format` and
forced tool choice are refused. It does not expose the Responses API.
Remote image URLs are refused; send supported inline image data instead.

## What a caller gets

The endpoint is the Ollama API your runtime already speaks: `/api/tags`, `/api/show`, `/api/chat`, `/api/generate`, `/api/embed`, `/api/version`. Messages on the chat and generate calls carry images and audio (`audios`, raw WAV) beside their text.

`/api/tags` returns only the models the key's claims allow, `/api/show` is where a client that cannot see the upstream learns a model's context window, output ceiling and capabilities, `/api/chat` and `/api/generate` are the metered turns, and `/api/embed` serves embeddings — one input or a batch, one vector each, in order. A model the claims do not name is refused before any allowance is touched.

Embeddings are measured on the input ceilings and priced from the model's declared input rate.

A turn is measured in tokens for text, image attachments for vision, and bytes for audio. All of it lands on one meter, so every ceiling applies: `--input-allowance` and `--five-hour-allowance` to tokens, `--image-allowance` to images per week, `--audio-allowance` to mebibytes per week, and `--monthly-budget` to the cost of all of it. `--image-price` and `--audio-price` charge attachments whether or not the model reports tokens for them. A key that states an output allowance and no input cap gets five times the output as its input cap.

```
$ curl -s http://127.0.0.1:11435/api/version
{"version":"0.5.1"}

$ curl -s http://127.0.0.1:11435/api/contenox
{"product":"contenox-gateway","version":"v1.0.0","build":"revision …","ollama_version":"0.5.1","extensions":["audios","session"]}
```

`version` on the handshake is the build answering, the same value `contenox version` reports; `ollama_version` is the API generation. Extensions are named, so a client can tell whether this gateway understands `audios` or `session` before it sends either. A contenox client probes the route once per backend, and vanilla Ollama answering `404` is what switches extensions off — an `audios` payload is then refused rather than dropped, and the model is not offered as audio-capable.

This matters for a client on the other side: it runs its own `contenox` against the gateway exactly as it would against a local Ollama, including the parts where a model's real limits are learned from `/api/show` rather than assumed.

### Which backend serves a conversation

A gateway in front of several backends that serve one model has a choice to make on every turn, and the choice is not free: a provider's prompt cache lives on the backend that built it, so a conversation that lands somewhere new each turn pays to read its own history again.

A request can name the conversation it belongs to:

```bash
curl -s http://127.0.0.1:11435/api/chat -H "Authorization: Bearer $KEY" -d '{
  "model": "qwen3:8b",
  "contenox_session": "trip-planning",
  "messages": [{"role": "user", "content": "plan a trip"}]
}'
```

Turns naming the same session reach the same backend, and the pinned choice is computed from the session key rather than stored, so every gateway over the same backends makes the same one without coordinating. A request that names none is identified by its opening — the system instruction plus the first thing the caller said — which is the part of a resent history that does not move. A turn that carries neither resolves freely across the deployment.

An assertion travels with it: the opening is declared the stable prefix, so a provider with an explicit cache control (Anthropic, Bedrock) places its breakpoint there, and a first turn, which has no repeated prefix yet, declares nothing.

Two things move a conversation off its backend, both deliberately: the backend being taken out of service by the breaker after repeated failures, and a rewrite of the opening itself. The first is worth the lost cache — a dead backend serves no turn at all. A backend the gateway has taken out of service is skipped when the session is resolved, so a session does not spend every turn rediscovering the same failure.

A `contenox` on the client side of the gateway names its own session, so the gateway is told which conversation a turn belongs to rather than inferring it from the opening. That matters for the shape this gateway is built for: a long mission compacts its context, and the opening of a compacted history is not the opening of the history it grew from — the inferred identity would change and the conversation would move. The session the client sends is the one it has held all along, so it does not move. A client that declared the extension and names no session is inferred from the opening like any other.

## Allowances

An allowance is a claim inside the key, and the gateway enforces it per model, keyed on the durable client id rather than the token — re-minting a key for the same client continues that client's metering instead of resetting it, which is what makes replacing a lost key safe.

| Claim | Flag | Refuses when |
| ----- | ---- | ------------ |
| `output_allowance:<model>` | `--output-allowance 5m` | the client's weekly output tokens reach the ceiling |
| `input_allowance:<model>` | `--input-allowance 25m` | the client's weekly effective input tokens reach the ceiling |
| `five_hour_allowance:<model>` | `--five-hour-allowance 400k` | the client's last five hours reach the burst ceiling |
| `monthly_budget_usd:<model>` | `--monthly-budget 20` | the client's realized spend in the window reaches the ceiling |
| `cache_discount_multiplier:<model>` | `--cache-discount 0.25` | — it scales the two input ceilings rather than refusing on its own |
| `allowed_models` | `--models qwen3:8b,llama3.1:8b` | the model is not named by the key |

Effective input is the figure the weekly and burst ceilings are measured on, and it is cache-aware: a cached prompt token counts at the share the key states, so a client that reuses context is not charged as if it had sent it again. `--cache-discount 0.25` means a quarter counts; a key that states none gets a tenth. The share is per model, like every other allowance, and it is read from the key on every turn — the gateway holds no default of its own.

Thinking is metered separately and `--thinking-discount 0.25` makes each reported thinking token consume one quarter of the output and burst allowances while its full upstream cost remains recorded; an authenticated client can read its raw and effective counters at `GET /api/contenox/usage?model=<model>`.

⚠ The spend ceiling is metered from rate cards, and a rate card is something you state:

```bash
contenox model capability set openai gpt-5-mini --input-price 1.25 --output-price 10 --backend my-reseller
```

With no pricing declared a model costs nothing as far as the meter is concerned, so `--monthly-budget` never trips. The token ceilings do not depend on pricing.

Ceilings are checked before a request and charged after generation. A turn can
cross a ceiling before it is recorded, and concurrent or in-flight requests can
increase that overshoot. These are usage allowances, not hard reservations of
upstream spend.

## Verification

Inference and catalog requests present the key as a bearer; the version and
Contenox identity endpoints are public probes. With the deployment token key
configured, the gateway checks before forwarding a turn:

1. the token is signed by your authority and its claims decrypt — a client deployment needs the authority's public key and the payload key, and nothing else;
2. the token's digest is in the ledger, so a token that was signed by your authority but never minted here fails closed;
3. the ledger row is not revoked and the token is inside its validity window;
4. no revocation for that key has been broadcast on the message bus.

Authentication failures are refused before a provider call. Without a deployment
token key the gateway warns at startup and verifies signed licenses without the
revocable ledger; configure `CONTENOX_TOKEN_KEY_FILE` for the operating journey.

## Revoking

```bash
contenox gateway key list                              # find the digest
contenox gateway key revoke 'h1$ucb66xFDk'               # durable: the ledger row
contenox gateway key revoke --client laptop-alex       # every key that client holds
contenox gateway key revoke 'h1$ucb66xFDk' --broadcast   # also cut off gateways with their own database
```

Revocation is durable by default: it is a row in the same database the gateway serves from, so every gateway reading that database refuses the key on its next request whether or not it is running. `--broadcast` additionally publishes the cutoff on the message bus, by digest and never by client, which reaches a gateway that keeps its own database.

## Watching the meter

```bash
contenox gateway usage --client laptop-alex --model qwen3:8b
contenox gateway usage --by-model
```

The meter stores the newest total per scope and no history, so a window that has rolled reads as zero. That is the refill. It answers "how much is spent against the ceiling", not "what happened last week"; the durable record of what happened is the event log.

## Deployment shapes

**One box.** The operator holds the authority's private key and the gateway runs beside it: `--authority-private-key-file`. Simplest, and fine when the same person runs the gateway and mints the keys.

**Split authority.** The authority keeps its private key and mints keys; the gateway holds only the authority's public key plus the 32-byte payload key the licenses are encrypted with:

```bash
contenox gateway serve \
  --authority-key-file ~/.contenox/authority.pub \
  --payload-key-file ~/.contenox/payload.key \
  --listen 0.0.0.0:11435
```

A gateway configured this way can verify and refuse, but cannot mint — which is the point of splitting it.

**Server-backed state.** Postgres deployments also require NATS and Valkey.
Gateway ownership and meter-writer leases add filesystem requirements beyond
shared database access. See [ownership and shared state](/docs/guide/gateway-operations/#server-backed-state-and-ownership)
before planning multiple processes or hosts.

The default listener is `127.0.0.1:11435`. Binding another interface exposes
HTTP; configure HTTPS termination or an encrypted tunnel for remote clients.
See the [team walkthrough](/docs/guide/tutorials/gateway-team/).

## Scope

The gateway is a self-hosted control point for models you already pay for. It has no accounts: a key names a client, and the client id is the whole identity — there is no signup, no session, no per-person directory, and no billing. Everything it enforces is enforced because the caller comes through it, which means a client that holds its own provider credentials can go around it. Revocation and metering cover the traffic the gateway carries.

The key ledger is also local to the deployment. There is no central place that knows which keys exist across two independent installations — if you run two, you run two ledgers.

## See also

- [`contenox gateway` reference](/docs/reference/contenox-cli/#contenox-gateway) — every flag and subcommand
- [Configuration](/docs/reference/config/) — backends and the state the gateway reads
- [Sovereignty](/docs/guide/sovereignty/) — why you decide where state and credentials live

## Harness usage

Local harness calls use the same meter in process. No gateway server, license or spending limit is required. `contenox usage` shows lifetime usage and calculated cost by model; `contenox usage --session <id>` limits the report to a session. Add `--json` for machine-readable output. Session `/stats` includes the reported thinking tokens within output tokens.

Costs use the configured upstream rate cards. Missing rates do not establish that a call was free. Token totals depend on provider reports; an interrupted stream may not report its final usage, and embedding input is estimated when provider usage is unavailable. Reports cover calls recorded after metering was enabled.
