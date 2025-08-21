package libtracker

import (
	"context"
	"log/slog"
	"time"
)

var _ ActivityTracker = (*LogActivityTracker)(nil)

// LogActivityTracker is a simple implementation of ActivityTracker that logs events using slog.
type LogActivityTracker struct {
	logger *slog.Logger
}

// NewLogActivityTracker creates a new instance of LogActivityTracker.
func NewLogActivityTracker(logger *slog.Logger) *LogActivityTracker {
	return &LogActivityTracker{
		logger: logger,
	}
}

// Start implements the ActivityTracker interface.
func (t *LogActivityTracker) Start(
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
	attrs = append(attrs, slog.String("request_id", requestID))
	attrs = append(attrs, slog.String("trace_id", traceID))
	attrs = append(attrs, slog.String("span_id", spanID))
	arrs := append(attrs, toSlogAttrs(kvArgs...)...)
	// Initial log entry: start of the operation
	t.logger.LogAttrs(ctx, slog.LevelInfo, "Operation started",
		arrs...,
	)

	// Return functions to be used by caller

	reportErrFunc := func(err error) {
		if err != nil {
			t.logger.ErrorContext(ctx, "Operation failed",
				slog.String("operation", operation),
				slog.String("subject", subject),
				slog.String("op_id", opID),
				slog.Any("error", err),
				slog.String("request_id", requestID),
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
			)
		}
	}

	reportChangeFunc := func(id string, data any) {
		t.logger.LogAttrs(ctx, slog.LevelInfo, "State changed",
			slog.String("operation", operation),
			slog.String("subject", subject),
			slog.String("op_id", opID),
			slog.String("change_id", id),
			slog.Any("change_data", data),
			slog.String("request_id", requestID),
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
		)
	}

	endFunc := func() {
		duration := time.Since(startTime)
		t.logger.LogAttrs(ctx, slog.LevelInfo, "Operation completed",
			slog.String("operation", operation),
			slog.String("subject", subject),
			slog.String("op_id", opID),
			slog.Duration("duration", duration),
			slog.String("request_id", requestID),
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
		)
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
