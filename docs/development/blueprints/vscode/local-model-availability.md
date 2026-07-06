# Local Model Availability in Editor Surfaces

> **Status:** decision blueprint, implemented.
> **Scope:** how editor-facing surfaces (VS Code bridge, ACP clients such as
> Zed) advertise and select local modeld-backed models without flapping across
> daemon restarts.
> **Sibling docs:** `../modeld/provisioning-detection.md` (detection seam),
> `../modeld/owner-coordination.md` (owner lease),
> `../modeld/single-active-model-slot.md` (single-slot execution model).

## Problem

Editor clients build a model picker from the runtime's capability catalog. A
local model's capabilities come from modeld, and modeld's reachability is
observed through a short-TTL owner lease. If capability advertisement is gated
directly on the live lease, every daemon restart opens a gap of a few seconds
in which every local model reports `CanChat=false`. The picker then drops the
whole local model group, a selection made during the gap is rejected as an
unknown model option, and the session silently continues on whichever remote
model was previously active.

The failure is structural, not a race to be shrunk: "is this model selectable"
was answered with "is the daemon answering this millisecond". Those are
different questions with different stability requirements.

## Core rule

Capability and selectability must track **"modeld can serve this"** (stable
across restarts), while execution must track **"modeld is serving now"**
(strict). Advertisement paths and execution paths therefore use different
availability checks, and no advertisement path may gate on a single live
probe.

## Design

### Layer 1: graced serveable-backend advertisement

`modeldconn.ServeableBackend()` answers the advertisement question:

- while a fresh lease is held, it returns the live backend;
- for a grace window after the lease drops, it returns the last-observed
  backend;
- past the grace window, it returns empty and local models disappear from
  pickers honestly.

Providers use `ServeableBackend()` when advertising capabilities. Live-decode
paths keep the strict `Backend()`/`Available()` checks: a request during a
restart gap still fails fast rather than executing against a daemon that is
not there.

Catalogs follow the same split. A model stays listed with its profile-declared
capabilities when `Describe` fails *because modeld is momentarily gone*; a
model that a running modeld cannot describe is still skipped. The distinction
keeps the picker stable without ever advertising a model the daemon has
actively rejected.

modeld serves one backend selected at startup and swaps only the loaded model
within a single slot. Hiding the non-selected backend's models is therefore
correct behavior, not a flap; only restart-gap blanking was the defect.

### Layer 2: resolution self-heal on miss

The runtime reconciles backend model state at engine init and on explicit
refresh; there is no periodic reconcile loop to repair state later. A
long-running runtime that reconciled while modeld was down would otherwise
hold an empty local-model state forever, failing every local request even
after the daemon returns.

`llmrepo`'s model manager self-heals at the point of failure instead: when
resolution fails with a no-available-models or no-satisfactory-model error, it
runs one debounced backend state cycle and retries resolution once. This hook
sits in all resolve paths (prompt, chat, embed, stream), so every entry point
— CLI, ACP, VS Code bridge — heals without any surface-specific logic. A
debounce guards against reconcile storms when the daemon is genuinely down.

A resolution miss triggers reconciliation; other resolver errors do not, so
downstream failures cannot masquerade as state staleness.

## Rejected alternatives

- **Auto-starting modeld from the runtime.** The runtime does not own the
  daemon lifecycle; provisioning stays a detected condition
  (`../modeld/provisioning-detection.md`).
- **Show-when-down.** Advertising local models while modeld is absent invites
  selections that can only fail at decode time.
- **Periodic reconcile ticker.** The runtime has no single long-lived server
  loop to host it; per-entry-point tickers would multiply. Healing on
  resolution miss is entry-point-agnostic and only does work when needed.

## How to apply

- When advertising local capability, go through the graced
  `ServeableBackend()`; never gate advertisement on `modeldconn.Backend()` or
  any other single live probe.
- Keep strict availability checks on paths that actually execute against the
  daemon.
- Treat model-resolution state as self-healing on miss; never assume
  startup reconciliation remains valid for the process lifetime.
- Resource management that unloads idle models must reap the model, not the
  lease, so idleness never flaps advertised capability.
