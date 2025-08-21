package libtracker

import "context"

var ContextKeyRequestID = contextKey("request_id")
var ContextKeyTraceID = contextKey("trace_id")
var ContextKeySpanID = contextKey("span_id")

func CopyTrackingValues(src context.Context, dst context.Context) context.Context {
	requestID := src.Value(ContextKeyRequestID)
	traceID := src.Value(ContextKeyTraceID)
	spanID := src.Value(ContextKeySpanID)
	ctx := context.WithValue(dst, ContextKeyRequestID, requestID)
	ctx = context.WithValue(ctx, ContextKeyTraceID, traceID)
	ctx = context.WithValue(ctx, ContextKeySpanID, spanID)
	return ctx
}
