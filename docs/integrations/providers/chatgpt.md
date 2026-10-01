---
title: ChatGPT subscription
description: Use ChatGPT subscription models in Contenox's native agent with device-code login.
---

# ChatGPT subscription

The experimental `openai-codex` backend uses ChatGPT subscription access for
Contenox's own agent loop. Contenox supplies the tools, runs approvals, and
stores the conversation. It works with Beam, native agents, and missions;
it does not launch the Codex CLI or require `codex-acp` or npm.

This is separate from [OpenAI API-key access](/docs/integrations/providers/openai/)
and from [hosting Codex as an external ACP agent](/docs/integrations/agents/external-acp/).
An external agent runs its own loop; this backend provides inference to Contenox.

The subscription endpoint is not a general OpenAI-compatible API. Compatibility
can change independently of Contenox. Account access, available models, and
usage limits are controlled by OpenAI; this integration does not bypass them.

## Enable device-code login

Before signing in, enable device-code login in **ChatGPT → Settings → Security**.
For a managed workspace, an administrator may need to enable it in workspace
permissions. See [OpenAI's authentication instructions](https://learn.chatgpt.com/docs/auth#login-on-headless-devices).

Only approve a code you just requested in your own terminal. Do not share device
codes, access tokens, refresh tokens, or credential databases.

## Set up the backend

Run `contenox setup` and select **ChatGPT subscription** to sign in and choose a
model from the authenticated catalog. Or configure it explicitly:

```bash
contenox backend add chatgpt --type openai-codex
contenox backend login chatgpt
```

Open the printed authorization link, sign in to the intended ChatGPT account,
and enter the one-time code. Keep the terminal running until login finishes.
If the browser asks you to enable device login, enable the account setting above
and return to the authorization page. If the code has expired, rerun the login
command. You do not need to run `codex login --device-auth` separately.

Check the login and discover models:

```bash
contenox backend show chatgpt
contenox model list
```

Choose a model listed for `openai-codex`, then set it explicitly:

```bash
contenox config set inference.provider openai-codex
contenox config set inference.model <available-model>
contenox beam
```

Replace `<available-model>` with its actual identifier. API models and subscription
models are not interchangeable; do not assume a model is available because it
appears in an API example. Login alone does not verify inference access, and
`backend login` does not change your defaults.

In Beam, use the native Contenox agent, not `--agent codex`: that flag selects a
separately registered external agent. Native agents use this backend through
their normal provider/model configuration. Their tools and approval policies
remain Contenox's; see [missions](/docs/guide/missions/).

## Credentials and limits

Credentials are scoped to the registered backend and stored in Contenox's configured
database (local SQLite by default). Login restricts the SQLite file and its existing
journal sidecars to owner-only permissions. Access tokens refresh automatically.
This is database storage, not an OS keychain or an encrypted vault: protect the
database and any backups like passwords. If you configured PostgreSQL, its access
controls protect the stored login. Contenox does not read or modify `~/.codex/auth.json`.

`backend show` displays authentication state without tokens. `refresh_required`
means the next authenticated request must renew the token, not necessarily that
you must sign in again. To replace the account, rerun `backend login chatgpt`.

```bash
contenox backend logout chatgpt
```

Logout clears the stored login but keeps backend registration and inference
defaults. It does not revoke authorization at OpenAI or erase the browser session.
Removing the backend also removes its stored credentials. Already-sent requests
may finish after logout.

ChatGPT subscription limits and account/workspace policies apply. This is not
unlimited inference and does not use an `OPENAI_API_KEY`. The backend never
silently falls back to API-key billing. An explicitly configured alternate
provider still follows Contenox's normal routing rules.

Support covers chat, streaming, and function tools. Image input follows the
model's advertised capabilities. This backend does not provide embeddings,
audio, or image generation. OpenAI API pricing and
API-specific data-residency claims should not be applied to this backend.
API-style sampling parameters and output-token caps are not sent on this
subscription wire; do not rely on `--max-tokens` as a provider-enforced limit here.

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Browser says device login must be enabled | Enable it in ChatGPT Security settings, or ask your workspace administrator. |
| Code expired or authorization was cancelled | Run `contenox backend login chatgpt` again. |
| Login required or refresh rejected | Sign in again; Contenox does not repeatedly reuse a rejected refresh token. |
| Model unsupported for a ChatGPT account | Run `contenox model list` and select a subscription model available to the account. A failed model request does not erase a successful login. |
| Quota or rate limit reached | Wait for the account's limit to reset or review its subscription access. Signing in again does not increase quota. |
| No models or connectivity error | Run `contenox doctor --json`; check the backend error, account/workspace access, and connectivity to OpenAI. |

Discovery uses the versioned catalog request implemented by
[Codex's model client](https://github.com/openai/codex/blob/c248f6d48b97eb4a2aa56147a0b11b7d763278b9/codex-rs/codex-api/src/endpoint/models.rs).
The client compatibility version affects which models the server returns.
If login succeeds but discovery reports no selectable models, update Contenox
before signing in again; an outdated compatibility level can produce that result.

The endpoint is fixed: do not pass `--url`, `--api-key`, or `--api-key-env` to this
backend. For a proxy or another OpenAI-compatible server, use the ordinary
[OpenAI backend](/docs/integrations/providers/openai/).
