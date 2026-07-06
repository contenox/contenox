# Plan: modeld Provisioning and Detection

> **Status:** decision blueprint.
> **Sibling docs:** `interface-boundary.md` (the compute boundary),
> `owner-coordination.md` (the owner lease), `llama/coding-node-plan.md`.
> **Purpose:** decide how `runtime` discovers the second binary (`modeld`),
> guides the user to install it, and fails honestly when it is missing or dead —
> so that local inference availability is a *detected* condition, not a hardcoded
> error.

---

## Two Binaries, One Detection Seam

`runtime` is pure Go and ships standalone; local inference lives in the separate
CGO `modeld` binary (see `interface-boundary.md`). The runtime must
therefore answer one question whenever local inference is requested:

```text
Is modeld installed, running, and reachable — and if not, what should the user do?
```

Today that condition is faked: `runtime/modelrepo/local` returns a constant
`errLocalUnavailable` from every connection. This document replaces that constant
with a **detection mechanism**: the local provider consults a probe, and the
error a user sees describes the *actual* state (not installed / not running /
stale) with the matching action. The same probe feeds the setup wizard.

### Honest caveat: detection proves install + liveness, not usability

The `modeld` side is not implemented yet (no wire transport). So a probe that
reports "running" does **not** mean inference works. Detection has a terminal
rung — *detected but transport not wired* — which keeps `errLocalUnavailable` as
the last state after the install/liveness checks pass. We are designing the seam
now; the reachability half lands with the transport.

## Detection Model

Two signals, combined into one status:

```text
1. Locate the binary.
   explicit path (CONTENOX_MODELD_BIN / setting) -> else PATH lookup ("modeld").
   The explicit override is mandatory, not a convenience: the VS Code flow
   installs modeld into an extension dir that is not on PATH.

2. Inspect the lease.
   liblease.Inspect(<dataRoot>/modeld.lease):
     - no lease file        -> no owner has ever run / not running now
     - lease present, fresh  -> a live owner (endpoint in Meta["endpoint"])
     - lease present, expired-> stale: the owner crashed or was killed
```

These resolve to four states with distinct user actions:

| State | Binary | Lease | Meaning | User action |
|---|---|---|---|---|
| **NotInstalled** | absent | — | modeld is not on the machine | install it (script / instructions) |
| **NotRunning** | present | none / no live owner | installed but no daemon | start modeld (or let runtime spawn it) |
| **Stale** | present | expired | the daemon died | restart; a successor takes over after TTL |
| **Running** | present | fresh | a live owner holds the lease | connect (transport — future) |

Liveness via lease-freshness is a *proxy*, not a health check: a wedged process
can still hold a fresh lease. A real liveness ping rides on the IPC transport and
is added when that exists. Until then, lease-freshness is the best pure-Go signal
and is documented as provisional.

## Error Taxonomy

The probe returns a status; `Status.Err()` maps it to a typed, actionable error
(nil when Running). The runtime keeps the messages short; the setup wizard
formats the rich install guidance.

```text
ErrNotInstalled  "modeld is not installed"          -> install
ErrNotRunning    "modeld is not running"            -> start
ErrStale         "modeld owner is stale (crashed?)" -> restart
(Running)        nil                                -> proceed (then transport)
```

`runtime/modelrepo/local` returns these directly. When the state is Running but
the transport is not wired, it returns the terminal `errLocalUnavailable`.

## Where It Lives

```text
runtime/internal/modeldprobe   pure-Go detector. Locates the binary, inspects the
                               lease via liblease, returns Status + typed errors.
                               No CGO, no modeld import beyond owner.EndpointMetaKey.

runtime/modelrepo/local        the boundary provider. Its connection methods call
                               the probe and return the detection error instead of
                               a constant.

runtime/internal/setupcheck    the setup wizard / CLI doctor. Adds a modeld
                               readiness check next to the existing ollama probe,
                               and formats install guidance per state.
```

The probe is consulted **lazily** — only when a local-inference connection is
actually requested, never at runtime startup.

## Setup / Install UX Layers

The same detection drives three surfaces, simplest first:

1. **Setup wizard / CLI doctor (now, text).** When the user selects a local
   backend and the probe reports NotInstalled/NotRunning/Stale, print the state
   and the exact next step. This is the minimum and ships first.

2. **Install script (next).** A platform-aware script the wizard can offer to run
   (`curl … | sh` style on unix; a documented manual path on Windows) that
   fetches the right `modeld` build for the OS/arch and places it where the
   locator will find it. The wizard never installs silently; it offers.

3. **VS Code-installed flow (next).** The extension bundles or downloads a
   matching `modeld` into its own storage dir and passes that path to runtime via
   `CONTENOX_MODELD_BIN` (the explicit-override requirement above). The same
   probe then reports Running once the extension starts it. No PATH assumption.

ACP/Zed external-agent installs follow the same shape: the agent binary is
`runtime`; `modeld` is provisioned alongside it and located by explicit path.

## How This Removes the Construction Error

Before: `runtime/modelrepo/local` returns a constant `errLocalUnavailable` —
true today, but a dead end that says nothing useful.

After: the provider asks the probe. A user without modeld gets
"modeld is not installed" plus a way to fix it; a user whose daemon crashed gets
"modeld owner is stale"; a user with a live daemon gets the honest
"transport not wired yet" until that half exists. The error becomes a function of
reality, and the same code path turns into the real connect path once the
transport lands — nothing in `local` is throwaway.

## Non-Goals

```text
- Not auto-installing modeld without user consent.
- Not a real liveness/health check beyond lease freshness (needs the IPC ping).
- Not spawning modeld from runtime in this pass (runtime detects; spawn is later).
- Not platform install scripts or VS Code packaging as code in this pass
  (designed here; built next).
```

## Phases

```text
P0  Detector package (runtime/internal/modeldprobe): locate + lease-inspect ->
    Status + typed errors. Unit-tested with injected clock and PATH lookup.

P1  Rewire runtime/modelrepo/local to derive its error from the detector;
    errLocalUnavailable becomes the terminal "Running but transport unwired" rung.

P2  setupcheck modeld readiness check + per-state guidance text in the wizard.

P3  Install script (OS/arch fetch + placement) the wizard can offer to run.

P4  VS Code extension provisions modeld into its storage dir and passes
    CONTENOX_MODELD_BIN; same probe reports Running.

P5  Real liveness/reachability via the IPC transport ping (depends on the
    transport landing).
```

## Scaffolded in This Pass

- **P0** — `runtime/internal/modeldprobe` detector + tests.
- **P1** — `runtime/modelrepo/local` rewired to the detector.

Everything from P2 on is designed here and built in later passes.
