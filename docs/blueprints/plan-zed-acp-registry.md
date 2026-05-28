# Blueprint: Listing contenox on the Zed / ACP registry

Goal: get `contenox acp` installable from inside Zed (and any ACP client) via the
shared Agent Client Protocol registry, such that a user who installs it ends up
with a **working agent that has models actually wired** — not just a binary that
launches.

This doc is written so it can be executed without re-deriving anything. Facts
marked **[verified]** were checked empirically against the code/registry on
2026-05-28; facts marked **[assumed]** still need confirmation.

---

## 0. TL;DR priority order

1. **Distribution** is the real listing blocker. The registry installs a binary
   per-platform (or npx/uvx). Today contenox ships `curl|sh`, which the registry
   cannot consume. Nothing merges until this exists.
2. **Setup liveness / confidence** is the real *product* blocker (the thing that
   makes the listing worth shipping): today there is **no way to know that
   completing setup produced an agent with working models**, because setup never
   asks a model to generate anything. Fix before promoting the listing.
3. Auth/protocol is **already done** — do not spend time here. [verified]
4. Secondary bootstrap-convergence gaps (self-heal, stale-preset refresh) —
   nice-to-have hardening, not gating.

---

## 1. Where the registry lives & how submission works

- Registry repo: `https://github.com/agentclientprotocol/registry`
- Contributing guide: `registry/blob/main/CONTRIBUTING.md`
- Schema: `registry/blob/main/agent.schema.json`
- Zed blog announcement: `https://zed.dev/blog/acp-registry`
- It is a **shared** registry — once merged, the agent appears in every
  ACP-speaking client, not just Zed. Versions auto-update hourly from
  npm / PyPI / GitHub releases.

Submission = a PR adding a directory named exactly your agent `id`:

```
contenox/
  agent.json
  icon.svg
```

### agent.json required fields
- `id` — lowercase letters/digits/hyphens, must start with a letter → `contenox`
- `name` — display name → `Contenox`
- `version` — semver (`0.25.0`); keep in sync with runtime/version/version.txt
- `description`
- `distribution` — at least one method (see §2)
- optional: `repository`, `website`, `authors`, `license`

### icon.svg
- Exactly 16×16, square (width/height or viewBox).
- Monochrome: only `fill="currentColor"` or `fill="none"`. No hex/rgb colors.
- Run through SVGOMG to clean markup.

### CI validation (registry side)
Runs `python3 .github/workflows/verify_agents.py --auth-check`, which:
- validates schema, id format, semver,
- checks every `distribution` URL returns HTTP 200,
- checks icon is 16×16 + currentColor,
- **launches the agent over stdio and requires the `initialize` response to
  carry an `authMethods` entry with `type:"agent"` or `type:"terminal"`.**

The validator's exact `initialize` payload (from its `client.py`):
```json
{
  "protocolVersion": 1,
  "clientInfo": {"name": "ACP Registry Validator", "version": "1.0.0"},
  "clientCapabilities": {
    "terminal": true,
    "fs": {"readTextFile": true, "writeTextFile": true},
    "_meta": {"terminal_output": true, "terminal-auth": true}
  }
}
```

---

## 2. Distribution — THE listing blocker (must build)

The registry supports, pick ≥1:
- **Binary**: per-platform `archive` URL + `cmd`, for `darwin-aarch64`,
  `darwin-x86_64`, `linux-aarch64`, `linux-x86_64`, `windows-x86_64`.
  Archive formats: `.zip`, `.tar.gz`, `.tgz`, `.tar.bz2`, `.tbz2`. URLs must 200.
- **npm (npx)**: `package` field, e.g. `@contenox/acp@0.25.0`.
- **PyPI (uvx)**: `package` field.

Current state: install is `curl -fsSL https://contenox.com/install.sh | sh`
producing a `contenox` binary. The registry can't use that.

**Work to do — choose one:**
- (A) Have release CI publish **per-platform archives** of the `contenox` binary
  to GitHub releases, then point `distribution` at those release-asset URLs.
  The binary builds from `./cmd/contenox` (`go build -o contenox ./cmd/contenox`).
  `cmd` in the manifest = `contenox` with `args: ["acp"]` (mirror
  `.zed/settings.json`, which uses `command:"contenox", args:["acp"]`). [verified]
- (B) Publish a thin npm/PyPI wrapper that downloads the right archive.

Recommendation: (A). It reuses existing release tooling and is the least new
surface. The per-platform archive build is the only genuinely new CI work.

---

## 3. Auth — ALREADY PASSES, do not touch for listing [verified]

`contenox acp` advertises a terminal auth method. Proven by piping the
validator's exact payload into the built binary:

```
$ printf '%s\n' '<validator initialize payload from §1>' | contenox acp
{"jsonrpc":"2.0","id":1,"result":{ ... "authMethods":[
  {"id":"terminal","type":"terminal","name":"Setup Contenox", ...}]}}
```

Code: `runtime/acpsvc/initialize.go:17-32` builds the method; gated on
`clientSupportsTerminalAuth` (`initialize.go:69`) which keys off the
`_meta.terminal-auth:true` flag the validator sends. `runtime/acpsvc/authenticate.go`
accepts that method id.

### Known robustness caveat (NOT gating)
The advertisement is gated on the custom `_meta.terminal-auth` flag, not the
standard `clientCapabilities.terminal`. A terminal-capable ACP client that does
NOT send that custom flag gets **no** auth method:
```
$ printf '%s\n' '{...clientCapabilities:{terminal:true, fs:{...}}}' | contenox acp
authMethods: ABSENT   # [verified]
```
Optional hardening: also honor `caps.Terminal` (consistent with
`commandrunner.go:30`, which already gates terminal execution on it). Tradeoff:
terminal-during-auth ≠ terminal-during-session, so a broadened gate could
advertise a method some client can't fulfill. Not required for the registry
(its validator sends the flag). Decide later.

---

## 4. The confidence problem — "did setup wire a working model?"

This is the part to get right before promoting the listing. **Today there is no
signal that completing setup yields a working agent**, because nothing in the
setup path ever generates a token. [verified]

Evidence:
- `runtime/contenoxcli/setup_cmd.go` `runSetup` ends with
  `"Setup complete. Close this tab and start chatting!"` after only writing
  config + registering a backend (`registerSetupBackend`). For OpenAI/Gemini it
  stores the API key and a backend URL **unvalidated**; the model name is
  **unverified**. For Ollama it confirms the daemon is up (`ProbeLocalOllamaAPI`)
  but not that the chosen model is pulled or that generation works.
- `runtime/internal/setupcheck/setupcheck.go:90` `Evaluate` is explicitly
  **pure / no I/O** — it reports readiness from already-gathered state
  (`Reachable`, `ChatModelCount`), never from a completion.
- The serve path `runtime/contenoxcli/acp_cmd.go` `runACPProfile` only checks
  `default-model != ""` (a non-empty string), then builds the engine. First real
  generation happens at the user's first Zed prompt — the worst place to find
  out the key was wrong.
- Even the test suite can't catch this: `runtime/acpsvc/prompt_test.go:49`
  injects a `fakeAgent` returning a canned `PromptResponse`, stubbing **above**
  the engine/provider layer. No test exercises config→provider→completion.

### The fix — one live round-trip, surfaced two ways

The only thing that constitutes "knowing models are wired" is a real completion
through the **same** `enginesvc.Build → chain → provider` path the ACP agent
uses (NOT a bespoke direct API call — that would prove the wrong path).

1. **UX (setup):** make the final step of `runSetup` send a trivial prompt
   (e.g. "reply OK") through the configured engine and report the result in the
   terminal where it's fixable:
   - success → `✓ gpt-5-mini replied in 1.4s — your agent is ready`
   - failure → `✗ model 'gpt-5-mini' rejected your key (401) — re-enter it`
   This replaces the unconditional "Setup complete!" line.

2. **Auth honesty:** make `runtime/acpsvc/authenticate.go` `Authenticate`
   succeed **iff** that smoke completion succeeds (re-run the check, or read a
   freshly-written readiness marker). Today it blind-returns success.

3. **CI proof:** add an end-to-end test that stubs at the **provider layer** (a
   fake LLM backend the *real* engine calls), then drives
   `initialize → authenticate → session/new → session/prompt` over stdio and
   asserts a model-generated chunk streams back. This is the test the current
   `fakeAgent` cannot be — it must go *through* the wiring, not around it.
   Building block exists (`prompt_test.go` harness) but must be re-pointed below
   the engine.

Acceptance criterion for this section: "a green setup guarantees a subsequent
`session/prompt` returns model output" is asserted by an automated test.

---

## 5. Secondary bootstrap-convergence gaps (hardening, not gating)

What a working env needs beyond the binary, per `runACPProfile`
(`acp_cmd.go:97`): migrated SQLite DB at `~/.contenox/local.db`, a `workspace.id`,
seeded HITL policy files, the `default-acp-chain.json` chain, `default-model` /
`default-provider` config rows, and provider credentials.

Already handled [verified]:
- **DB schema migrates additively** — `libdbexec/sqlite.go:49` runs ALTERs
  one-by-one tolerating "already exists", so an old DB upgrades forward.
- **DB path is consistent** — setup and serve both resolve to
  `~/.contenox/local.db` (`resolveDBPath`→`globalDBPath`); no split-brain in the
  default Zed launch (no `--db` flag).
- **Chain IS seeded** — `RunGlobalInit` (`init.go:199`) writes
  `default-acp-chain.json` and the other presets.

Genuine gaps:
- **Serve doesn't self-heal.** `runACPProfile` seeds HITL policies and (only for
  the `acpx` profile) a chain, but **never calls `RunGlobalInit`**. So a fresh
  install that hasn't run `--setup` hard-errors at launch
  (`default-model is not configured` / chain file not found) instead of seeding
  the floor and surfacing the auth method. The serve path should ensure the
  baseline itself, then drive setup, rather than exiting.
- **No-overwrite seeding never refreshes on upgrade.** All seeders are
  seed-if-missing: `RunGlobalInit`'s `writeFile` stats-and-skips,
  `writeEmbeddedHITLPolicies(dir, false)`, `seedHeadlessACPChainIfMissing`. When
  the registry auto-updates the binary to a version whose chain/policy preset
  changed, the **stale on-disk file silently persists** against the new engine —
  no signal. There is no version stamp on these artifacts. Fix: stamp presets
  with a version and re-emit when the embedded preset is newer than on-disk.

---

## 6. Testability ladder (how each claim above is proven)

1. **Unit** — `runtime/acpsvc/specgaps2_test.go` already covers
   Initialize/Authenticate via `transportWithMeta`. Add: "Initialize advertises
   a `type:terminal` method given the terminal-auth flag" (and, if §3 hardening
   is done, given plain `caps.Terminal`).
2. **stdio contract** — wrap the §3 manual check in a Go test: spawn
   `contenox acp`, pipe the validator's exact payload, assert an `authMethod`
   with `type ∈ {agent,terminal}`. Reproduces the registry gate locally.
3. **Provider-stub e2e** — the §4.3 test: real engine, fake provider, full
   prompt cycle, assert generated output. Proves "setup ⟹ wired agent".
4. **Vendor `verify_agents.py --auth-check`** against the built binary in CI.
5. **The registry PR itself** triggers their CI — the final gate.

---

## 7. Submission checklist

- [ ] Per-platform release archives published + URLs 200 (§2). **← blocker**
- [ ] Setup ends with a live model round-trip + reports result (§4.1).
- [ ] `Authenticate` gated on a real completion (§4.2).
- [ ] Provider-stub e2e test green in CI (§4.3).
- [ ] (optional) Serve self-heals + version-stamped preset refresh (§5).
- [ ] (optional) Broaden auth gate to `caps.Terminal` (§3 caveat).
- [ ] Author `contenox/agent.json` (id `contenox`, cmd `contenox` args `["acp"]`,
      distribution → release archives).
- [ ] Author `contenox/icon.svg` (16×16, currentColor, SVGOMG'd).
- [ ] Fork `agentclientprotocol/registry`, add `contenox/`, open PR.
- [ ] Confirm registry CI green; merge.

---

## 8. Anchored file references

- `runtime/contenoxcli/acp_cmd.go` — `runACPProfile` (serve bootstrap), profiles,
  the `default-model not configured` hard error.
- `runtime/acpsvc/initialize.go` — auth method advertisement + `clientSupportsTerminalAuth`.
- `runtime/acpsvc/authenticate.go` — blind-success Authenticate (to gate on liveness).
- `runtime/contenoxcli/setup_cmd.go` — `runSetup`, `registerSetupBackend`,
  the "Setup complete!" line to replace with a smoke test.
- `runtime/internal/setupcheck/` — pure `Evaluate` + Ollama probe (reachability,
  no generation); reuse but extend to a real completion.
- `runtime/contenoxcli/init.go` — `RunGlobalInit` preset seeding (seed-if-missing).
- `runtime/acpsvc/chain.go` — `LoadChainRegistryFrom` (hard-errors if chain absent).
- `runtime/acpsvc/prompt_test.go` — `fakeAgent` harness (stubs above engine;
  e2e test must stub below it).
- `libdbexec/sqlite.go:49` — additive migration mechanism.
- `.zed/settings.json` — current local launch (`command:"contenox", args:["acp"]`).
