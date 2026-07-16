# Runtime-Generated UI ("contenox pages")

Status: draft blueprint, 2026-07-16. From-scratch design. Not started.

## Why

Two structural diseases this kills, both observed in live testing 2026-07-16:

1. **CLI/UI parity drift.** The Settings page shipped 4 of 12 config keys with
   zero help text while `contenox config` had all of them documented. Every
   hand-built admin page is a copy of backend truth that rots. Beam's admin
   surface (~8.2k LOC) is mostly this kind of copy.
2. **The hardcoded-div battle.** Every AI-assisted (and human) UI change
   re-invents markup and styling. Review burden is endless because the design
   system is advisory, not structural. When UI is *data validated by a
   schema*, emitting styled divs is impossible by construction — the design
   system lives in exactly one place (the compiler) and evolves centrally.

## Core idea

- **Block vocabulary as data.** A page is a JSON/YAML spec: an envelope
  (`contenox_page/v1`) plus a tree of typed blocks. Blocks carry *semantics
  only* (what data, which action) — never styling, never layout beyond
  coarse composition (columns, card, section).
- **One compiler.** A single `PageCompiler` React component in `packages/ui`
  renders specs using the existing design-system components. All styling
  decisions live here. Specs are validated (zod client-side; JSON Schema is
  the wire contract) and unknown blocks render as an explicit "unsupported
  block" card, so old UIs degrade loudly, not silently.
- **Specs are served, not compiled in.** The runtime serves page specs
  (embedded defaults; `~/.contenox/pages/` overrides). Beam mounts one
  generic route that fetches and compiles them. Adding a backend capability
  ships its UI by shipping a spec — no frontend release, no parity drift.

## The unfair advantage: the OpenAPI generator

`docs/development/api_spec_generation.md`: the whole API surface already
self-describes from GoDoc annotations and compiler-checked helpers
(`make openapi` → `runtime/internal/openapidocs/openapi.json`). Therefore:

- **Forms are derived, not authored.** A `form` block references an
  operationId; its fields, types, enums, and help text come from the request
  schema + parameter descriptions that already exist in Go comments. The
  settings tooltips hand-ported from CLI help on 2026-07-16 are exactly the
  text this would have delivered for free.
- **Tables are derived.** A `table` block references a list operation;
  columns come from the response schema (with an optional column pick).
- **Actions are typed.** `action` blocks bind a button to a mutation
  operationId; request validation is the schema, error display is uniform.

Initial block set (walking skeleton): `section`, `headline`, `text`,
`form(operationId)`, `table(operationId)`, `action(operationId)`,
`columns`. Explicitly later: loop, dialog, image, query-composition.
Data-binding primitives (template strings/variables) start minimal:
path/query params only.

## Second front: HITL forms from tool schemas

Tools already carry JSON Schemas. The permission gate should render a real
form (schema → fields) instead of raw argument JSON. Same derivation
machinery, different schema source; cheapest visible win and independent of
the page-serving plumbing.

## Far end (ties to declared-agents blueprint)

Agents/chains emitting blocks as an ACP session/update payload → rendered
dashboards/forms in the transcript. Out of scope for the skeleton; the block
vocabulary must simply not preclude it (blocks must be self-contained data).

## Renderer decision (open): React compiler vs htmx (server-rendered)

Two ways to compile specs to pixels; the block/spec/derivation layer above is
identical for both.

**Option A — React `PageCompiler` in packages/ui.** Pros: one UI stack; reuses
existing design-system components directly; HITL forms inside the chat
transcript fall out of the same compiler. Cons: the spec, its zod mirror, and
TS types live client-side — a second implementation of truth the server
already has; still shipped through beam's build.

**Option B — htmx, rendered by the runtime.** Go html/template renders blocks
server-side from the spec + openapi schemas; htmx wires forms/actions
(hx-post → API, fragment swaps); SSE extension covers progress states (model
downloads). Pros: the SAME Go process that generates the OpenAPI doc renders
the forms derived from it — zero client mirror, zero build step, embedded in
the binary; admin UI ships with the runtime by definition; no JSX anywhere on
the admin surface, which ends the hardcoded-div problem in the strongest
possible way; maintainer has prior art (own htmx-based framework experiment).
Cons: second UI idiom next to the React chat; the design system must be
expressed as CSS (extract tokens/classes so htmx pages and beam share one
visual language — this is the riskiest integration point and belongs in the
skeleton); interactive blocks (dialogs, dependent selects) need htmx patterns
or a sprinkle of Alpine; HITL forms in the chat transcript cannot be
htmx-rendered and keep a small client-side schema→form renderer regardless.

**Decision (maintainer, 2026-07-16): B, all-in — one idiom, no React app.**
Parts that don't work or aren't replicable in hypermedia get cut, not ported.
"One idiom" means hypermedia-driven with small JS glue where needed (htmx +
minimal script for composer affordances like @-autocomplete); it does not
mean zero JavaScript — it means no client-side application state.

What this implies for the chat (the last migration, not the first):
- The transcript becomes server-rendered fragments pushed over SSE/WS from
  the taskengine event bus — the same events acpsvc already translates to
  ACP notifications drive template swaps instead. Sessions are already
  server-owned and DB-backed; the React client was always a remote view.
- Markdown, diffs, tool cards, thought boxes render server-side (arguably
  better: one renderer, no client hydration; token streaming batches into
  fragment appends).
- ACP is unaffected: it remains the interop protocol for external clients
  (Zed, VS Code, CLI, conformance suite). The built-in web UI simply stops
  being an ACP client and becomes the runtime's own face. libacp keeps its
  role.
- Casualties accepted by this decision: client-side optimistic UI,
  React-component reuse in the transcript, and beam itself —
  packages/beam (~23k LOC) is deleted at the end of the migration. The
  VS Code webview currently mounts beam's React ChatSurface; webviews can
  load served HTML, so it follows the same path or keeps a frozen copy
  until it does.

## Walking skeleton order

0. **Renderer spike (decides A vs B):** extract the design tokens/classes so a
   runtime-rendered page and beam share one visual language; render one static
   block server-side next to a beam page and compare. If they can't be made
   to look like one product, fall back to Option A.
1. Block schema (`contenox_page/v1`) with the initial block set, defined
   server-side (Go types → JSON Schema, same discipline as the API spec);
   loud unsupported-block rendering.
2. Schema-derived `form`/`table` against `openapi.json` (same process that
   generates it, under Option B — no client mirror).
3. Serve pages: `GET /pages/{name}` rendering specs (embedded defaults,
   `~/.contenox/pages/` override); beam links to them as first-class routes.
4. Prove it by replacement: re-express ONE real page (Settings) as a spec and
   delete the hand-built version. The deleted LOC is the success metric.
5. HITL approval form from tool JSON Schema — stays client-side (React) in
   the chat transcript regardless of renderer choice; can run parallel to 3–4.

## Non-goals (for the skeleton)

- No WYSIWYG block editor (authoring YAML is fine here).
- No styling/theming blocks, ever — that's the point.
- No replacement of the chat transcript; the transcript stays hand-crafted.
