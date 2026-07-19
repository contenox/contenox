// Package libprocess supervises an external OS process: it starts a command,
// watches for it to exit, and — per a configurable restart policy — starts it
// again.
//
// It is a thin layer over os/exec and deliberately stops there: no PTY, no
// output multiplexing to multiple viewers, no resource-limit enforcement.
//
// A supervised subprocess comes in two shapes and the package supports both.
// A job whose output is merely collected wires Config.Stdout/Stderr/Stdin as
// it would with exec.Cmd. A protocol peer that must be *talked to* — a
// JSON-RPC agent over stdio — sets Config.PipeStdio, and the supervisor
// claims the pipes and hands them back as one io.ReadWriteCloser
// (Process.Stdio). Without that second mode a supervisor can only ever be
// composed *around* a transport somebody else owns, which means two pieces of
// code both believe they own the subprocess's lifetime — and the one that
// calls cmd.Wait wins, because Wait closes the pipes out from under the other.
//
// The pieces exist because supervising a real-world subprocess is mostly
// handling the ways it refuses to die or refuses to be worth reviving:
//
//   - The restart policy covers auto-restart, a "compulsory" mode that
//     restarts regardless of exit code, a consecutive-restart limit, a
//     backoff function, and an injectable classifier for callers whose notion
//     of "worth retrying" is protocol-level rather than exit-code-level. A
//     failure to *start* is never retried at all: a retry cannot cure a bad
//     binary, and looping on one only hides the misconfiguration.
//   - Stop asks the command to shut down through an injectable strategy
//     (signal the process group by default, close stdin for a stdio peer),
//     escalates to killing the whole group after a grace period, and bounds
//     even that: a descendant that escaped the group can hold the pipes and
//     keep Wait from returning, and a loud error beats blocking a caller
//     forever on a process nobody can reach.
//   - Cancelling the context passed to Start runs that same shutdown, so a
//     dead context cannot leave a live subprocess behind.
//   - A state-change hook, an error handler, and an ActivityTracker cover
//     observability. An optional injected Lock gates supervision so a command
//     that must be a singleton across a cluster stays one — see WithLock.
//
// What it deliberately does not cover — PTY allocation, multiplexing output to
// several viewers, cgroup resource limiting, CPU/memory sampling — is a
// different concern with a different lifetime, and is implemented separately:
// runtime/shellsession keeps a persistent PTY-backed shell that outlives the
// individual commands submitted to it, retains their output in a bounded
// scrollback ring, and fans it out to subscribers. A supervisor's unit is one
// command's lifetime; a shared terminal's is the session's, and a consumer that
// wants one command's slice of that scrollback has to frame it out itself.
// Folding the two together would make every supervised job carry a
// pseudo-terminal, a scrollback ring, and a subscriber fan-out it has no use
// for, and would leave neither lifetime clearly owned.
//
// Every seam that reaches outside the package takes a context and returns an
// error (Lock, the state hook, Stop, New), so implementations can grow
// timeouts, cancellation, and failure reporting without a signature change.
// Errors raised inside the supervisor's own goroutines — a failed respawn, a
// lock that cannot be renewed — have no caller to return to, so they go to
// the handler registered by WithErrorHandler rather than being swallowed.
package libprocess
