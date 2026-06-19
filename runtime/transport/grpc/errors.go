package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sentinels maps the contract's canonical errors to a stable wire token + gRPC
// code so errors.Is keeps working across the boundary.
var sentinels = []struct {
	token string
	code  codes.Code
	err   error
}{
	{"context_canceled", codes.Canceled, context.Canceled},
	{"deadline_exceeded", codes.DeadlineExceeded, context.DeadlineExceeded},
	{"stale_fence", codes.FailedPrecondition, transport.ErrStaleFence},
	{"not_owner", codes.FailedPrecondition, transport.ErrNotOwner},
	{"session_closed", codes.FailedPrecondition, transport.ErrSessionClosed},
	{"context_overflow", codes.ResourceExhausted, transport.ErrContextOverflow},
	{"model_busy", codes.FailedPrecondition, transport.ErrModelBusy},
	{"model_not_active", codes.FailedPrecondition, transport.ErrModelNotActive},
	{"model_switch_required", codes.FailedPrecondition, transport.ErrModelSwitchRequired},
	{"model_load_failed", codes.Internal, transport.ErrModelLoadFailed},
	{"insufficient_memory", codes.ResourceExhausted, transport.ErrInsufficientMemory},
	{"slot_generation_stale", codes.FailedPrecondition, transport.ErrSlotGenerationStale},
	{"backend_mismatch", codes.FailedPrecondition, transport.ErrBackendMismatch},
	{"unsupported_feature", codes.Unimplemented, transport.ErrUnsupportedFeature},
	{"manifest_mismatch", codes.FailedPrecondition, contextasm.ErrManifestMismatch},
}

// encodeError turns a contract error into a gRPC status carrying a sentinel
// token, so the client can reconstruct the original sentinel.
func encodeError(err error) error {
	if err == nil {
		return nil
	}
	for _, s := range sentinels {
		if errors.Is(err, s.err) {
			return status.Error(s.code, s.token+": "+err.Error())
		}
	}
	return status.Error(codes.Internal, "internal: "+err.Error())
}

// decodeError reverses encodeError: a status whose message starts with a known
// token is rewrapped around the matching sentinel so errors.Is works client-side.
func decodeError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	msg := st.Message()
	token, rest := msg, msg
	if i := strings.Index(msg, ": "); i >= 0 {
		token, rest = msg[:i], msg[i+2:]
	}
	for _, s := range sentinels {
		if s.token == token {
			return fmt.Errorf("%w: %s", s.err, rest)
		}
	}
	switch st.Code() {
	case codes.Canceled:
		return fmt.Errorf("%w: %s", context.Canceled, msg)
	case codes.DeadlineExceeded:
		return fmt.Errorf("%w: %s", context.DeadlineExceeded, msg)
	}
	return err
}

func errorToken(err error) string {
	for _, s := range sentinels {
		if errors.Is(err, s.err) {
			return s.token
		}
	}
	return ""
}

func decodeWireError(token, msg string) error {
	if msg == "" {
		return nil
	}
	for _, s := range sentinels {
		if s.token == token {
			return fmt.Errorf("%w: %s", s.err, msg)
		}
	}
	return errors.New(msg)
}
