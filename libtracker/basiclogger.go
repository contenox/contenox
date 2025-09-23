package libtracker

import (
	"context"
	"log/slog"
	"time"
)

var _ ActivityTracker = (*logActivityTracker)(nil)

// logActivityTracker is a simple implementation of ActivityTracker that logs events using slog.
type logActivityTracker struct {
	logger *slog.Logger
}

// NewLogActivityTracker creates a new instance of logActivityTracker.
func NewLogActivityTracker(logger *slog.Logger) ActivityTracker {
	if logger == nil {
		logger = slog.Default()
	}
	return &logActivityTracker{
		logger: logger,
	}
}

// Start implements the ActivityTracker interface.
func (t *logActivityTracker) Start(
	ctx context.Context,
	operation string,
	subject string,
	kvArgs ...any,
) (reportErr func(error), reportChange func(string, any), end func()) {
	startTime := time.Now()

	// Generate an operation ID
	opID := "op-" + formatTimestamp(startTime)
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.String("subject", subject),
		slog.String("op_id", opID),
	}
	requestID := ""
	if val, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		requestID = val
	}
	traceID := ""
	if val, ok := ctx.Value(ContextKeyTraceID).(string); ok {
		traceID = val
	}
	spanID := ""
	if val, ok := ctx.Value(ContextKeySpanID).(string); ok {
		spanID = val
	}
	if len(requestID) > 0 {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	if len(traceID) > 0 {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	if len(spanID) > 0 {
		attrs = append(attrs, slog.String("span_id", spanID))
	}
	arrs := append(attrs, toSlogAttrs(kvArgs...)...)
	// Initial log entry: start of the operation
	t.logger.LogAttrs(ctx, slog.LevelInfo, "Operation started",
		arrs...,
	)

	// Return functions to be used by caller

	reportErrFunc := func(err error) {
		if err != nil {
			attr := []any{
				slog.String("operation", operation),
				slog.String("subject", subject),
				slog.String("op_id", opID),
				slog.Any("error", err),
			}
			if len(requestID) > 0 {
				attr = append(attr, slog.String("request_id", requestID))
			}
			if len(traceID) > 0 {
				attr = append(attr, slog.String("trace_id", traceID))
			}
			if len(spanID) > 0 {
				attr = append(attr, slog.String("span_id", spanID))
			}
			t.logger.ErrorContext(ctx, "Operation failed", attr...)
		}
	}

	reportChangeFunc := func(id string, data any) {
		attr := []slog.Attr{
			slog.String("operation", operation),
			slog.String("subject", subject),
			slog.String("op_id", opID),
			slog.String("change_id", id),
			slog.Any("change_data", data),
		}
		if len(spanID) > 0 {
			attr = append(attr, slog.String("span_id", spanID))
		}
		if len(traceID) > 0 {
			attr = append(attr, slog.String("trace_id", traceID))
		}
		if len(requestID) > 0 {
			attr = append(attr, slog.String("request_id", requestID))
		}
		t.logger.LogAttrs(ctx, slog.LevelInfo, "State changed", attr...)
	}

	endFunc := func() {
		duration := time.Since(startTime)
		attr := []slog.Attr{
			slog.String("operation", operation),
			slog.String("subject", subject),
			slog.String("op_id", opID),
			slog.Duration("duration", duration),
		}
		if len(spanID) > 0 {
			attr = append(attr, slog.String("span_id", spanID))
		}
		if len(traceID) > 0 {
			attr = append(attr, slog.String("trace_id", traceID))
		}
		if len(requestID) > 0 {
			attr = append(attr, slog.String("request_id", requestID))
		}
		t.logger.LogAttrs(ctx, slog.LevelInfo, "Operation completed", attr...)
	}

	return reportErrFunc, reportChangeFunc, endFunc
}

// Helper: Convert key-value pairs into slog attributes
func toSlogAttrs(kvArgs ...any) []slog.Attr {
	var attrs []slog.Attr
	for i := 0; i < len(kvArgs); i += 2 {
		key, ok := kvArgs[i].(string)
		if !ok || i+1 >= len(kvArgs) {
			continue
		}
		attrs = append(attrs, slog.Any(key, kvArgs[i+1]))
	}
	return attrs
}

// Helper: Format timestamp for op_id
func formatTimestamp(t time.Time) string {
	return t.Format("20060102-150405.000")
}

type contextKey string

func (c contextKey) String() string {
	return string(c)
}
