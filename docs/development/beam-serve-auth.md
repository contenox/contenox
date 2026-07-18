# Beam serve authentication

How `contenox serve` authenticates the Beam web UI and the HTTP/WebSocket API.
This is the contributor reference; the user-facing steps are in
[the Beam guide](../guide/beam.md#remote-access-and-login).

## Model: BFF with an HttpOnly cookie

`contenox serve` exposes one origin: the Beam SPA at `/`, the REST API under
`/api/*`, the chat WebSocket at `/acp`, and login endpoints under `/ui/*`. When a
`TOKEN` is configured (optional on loopback, **mandatory** for any non-loopback
bind — enforced by `ValidateLocalServeSecurity`), every data surface is gated.

The browser is a **pure-cookie backend-for-frontend**: it never handles the token
in JavaScript. The single operator token is the credential; the server wraps a
successful login into a signed session JWT held in an HttpOnly cookie the page
cannot read. Programmatic clients (scripts, other machines) authenticate with the
raw token directly. The local `contenox` CLI is **not** an HTTP client of serve —
it talks to the SQLite/engine directly — so none of this applies to it.

## Flow

1. **Login** — `POST /ui/login {token}` (`runtime/serverapi/ui_auth.go`).
   Constant-time compares the posted token against `TOKEN`, then mints an HS256
   JWT for the fixed principal `local-operator` and sets it as the `auth_token`
   cookie: `HttpOnly`, `SameSite=Strict`, `Secure` when the request is HTTPS (real
   TLS or `X-Forwarded-Proto: https`). Per-IP rate limited. The JWT is minted and
   validated in `runtime/serverapi/session_auth.go`; the signing secret is
   `HKDF-SHA256(TOKEN)` (domain-separated), so no separate secret config is needed
   and rotating `TOKEN` invalidates existing cookies. TTL 24h.
2. **Gate** — `ProtectAPI(token, allowedOrigins, next)`
   (`runtime/serverapi/local_security.go`) wraps `/api/*`. With a `TOKEN` set,
   **every** request on **every** method must present a valid credential or get
   `401`. `AuthenticateCredential` accepts either the raw `TOKEN` (constant-time,
   via `Authorization: Bearer` / `X-API-Key`) **or** a valid session-cookie JWT.
   With no `TOKEN` (loopback dev), reads pass and only cross-site browser mutations
   are refused.
3. **WebSockets** — `/acp` (`runtime/contenoxcli/acp_ws.go`) and the terminal
   (`runtime/internal/terminalapi/routes.go`) enforce the credential **in the
   handshake callback**, so an unauthenticated upgrade is rejected `403` *before*
   the `101` switch — never accepted-then-dropped. The browser needs no query
   param: the `auth_token` cookie rides the same-origin upgrade automatically.
4. **Status / logout** — `GET /ui/auth-status → {required, authenticated}` drives
   the UI gate; `POST /ui/logout` clears the cookie.

Routes reachable with no credential: `/` and static assets (so the login page can
load — they carry no data), `/health*`, `/version`, `/ui/login`, `/ui/auth-status`.

## Browser side (pure cookie)

`packages/beam/src/lib/fetch.ts` sends `credentials: 'same-origin'` on every
request and sets **no** `Authorization` header — there is no `localStorage` token
path. `AuthProvider` polls `/ui/auth-status`; `AuthGate` (`App.tsx`) renders the
`AuthPage` login form in place of the whole app whenever `required && !authenticated`.
`LoginForm` is a single masked **Access token** field that POSTs `/ui/login`.
`buildAcpWsUrl` (`lib/acp/AcpWorkspaceProvider.tsx`) returns a bare
`ws(s)://host/acp` — the cookie authenticates the upgrade. This matches
`mvp/frontend/src/lib/api.ts`.

> **Why no localStorage.** An earlier remote-access attempt bootstrapped the token
> via `?token=` into `localStorage` and injected it as a `Bearer` header. That left
> a stale raw token that kept authenticating a browser after the login flow moved to
> cookies, and put an XSS-readable copy of the secret in storage — defeating the
> HttpOnly cookie. The `localStorage` reader was removed (2026-07-18) so the browser
> authenticates **only** via the cookie. Pinned by a test in
> `packages/beam/src/lib/authApi.test.ts` asserting no `Authorization` header is sent
> even when a token sits in `localStorage`.

## Programmatic / remote clients

Non-browser callers pass the raw token as `Authorization: Bearer <TOKEN>` (or
`X-API-Key`); `/acp` additionally accepts `?token=<TOKEN>` since a browserless
WebSocket client cannot set the cookie. This is the mvp pattern and is unaffected
by the browser going cookie-only.

## CORS

`apiframework/middleware/cors.go` (`EnableCORS`), wired in
`runtime/contenoxcli/serve_cmd.go` and `runtime/serverapi/server.go`, is a 1:1 port
of mvp's `enableCORS`: `Vary: Origin`, an explicit origin allowlist (`*` supported),
`ProxyOrigin` entries additionally get `Access-Control-Allow-Credentials: true` with
the origin reflected (for the Vite dev proxy), the standard method/header sets, and
`OPTIONS` preflight answered `200`. The default `AllowedAPIOrigins` is empty — no
cross-origin API access unless configured. Same-origin (the BFF's normal path) needs
no CORS at all.

## Verify

With a `TOKEN`-protected serve on loopback:

```bash
# No credential → gated
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:32123/api/state          # 401
# Programmatic bearer → allowed
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:32123/api/state                                                  # 200
# /acp upgrade with no credential → rejected before 101
curl -s -o /dev/null -w '%{http_code}\n' -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
  -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' \
  http://127.0.0.1:32123/acp                                                        # 403
# Open routes
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:32123/ui/auth-status      # 200
```

Browser negative test: with `localStorage['contenox_api_token']` set to the raw
token and no cookie, opening `/#/chat` must render the login page and `/api/state`
must return `401` — the stale token is inert.
