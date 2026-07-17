# Agent-View Filter: see the file tree with the agent's eyes

Status: draft blueprint, 2026-07-16. API design settled against the code.
Backend-first. Not started.

## Concept

A filter on the workspace file tree that shows it **as the agent sees it** —
the VFS with policies applied — instead of the raw filesystem. It makes the
sandbox *visible*: which paths the agent can reach, which it can read/write
freely, which would trigger a HITL approval, which are denied. Designed as one
member of an **extensible filter family** (later: gitignore, filetype,
modified-since, touched-this-session).

## Why it's truthful by construction (the load-bearing design rule)

The verdict is computed by running the agent's OWN gates against a hypothetical
access — never a parallel reimplementation (which would drift, the exact bug
the `vfs` consolidation just fixed). Two gates, both already server-side:

1. **Reachability** — `runtime/vfs` `View.Resolve`/`Contains` (containment +
   symlink resolution + the agent's resolved baseDir, which a policy
   `_allowed_dir` may narrow below the workspace root).
2. **Policy verdict** — `hitlservice.Service.Evaluate(ctx, "local_fs",
   "read_file"|"write_file", {"path": P})` → `EvaluationResult{Action, Reason,
   MatchedRule}`. HITL is tri-state (`allow`/`approve`/`deny`, `policy.go:14-22`)
   and its conditions match tool-call args including `path` via `OpGlob`
   (`policy.go:44`, path.Clean-normalized). So "what would the policy do if the
   agent read/wrote path P" is exact — the same engine that gates real calls.

Path convention: the synthetic `path` arg MUST match what the agent actually
passes — workspace-root-relative (the `local_fs` tool doc: "relative to the
project root"). Using the wrong form silently produces wrong verdicts.

## Backend API (careful design)

### 1. Evaluator (new package `runtime/agentview`)

```go
type Op string
const ( OpRead Op = "read"; OpWrite Op = "write" )

// Verdict is the agent's access to one path. Actions mirror hitlservice.Action
// ("allow" | "approve" | "deny"); Read/Write are empty when !Reachable.
type Verdict struct {
    Reachable bool               `json:"reachable"`
    Read      hitlservice.Action `json:"read,omitempty"`
    Write     hitlservice.Action `json:"write,omitempty"`
    // Optional explanation for the read verdict (which rule / why), so the UI
    // can answer "why can't the agent touch this?". Populated from
    // EvaluationResult.Reason / MatchedRule. Omitted when allow-by-default.
    ReadReason  string `json:"readReason,omitempty"`
    WriteReason string `json:"writeReason,omitempty"`
}

type Evaluator struct { /* view *vfs.View; hitl hitlservice.Service; policyName string */ }

// NewEvaluator binds a workspace view + the SESSION's active HITL policy.
func NewEvaluator(view *vfs.View, hitl hitlservice.Service, policyName string) *Evaluator

// Verdict evaluates one workspace-root-relative path. For directories, Read is
// evaluated as local_fs.list_dir and Write as create-inside semantics.
func (e *Evaluator) Verdict(ctx context.Context, rootRelPath string, isDir bool) Verdict
```

Reachable=false short-circuits (no policy eval). Reachable=true runs two
`Evaluate` calls (read+write sub-tools; list_dir for dirs). The evaluator holds
no per-path state — cheap, called per listed entry.

### 2. HTTP: extend `/files` (in `runtime/internal/localfileapi`)

- New optional query param **`filter`** on `GET /files`. Absent/`full`
  (default) = today's raw tree, response byte-identical (backward compatible).
  `agent` = apply the agent view.
- New optional query param **`policy`** — the session's active HITL policy name
  (the chat has a per-session HITL Policy selector; beam passes it so the view
  matches THAT session's agent). Omitted → the config default policy
  (`hitl-policy-name`, else `hitl-policy-default.json`).
- Response `Entry` gains an **optional** `access: Verdict` field, present ONLY
  when `filter=agent`. Additive; full-tree responses are unchanged:

```json
{ "path": "src/main.go", "name": "main.go", "isDirectory": false, "size": 812,
  "access": { "reachable": true, "read": "allow", "write": "approve",
              "writeReason": "matched rule 2" } }
```

- **Mode: annotated (default agent view)** — every entry is returned WITH its
  verdict (unreachable ones marked `reachable:false`, not omitted), so the user
  sees the boundary, not just the inside. A future `&hide=unreachable` can
  narrow. (DECISION FOR MAINTAINER — annotated is recommended; confirm.)

### 3. Extensibility (the "later path for more filter types")

- `filter` is a small extensible token. Structure the handler around a
  server-side filter registry (`type FileFilter interface { Name() string;
  Apply(ctx, entries, opts) ([]Entry, error) }`) so future filters register as
  named implementations. MVP registers ONE (`agent`); do NOT build the whole
  registry speculatively — just put the single filter behind the interface so
  adding more is additive.
- Grammar: single token now; reserve CSV composition (`filter=agent,gitignore`)
  for later. Each filter may add its own OPTIONAL annotation fields to `Entry`
  (keep the shape open) and/or narrow the set.

## Frontend (deferred — see slices)

- A filter control on the workspace panel (`WorkspacePanel.tsx`): dropdown
  "Full tree" / "Agent view (policies applied)", built as an extensible list.
- When "Agent view", `useWorkspaceFiles` requests `?filter=agent&policy=<the
  session's active HITL policy>` and renders per-entry badges: reachable? and
  read/write allow(ok)/approve(caution)/deny(blocked), with a legend and the
  reason in a tooltip. Unreachable entries greyed.
- The policy passed = the session's live HITL selection (or staged/config on an
  empty chat). This is the tie that makes the view match reality.

## Decisions for the maintainer

- **Annotated vs subset** default (recommend annotated — the boundary is the
  point; subset can layer via `hide=unreachable`).
- **Per-op read/write verdicts** (recommend yes — write is the interesting one;
  policies distinguish).
- **Surface the reason** (recommend yes — `MatchedRule`/`Reason` are free and
  answer "why?"; the trust-tool payoff).

## Slices

### Slice 1 — Backend (FIRST; Go; no collision with beam UI efforts)
`runtime/agentview` evaluator + `/files` `filter`/`policy` params + optional
`access` annotation + the filter interface with the single `agent` filter.
Wire the session's HITL policy loading (mirror `acpsvc` `activeHITLPolicy` /
`clikv`). Tests: verdict truthfulness (a deny-glob policy → `read:"deny"` for
matching paths, `allow` elsewhere; an unreachable/escaping symlink →
`reachable:false`; approve-glob → `approve`), and `/files?filter=agent` shape +
backward-compat of the default. `go build ./... && go test ./runtime/...`.

### Slice 2 — Frontend (LATER; after the composer-file-preview effort frees
`WorkspacePanel.tsx` / `useWorkspaceFiles.ts`)
The filter control + annotated rendering + passing the session HITL policy.
