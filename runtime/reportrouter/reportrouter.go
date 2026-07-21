// Package reportrouter is the supervision edge's delivery half: it subscribes to
// missionservice's ReportAddedEvent and routes each report to whoever fired the
// mission (docs/development/blueprints/acp/fleet-consolidation.md, "Mission
// mode", M3 — "its reports must reach whoever fired it, which is not always the
// operator").
//
//   - A mission fired FROM a session (ParentSessionID set) is supervised by that
//     session: the report is DELIVERED into its update stream, so a coordinating
//     agent or an attached operator sees "unit X reported: …" in the transcript
//     and can act on it on its next turn. This is async talk-back — the floor the
//     blueprint chose to build first, over synchronous blocking.
//   - A mission an operator fired directly (ParentSessionID empty) has no
//     upstream session; its report lands in the operator inbox instead.
//   - A mission whose parent session is GONE by the time the report arrives falls
//     back to the inbox rather than being lost. A supervisor ending must never
//     drop a report.
//
// # Why a bus consumer and not a call from AddReport
//
// missionservice publishes that a report exists and stays ignorant of sessions
// and inboxes; this package subscribes and owns the WHERE. That is the libbus
// decoupling idiom (CONTRIBUTING.md), and it is what makes this slice work today
// off a REST-added report AND compose automatically with the mission-tools slice
// when a unit files its own report through a tool — both paths go through
// AddReport, so both publish, so both route, with no coupling between the slices.
//
// # The best-effort invariant
//
// The report is the durable fact; routing is best-effort DELIVERY on top of it.
// The router runs asynchronously off the bus, so nothing it does can fail the
// AddReport that produced the event. Within the router, a delivery that cannot
// reach a live parent falls back to the inbox, and an inbox write that fails is
// reported to the tracker — never retried into a crash. The durable report
// remains readable via ListReports regardless.
package reportrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/contenox/runtime/libacp"
	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/operatorinbox"
)

// SessionDeliverer injects a report update into a supervising session's stream.
// It is the NARROWEST slice of agentinstance.Manager the router needs — inject
// one update into one session, nothing else — declared here (the consumer owns
// the interface it depends on) so this package never imports the kernel Manager
// wholesale. agentinstance.Manager satisfies it.
//
// A non-nil error means the session was not reachable (ErrNotFound: gone, or not
// hosted here); the router treats that as "route to the inbox", never a fault.
type SessionDeliverer interface {
	DeliverToSession(ctx context.Context, sessionID libacp.SessionID, n libacp.SessionNotification) error
}

// InboxWriter records a report that reached no live supervisor. The narrowest
// slice of operatorinbox.Service the router needs (Add only).
type InboxWriter interface {
	Add(ctx context.Context, item *operatorinbox.Item) error
}

// Subscriber is the narrow slice of the event bus the router consumes (Stream
// only). libbus.Messenger satisfies it.
type Subscriber interface {
	Stream(ctx context.Context, subject string, ch chan<- []byte) (libbus.Subscription, error)
}

// Deps are the router's collaborators. Bus, Sessions, and Inbox are required;
// Tracker degrades to a Noop when nil.
type Deps struct {
	Bus      Subscriber
	Sessions SessionDeliverer
	Inbox    InboxWriter
	Tracker  libtracker.ActivityTracker
}

// Router subscribes to report-added events and routes each to a session or the
// inbox. Build with New, run with Start.
type Router struct {
	deps Deps
}

// New validates deps and returns a Router. A nil Bus/Sessions/Inbox is a wiring
// defect (the router cannot function without any of them) and is rejected up
// front rather than surfacing later as a silent no-route.
func New(deps Deps) (*Router, error) {
	if deps.Bus == nil {
		return nil, fmt.Errorf("reportrouter: Bus is required")
	}
	if deps.Sessions == nil {
		return nil, fmt.Errorf("reportrouter: Sessions is required")
	}
	if deps.Inbox == nil {
		return nil, fmt.Errorf("reportrouter: Inbox is required")
	}
	if deps.Tracker == nil {
		deps.Tracker = libtracker.NoopTracker{}
	}
	return &Router{deps: deps}, nil
}

// streamBuffer bounds the per-router event channel. Reports are low-volume (a
// unit files a handful over a mission), so a small buffer is ample; the bus's
// own backpressure policy (drop on a full at-most-once backend, durable on
// SQLite) applies beyond it.
const streamBuffer = 64

// Start subscribes to ReportAddedSubject and processes events until the returned
// stop function is called or ctx is cancelled. It returns after the subscription
// is established, so an event published after Start returns is seen (the bus
// registers the subscription before Stream returns). The stop function cancels
// the loop, unsubscribes, and waits for the loop goroutine to exit, so no
// delivery is in flight once it returns.
func (r *Router) Start(ctx context.Context) (func(), error) {
	ch := make(chan []byte, streamBuffer)
	sub, err := r.deps.Bus.Stream(ctx, missionservice.ReportAddedSubject, ch)
	if err != nil {
		return nil, fmt.Errorf("reportrouter: subscribe %q: %w", missionservice.ReportAddedSubject, err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.loop(runCtx, ch)
	}()
	return func() {
		cancel()
		_ = sub.Unsubscribe()
		wg.Wait()
	}, nil
}

func (r *Router) loop(ctx context.Context, ch <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			r.handle(ctx, data)
		}
	}
}

func (r *Router) handle(ctx context.Context, data []byte) {
	var ev missionservice.ReportAddedEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		// A malformed event is a bug in the publisher, not a routable report;
		// record it and move on rather than wedging the loop.
		reportErr, _, end := r.deps.Tracker.Start(ctx, "fleet", "route_report")
		reportErr(fmt.Errorf("reportrouter: decode event: %w", err))
		end()
		return
	}
	r.route(ctx, ev)
}

// route is the routing decision, exported to the package's own tests via a
// direct call so the branch table can be asserted without driving the bus.
func (r *Router) route(ctx context.Context, ev missionservice.ReportAddedEvent) {
	reportErr, reportChange, end := r.deps.Tracker.Start(ctx, "fleet", "route_report",
		"mission_id", ev.MissionID, "report_id", ev.Report.ID, "report_kind", string(ev.Report.Kind))
	defer end()

	if ev.ParentSessionID != "" {
		reportChange("parent_session_id", ev.ParentSessionID)
		n := buildReportNotification(ev)
		if err := r.deps.Sessions.DeliverToSession(ctx, libacp.SessionID(ev.ParentSessionID), n); err == nil {
			reportChange("routed", "session")
			return
		}
		// Parent named but not deliverable (the supervisor ended, or its session
		// is not hosted by this Manager): fall back to the inbox, marked so an
		// operator sees a supervisor was intended but missed. Never dropped.
		reportChange("routed", "inbox_parent_gone")
		if err := r.toInbox(ctx, ev, operatorinbox.ReasonParentGone); err != nil {
			reportErr(err)
		}
		return
	}

	reportChange("routed", "inbox_operator_fired")
	if err := r.toInbox(ctx, ev, operatorinbox.ReasonOperatorFired); err != nil {
		reportErr(err)
	}
}

func (r *Router) toInbox(ctx context.Context, ev missionservice.ReportAddedEvent, reason operatorinbox.Reason) error {
	item := &operatorinbox.Item{
		MissionID:       ev.MissionID,
		AgentName:       ev.AgentName,
		Intent:          ev.Intent,
		ParentSessionID: ev.ParentSessionID,
		Reason:          reason,
		Report:          ev.Report,
	}
	if err := r.deps.Inbox.Add(ctx, item); err != nil {
		return fmt.Errorf("reportrouter: write inbox item for mission %q: %w", ev.MissionID, err)
	}
	return nil
}

// reportUpdateMeta namespaces the mission-report attribution the delivered
// update carries in its ACP _meta envelope, so a consumer (beam's transcript,
// a coordinating agent) can recognise and render it as a mission report rather
// than as an ordinary agent message. The key is dotted-namespaced so it never
// collides with another producer's _meta.
type reportUpdateMeta struct {
	Report *reportAttribution `json:"contenox.missionReport,omitempty"`
}

type reportAttribution struct {
	MissionID string `json:"missionId"`
	ReportID  string `json:"reportId"`
	Kind      string `json:"kind"`
	AgentName string `json:"agentName,omitempty"`
}

// buildReportNotification renders a report as the session update delivered into
// the supervising session's stream: an agent_message_chunk (the kind that lands
// in the transcript the parent reads on its next turn), plus a _meta envelope
// carrying the mission/report attribution. The text is human-first — "unit X
// reported (kind): summary" — so it is legible whether the supervisor is a human
// at beam or another agent reading its transcript.
func buildReportNotification(ev missionservice.ReportAddedEvent) libacp.SessionNotification {
	update := libacp.NewAgentMessageChunk(reportText(ev))
	meta := reportUpdateMeta{Report: &reportAttribution{
		MissionID: ev.MissionID,
		ReportID:  ev.Report.ID,
		Kind:      string(ev.Report.Kind),
		AgentName: ev.AgentName,
	}}
	if raw, err := json.Marshal(meta); err == nil {
		update.Meta = raw
	}
	return libacp.SessionNotification{
		SessionID: libacp.SessionID(ev.ParentSessionID),
		Update:    update,
	}
}

// reportText composes the human-readable body of a delivered report. Kept
// deterministic and content-bounded: summary is a single line by validation,
// detail and refs are appended only when present.
func reportText(ev missionservice.ReportAddedEvent) string {
	unit := strings.TrimSpace(ev.AgentName)
	if unit == "" {
		unit = "a mission unit"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "unit %s reported (%s): %s", unit, ev.Report.Kind, ev.Report.Summary)
	if d := strings.TrimSpace(ev.Report.Detail); d != "" {
		b.WriteString("\n")
		b.WriteString(d)
	}
	if len(ev.Report.Refs) > 0 {
		b.WriteString("\nrefs: ")
		b.WriteString(strings.Join(ev.Report.Refs, ", "))
	}
	return b.String()
}
