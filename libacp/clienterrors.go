package libacp

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"syscall"
)

// Client-side failure sentinels for a consumer that drives an agent through a
// ClientSideConnection (and, typically, an acpexec subprocess). They exist so
// a driver can classify failures — see IsStartupError / IsTimeoutError /
// IsRetryableError — instead of string-matching, the lesson hash learned the
// hard way (tmp/hash/internal/agent/errors.go). libacp itself never returns
// these from its wire methods; they are the vocabulary a driver wraps its own
// transport/lifecycle failures in.
var (
	// ErrAgentStartFailed marks a failure to launch or initialize the agent
	// subprocess. It is a startup error (IsStartupError): a bad binary path or
	// broken agent build will not fix itself on retry, so a supervisor must
	// surface it rather than loop.
	ErrAgentStartFailed = errors.New("libacp: agent start failed")

	// ErrIdleTimeout marks a turn that went silent — no session/update and no
	// result — past a driver's idle deadline. Distinct from an overall
	// context deadline so a driver can reset an idle watchdog on every received
	// message (hash's pattern) rather than cap total turn time.
	ErrIdleTimeout = errors.New("libacp: agent idle timeout")

	// ErrNoDisplayableOutput marks a prompt turn that ended with a normal stop
	// reason but never produced a renderable agent message — the client-side
	// mirror of the agent-side empty-response bug this repo just fixed. A driver
	// gets an explicit, observable failure class instead of silently showing an
	// empty answer (hash's noOutputPromptError, acp.go:979). Use TurnTracker to
	// detect it over a turn's session/update stream.
	ErrNoDisplayableOutput = errors.New("libacp: prompt turn produced no displayable output")
)

// IsStartupError reports whether err indicates the agent could not be started
// or is unusable as configured — conditions a retry cannot cure. Adopts hash's
// classification (errors.go:26): a missing binary or a marked start failure is
// terminal, not transient.
func IsStartupError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrAgentStartFailed) || errors.Is(err, exec.ErrNotFound)
}

// IsTimeoutError reports whether err indicates a turn ran out of time — either
// a context deadline or an idle-watchdog trip. Split from IsRetryableError so a
// driver can, like hash (errors.go:36), treat "slow/stuck" differently from a
// hard protocol error.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrIdleTimeout)
}

// IsRetryableError reports whether retrying the turn (typically after respawning
// the agent) might succeed. Mirrors hash's taxonomy (errors.go:44): an explicit
// cancellation and startup failures are never retryable; timeouts, a dropped
// transport (ErrConnectionClosed / EOF / closed pipe / EPIPE / ECONNRESET) and
// an empty turn are. The trailing string match is a cross-platform safety net
// for transport errors that do not wrap into a recognizable sentinel.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || IsStartupError(err) {
		return false
	}
	if IsTimeoutError(err) {
		return true
	}
	if errors.Is(err, ErrConnectionClosed) ||
		errors.Is(err, ErrNoDisplayableOutput) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "file already closed")
}
