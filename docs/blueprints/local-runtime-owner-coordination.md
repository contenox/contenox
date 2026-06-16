# Plan: In-Process Runtime Owner — Lease Election and State Coordination

> **Status:** decision blueprint, drafted 2026-06-16.
> **Sibling docs:** `local-runtime-multi-client-coordination.md` (parent decision —
> picks the per-user owner model), `local-coding-node-goals.md`,
> `vscode-acp-permission-bridge.md`, `plan-zed-acp-registry.md`,
> `plan-openvino.md`, `plan-llamacpp.md`.
> **Purpose:** decide *how* the per-user local runtime owner is elected, reached,
> and recovered — given that it must start **in-process**, work on
> Linux/macOS/Windows, and require **no install or elevation**, because the
> primary distribution is a single ACP binary.

---

## Executive Verdict

The parent doc chose **Option E: a per-user local runtime owner**. This doc
decides the mechanism, under a constraint the parent left implicit:

```text
The owner cannot assume it is an OS-managed daemon.
```

Contenox ships primarily as an **ACP external agent**: a single binary the editor
downloads, the user authenticates once, and the editor exec's over stdio. There
is no install step, no privilege, and nowhere to register a systemd/launchd/
Windows service. The VS Code extension and the CLI are likewise plain binaries.
So "owner" must be a **role any instance can assume in-process**, not a separate
service someone installs first.

The recommended mechanism, all of whose pieces already exist or are one small
extension away:

```text
Election + discovery  -> a cross-platform TTL file lease (liblease).
Identity / fencing     -> the lease's instance UUID, checked on every call.
Coordination state     -> shared local SQLite, owner writes / followers read.
Live model/KV state    -> process-resident; REBUILT on takeover, never transferred.
Reaching the leader    -> the leader advertises an endpoint in the lease record;
                          deferred until cross-frontend sharing is actually needed.
OS-installed daemon     -> an OPTIONAL later setup-menu step, never required.
```

On the specific question that prompted this doc — *"a file or SQLite to share
state in case a process dies?"* — the answer splits by **what kind of state**:

- **Coordination metadata** (registered backends, workspace leases, active turns,
  manifest/cache index): **yes, put it in shared SQLite.** It is durable,
  queryable, already the project's backbone (`~/.contenox/local.db`), and lets a
  successor resume coordination immediately. Not too much.
- **Live model state** (loaded weights, KV cache, GPU/NPU residency, open
  inference sessions): **no — do not try to share it via a file or DB.** It is
  gigabytes of device-resident native memory bound to a process and a device
  context; you cannot hand a live llama.cpp/OpenVINO KV+GPU context to another
  process through SQLite. On takeover the successor **rebuilds** warm state from
  durable inputs (model store + context manifests). That *is* too much for the
  first pass; KV snapshot-to-disk is a known, heavy, backend-specific
  optimization for later (OpenVINO S0 proved KV round-trips, but only at
  `KV_CACHE_PRECISION=f16`).

## Source Facts

```text
- ACP external agents are single editor-spawned subprocesses over JSON-RPC/stdio,
  or remote agents over HTTP/WS. No install hook, no privilege escalation.
- A Zed external agent owns its own runtime, model selection, and config. The
  editor just downloads + runs the binary after a one-time auth.
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

The "puzzle pieces" this doc assembles — built and tested in the runtime today:

```text
liblease/                      cross-platform TTL file lease. Acquire/Renew/
                               Release/Inspect. Race-free cold acquire via
                               exclusive create (os.Link); expired-takeover via
                               atomic overwrite + readback; instance UUID as the
                               fencing token; atomic writes throughout. No OS
                               code. Unit + race tested (single-winner proven).

modeld/                        the model library + an in-process Daemon singleton
                               (state behind a RWMutex). Embeddable; not a
                               separate required binary.

modeld/transport/Service       the protocol-agnostic owner API, with a LOCAL
  + FromDaemon                 adapter (direct to the in-process Daemon) and an
                               initial gRPC REMOTE adapter. The service already
                               carries backend register/remove/list and
                               ListModels; inference/session RPCs are the next
                               expansion.

cmd/modeld/ (optional)         a standalone form that acquires the lease, runs a
                               self-fencing renew loop, and shuts down cleanly.
                               Demoted to one optional deployment shape.
```

## The Actual Problem

A single user runs several frontends against one machine and data root:

```text
VS Code window A    Zed ACP thread    CLI session    background command
```

Each is the same binary with an embedded runtime. Three things must be true, and
none is given for free under the no-daemon constraint:

### 1. Exactly one owner of resident model state

Two embedded runtimes that both load the same model duplicate memory, evict each
other from VRAM/unified memory, and destroy warm prefix reuse. The disk model
store is shareable; the live session is not. We need single-owner election that
works in-process, cross-platform, with no install.

### 2. The lease elects, but does not carry the calls

A lease tells everyone *who* the owner is and *where* it is. It does not move a
request to that process. If a follower wants the owner's warm state, it must
actually connect to a served endpoint. So discovery (lease) and transport (the
call path) are **two** mechanisms — and the transport is only needed once
followers must *call* the leader rather than *defer* to it.

### 3. State must survive owner death

When the owner process dies, a successor must be able to take over. The question
is *which* state survives and how — answered above and detailed in
"State and Takeover."

### 4. None of this may require elevation

No systemd unit, no service install, no admin rights — those are an optional
convenience the user may enable later from a setup menu, never a prerequisite.

## Non-Goals

```text
- Not an OS-managed daemon as the default or a requirement.
- Not a multi-user or LAN inference server.
- Not transferring live KV/GPU state between processes in the first pass.
- Not OS-specific lock/IPC primitives as the election mechanism.
- Not putting the lease itself inside SQLite (circular, heavier, less inspectable).
- Not sharing coordination storage over a network filesystem.
```

## Terms

```text
instance        one running binary (an embedded runtime in a frontend).
owner / leader  the instance currently holding the lease; owns resident model
                state and is the writer of coordination metadata.
follower        an instance that does not hold the lease.
lease           the TTL file (liblease) that elects the owner and advertises it.
fencing token   the owner's instance UUID; checked on every cross-process call so
                a stale owner cannot act.
coordination    durable metadata in SQLite: backends, workspace leases, active
  state         turns, manifest/cache index. Survives owner death.
resident state  process-local model weights, KV cache, GPU residency, live
                sessions. Lost on owner death; rebuilt, not transferred.
```

## Hard Invariants

```text
1.  The owner runs in-process by default. An OS-installed daemon is optional.
2.  One owner per user + data root, elected by the lease.
3.  The lease is a plain file with atomic writes — no DB, no OS lock — so it is
    portable and inspectable (cat the file to see who owns it).
4.  The lease record carries the owner's fencing token, and its endpoint once it
    serves one.
5.  Every cross-process call to the owner is fenced by the instance UUID; a
    follower never acts on a stale owner.
6.  The owner self-fences: if it cannot renew before expiry, it stops touching
    resident state BEFORE any takeover.
7.  Durable coordination metadata lives in shared local SQLite; the owner writes,
    followers read.
8.  Live model/KV/GPU state is process-resident and rebuilt on takeover from
    durable inputs — never serialized through a file or DB in the first pass.
9.  Lease file and SQLite stay on the local data root, never a networked mount.
```

## Options — Coordination Substrate

### Option A: OS primitives (flock + AF_UNIX/named pipe + peer-cred)

`flock` for election, a unix socket / named pipe for IPC, `SO_PEERCRED` for auth.

```text
+ Kernel-enforced, auto-release on crash, no clock dependency.
- Per-OS code on every axis; Windows diverges entirely.
- Assumes a place to put sockets/locks that survives the no-install model.
Verdict: rejected as the base mechanism. Windows portability and the per-OS
surface are exactly what derailed the first attempt.
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
- Must stay on local disk; the lease, not SQLite, remains the single-writer guard.
Verdict: RECOMMENDED. Right-sized durability without overreach.
```

### Option D: Lease + live state transfer (KV/session snapshots to disk)

Persist live inference/KV state so a successor resumes hot, not cold.

```text
+ Best-case warm handoff with no rebuild.
- Backend-specific, heavy, and partial: GPU/native contexts do not serialize
  generally; KV snapshot works only under specific precisions and is slow.
- Large failure surface for a rare event (owner crash mid-decode).
Verdict: too much now. Keep as an optional later optimization layered on C.
```

## Options — Follower Behavior

Orthogonal to storage: what does an instance that loses the lease do?

```text
Defer  (no transport): owner loads/serves models in-process; a follower that
       cannot get the lease waits, or runs degraded (non-resident/remote
       backend), or blocks resident-model work. Safety floor with zero IPC.

Call   (leader serves): the owner serves an endpoint advertised in the lease;
       followers route model work to it via the transport.Service REMOTE adapter
       and reconnect when the leader changes. Shared warm state from day one.
```

```text
Recommendation: DEFER first. Ships the safety floor with no transport; CALL
layers on later via the Service interface without rework.
```

## Decision Matrix

| Path | Cross-platform | Takeover quality | Build cost | Recommendation |
|---|---:|---:|---:|---|
| A. OS primitives | Low | High | High | Rejected base |
| B. Lease only | High | Cold restart | Lowest | Floor / step 1 |
| C. Lease + SQLite metadata | High | Warm coordination | Medium | **Default** |
| D. Lease + live-state transfer | Low | Hot | High | Later optimization |
| Follower: Defer | High | n/a | Lowest | **Start here** |
| Follower: Call | High | Shared | Medium | Add when needed |

## Recommended Architecture

```text
every frontend = one binary embedding { runtime, modeld, liblease }

startup:
  try liblease.Acquire(dataRoot/owner.lease, ttl)
    won  -> OWNER: renew loop (ttl/3, self-fence on failure);
                   write coordination state to SQLite;
                   [later] serve an endpoint, advertise it in the lease record.
    held -> FOLLOWER: read the lease for owner identity (+ endpoint when serving);
                   DEFER model work (step 1) or CALL the owner (later).

storage layering:
  dataRoot/owner.lease   liblease file   -> election + fencing token + endpoint
  dataRoot/local.db      shared SQLite   -> backends, workspace leases, active
                                            turns, manifest/cache index (owner
                                            writes, followers read)
  process memory         resident model state -> rebuilt on takeover, not shared

the seam that keeps it reliable:
  one transport.Service interface, two implementations —
    local  = FromDaemon (direct, in-process)        [exists]
    remote = RPC client to the leader's endpoint     [the missing twin]
  the rest of the runtime codes against the interface and never knows which.
```

The lease record gains one optional field (an opaque endpoint / metadata string)
so discovery stays a single atomic read; `liblease` itself stays generic.

## State and Takeover

The crux of the prompting question, stated as rules:

```text
Lease (file):
  who is the owner, since when, until when, instance UUID, [endpoint].
  Lost on crash via TTL expiry -> a follower acquires and becomes owner.

Coordination metadata (SQLite, durable):
  backend config, workspace leases, active/abandoned turns, manifest index.
  Survives crash -> new owner reads it and resumes coordination immediately.
  Owner is the sole writer (the lease guarantees this); followers read via WAL.

Resident model state (process memory, volatile):
  weights, KV cache, GPU residency, open sessions.
  Lost on crash -> new owner REBUILDS from the model store + manifests.
  No file/DB transfer in the first pass. KV snapshot = optional later (heavy,
  backend-specific, f16-only on OpenVINO today).
```

Why this split is "not too much": the metadata is small, structured, and already
lives in SQLite — persisting it costs almost nothing and buys clean recovery. The
resident state is large, native, and device-bound — persisting it is expensive,
fragile, and mostly infeasible, for a rare event. Durable where cheap; rebuilt
where not.

## Implementation Phases

```text
P0  Adopt this policy. Owner is in-process; OS daemon optional; lease is the
    election mechanism; metadata in SQLite; live state rebuilt.

P1  In-process lease election (liblease — done) + an owner record carrying the
    fencing token. Followers DEFER. Embed the renew/self-fence loop from
    cmd/modeld into the runtime startup. -> safety floor, no transport, no DB.

P2  Durable coordination metadata in SQLite (backends, workspace leases, active
    turns). Owner writes; followers read. Takeover resumes coordination.

P3  Owner serves an endpoint (advertised in the lease) + the transport.Service
    REMOTE adapter. Followers switch from DEFER to CALL. Reconnect on leader
    change, fenced by instance UUID. -> cross-frontend warm sharing.

P3a Canonical model package cutover: top-level modeld becomes the only provider
    package, runtime imports move from runtime/modelrepo to modeld, and the old
    runtime/modelrepo tree is deleted once focused compile checks pass. This is
    not optional cleanup; it prevents split-brain provider registries.

P3b gRPC grows from catalog/control RPCs to resident execution RPCs
    (chat/stream/embed/prompt or a session-shaped API). Followers route through
    the lease-advertised endpoint and include the lease instance UUID on every
    RPC; the owner rejects stale tokens before touching resident state.

P4  Optional self-spawned persistent owner (same binary re-exec'd) so the owner
    survives frontend churn. Still no install, no elevation.

P5  Optional OS-service install via a setup-menu choice (systemd --user /
    launchd / Windows service). Packaging only; the protocol is unchanged.

P6  Optional live KV snapshot/handoff for hot takeover (Option D), layered on C.
```

## Immediate Decisions Needed

```text
Decision 1 — Storage substrate:  Option C (lease + SQLite metadata). RECOMMENDED.
Decision 2 — Follower behavior:  DEFER first, CALL later. RECOMMENDED.
Decision 3 — Owner lifetime:     in-process for the elected instance's lifetime;
                                 self-spawned persistence (P4) and OS service
                                 (P5) are later, opt-in.
Decision 4 — Lease record shape: add one optional endpoint/meta field to carry
                                 discovery in the same atomic file.
```

## What Is In / Out

```text
In:
  in-process lease election (liblease)
  instance-UUID fencing
  self-fencing renew loop
  canonical top-level modeld package (runtime/modelrepo removed after cutover)
  SQLite coordination metadata with the owner as writer
  rebuild-on-takeover for resident state
  transport.Service local + remote split (remote when CALL lands)

Out (now):
  OS-managed daemon as a requirement
  OS-specific lock/IPC primitives as the base mechanism
  live KV/GPU state transfer between processes
  the lease living in SQLite
  coordination storage on a networked filesystem
```

## Risks and Mitigations

| Risk | Why it matters | Mitigation |
|---|---|---|
| Process-pause split-brain | A paused owner past TTL can be taken over while it still thinks it owns state | Self-fence on renew failure; fence every call by instance UUID |
| Wall-clock jump (suspend/NTP) | Lease validity is time-based | Treat timestamps as advisory; UUID fencing is the real guard; PID-liveness fast-path optional |
| Two SQLite writers | Corrupt/contended coordination state | Lease guarantees a single owner = single writer; followers read-only |
| SQLite on a network mount | Broken locking | Keep lease + DB on the local data root only |
| Stale lease after clean crash | Successor waits out the whole TTL | TTL backstop now; optional PID-liveness fast-path for instant takeover (the one bit of per-OS code) |
| Owner crash loses warm KV | Cold rebuild costs latency | Rebuild from manifests; KV snapshot optional later (P6) |
| Follower talks to a dead endpoint | Calls hang or hit a stale owner | Validate UUID on connect; reconnect by re-reading the lease on leader change |

## Minimal First Implementation

```text
1. liblease (done).
2. Embed Acquire + self-fencing renew into runtime startup; on `held`, DEFER or
   dial the leader if the needed RPC surface exists.
3. One optional endpoint/meta field on the lease record; owner writes its gRPC
   endpoint there, followers dial it and fence calls with the instance UUID.
4. A smoke test: N instances start at once -> exactly one owner; kill it ->
   a follower takes over after TTL with a fresh instance UUID.

Do NOT build live-state transfer in this slice. Prove single-owner-in-process
first, then route through gRPC with fencing before adding inference/session RPCs.
```

## Acceptance Tests

```text
single_owner_in_process:
  start N embedded instances concurrently -> exactly one acquires the lease.

self_fence_on_renew_failure:
  block the owner's renew past TTL -> owner stops touching resident state.

takeover_after_crash:
  kill the owner -> a follower acquires after expiry with a new instance UUID.

coordination_survives_takeover (P2):
  register backends -> kill owner -> successor reads them from SQLite, no reconfig.

follower_defer (P1):
  a follower that cannot get the lease blocks/degrades cleanly, never loads a
  second resident copy.

follower_call_and_failover (P3):
  follower routes work to the owner; kill the owner; follower reconnects to the
  new owner, fenced by UUID, with no call sent to the dead one.
```

## Recommended Decision

```text
Adopt Option C (TTL file lease + shared SQLite coordination metadata), with
followers DEFER-first, owner in-process by default, and live model state rebuilt
on takeover rather than transferred.

Build P1 now (lease election embedded in the runtime, followers defer); it ships
the safety floor on every platform with no install and no transport. Add P2 (DB
metadata) and P3 (serve/call) when cross-frontend warm sharing is the concrete
need. Keep self-spawned persistence (P4), OS-service install (P5), and live KV
handoff (P6) as opt-in later steps that do not change the protocol.
```

## References

```text
- Parent decision: docs/blueprints/local-runtime-multi-client-coordination.md
- liblease/ (this repo): cross-platform TTL file lease
- modeld/, modeld/transport/ (this repo): embeddable owner + Service seam
- OpenVINO KV snapshot finding (internal): KV round-trips only at f16
- Zed external agents: https://zed.dev/docs/ai/external-agents
- Agent Client Protocol: https://agentclientprotocol.com/get-started/introduction
- SQLite WAL: https://sqlite.org/wal.html
- SQLite isolation: https://sqlite.org/isolation.html
```

## Current Implementation Gaps (TODOs)

As of the current implementation, the foundational primitives have been built, but they are not yet wired together to fulfill the "in-process owner" design. The following gaps remain:

- **[ ] P1: In-process Lease Election:** The `runtime/enginesvc` does not yet import or use `liblease`. The lease acquisition and renew loop is currently isolated in the standalone `cmd/modeld` daemon. We need to embed the lease logic into `enginesvc.Build` so that multiple instances coordinate cleanly, with followers deferring resident model work.
- **[ ] P2: Single-writer Fencing for SQLite:** While the SQLite storage is implemented (`runtimestate`/`runtimetypes`), there is no lease-based fencing protecting these writes in the runtime. Followers currently might act as concurrent writers.
- **[ ] P3a: Canonical Model Package Cutover:** The resolver and execution logic remain in `runtime/llmrepo` rather than being fully consolidated into the top-level `modeld` package.
- **[ ] P3b: Execution RPCs (Chat/Stream/Embed):** The gRPC service (`modeld/transport/grpc/modeld.proto`) only implements catalog and control methods (`RegisterBackend`, `ListModels`, etc.). Inference/execution RPCs must be added to allow followers to route work to the owner's warm state.
