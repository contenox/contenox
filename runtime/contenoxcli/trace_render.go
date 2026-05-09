package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/taskengine"
)

// traceDrainGrace is the time we wait after a chain returns before cancelling
// the trace subscription, so the SQLite bus poller (default 200ms cadence)
// has a chance to deliver any final published step.
const traceDrainGrace = 500 * time.Millisecond

// startTraceStream subscribes to the per-request state bus subject and renders
// each captured step to w in real time. Returns a stop function to call (via
// defer) when the chain completes.
//
// No-ops when --trace is off, the engine has no bus, the ctx has no
// requestID, or the bus subscription itself fails (reported via tracker).
func startTraceStream(ctx context.Context, opts chatOpts, engine *Engine, w io.Writer) func() {
	if !opts.EffectiveTracing || engine == nil || engine.Bus == nil {
		return func() {}
	}
	reqID, ok := ctx.Value(libtracker.ContextKeyRequestID).(string)
	if !ok || reqID == "" {
		return func() {}
	}

	subject := taskengine.StateSubject(reqID)

	tracker := engine.Tracker
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	reportErr, _, end := tracker.Start(ctx, "subscribe", "state_bus", "subject", subject)
	defer end()

	streamCtx, cancel := context.WithCancel(ctx)
	rawCh := make(chan []byte, 32)
	sub, err := engine.Bus.Stream(streamCtx, subject, rawCh)
	if err != nil {
		cancel()
		reportErr(err)
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		renderTraceUnits(streamCtx, rawCh, w)
		close(done)
	}()

	return func() {
		time.Sleep(traceDrainGrace)
		cancel()
		_ = sub.Unsubscribe()
		<-done
	}
}

func renderTraceUnits(ctx context.Context, ch <-chan []byte, w io.Writer) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			var unit taskengine.CapturedStateUnit
			if err := json.Unmarshal(payload, &unit); err != nil {
				continue
			}
			if _, err := fmt.Fprintln(w, formatTraceUnit(unit)); err != nil {
				return
			}
		}
	}
}

func formatTraceUnit(u taskengine.CapturedStateUnit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[trace] task=%s handler=%s retry=%d dur=%s trans=%s",
		u.TaskID, u.TaskHandler, u.RetryIndex, u.Duration, u.Transition)
	if u.ModelName != "" {
		fmt.Fprintf(&b, " model=%s", u.ModelName)
	}
	if u.ProviderType != "" {
		fmt.Fprintf(&b, " provider=%s", u.ProviderType)
	}
	if len(u.ToolNames) > 0 {
		fmt.Fprintf(&b, " tools=%s", strings.Join(u.ToolNames, ","))
	}
	if u.TokenUsage != nil {
		fmt.Fprintf(&b, " tokens=%d+%d=%d", u.TokenUsage.Prompt, u.TokenUsage.Completion, u.TokenUsage.Total)
	}
	switch {
	case u.TimedOut:
		b.WriteString(" TIMED-OUT")
	case u.Cancelled:
		b.WriteString(" CANCELLED")
	}
	if u.Error.Error != "" {
		fmt.Fprintf(&b, " ERROR: %s", u.Error.Error)
	}
	return b.String()
}
