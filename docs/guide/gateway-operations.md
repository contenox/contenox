---
title: Gateway operations
description: Run, monitor, restart, back up and upgrade a Contenox gateway; rotate client keys and diagnose failed requests.
order: 19
---

# Gateway operations

Start with [gateway: day one](/docs/guide/tutorials/gateway-local/) and prove a
complete inference request before installing a service. The commands here use
that walkthrough's `GATEWAY_DIR`, `CONTENOX_TOKEN_KEY_FILE` and `gw` shell function.
Use the same database, secrets, executable and operating-system user each time.

## Keep the process running

`gateway serve` runs in the foreground. Ctrl-C or SIGTERM stops it; a service
manager can restart it. On Linux with a systemd user manager, create
`~/.config/systemd/user/contenox-gateway.service`:

```ini
[Unit]
Description=Contenox model gateway

[Service]
Type=simple
ExecStart=/absolute/path/to/contenox --db %h/.local/share/contenox-gateway/local.db --data-dir %h/.local/share/contenox-gateway gateway serve --listen 127.0.0.1:11435 --authority-private-key-file %h/.local/share/contenox-gateway/authority
Environment=CONTENOX_TOKEN_KEY_FILE=%h/.local/share/contenox-gateway/token.key
UnsetEnvironment=CONTENOX_POSTGRES_URL CONTENOX_NATS_URL CONTENOX_VALKEY_URL
WorkingDirectory=%h/.local/share/contenox-gateway
UMask=0077
Restart=on-failure
RestartSec=5
TimeoutStopSec=30

[Install]
WantedBy=default.target
```

Replace `/absolute/path/to/contenox` with the result of `command -v contenox`.
Stop the foreground gateway before starting the service:

```bash
systemctl --user daemon-reload
systemctl --user enable --now contenox-gateway
systemctl --user status contenox-gateway
journalctl --user -u contenox-gateway -n 100 --no-pager
```

To keep the user manager running after logout and start it at boot, configure
lingering for this operating-system account using your host's administration
policy (`loginctl enable-linger USER`). A system service under a dedicated
account is another option; use that account's paths and permissions consistently.

This unit selects SQLite and explicitly clears the remote-state settings. For
backends requiring ambient credentials or modeld engine overrides, configure
those in the service environment too. A service does not inherit the exports
from your interactive shell. Provider keys supplied through `backend add
--api-key-env` are read and saved during registration.

## Check health and usage

Use the service status/logs and HTTP probes together:

```bash
curl -fsS "$GATEWAY_URL/api/contenox"
curl -fsS -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/v1/models"
gw backend list
gw model list
gw gateway key list
gw gateway usage --client first-client --model "$GATEWAY_MODEL" --window 5h
gw gateway usage --client first-client --model "$GATEWAY_MODEL" --window week
gw gateway usage --by-model
```

Use an active token; the day-one revocation exercise invalidates its original
token. The public handshake checks the listener. The authenticated catalog checks
access and observed inventory. A short chat request checks the actual upstream
path; run one after deployment changes. Neither the handshake nor a Docker
healthcheck proves successful inference or metering.

Window reports show the newest counter for that window; `--by-model` shows
all-time deployment totals. Token usage depends on upstream reports, and an
interrupted stream can lack final usage. Allowances are checked before a request
and usage is recorded afterward, so concurrent or in-flight requests can exceed
a ceiling. Configure rate cards before relying on a monetary ceiling.

## Issue, replace and revoke client credentials

Use a stable client ID per application or person. A replacement token with the
same ID shares that client's existing usage. Replacing a token does not revoke
the old one automatically.

For planned rotation:

1. **Identify the old key.** Run `gw gateway key list --client first-client` and
   retain its digest.
2. **Mint the replacement.** Repeat `gateway key create` with the same client ID,
   the intended model allowlist/allowances and a new output filename.
3. **Switch the client.** Install its new token and verify a request succeeds.
4. **Revoke the old digest.** Run `gw gateway key revoke 'OLD-DIGEST'`, then
   verify the old token returns `401` and the replacement still works.

Quote digests: they contain `$`, which a shell would otherwise expand. Revoking
`--client first-client` revokes all that client's active keys, including a newly
minted replacement. For a compromised client, revoke the client first, then
issue its replacement through a trusted channel.

Allowances and model permissions are signed into the token. Change them by
minting a replacement and revoking the old key. Preserve the authority and
`token.key` across restarts and routine upgrades. Changing either secret requires
planning replacement client credentials; there is no documented seamless
multi-key rotation procedure.

## Change an upstream

Register another backend with a distinct name, then check `gw model list` and
make a request through a client key allowed to use its model. Removing a backend
registration does not delete the remote provider or its weights:

```bash
gw backend remove OLD-BACKEND
```

There is no `backend update`; changing a registration under the same name means
removing and adding it. Model discovery refreshes in the running gateway. Check
the authenticated catalog and a real request after the refresh; restart during
a maintenance window if an environment change is required. Backend removal can
move subsequent conversation turns to a different backend and lose warm cache.

## Back up and restore a SQLite deployment

The database contains backend configuration, stored provider credentials, client
key digests and usage. The deployment also needs its authority and token key.
Native worker binaries, model weights and worker state can live outside
`GATEWAY_DIR`; manage their recovery separately when using modeld.

For a simple consistent offline backup, stop the gateway and any operator CLI
commands using its database. Copy the whole deployment directory, including
SQLite sidecar files if present:

```bash
systemctl --user stop contenox-gateway
umask 077
export GATEWAY_BACKUP_DIR="$HOME/.local/share/contenox-gateway-backups"
mkdir -p "$GATEWAY_BACKUP_DIR"
export GATEWAY_BACKUP="$GATEWAY_BACKUP_DIR/gateway-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
tar -C "$GATEWAY_DIR" -czf "$GATEWAY_BACKUP" .
systemctl --user start contenox-gateway
```

Store the backup securely off the serving machine. Retain the executable version
and service configuration alongside it. Stop a foreground process with Ctrl-C
instead of using `systemctl` if you have not installed the service.

Rehearse recovery into a new directory without replacing the active database:

```bash
export GATEWAY_RESTORE_DIR="$(mktemp -d "$HOME/.local/share/contenox-gateway-restore.XXXXXX")"
tar -C "$GATEWAY_RESTORE_DIR" -xzf "$GATEWAY_BACKUP"
CONTENOX_TOKEN_KEY_FILE="$GATEWAY_RESTORE_DIR/token.key" contenox \
  --db "$GATEWAY_RESTORE_DIR/local.db" --data-dir "$GATEWAY_RESTORE_DIR" \
  gateway key list
```

For a full recovery rehearsal, use the matching executable to serve the restored
copy on another port:

```bash
CONTENOX_TOKEN_KEY_FILE="$GATEWAY_RESTORE_DIR/token.key" contenox \
  --db "$GATEWAY_RESTORE_DIR/local.db" --data-dir "$GATEWAY_RESTORE_DIR" \
  gateway serve --listen 127.0.0.1:11436 \
  --authority-private-key-file "$GATEWAY_RESTORE_DIR/authority"
```

Repeat the HTTP checks against `http://127.0.0.1:11436`. Verify a previously active
token works, a token revoked before the backup remains refused, and inspect usage
with `contenox --db "$GATEWAY_RESTORE_DIR/local.db" gateway usage --by-model`.
Make a short inference request, then stop the restored process.
It uses the registered upstreams and can incur their normal cost.

Restoring an older backup also restores older usage and revocation state. Reapply
any later revocations before admitting clients to a recovered deployment.

## Upgrade and roll back

1. **Record and back up.** Save `contenox version`, service configuration and a
   consistent backup. Keep the previous executable.
2. **Replace during maintenance.** Stop the gateway, install the chosen CLI
   release, and update the unit's executable path if necessary. Follow that
   release's migration notes; native worker upgrades are a separate dependency.
3. **Verify.** Start the service with the existing database and secrets. Check
   the build handshake, catalog, streamed chat, usage and a revoked token.
4. **Recover if needed.** Stop the failed candidate. Use the previous executable
   with its matching pre-upgrade backup unless backward database compatibility
   is established. Account for usage and revocations since that backup.

Do not assume an older executable can read a database migrated by a newer one.

## Diagnose a failed request

| Symptom | Check |
| --- | --- |
| Connection refused | Service exit/logs, listen address, port and network reachability |
| `401` | Bearer token, expiry/revocation, matching authority/token key and deployment database |
| `403` | Model allowlist in the client's token |
| `429` | Client/window allowances and configured deployment cap; then upstream rate limits |
| Empty catalog or missing model | Upstream reachability, discovered model ID and the caller's allowlist |
| Generation fails after catalog succeeds | Upstream logs, credentials, model capacity and request options; catalog success alone does not prove inference |
| Unsupported OpenAI option | Use the supported chat-completions contract in the [gateway reference](/docs/guide/gateway/#client-protocols) |
| Ownership error or lease-related `503` | Another holder, lease directory permissions or failed renewal; inspect before restarting |
| Usage stays unchanged after a completed turn | Provider usage report, usage-consumer warnings, meter lease and the database used by the inspection command |

There is no `gateway status` command. Preserve the HTTP error, build version,
model, backend type and relevant service logs when reporting a failure; exclude
client tokens and provider credentials.

## Server-backed state and ownership

SQLite is the default database, message bus and key-value store. Selecting
Postgres requires `CONTENOX_POSTGRES_URL`, `CONTENOX_NATS_URL` and
`CONTENOX_VALKEY_URL`, and omitting `--db`. Key commands must use that same
state configuration.

`--lease-file` controls gateway ownership. Processes contending for the same
lease path do not serve simultaneously; ownership loss stops the holder.
`--meter-lease` separately elects the usage writer. The defaults are
`gateway.lease` and `meter-writer.lease` under `--data-dir`.

Shared database credentials alone do not establish a correct replicated gateway.
Containers must preserve the lease directory across replacement, and contenders
must see the same intended lease files. Separate machine-local paths do not
coordinate separate hosts. This walkthrough establishes a single serving
process, not a validated multi-host failover deployment.

The source tree's `compose.yaml` includes Postgres, NATS, Valkey and scripted API
test services. Starting that whole file also provisions test models and test
keys; it is an integration fixture, not a configured production deployment.
