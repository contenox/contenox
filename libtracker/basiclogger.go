package libtracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

var _ ActivityTracker = (*logActivityTracker)(nil)

const (
	logChangeDataMaxJSONBytes = 32 * 1024
	logChangeDataPreviewBytes = 4 * 1024
)

// logActivityTracker is a simple implementation of ActivityTracker that logs events using slog.
type logActivityTracker struct {
	logger *slog.Logger
}

// NewTextActivityTracker returns an ActivityTracker that emits structured text
// logs to w at Info level. It owns its slog wiring so callers (e.g. command
// entrypoints) don't have to import log/slog just to obtain a tracker.
func NewTextActivityTracker(w io.Writer) ActivityTracker {
	return NewLogActivityTracker(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
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
			slog.Any("change_data", boundedLogValue(data)),
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

func boundedLogValue(v any) any {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return map[string]any{
			"truncated":     true,
			"reason":        "json_marshal_error: " + capLogString(err.Error(), 512),
			"original_type": logTypeName(v),
		}
	}
	if len(raw) <= logChangeDataMaxJSONBytes {
		return v
	}
	sum := sha256.Sum256(raw)
	preview := previewLogBytes(raw, logChangeDataPreviewBytes)
	return map[string]any{
		"truncated":           true,
		"reason":              "payload_exceeds_limit",
		"original_type":       logTypeName(v),
		"original_json_bytes": len(raw),
		"sha256":              hex.EncodeToString(sum[:]),
		"preview":             preview,
		"preview_bytes":       len([]byte(preview)),
	}
}

func capLogString(s string, maxBytes int) string {
	if maxBytes <= 0 || len([]byte(s)) <= maxBytes {
		return s
	}
	return previewLogBytes([]byte(s), maxBytes)
}

func previewLogBytes(raw []byte, maxBytes int) string {
	if maxBytes > 0 && len(raw) > maxBytes {
		raw = raw[:maxBytes]
	}
	return strings.ToValidUTF8(string(raw), "?")
}

func logTypeName(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return fmt.Sprintf("%T", v)
	}
	return t.String()
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
