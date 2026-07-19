package libacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternalError  = -32603

	ErrAuthRequired = -32000
	// ErrRequestTimeout is the wire signal that a peer's handler ran out of
	// time. It exists because a Go sentinel cannot cross the JSON-RPC boundary:
	// a remote peer sees only code/message/data, so unless the deadline is
	// encoded in the code, the receiving side has no way to tell a transient
	// timeout from a permanent internal failure and a supervisor gives up on a
	// restartable agent. Matches the number MCP implementations use for the
	// same condition.
	ErrRequestTimeout   = -32001
	ErrResourceNotFound = -32002
)

// Error is a JSON-RPC error object. The exported fields are the entire wire
// contract; cause is process-local and never serialized, so an Error that has
// actually travelled over a transport carries only what its code and message
// can express.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`

	// cause is the handler error this Error was built from, kept so that
	// errors.Is/As still work while the value has not left the process.
	cause error
}

// Unwrap exposes the originating handler error, but only for an Error still in
// the process that created it — an Error decoded from the wire has no cause and
// returns nil. Callers that must classify a remote failure have to read Code
// (see IsTimeoutError, which accepts ErrRequestTimeout for exactly this
// reason).
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil libacp.Error>"
	}
	return fmt.Sprintf("libacp: rpc error %d: %s", e.Code, e.Message)
}

func NewError(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

func NewErrorf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func ParseError(msg string) *Error     { return NewError(ErrParseError, msg) }
func InvalidRequest(msg string) *Error { return NewError(ErrInvalidRequest, msg) }
func MethodNotFound(method string) *Error {
	return NewError(ErrMethodNotFound, "method not found: "+method)
}
func InvalidParams(msg string) *Error { return NewError(ErrInvalidParams, msg) }
func InternalError(msg string) *Error { return NewError(ErrInternalError, msg) }

// AsError converts a handler error into the JSON-RPC error that goes on the
// wire. It retains err as the cause so a same-process caller can still match
// sentinels, and it promotes a deadline to ErrRequestTimeout so that a *remote*
// caller — for whom the cause is unrecoverable by construction — can still tell
// "too slow, retry" from "broken, give up". Everything else stays an internal
// error: guessing a more specific code from an opaque failure would be worse
// than admitting we do not know.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	code := ErrInternalError
	if errors.Is(err, context.DeadlineExceeded) {
		code = ErrRequestTimeout
	}
	return &Error{Code: code, Message: err.Error(), cause: err}
}

// HandlerDrainTimeout bounds how long Run waits, after shutdown has cancelled
// everything, for in-flight handler goroutines to return. It is a backstop for
// a handler that ignores its cancelled context, not a normal budget: a
// well-behaved handler returns immediately once cancelled, so this timer
// should never fire in practice.
const HandlerDrainTimeout = 10 * time.Second

// ErrHandlerDrainTimeout reports that Run gave up waiting for handler
// goroutines to return (see HandlerDrainTimeout). It means some handler
// ignored its cancelled context and MAY STILL BE RUNNING, so a caller must
// treat its own teardown as unsafe: the state those handlers touch (sessions,
// drivers, database handles) is not yet free to release.
var ErrHandlerDrainTimeout = errors.New("libacp: timed out waiting for handler goroutines to return")
