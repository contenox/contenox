---
title: "Gateway: day one"
description: Install Contenox, register a model backend, issue a client key, and complete a metered request through the gateway.
order: 5
---

# Gateway: day one

Run one endpoint for applications that speak the Ollama or OpenAI chat API.
The gateway keeps upstream credentials on its host, restricts models per client,
and records usage against the allowances you issue.

This walkthrough uses Bash on a Linux or macOS host, a local SQLite database,
and one gateway process. You need `curl`, `openssl`, `ssh-keygen`, and a reachable
model backend. Native modeld additionally needs a compatible worker package and
hardware; an existing remote backend does not require a local GPU.

## Install and choose the deployment directory

Install the CLI, then check the effective executable and gateway command:

```bash
curl -fsSL https://contenox.com/install.sh | sh
command -v contenox
contenox version
contenox gateway serve --help
```

Use a release that includes the gateway commands and API endpoints needed by your
clients. A source checkout can build the current CLI with `task build`; its
executable is `bin/contenox`. Verify that executable explicitly when testing
unreleased changes.

Use one directory for this deployment, separate from your normal agent state:

```bash
umask 077
export GATEWAY_DIR="$HOME/.local/share/contenox-gateway"
mkdir -p "$GATEWAY_DIR"
export CONTENOX_TOKEN_KEY_FILE="$GATEWAY_DIR/token.key"
unset CONTENOX_POSTGRES_URL CONTENOX_NATS_URL CONTENOX_VALKEY_URL

gw() {
  contenox --db "$GATEWAY_DIR/local.db" --data-dir "$GATEWAY_DIR" "$@"
}
```

Repeat this block in each operator terminal. `gw` is a shell convenience for this
walkthrough, not an installed command. `--db` selects the database for backend
registrations, keys and usage. `--data-dir` selects the configuration and lease
directory; it does **not** relocate the database by itself. Every operator
command below uses both paths.

## Register an upstream and choose a model

For an existing Ollama server with a model already downloaded:

```bash
gw backend add upstream --type ollama --url http://127.0.0.1:11434
gw backend list
gw model list
```

Use the upstream address reachable from the gateway host. For example,
`http://gpu-host:11434` can name a separate machine. The gateway discovers models;
registering a backend does not download weights into that backend.

Other upstream choices use the same deployment database:

| Upstream | Registration / setup |
| --- | --- |
| Managed native modeld | `gw auto --dry-run`, then `gw auto --no-tui`; see [native setup](/docs/integrations/providers/modeld/) for worker availability and downloads |
| Existing vLLM endpoint | `gw backend add upstream --type vllm --url http://gpu-host:8000` |
| OpenAI | `gw backend add upstream --type openai --api-key-env OPENAI_API_KEY`, with the credential exported in that shell |

Choose one route for the walkthrough. Use distinct backend names when combining
providers. `--api-key-env` reads the credential at registration and stores it
with the backend; protect and back up the database as credential-bearing state.
See the [Ollama](/docs/integrations/providers/ollama/) and
[OpenAI](/docs/integrations/providers/openai/) guides for provider-specific setup.

Set the exact model identifier shown by `gw model list`:

```bash
export GATEWAY_MODEL='qwen3:8b'
```

`qwen3:8b` is an example; replace it with a model your upstream actually serves.
An empty model inventory is a setup failure to resolve before continuing.

## Create the authority and first client key

Create the deployment secrets once. The authority signs client tokens; the token
key lets the gateway match them to its revocable key ledger.

```bash
ssh-keygen -t ed25519 -N '' -f "$GATEWAY_DIR/authority"
openssl rand -hex 32 > "$CONTENOX_TOKEN_KEY_FILE"
chmod 600 "$GATEWAY_DIR/authority" "$CONTENOX_TOKEN_KEY_FILE"

gw gateway key create \
  --client first-client --models "$GATEWAY_MODEL" \
  --output-allowance 5m --input-allowance 25m \
  --five-hour-allowance 400k --ttl 30d \
  --authority-private-key-file "$GATEWAY_DIR/authority" \
  > "$GATEWAY_DIR/first-client.key"

gw gateway key list --client first-client
```

Keep these secrets for restarts. Re-running key generation changes the
deployment's identity. The client token is printed once; the database stores its
digest. Give the client only its own token.

## Start the gateway

In the operator terminal:

```bash
gw gateway serve \
  --listen 127.0.0.1:11435 \
  --authority-private-key-file "$GATEWAY_DIR/authority"
```

Keep it running. Startup must report `Key ledger enforced` and your active key.
A warning that no ledger is enforced means `CONTENOX_TOKEN_KEY_FILE` was not
supplied correctly. The HTTP listener does not configure TLS.

## Complete the first request

In another terminal, repeat the deployment-directory block, set `GATEWAY_MODEL`
to the same model, and load the client token:

```bash
export GATEWAY_URL='http://127.0.0.1:11435'
export GATEWAY_TOKEN="$(cat "$GATEWAY_DIR/first-client.key")"

curl -fsS "$GATEWAY_URL/api/contenox"
curl -fsS -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/v1/models"
```

The first response identifies the Contenox build. The authenticated catalog must
include the selected model. A catalog response proves discovery and access;
complete a generation to prove inference:

```bash
curl -fsS "$GATEWAY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $GATEWAY_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$GATEWAY_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Say hello in one sentence.\"}],\"stream\":false}"
```

For an Ollama client, use the same token and model through `/api/chat`:

```bash
curl -N -fsS "$GATEWAY_URL/api/chat" \
  -H "Authorization: Bearer $GATEWAY_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$GATEWAY_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Say hello in one sentence.\"}],\"stream\":true}"

gw gateway usage --client first-client --model "$GATEWAY_MODEL"
```

Expect an answer, a completed stream, and recorded usage. A zero cost with token
usage can mean no upstream rate card was configured; it does not establish that
the provider charged nothing. See [allowances](/docs/guide/gateway/#allowances).

## Connect an application

| Client setting | Value |
| --- | --- |
| OpenAI-compatible base URL | `http://127.0.0.1:11435/v1` |
| Ollama-compatible host | `http://127.0.0.1:11435` |
| API key / bearer token | The issued client token |
| Model | The identifier from the authenticated catalog |

The OpenAI-compatible surface supports chat completions, model listing and
embeddings; it is not the entire OpenAI API. Use chat completions rather than a
client mode that requires the Responses API. See the
[gateway reference](/docs/guide/gateway/) for supported requests.

On a separate Contenox client installation:

```bash
contenox backend add gateway --type ollama --url "$GATEWAY_URL" \
  --api-key-env GATEWAY_TOKEN
contenox config set inference.provider ollama
contenox config set inference.model "$GATEWAY_MODEL"
```

The client uses its own database. Do not register the gateway's own endpoint as
an upstream in the gateway deployment. On another machine, use the reachable
HTTPS URL prepared in [gateway for a team](/docs/guide/tutorials/gateway-team/).

## Prove revocation

After testing with this first token:

```bash
gw gateway key revoke --client first-client
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/v1/models"
```

Expect `401`. Mint a new token with the same client ID before reconnecting the
application. Existing usage remains attached to that client ID.

You now have a configured upstream, a working inference endpoint, metering and a
verified cutoff. Continue with [day-two operations](/docs/guide/gateway-operations/)
for service startup, credentials, backups, upgrades and troubleshooting.
