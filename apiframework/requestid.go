package apiframework

import (
	"context"
	"net/http"
	"strings"

	"github.com/contenox/contenox/libtracker"
	"github.com/google/uuid"
)

// The correlation ids are stamped under libtracker's OWN exported context keys,
// and that is load-bearing rather than incidental.
//
// This package used to declare `type contextKey string` with the same three
// string values and stamp those. context.Value compares the key's TYPE as well
// as its value, so a distinct named type never matched libtracker's lookup:
// request_id, trace_id and span_id were read on every tracker line and were
// silently absent from every one of them, while the comment here claimed the
// opposite. Two named types with equal underlying values are the one case where
// the failure is invisible at the call site and at the log line both.
//
// ⚠ Do not reintroduce a local key type. If libtracker's keys ever stop being
// exported, the fix is a change there, not a second set here.

// ContextKeyRequestID, ContextKeyTraceID and ContextKeySpanID are re-exported
// so callers in this module have one import for the middleware and the keys.
var (
	ContextKeyRequestID = libtracker.ContextKeyRequestID
	ContextKeyTraceID   = libtracker.ContextKeyTraceID
	ContextKeySpanID    = libtracker.ContextKeySpanID
)

// RequestIDMiddleware stamps an inbound X-Request-ID, or mints one, and echoes
// it on the response.
//
// It has to be MOUNTED to do anything; it was written and never wired, so no
// host response has ever carried the header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TracingMiddleware extracts trace and span IDs from an inbound W3C traceparent
// header, or mints them. A minted span ID is 16 hex characters, so the UUID's
// dashes are stripped before truncating.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		traceID := ""
		spanID := ""

		traceparent := r.Header.Get("traceparent")
		if traceparent != "" {
			parts := strings.Split(traceparent, "-")
			if len(parts) == 4 {
				traceID = parts[1]
				spanID = parts[2]
			}
		}

		if traceID == "" {
			traceID = uuid.New().String()
			spanID = strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
		}

		ctx = context.WithValue(ctx, ContextKeyTraceID, traceID)
		ctx = context.WithValue(ctx, ContextKeySpanID, spanID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
