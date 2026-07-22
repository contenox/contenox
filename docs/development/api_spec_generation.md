# OpenAPI Spec Generation

The OpenAPI spec for the HTTP API is generated from the route registrations
and inline annotations in the route packages. The generation logic lives in
`internal/openapigen`; `tools/openapi-gen` is a thin command around it that
writes `runtime/internal/openapidocs/openapi.json`. That file is tracked in
git and embedded into the binary via `go:embed`
(`runtime/internal/openapidocs/embed.go`); `contenox serve` serves it at
`/api/openapi.json` with a RapiDoc viewer at `/api/docs`.

## Running the generator

From the repo root:

```bash
make openapi
# equivalent: go run ./tools/openapi-gen
```

(Also wired to `go generate` through the directive in
`runtime/internal/openapidocs/embed.go`.)

Generation is deterministic: the same tree always produces byte-identical
output. It is also **strict** — every problem is collected and reported with
its `file:line`, and the command exits non-zero. There is no silent
degradation.

Two gates keep the spec from rotting:

- **Coverage gate** (inside the generator): every documented operation must
  carry a `@response` annotation, and every non-GET/non-DELETE operation must
  carry a `@request` annotation. There is no exemption list — the explicit
  `none` forms below are the exemption mechanism, with the reason recorded
  in-source.
- **Freshness gate** (`TestUnit_OpenAPISpec_Fresh` in
  `runtime/internal/openapidocs`): regenerates the spec from the working tree
  and byte-compares it against the embedded `openapi.json`. It runs under
  plain `go test ./...` and `make test-unit`, and fails with
  `openapi.json is stale — run: make openapi`.

So: change a route or annotation, run `make openapi`, commit the regenerated
`openapi.json` with the change.

## What gets scanned

Two directory sets, defined at the top of `internal/openapigen/openapigen.go`:

- **Route packages** (`routeScanGlobs`): `runtime/internal/*api` and
  `runtime/serverapi`. Scanned for route registrations, handler annotations,
  and parameter-helper calls; their types are available for schemas.
- **Type packages** (`typeScanDirs`): an explicit list of additional packages
  (e.g. `runtime/runtimetypes`, `runtime/taskengine`, `apiframework`) parsed
  only to resolve request/response types referenced from route packages.

An annotation that references a type in neither set is a strict error — add
the package to `typeScanDirs` to fix it.

## Route discovery and handler binding

The generator finds routes in `HandleFunc` calls whose first argument is a
`"METHOD /path"` string literal:

```go
mux.HandleFunc("GET /backends/{id}", b.getBackend)
```

Annotations bind to the handler the route registers, keyed by
`ReceiverType.method` — two handler types in one package may share method
names without cross-contaminating each other's routes. The registrar's simple
local bindings (`h := &fooHandler{...}`, `var h fooHandler`) and its own
receiver resolve `h.get` to its receiver type. When the receiver cannot be
resolved (e.g. the handler comes from a constructor call), the bare method
name binds only if exactly one documentable function in the package carries
it; more than one is an ambiguous-binding error.

Two more registration shapes are understood:

- **Wrapper see-through**: `mux.HandleFunc(spec, wh.wrap((*handler).list))` —
  a single-argument wrapper call whose argument names a function or method
  binds the route to the NAMED inner function (annotations, operationId,
  summary). The wrapper's whole body, including the closure it returns,
  additionally feeds parameter derivation, because that closure is the code
  that runs on the route (localfileapi's workspace mount reads `root` there).
- **Closure handlers**: `mux.HandleFunc(spec, func(w, r) { ... })` — comments
  inside the closure body are the route's annotations, and helper calls in
  the closure body feed its parameters (`/health`, `/version`, and the
  `/workspace/roots` verbs are documented this way).

Per operation the generator emits:

- `operationId`: `<tag>_<handlerName>` (tag = package name minus the `api`
  suffix); closures fall back to `<method>_<path-slug>`.
- `summary`: the first sentence of the handler's GoDoc; a handler without
  GoDoc (and any closure) keeps the mechanical `METHOD /path` form.

## Annotations

Annotations are inline comments **inside the handler (or closure) body**,
usually placed on the decode/encode lines they describe. The first `@request`
and the first `@response` line win.

### `@request`

```go
// @request runtimetypes.Backend          JSON body (ignored on GET routes)
// @request none <reason>                 explicitly bodyless non-GET
// @request binary <description>          raw application/octet-stream body
```

`none` requires a reason (why there is no body — e.g. `POST /setup/refresh`
is a trigger). `binary` requires a description and emits an
`application/octet-stream` request body (`POST /backends/{id}/models/push`
streams raw model bytes).

### `@response`

```go
// @response runtimetypes.Backend         JSON 200 (also: []T, []*T, *T, string, any)
// @response none <reason>                204, no content (e.g. DELETE /terminal/sessions/{id})
// @response binary <description>         200 application/octet-stream (GET /files/download)
// @response sse <description>            200 text/event-stream (GET /task-events, GET /workspace/search)
// @response redirect <description>       302, no content, no 200 entry (GET /mcp/oauth/callback)
```

The non-JSON forms require their trailing reason/description; it becomes the
response's `description` in the spec. Annotate what the handler actually
writes — the coverage gate makes the annotation mandatory, and the grammar is
how a non-JSON truth stays honest instead of being faked as an empty object.

### Parameters

Query and path parameter documentation is derived primarily from the
`apiframework` helper calls in the handler's own body:

- `GetQueryParam(r, "name", "default", "description")` → string query
  parameter with the description, plus `schema.default` when the default
  literal is non-empty.
- `LimitParam(r, n)` → integer `limit`; `CursorParam(r)` → string `cursor`;
  `ListParams(r, n)` → both. Each carries its canonical description.
- `GetPathParam(r, "name", "description")` → attaches the description to the
  path parameter the route template already declares; a name found in no
  bound route template is a strict error.

Helpers are matched by function name under any import alias. Only string
literals are consumed: a non-literal parameter NAME is a strict error at the
call site; a non-literal default/description is consumed as absent (counted
and reported by the command). Calls inside nested function literals only
count for wrapper bodies (see above) and closure handlers.

The escape hatch for parameters the scan cannot see:

```go
// @param name type description...   (type: string | integer | number | boolean)
```

Sources merge first-wins by name — the handler's own helper calls, then the
wrapper's, then `@param` — so code-derived truth beats the escape hatch.
Emitted order is deterministic: path parameters in template order
(`required: true`), then query parameters sorted by name (`required: false`).

Every `{name}` segment in the registered path becomes a required path
parameter automatically — no annotation needed.

### `openapi:exclude`

A route-registration function whose doc comment contains

```go
// openapi:exclude <one-line reason>
```

is skipped entirely — no `HandleFunc` call inside its body is documented. The
spec declares `servers: [{"url": "/api"}]`, so this is for registrars serving
mounts OUTSIDE the `/api` prefix (the root-mux `/v1/*` and Ollama-native
compat aliases), alternate mounts re-registering an already documented
surface, and the `GET /` not-found catch-all. The reason is mandatory.

## Schemas

Annotated types are resolved to JSON Schema by walking the Go struct
definitions in the scanned packages. Component names are the sanitized
`pkg_Type` (e.g. `runtimetypes_Backend`).

- **Property names** come from the `json:"..."` tag (fields with `json:"-"`
  are skipped; untagged exported fields use the Go field name).
- **Embedded structs** are flattened into the parent's properties.
- **Named non-struct types** (e.g. `type StopReason string`) resolve to their
  underlying scalar type.
- **Special cases**: `time.Time` → `string` (`date-time` format),
  `time.Duration` → `integer` (nanoseconds), `json.RawMessage`, `any`, and
  interfaces → unconstrained schema.

The `openapi_include_type:"pkg.Type"` struct tag replaces one field's schema
with a named type reference — for aliases or types the field expression alone
does not resolve. It accepts the same forms as the annotations (`pkg.Type`,
`[]pkg.Type`, bare primitives).

## Error responses

Every operation gets a `default` error response referencing the hard-coded
`APIError` component, matching the apiframework error envelope:

```json
{ "error": { "message": "...", "type": "...", "code": "..." } }
```

## Strict errors

All of these are collected in full and fail the run together, each with a
`file:line`:

- a scanned directory that fails to parse, or a route-scan glob matching no
  directories;
- a type reference that does not resolve to a known struct or named alias;
- the same `METHOD /path` registered twice in the scanned set (both sites
  reported);
- an ambiguous bare-name annotation binding (candidates listed);
- a missing `@response`, or a missing `@request` on a non-GET/non-DELETE
  operation (the coverage gate — route and handler named);
- a `none`/`binary`/`sse`/`redirect` form without its reason/description;
- a non-literal parameter name in a consumed helper call;
- a malformed or invalid-typed `@param`;
- a `GetPathParam` name that appears in no bound route's path template;
- `openapi:exclude` without a reason.

## Spec metadata

The document is OpenAPI 3.1.0, titled `Contenox Runtime API`, with the
version read from `runtime/version/version.txt` and a single server entry of
`/api` (the prefix `contenox serve` mounts the API under).
