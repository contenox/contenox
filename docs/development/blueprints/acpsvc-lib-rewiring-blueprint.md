# Blueprint: acpsvc → lib\* Rewiring

Owner: runtime

Status (2026-07-19): Phases 0–2 are implemented in the working tree
(uncommitted). Verified: repo builds/vets clean, full unit suite green,
`acp-validator` conformance 12/12, `agenthost` self-loopback e2e green,
lib-import invariant holds. Phase 3 not started. Line citations in the
body were written against the pre-change tree and have drifted where noted.

Purpose: move the generic infrastructure currently living in `runtime/acpsvc`
into the `lib*` packages, so that protocol semantics learned once are encoded
once — and so the three independent supervision loops in this repo converge on
one. The goal is not tidiness. It is that a second consumer of ACP (a second
driver, a second harness, an external user of `libacp`) should not have to
re-derive what acpsvc already knows.

## Core Rule

Code belongs in a `lib*` package only if it would be correct and useful to a
caller with **zero** knowledge of contenox's runtime. Everything else stays in
the service, no matter how generic it looks. A wrong promotion is worse than a
missed one: it drags service policy into a package that other code is entitled
to treat as timeless.

The corollary, which governs the whole rewire: **no `lib*` package may import
`runtime/`.** Today that holds — the lib import graph is `lib*` → `lib*` only
(`libacp`, `libtracker`, `libbus`, `libprocess`, `libdbexec`). Every step below
preserves it.

## Verified Current State

Established by reading the code, not inferred from names. Two widely-assumed
facts are false and are recorded here so the rewire is not planned around them:

- **acpsvc contains no process spawning.** `exec.Command` does not appear in
  any non-test file under `runtime/acpsvc`. Spawning is delegated
  (`external.go:1455` → `runtime/agenthost` → `libacp/acpexec.Spawn`).
- **acpsvc contains no JSON-RPC framing.** No `bufio`, `Scanner`,
  `json.NewDecoder/Encoder`, or ndjson handling in any non-test file. Framing
  is entirely `libacp/conn.go` + `libacp/ndjson.go`.

So the rewire is *not* "extract a duplicated transport from acpsvc". It is
narrower on the protocol side and deeper on the supervision side.

### Three supervision loops, three different clocks

| Loop | Waits on | Lifetime | States |
|---|---|---|---|
| `libprocess.Process.watch` | `cmd.Wait()` | one OS process | `Stopped/Starting/Running/Crashed` |
| `acpexec.Supervisor.Serve` (`supervisor.go:60`) | one `session()` call returning | one session, blocking, retry-until-success | none (returns an error) |
| `agentinstance.instance.watchDog` (`instance.go:248`) | `<-h.Conn.Closed()` | long-lived instance, re-arms per restart | `Starting/Running/Stopped/Error/Warning` |

All three implement "wait for death → decide whether this was intentional →
apply a restart budget → respawn". They differ in *what death means*, and that
is the whole problem:

- `libprocess` observes **process** death.
- The other two observe **connection** death — which is what actually matters
  for an ACP agent, because a subprocess whose JSON-RPC connection has died is
  useless even while its PID is alive.

**This is why `libprocess` has zero consumers.** Not because acpsvc hoards a
copy, but because `libprocess.Config` (`Command/Args/Dir/Env/Stdout/Stderr/
Stdin`) cannot express "restart this and re-attach a protocol client to it".
`agentinstance` also carries `StateWarning` ("restart budget exhausted"), which
`libprocess` collapses into `Crashed`.

Consolidation therefore requires a change **to `libprocess`**, not a move out of
acpsvc. See Phase 3.

## Promotion Ledger

### PROMOTE

**P1 — terminal driver loop → `libacp`.** `runtime/acpsvc/commandrunner.go:30-160`.
DONE — landed as `libacp.RunTerminal` (`libacp/terminalrun.go`); commandrunner
is now the adapter described below.

The highest-value item, because it is hard-won protocol semantics that `libacp`
does not encode anywhere: `libacp/terminal.go` contains **zero functions** — it
is pure wire structs. Every consumer wanting to run a command through a client's
terminal capability must currently re-derive that `WaitForTerminalExit` needs a
kill-on-cancel companion; that the output fetch must use a **detached** context
or you lose the output you just paid for; that release needs a deferred detached
context; that the exit code lives in two places (`exitResp.ExitCode` falling back
to `outputResp.ExitStatus.ExitCode`); and that a deadline and a cancellation must
be told apart, because reporting user cancellation as "timeout" gives the model a
false causal story (`commandrunner.go:88-92`).

Target shape:

```go
type TerminalPeer interface { // *AgentSideConnection already satisfies this
    CreateTerminal(context.Context, CreateTerminalRequest) (CreateTerminalResponse, error)
    TerminalOutput(context.Context, TerminalOutputRequest) (TerminalOutputResponse, error)
    WaitForTerminalExit(context.Context, WaitForTerminalExitRequest) (WaitForTerminalExitResponse, error)
    KillTerminal(context.Context, KillTerminalRequest) (KillTerminalResponse, error)
    ReleaseTerminal(context.Context, ReleaseTerminalRequest) (ReleaseTerminalResponse, error)
}

type TerminalResult struct {
    Output    string
    Truncated bool
    ExitCode  int
    Signal    *string
    Cancelled bool // ctx cancelled; the terminal was killed
    TimedOut  bool // ctx deadline; the terminal was killed
}

func RunTerminal(ctx context.Context, p TerminalPeer, req CreateTerminalRequest,
                 onCreated func(terminalID string)) (TerminalResult, error)
```

`onCreated` is the seam for surfacing the terminal to a UI. Stays in acpsvc: the
`getClientCaps().Terminal` gate and OS fallback (`:35-37`), shell wrapping in
`terminalCommand` (`:162-176`), cwd resolution from `t.sessions` (`:55-67`), the
`terminalAttachNotification` side effect, `Truncated` →
`localtools.ErrOutputBudgetExceeded`, the `\n\n\n` trim and `[command killed: …]`
banners (beam presentation policy). Roughly a 40-line adapter.

**P2 — `flattenPromptBlocks` → `libacp.FlattenContent`.**
`runtime/acpsvc/content.go:9-58`. DONE — landed as `libacp/flatten.go`;
`content.go` is deleted. Imports were exactly `strings` and `libacp`;
zero runtime coupling, verified. `libacp/content.go` already hosts the
constructor half (`NewTextContent`, `NewResourceLink`); this is the missing
inverse. The `dropped []string` return is deliberate and reusable — it lets a
caller warn rather than silently lose an image.

Promote it as a documented **lossy text projection**, not as canonical: the
newline join and `name: uri` rendering are *a* policy, not *the* policy.

**P3 — `mapACPNotExist` → `libacp` client errors.**
`runtime/acpsvc/fileio.go:63-79`. DONE — landed as `libacp/notfound.go`
(`AsNotExist`/`IsNotFound`); the narrowed string shim stayed behind at
`fileio.go:70-81`, typed-error-exempt as prescribed. Zero runtime coupling,
verified.
`libacp/clienterrors.go` is exactly the right home — its stated job is
classification instead of string-matching (`IsStartupError`, `IsTimeoutError`,
`IsRetryableError`). A missing path is the most common ACP filesystem error
there is.

Promote the **typed** branch only. The raw-string fallback (`fileio.go:75-77`)
matches any error whose text contains "not found" — including an
agent-not-found startup error — and must not become library behaviour. Keep it
behind in acpsvc as a compat shim if it is still needed.

### KEEP — service policy, do not move

- **`external_terminal.go` marker framing** (`:83-215`, `:483-543`). The most
  tempting-looking promotion in the package and the most wrong. Every clever
  part — marker nonces, the `%d`-vs-digit trick that stops the echoed input line
  from matching, the `panelGuard` partial-marker holdback, Ctrl-C instead of
  kill — exists *only* because `shellsession.Manager` provides one shared
  line-oriented PTY with no per-command exit code. A generic library would spawn
  a real subprocess per terminal and have none of these problems. Hard-coupled to
  `shellsession.Manager`, `runtimetypes.SessionIDContextKey`, `t.deps.*`.
- **`terminal.go`** — the `contenox.terminalOutput` `_meta` extension and
  `_contenox/terminal/run` method. Proprietary protocol extension by construction.
- **`transport.go` `Deps`** — carries `*enginesvc.Engine`, `libdb.DBManager`,
  `*ChainRegistry`, `*vfs.Factory`, `shellsession.Manager`, `agentinstance.Manager`.
  This is the service costume itself, not infrastructure wearing one.
- **`external.go` KV persistence** (`:188-410`) and **`fileio.go` cwd resolvers**
  (`:83-142`) — `runtimetypes.Store`, raw SQL against `message_indices`.

## Blocking Prerequisite — FIXED (Phase 0.1)

Resolved in the working tree: `*Error` carries an `Unwrap`-able cause,
`AsError` promotes `context.DeadlineExceeded` to the `ErrRequestTimeout`
code (the part that survives the wire), and `libacp/errorcause_test.go`
pins the full encode→decode→classify round trip. Original analysis kept
below for the record.

**The wire boundary destroys error identity, which defeats `libacp`'s own retry
classification.** `libacp/errors.go:48-56` (`AsError`) flattens any non-`*Error`
handler error into `InternalError(err.Error())`. `*Error` implements only
`Error() string` — no `Unwrap`/`Is` — so `context.DeadlineExceeded`, `io.EOF`,
and friends are unrecoverable once they cross the wire. `IsTimeoutError` /
`IsRetryableError` (`clienterrors.go:45-93`) are built on `errors.Is`.

Consequence: an agent handler that returns `ctx.Err()` on its own deadline comes
back to the client as an opaque `InternalError`, so `IsRetryableError` returns
false, so `acpexec.Supervisor.Serve` (`supervisor.go:85`) treats a **transient
timeout as fatal and permanently gives up**. `clienterrors_test.go:65` asserts
the opposite is intended, but only ever tests the raw sentinel — never the
round-tripped wire form that real callers receive.

This must be fixed **before** Phase 3. Supervision convergence consists of
deciding when to restart, and that decision reads this classification.

Fix direction: give `*Error` an `Unwrap`, or carry a structured cause across the
boundary, so that classification survives the round trip. Pin it with a test
that builds the error via `AsError` and *then* classifies it.

## Phases

Sequenced so that each phase is independently landable and reversible.

### Phase 0 — Prerequisites in `libacp` — DONE

1. Wire-boundary error classification (above). **Blocks Phase 3.**
2. `Run()`/`shutdown()` do not join spawned handler goroutines
   (`conn.go:246-254`, `clientconn.go:210-221` — bare `go func()`, no
   `WaitGroup`). `shutdown` only cancels contexts. Real impact:
   `runtime/contenoxcli/acp_cmd.go:321-330` calls `transport.Close()` immediately
   after `Run` returns, while a handler may still be touching `Transport` state.
   Add a `WaitGroup` and join before `Run` returns. **Blocks nothing, but it is a
   live race — do it first.**

Exit criterion: a test that round-trips a `context.DeadlineExceeded` through
`AsError` and asserts `IsRetryableError` is true; a test asserting no handler
goroutine outlives `Run`.

### Phase 1 — Zero-risk promotions — DONE

P2 (`FlattenContent`) and P3 (`AsNotExist`). Pure moves with zero runtime
coupling; acpsvc call sites (`prompt.go:86`, `external.go:1763`, `fileio.go:63-81`)
became one-line delegations. No behaviour change. Land these first to establish
the pattern cheaply.

### Phase 2 — Terminal driver promotion — DONE

P1 (`RunTerminal`). Behaviour-preserving: acpsvc keeps the gate, the shell
wrapping, the cwd lookup, the attach notification, and the presentation policy.
Verify against the existing conformance harnesses (`testy` / `acp-validator`)
before and after — the wire trace must be identical.

### Phase 3 — Supervision convergence — UNPARKED, not started

The real work, and the reason the earlier phases exist.

It was parked while the agent-manager migration was mid-flight: `6d16ce9`
removed `Conn` from `agentinstance.Manager` ("a consumer DRIVES the downstream
via these methods, holding no connection") but left `runtime/acpsvc` calling
it, so HEAD did not compile. That migration is now finished — acpsvc drives
through `OpenSession`/`Prompt`/`Cancel`/`SetConfigOption`, holds no downstream
connection on the Instances path, and the whole repo builds.

The touchpoints below were written against the pre-migration `agentinstance`
and must be re-read before starting. In particular the driver's attached
sentinel is now `bridge != nil` rather than `conn != nil`, and the connCtx
fallback path is the only remaining holder of a raw
`*libacp.ClientSideConnection`.

`libprocess` cannot absorb the other two loops as it stands, because it models
process lifetime and they model connection lifetime. Close that with a
**liveness seam** rather than by teaching `libprocess` about ACP:

```go
// Liveness reports when a supervised instance should be considered dead, for
// callers whose notion of death is not "the process exited" — an agent whose
// JSON-RPC connection has closed is dead even while its PID is alive.
type Liveness interface {
    Dead(ctx context.Context) <-chan struct{}
    Err() error
}
```

Supervision then waits on `min(process exit, liveness death)`. `agentinstance`
supplies a `Liveness` backed by `h.Conn.Closed()`; the plain process case keeps
today's behaviour with a nil seam.

Also required before `agentinstance` can sit on `libprocess`:

- **A distinct "restart budget exhausted" outcome.** `agentinstance` has
  `StateWarning`; `libprocess` collapses it into `Crashed`. Either add the state
  or make `Crashed` carry a typed reason. Do not silently lose the distinction —
  it is the difference between "this is broken" and "this gave up retrying".
- **A re-attach hook.** A restart must be able to re-run
  `spawner.Connect(rootCtx, harness)`; a fresh subprocess needs re-`Initialize`.
- **Honest documentation of what a restart costs.** `instance.go:244-247` records
  it: a restart loses the downstream agent's conversation context while viewers
  and the journal survive, so they now describe a conversation the new process
  has never heard of. Any consolidated API must keep saying this.

`acpexec.Supervisor.Serve` is a *different* shape — blocking, session-scoped,
returns on success — and should **not** be forced into the same type. Converge it
on the shared restart-classification policy only.

`acpexec.Process.Close` (`acpexec.go:141-192`) and `libprocess.Process.Stop`
duplicate intent but differ where it matters: acpexec closes **stdin** as the
graceful signal, which is correct for a stdio JSON-RPC peer, whereas
`libprocess` sends SIGINT to the process group. Make the graceful signal
injectable rather than picking one.

### Phase 4 — Delete the duplicates

Only once Phase 3's seam is proven by both consumers. A consolidation that
leaves all three loops in place has made things worse, not better.

## Invariants

Hold these throughout; each is a review gate, not an aspiration.

- No `lib*` package imports `runtime/`. Currently true; verify per PR.
- Every seam reaching outside a lib package takes a `context.Context` and
  returns an `error`, so implementations can grow timeouts, cancellation, and
  failure reporting without a signature change.
- Errors raised inside a library's own goroutines — where no caller remains to
  return them to — go to an injected error handler, never to a swallow. This is
  the established pattern in `libprocess` (`WithErrorHandler`).
- Promoted code carries its tests with it, and the acpsvc call site keeps an
  integration test proving the delegation is behaviour-preserving.
- The `testy` / `acp-validator` conformance harnesses pass before and after every
  phase. A promotion that changes the wire trace is a defect.
- Presentation policy (trailing-newline trims, `[command killed: …]` banners,
  `\x1b[2K\r` erase sequences, `$ <command>` headers) never crosses into a lib.

## Current libprocess Readiness

Work already landed, so Phase 3 starts from a known base. `libprocess` had no
consumers, so its API was still free; it now has ctx/error on every outward
seam, construction-time validation, an injected `Lock` (with a `liblease`
adapter) for single-supervisor-across-a-cluster, `WithErrorHandler`, and
process-group kill so wrapper commands (`npx`, `uvx`, `sh -c`) do not leak
grandchildren.

Four defects were fixed there, each proven with a failing repro first: `Stop`
hanging forever and orphaning a process during a restart delay; the restart
counter never resetting on `Start`; grandchildren surviving `Stop`; and `done`
closing before the terminal state was published.

Update (2026-07-19): the working tree closed this gap ahead of the schedule
stated below — cancelling `Start`'s context now runs a full graceful `Stop`
(pinned by `TestUnit_Process_ContextCancellationShutsTheProcessDown`). The
ownership decision this paragraph deferred to Phase 3 has therefore
effectively been made; ratify or revert it there. Original text: `Start(ctx)`'s
context governs only the spawn and the restart delay — cancelling it does not
terminate a running process. Decide this when Phase 3 fixes the ownership
model, not before.

## Acceptance

The rewire is done when:

1. MET — `libacp` encodes the terminal-driver semantics, content flattening,
   and not-found classification, each with tests, and acpsvc's copies are
   delegations.
2. OPEN (Phase 3) — Exactly one supervision implementation decides "wait for
   death → intentional? → restart budget → respawn", parameterised by what
   death means, and both `agentinstance` and the plain-process case sit on it.
3. MET — Error classification survives the wire boundary, pinned by a
   round-trip test.
4. MET — No `lib*` package imports `runtime/`.
5. MET for `acp-validator` (12/12) and the wire e2e suites; the `testy`
   harness has not been run against this tree (binary not built locally).

Anything less than 2 is a partial credit that leaves the duplication in place —
and duplication in restart logic is how a crash-looping agent becomes an
outage.
