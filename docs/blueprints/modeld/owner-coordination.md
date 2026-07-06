# Plan: Local Runtime Owner — Lease Election and State Coordination

> **Status:** decision blueprint.
> **Sibling docs:** `multi-client-coordination.md` (parent decision —
> picks the per-user owner model), `interface-boundary.md` (the compute
> boundary modeld exposes), `../local-coding-node-goals.md`,
> `../vscode/acp-permission-bridge.md`, `../acp/zed-registry.md`,
> `openvino/coding-node-plan.md`, `llama/coding-node-plan.md`.
> **Purpose:** decide *how* the per-user local runtime owner — the `modeld`
> daemon — is elected, reached, and recovered across Linux, macOS, and Windows,
> without requiring privilege or a system-managed service.

---

## Two Binaries

Contenox ships as two binaries with a strict division of responsibility:

```text
runtime   pure Go. Orchestration: model downloads, backend and catalog
          management, cloud providers, agent and workflow execution. Ships and
          runs on its own. Cloud inference (Anthropic, OpenAI, Gemini) goes
          direct from runtime and never touches modeld.

modeld    CGO. Native local inference (llama.cpp, OpenVINO). Owns device memory
          (VRAM / unified memory), KV cache, and live inference sessions. It is
          the hardware owner.
```

The dividing line is the native boundary: **everything CGO lives in `modeld`;
everything else lives in `runtime`.** runtime is always a client of modeld for
local inference. When local inference is requested and modeld is not present,
runtime guides the user to install it; runtime itself keeps working with cloud
providers in the meantime.

## Executive Verdict

The parent doc chose **Option E: a per-user local runtime owner**. The owner is
the `modeld` daemon. This doc decides the election, discovery, and recovery
mechanism under one constraint:

```text
modeld must run as a per-user process without privilege or a system service.
```

modeld is installed alongside runtime, but it runs as an ordinary user-level
process — there is no required systemd/launchd/Windows service and no elevation.
A single modeld owns the local hardware per user and data root; multiple runtime
frontends (VS Code, Zed, CLI) connect to that one owner over local IPC.

The recommended mechanism:

```text
Election + discovery   -> a cross-platform TTL file lease (liblease) held by the
                          owning modeld instance.
Identity / fencing      -> the lease's instance UUID, checked on every IPC call.
Coordination state      -> shared local SQLite, owner writes / followers read.
Live model/KV state     -> process-resident in modeld; REBUILT on takeover.
Reaching the owner      -> modeld advertises an IPC endpoint in the lease record;
                          runtime clients dial it and fence calls by UUID.
OS-installed service     -> an OPTIONAL later setup step, never required.
```

On the question of *what state to persist so a successor can recover*, the
answer splits by **kind of state**:

- **Coordination metadata** (registered backends, workspace leases, active turns,
  manifest/cache index): **put it in shared SQLite.** It is durable, queryable,
  already the project's backbone (`~/.contenox/local.db`), and lets a successor
  resume coordination immediately.
- **Live model state** (loaded weights, KV cache, GPU/NPU residency, open
  inference sessions): **do not share it via a file or DB.** It is gigabytes of
  device-resident native memory bound to a process and device context. On
  takeover the successor modeld **rebuilds** warm state from durable inputs
  (model store + context manifests). KV snapshot-to-disk is a known,
  backend-specific optimization for later (OpenVINO S0 proved KV round-trips, but
  only at `KV_CACHE_PRECISION=f16`).

## Source Facts

```text
- runtime and modeld are separate binaries. runtime is pure Go; modeld carries
  the CGO inference bindings. Cloud providers run inside runtime and bypass
  modeld entirely.
- ACP external agents are single editor-spawned subprocesses over JSON-RPC/stdio,
  or remote agents over HTTP/WS. runtime is the agent; it connects to modeld for
  local inference.
- OS-specific single-instance/IPC primitives do not generalize cleanly:
  flock + AF_UNIX + SO_PEERCRED are unix-only; Windows uses named pipes,
  LockFileEx, named mutexes, and ACLs. Per-OS code is the cost of using them.
- A content lock file does NOT auto-release on crash the way flock does, so a
  durable lock needs a TTL to be self-healing.
- All instances run on one host, so there is no cross-node clock skew — but the
  wall clock can still jump (NTP step, suspend/resume).
- SQLite WAL allows many concurrent readers plus one writer across processes on a
  local filesystem; it serializes writers. Its locking is unreliable on NFS.
- Contenox already keeps a large local SQLite store at ~/.contenox/local.db.
```

## What We Already Have

```text
liblease/                      cross-platform TTL file lease. Acquire/Renew/
                               Release/Inspect. Race-free cold acquire via
                               exclusive create (os.Link); expired-takeover via
                               atomic overwrite + readback; instance UUID as the
                               fencing token; atomic writes throughout. No OS
                               code. Unit + race tested (single-winner proven).

modeld                         the CGO inference daemon. Owns device memory, KV
                               cache, and sessions. Runs as a per-user process.

runtime/transport              the compute contract (see
                               modeld/interface-boundary.md for the surface).
                               runtime owns it; modeld implements it. runtime
                               reaches modeld exclusively through this boundary.
```

## The Actual Problem

A single user runs several runtime frontends against one machine and data root:

```text
VS Code window A    Zed ACP thread    CLI session    background command
```

Each is a runtime client that needs local inference from modeld. Three things
must be true:

### 1. Exactly one owner of resident model state

Two modeld processes that both load the same model duplicate memory, evict each
other from VRAM/unified memory, and destroy warm prefix reuse. The disk model
store is shareable; the live session is not. We need single-owner election that
works cross-platform with no privilege.

### 2. The lease elects, but does not carry the calls

A lease tells everyone *who* the owning modeld is and *where* it is. It does not
move a request to that process. A runtime client must connect to modeld's served
endpoint. So discovery (lease) and transport (the call path) are **two**
mechanisms.

### 3. State must survive owner death

When the owning modeld dies, a successor must take over. Which state survives and
how is answered in "State and Takeover."

### 4. None of this may require elevation

No systemd unit, no service install, no admin rights — an OS-managed service is
an optional convenience the user may enable later, never a prerequisite.

## Non-Goals

```text
- Not an OS-managed service as the default or a requirement.
- Not a multi-user or LAN inference server.
- Not transferring live KV/GPU state between processes in the first pass.
- Not OS-specific lock/IPC primitives as the election mechanism.
- Not putting the lease itself inside SQLite (circular, heavier, less inspectable).
- Not sharing coordination storage over a network filesystem.
- Not routing cloud providers through modeld; runtime calls them directly.
```

## Terms

```text
instance        one running process holding (or contending for) the owner lease.
owner           the modeld instance currently holding the lease; owns resident
                model state and is the writer of coordination metadata.
follower        a modeld instance that does not hold the lease.
client          a runtime frontend that connects to the owning modeld for local
                inference.
lease           the TTL file (liblease) that elects the owner and advertises it.
fencing token   the owner's instance UUID; checked on every cross-process call so
                a stale owner cannot act.
coordination    durable metadata in SQLite: backends, workspace leases, active
  state         turns, manifest/cache index. Survives owner death.
resident state  modeld's process-local model weights, KV cache, GPU residency,
                live sessions. Lost on owner death; rebuilt, not transferred.
```

## Hard Invariants

```text
1.  The owner is the modeld daemon, a separate per-user process. An OS-managed
    service is optional; privilege is never required.
2.  One owner per user + data root, elected by the lease.
3.  The lease is a plain file with atomic writes — no DB, no OS lock — so it is
    portable and inspectable (cat the file to see who owns it).
4.  The lease record carries the owner's fencing token and its IPC endpoint.
5.  Every call to the owner is fenced by the instance UUID; a client never acts
    on a stale owner.
6.  The owner self-fences: if it cannot renew before expiry, it stops touching
    resident state BEFORE any takeover.
7.  Durable coordination metadata lives in shared local SQLite; the owner writes,
    followers read. Writes are guarded by an explicit owner assertion, not by
    convention.
8.  Live model/KV/GPU state is process-resident and rebuilt on takeover from
    durable inputs — never serialized through a file or DB in the first pass.
9.  Lease file and SQLite stay on the local data root, never a networked mount.
10. The owner acquires the lease lazily — on the first resident local-model
    operation, not at startup — so idle frontends never hold ownership.
```

## Options — Coordination Substrate

### Option A: OS primitives (flock + AF_UNIX/named pipe + peer-cred)

`flock` for election, a unix socket / named pipe for IPC, `SO_PEERCRED` for auth.

```text
+ Kernel-enforced, auto-release on crash, no clock dependency.
- Per-OS code on every axis; Windows diverges entirely.
Verdict: rejected as the base mechanism. Windows portability and the per-OS
surface are the cost.
```

### Option B: TTL file lease only, rebuild state on takeover

The lease (liblease) elects; nothing else is persisted; a successor rebuilds.

```text
+ Simplest possible; fully cross-platform; already built and tested.
+ Gives the safety floor everywhere with zero IPC and zero DB.
- A successor starts cold on every takeover (re-discovers backends, re-derives
  in-flight coordination).
Verdict: correct floor; insufficient alone once coordination state matters.
```

### Option C: TTL file lease + shared SQLite coordination metadata

The lease elects and fences; durable coordination metadata lives in SQLite so a
successor resumes cleanly. Live model state is still rebuilt.

```text
+ Cross-platform; reuses the existing local.db backbone.
+ Takeover resumes coordination (backends, leases, active turns) immediately.
+ WAL gives followers consistent reads while the owner writes.
- Two mechanisms to keep coherent (lease for liveness, DB for metadata).
Verdict: RECOMMENDED. Right-sized durability without overreach.
```

### Option D: Lease + live state transfer (KV/session snapshots to disk)

Persist live inference/KV state so a successor resumes hot, not cold.

```text
+ Best-case warm handoff with no rebuild.
- Backend-specific, heavy, and partial: GPU/native contexts do not serialize
  generally; KV snapshot works only under specific precisions and is slow.
Verdict: too much now. Keep as an optional later optimization layered on C.
```

## Decision Matrix

| Path | Cross-platform | Takeover quality | Build cost | Recommendation |
|---|---:|---:|---:|---|
| A. OS primitives | Low | High | High | Rejected base |
| B. Lease only | High | Cold restart | Lowest | Floor / step 1 |
| C. Lease + SQLite metadata | High | Warm coordination | Medium | **Default** |
| D. Lease + live-state transfer | Low | Hot | High | Later optimization |

## Recommended Architecture

```text
runtime (client)              modeld (owner candidate)
  cloud providers               native inference (CGO)
  downloads / catalogs          device memory + KV + sessions
  agent / workflow exec         holds the owner lease

runtime needs local inference:
  resolve the owner from the lease record (owner UUID + IPC endpoint)
    present -> dial modeld over the transport, fence calls by UUID
    absent  -> guide the user to install modeld; cloud paths keep working

modeld startup:
  on first resident local-model op, try liblease.Acquire(owner.lease, ttl)
    won  -> OWNER: renew loop (ttl/3, self-fence on failure);
                   write coordination state to SQLite;
                   serve an IPC endpoint, advertise it in the lease record.
    held -> FOLLOWER: defer to the owner; do not load a second resident copy.

storage layering:
  dataRoot/owner.lease   liblease file   -> election + fencing token + endpoint
  dataRoot/local.db      shared SQLite   -> backends, workspace leases, active
                                            turns, manifest/cache index (owner
                                            writes, followers read)
  modeld process memory  resident model state -> rebuilt on takeover, not shared
```

The lease record carries an opaque endpoint/metadata string so discovery stays a
single atomic read; `liblease` itself stays generic.

## State and Takeover

```text
Lease (file):
  who is the owner, since when, until when, instance UUID, endpoint.
  Lost on crash via TTL expiry -> a follower acquires and becomes owner.

Coordination metadata (SQLite, durable):
  backend config, workspace leases, active/abandoned turns, manifest index.
  Survives crash -> new owner reads it and resumes coordination immediately.
  Owner is the sole writer (the lease guarantees this); followers read via WAL.

Resident model state (modeld process memory, volatile):
  weights, KV cache, GPU residency, open sessions.
  Lost on crash -> new owner REBUILDS from the model store + manifests.
  No file/DB transfer in the first pass. KV snapshot = optional later (heavy,
  backend-specific, f16-only on OpenVINO today).
```

The metadata is small, structured, and already lives in SQLite — persisting it
costs almost nothing and buys clean recovery. The resident state is large,
native, and device-bound — persisting it is expensive and mostly infeasible for
a rare event. Durable where cheap; rebuilt where not.

## Implementation Phases

```text
P0  Adopt this policy. Owner is the modeld daemon; OS service optional; lease is
    the election mechanism; metadata in SQLite; live state rebuilt.

P1  Lazy lease election in modeld (liblease) on the first resident local-model
    op, with an owner record carrying the fencing token and IPC endpoint. A
    follower modeld defers. -> single-owner safety floor.

P2  Durable coordination metadata in SQLite (backends, workspace leases, active
    turns), guarded by an explicit owner assertion. Owner writes; followers read.
    Takeover resumes coordination.

P3  runtime reaches the owner: resolve the lease, dial modeld's advertised IPC
    endpoint, fence every call by instance UUID, reconnect on owner change.
    Compute/session RPCs follow the boundary in modeld/interface-boundary.md.

P4  Optional persistent owner: modeld stays resident across frontend churn so
    warm state survives editor restarts. Still no privilege, no system service.

P5  Optional OS-service install via a setup-menu choice (systemd --user /
    launchd / Windows service). Packaging only; the protocol is unchanged.

P6  Optional live KV snapshot/handoff for hot takeover (Option D), layered on C.
```

## Immediate Decisions Needed

```text
Decision 1 — Storage substrate:  Option C (lease + SQLite metadata). RECOMMENDED.
Decision 2 — Owner lifetime:     modeld holds ownership for its lifetime;
                                 persistent owner (P4) and OS service (P5) are
                                 later, opt-in.
Decision 3 — Lease record shape: an optional endpoint/meta field carries
                                 discovery in the same atomic file.
Decision 4 — Lease timing:       lazy acquisition on first resident local-model
                                 op, never at startup.
```

## What Is In / Out

```text
In:
  lazy lease election in modeld (liblease)
  instance-UUID fencing on every runtime->modeld call
  self-fencing renew loop
  SQLite coordination metadata with the owner as the guarded writer
  rebuild-on-takeover for resident state
  runtime as a client of modeld over the transport; cloud paths bypass modeld

Out (now):
  OS-managed service as a requirement
  OS-specific lock/IPC primitives as the base mechanism
  live KV/GPU state transfer between processes
  the lease living in SQLite
  coordination storage on a networked filesystem
  routing cloud providers through modeld
```

## Risks and Mitigations

| Risk | Why it matters | Mitigation |
|---|---|---|
| Process-pause split-brain | A paused owner past TTL can be taken over while it still thinks it owns state | Self-fence on renew failure; fence every call by instance UUID |
| Wall-clock jump (suspend/NTP) | Lease validity is time-based | Treat timestamps as advisory; UUID fencing is the real guard; PID-liveness fast-path optional |
| Two SQLite writers | Corrupt/contended coordination state | Lease guarantees a single owner = single writer; followers read-only; writes assert ownership |
| SQLite on a network mount | Broken locking | Keep lease + DB on the local data root only |
| Stale lease after clean crash | Successor waits out the whole TTL | TTL backstop now; optional PID-liveness fast-path for instant takeover |
| Owner crash loses warm KV | Cold rebuild costs latency | Rebuild from manifests; KV snapshot optional later (P6) |
| Client talks to a dead endpoint | Calls hang or hit a stale owner | Validate UUID on connect; reconnect by re-reading the lease on owner change |
| modeld absent on a client request | Local inference unavailable | runtime detects it, guides install, and keeps cloud paths working |

## Minimal First Implementation

```text
1. liblease (done).
2. modeld acquires the lease lazily on the first resident local-model op, runs a
   self-fencing renew loop, and writes its IPC endpoint to the lease record.
3. runtime resolves the lease, dials modeld, and fences calls with the UUID.
4. A smoke test: N modeld instances contend at once -> exactly one owner; kill it
   -> a follower takes over after TTL with a fresh instance UUID.

Do NOT build live-state transfer in this slice. Prove single-owner election and
fenced IPC first, then add compute/session RPCs.
```

## Acceptance Tests

```text
single_owner:
  start N modeld instances contending concurrently -> exactly one acquires the
  lease.

self_fence_on_renew_failure:
  block the owner's renew past TTL -> owner stops touching resident state.

takeover_after_crash:
  kill the owner -> a follower acquires after expiry with a new instance UUID.

coordination_survives_takeover (P2):
  register backends -> kill owner -> successor reads them from SQLite, no reconfig.

follower_defer (P1):
  a modeld that cannot get the lease never loads a second resident copy.

client_call_and_failover (P3):
  a runtime client routes work to the owner; kill the owner; the client
  reconnects to the new owner, fenced by UUID, with no call sent to the dead one.

modeld_absent (P3):
  a local-inference request with no modeld present surfaces an install prompt;
  cloud provider requests are unaffected.
```

## Recommended Decision

```text
Adopt Option C (TTL file lease + shared SQLite coordination metadata), with
modeld as the per-user owner, runtime as its client, lazy lease acquisition, and
live model state rebuilt on takeover rather than transferred.

Build P1 now (lazy lease election in modeld, followers defer); it ships the
single-owner safety floor on every platform with no privilege. Add P2 (DB
metadata) and P3 (runtime dials the owner) when cross-frontend warm sharing is
the concrete need. Keep persistent owner (P4), OS-service install (P5), and live
KV handoff (P6) as opt-in later steps that do not change the protocol.
```

## References

```text
- Parent decision: multi-client-coordination.md (this directory)
- Compute boundary: docs/blueprints/modeld/interface-boundary.md
- liblease/ (this repo): cross-platform TTL file lease
- OpenVINO KV snapshot finding (internal): KV round-trips only at f16
- Zed external agents: https://zed.dev/docs/ai/external-agents
- Agent Client Protocol: https://agentclientprotocol.com/get-started/introduction
- SQLite WAL: https://sqlite.org/wal.html
- SQLite isolation: https://sqlite.org/isolation.html
```

## Current Implementation Gaps (TODOs)

The foundational primitives exist but are not yet wired into the owner model:

- **[ ] P1: Lazy Lease Election in modeld:** modeld does not yet acquire the
  owner lease on the first resident local-model op or run the renew/self-fence
  loop as the ownership gate. Follower instances must defer resident work.
- **[ ] P2: Owner-Guarded SQLite Writes:** SQLite storage is implemented
  (`runtimestate`/`runtimetypes`), but coordination writes are not yet guarded by
  an explicit owner assertion. Followers must be read-only.
- **[ ] P3: runtime → modeld Transport:** runtime resolves the lease and dials
  modeld's advertised endpoint with UUID fencing; compute/session RPCs follow the
  boundary in `interface-boundary.md`.
