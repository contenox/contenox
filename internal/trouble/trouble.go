// Package trouble records the runtime's own failures as deployment-scoped
// events, so that a 500 raises an operator alarm instead of ending in a tracker
// span nobody retains and nobody reads.
//
// It is the producer half of the alert path: the event log consumer delivers
// [EventRequestFailed]. Nothing here reaches an operator directly, and nothing
// here may append anything but the one event.
package trouble

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
)

// Floor is the least time between two [EventRequestFailed] rows for one surface
// and operation.
//
// A broken dependency fails every request that touches it, and the event exists
// to raise one alarm rather than to count the attempts. One row per interval
// still shows a run — the signal — while a burst costs one write. The floor
// bounds the log, never the alert: the consumer has its own cooldown above this.
const Floor = time.Minute

// PlatformScope scopes a failure that belongs to the deployment rather than to
// any tenant: the cause is the deployment's, never the caller's.
const PlatformScope = "platform"

// EventRequestFailed is a request answered 500.
const EventRequestFailed = "request.failed"

// EventSourceRuntime names the producer of a recorded failure.
const EventSourceRuntime = "runtime"

// Recorder appends request failures, floored per surface and operation.
type Recorder interface {
	// Record appends one [EventRequestFailed] for a request the caller answered
	// 500, at most once per surface and operation per [Floor].
	//
	// ⚠ surface and operation must be COMPILE-TIME LABELS. They are the whole
	// payload, they travel to the operator's alert channel, and that channel is
	// outside the host — a value derived from a request would publish it.
	//
	// A failed append is reported and swallowed. Recording trouble must never
	// change what the caller answers, and by this point the response is written.
	Record(ctx context.Context, surface, operation string)
}

type recorder struct {
	events  runtimetypes.EventStore
	tracker libtracker.ActivityTracker
	now     func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

// NewRecorder returns a recorder writing to db, or one that records nothing when
// db is nil. A nil tracker degrades to [libtracker.NoopTracker] and a nil now
// takes time.Now.
func NewRecorder(db libdb.DBManager, tracker libtracker.ActivityTracker, now func() time.Time) Recorder {
	if db == nil {
		return noopRecorder{}
	}
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &recorder{
		events:  runtimetypes.NewEventStore(db),
		tracker: tracker,
		now:     now,
		last:    map[string]time.Time{},
	}
}

type noopRecorder struct{}

func (noopRecorder) Record(context.Context, string, string) {}

func (r *recorder) Record(ctx context.Context, surface, operation string) {
	if !r.claim(surface, operation) {
		return
	}
	data, err := json.Marshal(map[string]string{"surface": surface, "operation": operation})
	if err != nil {
		r.report(ctx, err)
		return
	}
	if err := r.events.AppendEvent(ctx, &runtimetypes.Event{
		WorkspaceID: PlatformScope,
		Type:        EventRequestFailed,
		Source:      EventSourceRuntime,
		Time:        r.now(),
		Data:        data,
	}); err != nil {
		r.report(ctx, err)
	}
}

func (r *recorder) claim(surface, operation string) bool {
	key := surface + "/" + operation
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.last[key]; ok && now.Sub(last) < Floor {
		return false
	}
	r.last[key] = now
	return true
}

func (r *recorder) report(ctx context.Context, err error) {
	reportErr, _, end := r.tracker.Start(ctx, "trouble", "record_request_failed")
	reportErr(err)
	end()
}
