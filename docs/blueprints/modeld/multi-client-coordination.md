# Plan: Local Runtime Multi-Client Coordination

> **Status:** decision blueprint, researched 2026-06-16.  
> **Sibling docs:** `../local-coding-node-goals.md`, `llama/coding-node-plan.md`,
> `openvino/coding-node-plan.md`, `ortgenai-windows-ai.md`,
> `../vscode/acp-permission-bridge.md`.  
> **Purpose:** decide how Contenox should behave when one user has multiple
> editors, ACP clients, CLI sessions, and local runtime processes pointed at the
> same machine and possibly the same workspace.

---

## Executive Verdict

The original local-node wording says "single user, one machine, one active
repo/workspace." That must not be interpreted as "one editor process." A real
developer can have:

```text
VS Code window A
VS Code window B
Zed ACP thread
terminal CLI session
background setup/status command
```

all touching the same local Contenox data directory and sometimes the same
workspace. That is normal, not an edge case.

The local runtime therefore needs one explicit ownership model:

```text
Multiple frontends are allowed.
One local runtime owner controls resident model sessions and live KV state.
Workspace mutations are serialized by lease.
Model artifacts are immutable once published.
```

The recommended path is **a per-user local node owner process, started on demand
by the first client**, with frontends attaching over local IPC. It is not a
helper-model sidecar and not a multi-tenant server. It is the one Contenox
runtime process that owns expensive resident state for the current user.

Plain file locks or SQLite leases are not enough. They are useful for discovery,
metadata, and model-store safety, but they cannot own a llama.cpp/OpenVINO
context, live KV cache, GPU residency, cancellation, or streaming decode.

## Source Facts

These facts drive the decision:

- VS Code does not imply one extension host. Official docs describe local, web,
  and remote extension hosts, with workspace extensions able to run where the
  workspace lives. Remote development can transparently run extensions remotely.
- Zed external agents are explicitly separate agent processes that usually own
  their own runtime, model selection, tools, and native configuration. ACP also
  defines local agents as editor subprocesses using JSON-RPC over stdio and
  remote agents over HTTP/WebSocket.
- Unix domain sockets are the standard local IPC primitive on Unix-like systems.
  Linux documents `AF_UNIX` / `AF_LOCAL` sockets as communication between
  processes on the same machine.
- Windows named pipes are the standard Windows IPC primitive for this shape.
  Microsoft documents named pipes as one-way or duplex IPC between one pipe
  server and one or more pipe clients, with separate pipe instances allowing
  multiple clients.
- `flock`-style file locks are advisory and attached to open file descriptors.
  They are good cooperation primitives for Contenox-owned processes, but they
  do not prevent non-cooperating processes from reading or writing files.
- SQLite is useful coordination storage. Its WAL mode allows readers and a
  writer to overlap, but SQLite still serializes writes: there can only be one
  writer at a time. That is fine for leases and metadata, not for native model
  session ownership.
- systemd and launchd both support on-demand/user-level service patterns. They
  are good later distribution options, but they are not required for the first
  implementation.

## The Actual Problem

The dangerous case is not just "two chat tabs." The dangerous case is two
independent Contenox processes believing they each own local state that should
really be singleton or serialized.

### 1. Resident model state is process-local

llama.cpp and OpenVINO sessions hold native resources:

```text
model handles
contexts / compiled models / pipelines
KV cache
tokenizer/runtime state
GPU/NPU/CPU memory residency
streaming decode state
cancel/error state
```

If two editor processes each start their own embedded runtime, they can both
load the same model, duplicate memory, evict each other from VRAM/unified
memory, and lose the whole point of warm prefix reuse. The model store can be
shared on disk; the live session cannot.

### 2. Workspace writes are more dangerous than duplicate inference

Two agents can race on:

```text
file edits
shell/test commands
git operations
package installs
generated artifacts
session transcripts
context manifests
```

Even if the model memory problem is solved, Contenox must avoid two write-capable
turns mutating the same workspace at the same time unless the user explicitly
chooses that risk.

### 3. Cache correctness depends on one authority

The context manifest work makes cache hits precise:

```text
model digest
tokenizer/template digest
runtime digest
segment byte hashes
segment token hashes
token ranges
stable-prefix boundary
```

That prevents false cache hits inside one runtime. It does not, by itself,
coordinate two processes. A process-local cache can only be trusted by the
process that owns it. Cross-process reuse requires a coordinator, snapshot files,
or a backend-supported shared cache. llama.cpp/OpenVINO do not give us a
finished cross-process workspace cache product today.

### 4. Model artifacts need publication safety

Model downloads/conversions must not expose half-written artifacts to another
process. The model store needs content-addressed immutable directories plus a
staging/publish protocol.

### 5. Editor topology is not fixed

VS Code can run extensions locally, remotely, or in a browser/remote split. Zed
ACP agents can be separate processes. The CLI can run outside either editor.
The runtime must discover "where the workspace and accelerator actually are"
instead of assuming the UI process and model process are the same process.

## Non-Goals

- Not a multi-user inference server.
- Not a cloud or LAN server.
- Not a required helper-model sidecar architecture.
- Not sharing raw KV memory across unrelated processes in the first pass.
- Not relying on OpenAI-compatible local tool calls as a coordination boundary.
- Not keeping old `local` / `localnode` compatibility surfaces as architecture.
  The user-facing embedded GGUF runtime remains `llama`; `local` is only a
  compatibility keyword shim to `llama`.

## Terms

```text
frontend
  VS Code extension, Zed ACP client, CLI, or another UI/client process.

runtime owner
  The Contenox process that owns native model sessions, live KV state, model
  loading/unloading, cancellation, metrics, and streaming decode.

workspace lease
  A local coordination record that grants read-only, write-turn, or exclusive
  access for one workspace and one client session.

model store lock
  A cooperative lock around staging/publishing model artifacts, not around every
  read of an immutable model.

session key
  Runtime identity for a live model session:
  workspace id + model digest + backend + runtime profile + manifest identity.
```

## Hard Invariants

These should become implementation rules:

1. A published model artifact is immutable.
2. Only one runtime owner process owns live native sessions for a given user and
   runtime data root, unless the user explicitly opts into isolation.
3. Frontends attach to the runtime owner when available. They do not create
   independent resident model sessions for the same profile by default.
4. Every write-capable tool turn requires a workspace write lease.
5. Read-only context assembly can run concurrently, but write turns and shell
   turns that can mutate the repo are serialized per workspace.
6. Cache reuse is allowed only when the context manifest is compatible with the
   runtime owner's profile and current session.
7. Stale owner/lease records must be recoverable by heartbeat, PID/process
   checks, socket liveness, and epoch numbers.
8. Cancellation has an explicit outcome: session remains valid, session is
   reset, or session is dead and must be rebuilt.

## Option A: Status Quo, Every Frontend Owns Its Runtime

```text
VS Code A -> embedded runtime A
VS Code B -> embedded runtime B
Zed      -> embedded runtime C
CLI      -> embedded runtime D
```

### Benefits

- Lowest implementation cost.
- Simple mental model during backend development.
- No IPC protocol or discovery layer.

### Costs

- Duplicates model memory and KV.
- Allows multiple warm caches that do not help each other.
- Can push unified-memory devices into swap when several models are resident.
- Makes cancellation and metrics process-local.
- Does not serialize workspace mutations.
- Turns "single-user local node" into accidental multi-process contention.

### Verdict

Keep only as a developer fallback and explicit isolation mode:

```bash
contenox --runtime-isolation=process
```

It should not be the default once llama/OpenVINO sessions become expensive and
long-lived.

## Option B: File Locks Only

Use lock files for:

```text
model downloads
workspace writes
maybe "active runtime" metadata
```

### Benefits

- Easy to implement.
- Works with simple CLI workflows.
- Good for model-store publication safety.

### Costs

- File locks are cooperative and advisory on Unix.
- Locks do not hold native model/KV state.
- Lock files alone do not provide request routing, streaming, cancellation, or
  cache ownership.
- Stale metadata still needs heartbeat/epoch recovery.
- Cross-platform semantics differ.

### Verdict

Use locks as supporting machinery. Do not use file locks as the local-node
architecture.

## Option C: SQLite Lease Registry Only

Use SQLite as the source of truth for:

```text
owner records
workspace leases
client sessions
model profiles
active turns
heartbeats
```

### Benefits

- Durable, queryable, and testable.
- WAL mode is a good fit for many readers and a small amount of serialized
  metadata writes.
- Easier crash inspection than lock files alone.

### Costs

- SQLite cannot own a live llama.cpp/OpenVINO context.
- A DB record saying "process X owns session Y" is useful only if there is also
  an IPC route to that process.
- Long transactions can create avoidable contention.

### Verdict

Use SQLite for metadata and leases, not as the owner of runtime state.

## Option E: Per-User Local Runtime Owner

One Contenox process owns local model sessions for the current user and data
root. All frontends attach to it:

```text
VS Code A --\
VS Code B ----> contenox node owner -> llama/OpenVINO sessions
Zed ACP  --/             |
CLI     --/              +-> workspace leases
```

The process can be started on demand by the first client:

```text
client checks run record
client probes socket
if alive: attach
if absent/stale: acquire startup lock and spawn owner
owner publishes socket + epoch + heartbeat
client sends requests over local IPC
```

### Benefits

- One resident model/profile per user by default.
- Warm prefix reuse survives editor window churn.
- Central place for memory budget, model eviction, cancellation, metrics, and
  context explanation.
- Workspace write leases can be enforced before tool execution.
- Works for VS Code, Zed ACP, CLI, and future IDE clients through the same local
  API.
- Keeps backend packages (`llama`, OpenVINO, ORT GenAI) behind one session
  contract.

### Costs

- Requires local IPC and a small node API.
- Needs robust discovery and stale-owner recovery.
- Requires cross-platform packaging decisions.
- A process crash affects all active local sessions for that user.
- Needs explicit remote-workspace behavior.

### Verdict

Recommended default. This is the cleanest way to make "one local node" real
without pretending the user only opens one editor.

## Option F: OS-Managed User Service

Package the local runtime owner as a user service:

```text
Linux:  systemd --user service/socket
macOS:  launchd user agent
Windows: user-started background process or service-style launcher
```

### Benefits

- Best lifecycle supervision.
- Can use socket activation on Linux/macOS-style systems.
- Avoids every editor implementing process supervision.

### Costs

- Packaging and install complexity jumps.
- Harder early development loop.
- User-service behavior differs significantly across OSes.
- Windows service/user-session behavior needs careful UX and permissions.

### Verdict

Good later distribution shape. Do not block the first coordination fix on it.
Build the owner so it can later run under systemd/launchd without changing the
protocol.

## Decision Matrix

| Path | Runtime correctness | Workspace safety | UX | Cost | Recommendation |
|---|---:|---:|---:|---:|---|
| A. Every frontend owns runtime | Low | Low | Medium | Low | Dev fallback only |
| B. File locks only | Low | Medium | Medium | Low | Supporting primitive |
| C. SQLite leases only | Low | Medium | Medium | Medium | Supporting primitive |
| E. Per-user local runtime owner | High | High | High | Medium-high | **Default path** |
| F. OS-managed user service | High | High | High | High | Later packaging |

## Recommended Architecture

```text
frontend clients
  VS Code extension
  Zed ACP process
  CLI
  future IDE adapters

local IPC
  Unix: AF_UNIX socket under XDG_RUNTIME_DIR or ~/.contenox/run
  Windows: named pipe with current-user ACL
  macOS: Unix socket; launchd user agent later

runtime owner
  process discovery
  client auth: same-user local only
  workspace lease manager
  model store manager
  session manager
  backend adapters
  telemetry and explain-context

storage
  immutable model store
  SQLite lease/session metadata
  context manifests and transcripts
```

### IPC Surface

Start small. The first protocol should carry runtime ownership and workspace
leases, not every product feature.

```text
node/hello
node/status
node/shutdown

workspace/open
workspace/lease_acquire
workspace/lease_renew
workspace/lease_release
workspace/active_turns

model/list
model/ensure
model/unload

session/open
session/close
session/generate
session/cancel
session/metrics
session/explain_context
```

The existing VS Code stdio bridge can remain a frontend protocol. Internally it
should call the local runtime owner instead of directly constructing resident
model sessions.

### Run Directory

Prefer OS runtime directories:

```text
Linux:
  $XDG_RUNTIME_DIR/contenox/node.json
  $XDG_RUNTIME_DIR/contenox/node.sock
  fallback: ~/.contenox/run/

macOS:
  ~/.contenox/run/node.json
  ~/.contenox/run/node.sock

Windows:
  %LOCALAPPDATA%\Contenox\run\node.json
  \\.\pipe\contenox-<user-hash>-node
```

`node.json` should be metadata, not the trust boundary:

```json
{
  "version": 1,
  "pid": 12345,
  "epoch": "2026-06-16T14:30:00Z-6f4f...",
  "socket": "/run/user/1000/contenox/node.sock",
  "data_root": "/home/naro/.contenox",
  "started_at": "2026-06-16T14:30:00Z",
  "last_heartbeat": "2026-06-16T14:31:05Z"
}
```

The client must still connect to the socket and perform `node/hello`. A stale
JSON file is not proof of a live owner.

### Startup Protocol

```text
1. Client reads run record.
2. Client probes socket/named pipe.
3. If owner responds with compatible data root and protocol version, attach.
4. If no owner responds, client takes startup lock.
5. Client rechecks, then starts `contenox node serve --owned`.
6. Owner opens IPC endpoint, writes run record with epoch, starts heartbeat.
7. Client attaches.
8. If startup fails, client falls back to explicit process-isolation mode only
   if the user or config allows it.
```

Use a startup lock to avoid two clients spawning owners at the same time. Use
socket liveness and epoch to avoid trusting stale files.

### Workspace Lease Model

Leases should be per workspace identity, not per path string only.

Workspace identity should include:

```text
canonical realpath
git root if present
remote authority / WSL / container identity if applicable
device/inode where available
data-root namespace
```

Lease modes:

```text
read
  context assembly, status, explain, model load, no mutation

write_turn
  one agent turn may edit files, run mutating tools, run tests, or change git

exclusive
  destructive operations, migrations, repo-wide rewrites, model/profile changes
  that invalidate active workspace sessions
```

Default policy:

```text
read leases can overlap
write_turn is one at a time per workspace
exclusive blocks everything else
lease TTL requires heartbeat renewal
lost heartbeat cancels or marks the turn abandoned
```

Contention behavior should be configurable:

```text
interactive default:
  tell the second client who owns the active turn and ask whether to queue,
  attach, cancel, or force after stale timeout

CLI default:
  fail fast unless --wait or --queue is passed
```

### Model Store Protocol

Use content-addressed immutable model directories:

```text
models/
  sha256-<digest>/
    model.gguf
    tokenizer.json
    manifest.json
  staging/
    <tmp-id>/
```

Publication:

```text
1. Acquire model-store publish lock.
2. Download/convert into staging.
3. Verify digest and metadata.
4. Atomically rename into content-addressed final directory.
5. Release lock.
```

Reads:

```text
published content-addressed models are immutable
readers never use staging paths
runtime sessions store the resolved digest, not just the path
mutable aliases point to immutable digests
```

This prevents half-written model files from being loaded by a second process.

### Session Routing

The runtime owner should route live sessions by an explicit key:

```text
data_root
workspace_id
backend: llama | openvino | ortgenai
model_digest
runtime_profile_digest
context_size / KV config / RoPE config
chat_template_digest
manifest stable-prefix identity
```

That gives the owner enough information to:

```text
reuse a compatible session
reject a false warm hit
reset a poisoned session
evict lower-priority sessions under memory pressure
explain why a session was reused or rebuilt
```

## Implementation Phases

### Phase C0: Decide Runtime Owner Policy

Make the policy explicit in docs/config:

```text
default: per-user local runtime owner
fallback: process isolation only by explicit config or dev flag
future: OS-managed user service packaging
```

Add a product note to `../local-coding-node-goals.md`:

```text
single-user does not mean single frontend process
```

### Phase C1: Model Store Safety

Implement immutable content-addressed model publication before relying on
multi-client runtime attachment.

Acceptance:

```text
two `model ensure` commands cannot expose a partial artifact
published model digest is stable
runtime opens only published immutable paths
```

### Phase C2: Owner Discovery and Local IPC

Add:

```text
contenox node serve
contenox node status --json
startup lock
run record
heartbeat
Unix socket / Windows named pipe transport
node/hello protocol
```

Acceptance:

```text
two simultaneous clients start exactly one owner
stale run record is recovered
protocol version mismatch fails clearly
same-user local-only access is enforced
```

### Phase C3: Workspace Leases

Add read/write/exclusive leases around tool execution and editing.

Acceptance:

```text
two read-only requests can overlap
two write turns in the same workspace cannot overlap by default
lease heartbeat expiry releases abandoned work safely
force takeover requires stale-owner proof or explicit user action
```

### Phase C4: Frontend Attachment

Change VS Code, Zed ACP, and CLI paths so they attach to the owner for local
model work.

Acceptance:

```text
two VS Code windows share one local runtime owner
Zed ACP and CLI can attach to the same owner
closing one frontend does not kill the runtime owner if other clients remain
closing the last client starts idle eviction/shutdown timer
```

### Phase C5: Session Manager Integration

Move llama/OpenVINO resident sessions behind the owner.

Acceptance:

```text
one model/profile loads once per owner
warm prefix reuse survives frontend window churn
session metrics are visible from any client
cancel from one client has a defined effect on that client's active turn
```

### Phase C6: Failure Recovery

Add structured failure handling:

```text
owner crash during decode
client crash during approval
client crash during write turn
cancel during prefill
cancel during decode
model load OOM
workspace lease timeout
```

Acceptance:

```text
no permanent stuck lease
no silent second writer
no client sees stale owner as healthy
session validity after cancel/crash is explicit
```

### Phase C7: OS Service Packaging

After the on-demand owner is stable:

```text
Linux: optional systemd --user socket/service
macOS: optional launchd user agent
Windows: optional tray/user-session launcher or service-style install
```

This is packaging. The protocol should already work before this phase.

## Immediate Decisions Needed

### Decision 1: Default Owner Scope

Recommended:

```text
one owner per user + data root
```

Why: best model residency and cross-editor behavior.

Alternative:

```text
one owner per workspace
```

Why not default: duplicates model memory across workspaces and makes ownership
feel arbitrary when the first owner is an editor window.

### Decision 2: Write Turn Contention

Recommended interactive default:

```text
queue or attach after showing active owner/client/session
```

Recommended CLI default:

```text
fail fast unless --wait is supplied
```

### Decision 3: Owner Lifetime

Recommended:

```text
on-demand start
idle shutdown after configurable timeout
model unload under memory pressure before process shutdown
```

Do not make it always-on until service packaging exists.

### Decision 4: Process Isolation Escape Hatch

Recommended:

```text
hidden/dev config and explicit CLI flag only
```

Process isolation is useful for debugging a backend. It should not be normal UX
because it breaks the local-node memory and cache story.

## What Is Out

These are explicitly out for this decision:

```text
sidecar helper models
multi-node Pi + Jetson orchestration
remote LAN inference server
cross-process shared raw KV memory
generic multi-user scheduling
keeping `local` and `localnode` as compatibility surfaces
```

These are in:

```text
one local runtime owner process
local IPC
model-store publication safety
workspace write leases
backend-neutral session manager
frontend attachment from VS Code/Zed/CLI
```

The owner process may look like a daemon later, but architecturally it is the
Contenox local runtime itself, not a helper sidecar.

## Risks And Mitigations

| Risk | Why it matters | Mitigation |
|---|---|---|
| Split-brain owners | Two owners duplicate models and both accept writes | startup lock, socket probe, epoch, heartbeat, owner self-check |
| Stale run file | Client attaches to dead metadata | always validate socket `node/hello`; never trust JSON alone |
| Stale workspace lease | User cannot continue after crash | TTL + heartbeat + explicit stale takeover |
| Socket path too long | Unix socket paths have length limits | prefer `$XDG_RUNTIME_DIR`; hash long data-root paths |
| Windows pipe exposure | Named pipes can be remote unless secured | current-user ACL; deny network access for local-only pipe |
| Remote workspace ambiguity | UI and workspace may be on different machines | workspace host owns the runtime; UI attaches through bridge |
| Network filesystem locks | Lock semantics can be weak or surprising | keep run/lease DB on local data root, not repo NFS mount |
| Owner crash loses warm KV | Live cache disappears | rebuild from manifests; snapshots optional later |
| Long decode blocks writes | User wants another action during generation | per-turn cancel; queue policy; read-only operations may continue |
| Memory pressure | One owner may keep too much resident | session LRU by profile priority, explicit unload, metrics |

## Minimal First Implementation

The smallest useful slice is:

```text
1. `contenox node serve` with Unix socket on Linux/macOS.
2. `contenox node status --json`.
3. run record + startup lock + heartbeat.
4. one RPC: `node/hello`.
5. one RPC: `workspace/lease_acquire` / `lease_release`.
6. model-store publish lock and immutable final paths.
7. CLI smoke test that starts two clients and proves one owner.
```

Do not start by moving every chat request through IPC. First prove ownership,
discovery, and lease semantics. Then put llama/OpenVINO sessions behind it.

## Acceptance Tests

```text
owner_start_race:
  start 20 clients concurrently
  exactly one owner survives
  all clients attach to the same epoch

stale_owner_recovery:
  write fake node.json
  no socket responds
  client starts a new owner and replaces metadata

workspace_write_serialization:
  client A acquires write_turn
  client B write_turn fails or queues
  read lease still succeeds if policy allows

client_crash_during_write:
  client A exits without release
  lease remains until TTL
  stale takeover works after heartbeat expiry

model_publish_race:
  two ensure operations for same model
  one publishes final digest
  neither runtime loads staging

frontend_window_churn:
  VS Code A starts owner and model session
  VS Code B attaches
  close A
  B can continue using owner/session

backend_cancel_state:
  cancel during prefill/decode
  owner reports session_valid | session_reset | session_dead
```

## Recommended Decision

Adopt **Option E: per-user local runtime owner**, implemented on demand and
backed by model-store locks plus SQLite lease metadata.

Use **Option F: OS-managed user service** later, after the protocol and owner
lifecycle work on demand.

Do not ship the graduated llama/OpenVINO local node with Option A as default.
That would make the cache/session work correct only in the narrow case where the
user happens to open one editor window.

## References

- VS Code Extension Host configurations:
  https://code.visualstudio.com/api/advanced-topics/extension-host
- VS Code Remote Development extension architecture:
  https://code.visualstudio.com/api/advanced-topics/remote-extensions
- Zed External Agents:
  https://zed.dev/docs/ai/external-agents
- Agent Client Protocol introduction:
  https://agentclientprotocol.com/get-started/introduction
- Linux `flock(2)` advisory locks:
  https://man7.org/linux/man-pages/man2/flock.2.html
- Linux `unix(7)` Unix domain sockets:
  https://man7.org/linux/man-pages/man7/unix.7.html
- Microsoft Named Pipes:
  https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipes
- SQLite WAL:
  https://sqlite.org/wal.html
- SQLite Isolation:
  https://sqlite.org/isolation.html
- systemd socket units:
  https://www.freedesktop.org/software/systemd/man/systemd.socket.html
- Apple launchd daemons and agents:
  https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html
