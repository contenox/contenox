package libacp

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// IsNotFound reports whether err is a peer's answer of "that resource does not
// exist" — the file/resource sense of missing, not the lifecycle sense. It
// belongs to the same family as IsStartupError / IsTimeoutError / IsRetryableError
// (clienterrors.go): classify, do not string-match at the call site.
//
// Only a *typed* *Error counts. That restriction is the whole point: a driver
// that matched "not found" against a raw error string would also swallow
// "acp agent not found" or exec.ErrNotFound — a startup failure — and report a
// dead binary as a missing file. A raw error is therefore never classified here,
// however suggestive its text.
//
// Within a typed *Error the canonical signal is Code == ErrResourceNotFound. The
// message-substring check the acpsvc original also performed is kept, because in
// practice agents answer fs/read_text_file with a generic ErrInternalError whose
// message is just the underlying "open /tmp/x: not found" — but it is narrowed:
// it is only consulted for codes that describe the request's *subject*. The
// protocol-level codes (parse/invalid request/method not found/invalid
// params/auth required) describe the request *itself*, and applying the check to
// them mis-reads "method not found: fs/read_text_file" — an unimplemented
// capability — as a missing file. ErrRequestTimeout is excluded for the same
// reason: a timeout already says what went wrong, and reading a missing file
// out of its message would both lie about the cause and lose the retryability
// that IsRetryableError grants it. That is the line: message sniffing is
// acceptable inside a request-scoped protocol error whose code leaves the
// subject open, never on a code that already says what went wrong, and never on
// an arbitrary Go error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var e *Error
	if !errors.As(err, &e) || e == nil {
		return false
	}
	if e.Code == ErrResourceNotFound {
		return true
	}
	switch e.Code {
	case ErrParseError, ErrInvalidRequest, ErrMethodNotFound, ErrInvalidParams, ErrAuthRequired, ErrRequestTimeout:
		return false
	}
	return strings.Contains(strings.ToLower(e.Message), "not found")
}

// AsNotExist normalizes a not-found failure (per IsNotFound) into an error that
// satisfies errors.Is(err, os.ErrNotExist), so filesystem-shaped callers built
// on ACP's fs/* methods can branch with the same predicate they use for local
// I/O. Any other error — including nil — is returned unchanged, so it is safe to
// wrap every fs call site in it.
func AsNotExist(err error) error {
	if err == nil {
		return nil
	}
	if IsNotFound(err) {
		return fmt.Errorf("%w: %v", os.ErrNotExist, err)
	}
	return err
}
