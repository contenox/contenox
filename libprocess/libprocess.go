// Package libprocess supervises an external OS process: it starts a command,
// watches for it to exit, and — per a configurable restart policy — starts it
// again.
//
// It is a thin layer over os/exec and deliberately stops there: no PTY, no
// output multiplexing to multiple viewers, no resource-limit enforcement.
// Callers wire Stdout/Stderr/Stdin themselves via Config, exactly as they
// would with exec.Cmd, and compose libprocess with whatever transport a
// specific subprocess needs (e.g. libacp/acpexec for an ACP agent's stdio
// JSON-RPC channel) rather than libprocess owning that concern itself.
//
// The restart policy covers auto-restart, a "compulsory" mode that restarts
// regardless of exit code, and a consecutive-restart limit; graceful Stop
// signals the process group and only kills it after a grace period; a
// state-change hook and an error handler cover observability. An optional
// injected Lock gates supervision so a command that must be a singleton
// across a cluster stays one — see WithLock. What it deliberately does not
// cover — PTY/terminal multiplexing, cgroup resource limiting, CPU/memory
// sampling — belongs to an interactive terminal use case that contenox's
// headless subprocess agents don't have.
//
// Every seam that reaches outside the package takes a context and returns an
// error (Lock, the state hook, Stop, New), so implementations can grow
// timeouts, cancellation, and failure reporting without a signature change.
// Errors raised inside the supervisor's own goroutines — a failed respawn, a
// lock that cannot be renewed — have no caller to return to, so they go to
// the handler registered by WithErrorHandler rather than being swallowed.
package libprocess
