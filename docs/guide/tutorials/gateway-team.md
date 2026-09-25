---
title: "Gateway for a team"
description: Expose a verified gateway over HTTPS and issue separate model access and allowances to each client.
order: 6
---

# Gateway for a team

Complete [gateway: day one](/docs/guide/tutorials/gateway-local/) on the serving
host first. Use its deployment directory, secrets and `gw` function for every
operator command here. One gateway process can serve multiple clients, each
with its own model permissions and usage.

## Establish the client URL

The gateway listens over HTTP. Put an HTTPS reverse proxy in front of its
loopback listener, or use an authenticated encrypted tunnel. Configure the proxy
to preserve `Authorization`, stream responses without buffering, and allow the
request durations your models need. Provide the client with the resulting URL,
such as `https://gateway.example.com`.

Binding `--listen 0.0.0.0:11435` exposes HTTP on network interfaces; it does not
enable HTTPS. Limit direct access to that listener to the intended proxy or
protected transport. Verify the certificate and reachability from the actual
client machine before distributing tokens.

## Issue a key per person or application

On the operator host, use the exact discovered model ID:

```bash
gw gateway key create \
  --client ana --models "$GATEWAY_MODEL" \
  --output-allowance 5m --input-allowance 25m \
  --five-hour-allowance 400k --ttl 30d \
  --authority-private-key-file "$GATEWAY_DIR/authority" \
  > "$GATEWAY_DIR/ana.key"

gw gateway key create \
  --client ben --models "$GATEWAY_MODEL" \
  --output-allowance 2m --input-allowance 10m --ttl 30d \
  --authority-private-key-file "$GATEWAY_DIR/authority" \
  > "$GATEWAY_DIR/ben.key"
```

Run with `umask 077`, as in day one. Deliver only the intended token to each
client through a trusted channel. Keep client IDs stable when replacing tokens.
Allowances apply per model to every model named in `--models`. Dollar budgets
additionally require declared upstream rate cards; see
[allowances](/docs/guide/gateway/#allowances).

## Verify from the client machine

Ana loads the delivered token from her own file:

```bash
export GATEWAY_URL='https://gateway.example.com'
export GATEWAY_TOKEN="$(cat ana.key)"
export GATEWAY_MODEL='qwen3:8b'

curl -fsS "$GATEWAY_URL/api/contenox"
curl -fsS -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/v1/models"
curl -N -fsS "$GATEWAY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $GATEWAY_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$GATEWAY_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Say hello in one sentence.\"}],\"stream\":true}"
```

Replace the example model with the granted model. Expect incremental chat events
and a completed stream. Configure an OpenAI-compatible application with
`https://gateway.example.com/v1`; an Ollama-compatible application uses
`https://gateway.example.com`. Both use the issued token, not an upstream
provider credential.

For a Contenox client, register that endpoint in the client's own database:

```bash
contenox backend add gateway --type ollama --url "$GATEWAY_URL" \
  --api-key-env GATEWAY_TOKEN
contenox config set inference.provider ollama
contenox config set inference.model "$GATEWAY_MODEL"
```

## Operate access and limits

On the operator host:

```bash
gw gateway key list --client ana
gw gateway usage --client ana --model "$GATEWAY_MODEL" --window week
gw gateway usage --client ben --model "$GATEWAY_MODEL" --window week
gw gateway usage --by-model
```

Each client's allowance is separate. When one client reaches a ceiling, further
inference requests are refused with `429`; other clients retain their own
allowances, subject to any deployment-wide cap. In-flight requests can overshoot
because charging happens after generation.

To remove Ben's access:

```bash
gw gateway key revoke --client ben
```

Ben's next authenticated request returns `401`; verify Ana can still generate.
For credential rotation, service supervision, backups, upgrades and failure
diagnosis, follow [gateway operations](/docs/guide/gateway-operations/).

Adding clients does not require adding gateway replicas. Consult the
[ownership and shared-state boundary](/docs/guide/gateway-operations/#server-backed-state-and-ownership)
before changing the deployment topology.
