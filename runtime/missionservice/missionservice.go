// Package missionservice stores mission records — the durable, agent-reportable
// half of the fleet manager (see
// docs/development/blueprints/acp/fleet-consolidation.md, "Mission mode"). A
// mission is not a note: it is the headless interaction model. An operator
// fires a one-line intent at a declared agent and detaches; the resulting
// unit runs unattended inside a permission envelope (a named HITL policy
// bound to the mission) and reports back — or asks for attention — through
// tools it holds only while on the mission. One mission binds exactly one
// session and one instance: the work a mission names is a single unit, so the
// record is too.
//
// Storage is runtimetypes KV records keyed by mission id under a shared prefix
// (pattern of acpsvc's acp:session_* keys, runtime/acpsvc/external.go), listed
// server-side via the store's prefix scan — zero migration. The validated-CRUD
// shape mirrors runtime/agentregistryservice so the two registries stay easy to
// compare.
//
// # Reports
//
// A mission's reports live under a second, sibling KV prefix keyed by mission
// id (missionReportKVPrefix; see that constant's comment for why it is a
// sibling of missionKVPrefix rather than nested under it). A Report's Kind is
// drawn from a small, closed set — progress, finding, blocker, result —
// rather than free text, and that closedness is a deliberate prompt-design
// choice: the set IS the hint to an unattended agent about what is worth
// reporting at all. A short, named enum tells the agent "these four shapes of
// thing are worth recording"; free text invites narration, which is the wrong
// incentive for a unit that should only cost a human's attention when it
// matters. Reports never carry artifact content — Refs points at it by path
// or URL only, keeping a report cheap to store and cheap to read from an
// attention inbox.
//
// # Liveness
//
// A mission runs unattended, so "is this unit still alive?" cannot be
// answered by a human watching a transcript — nobody is watching by
// construction. Without an explicit liveness signal, a mission whose unit
// crashed sits at StatusOpen forever, indistinguishable from one making slow,
// silent progress: exactly the failure mission mode must not have.
// LastHeartbeat and LastError turn liveness and last-failure into queryable
// facts instead of inferences: Heartbeat stamps LastHeartbeat and records (or
// clears) LastError on every call, so a caller can tell "working",
// "erroring", and "gone dark past some staleness threshold" apart without
// attaching to anything.
package missionservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
)

// Status is a mission's lifecycle state.
type Status string

const (
	StatusOpen      Status = "open"
	StatusLanded    Status = "landed"
	StatusDerailed  Status = "derailed"
	StatusAbandoned Status = "abandoned"
)

// missionKVPrefix namespaces mission records in the KV store; each mission is
// stored at missionKVPrefix+ID and the set is listed by this prefix.
const missionKVPrefix = "fleet:mission:"

// missionReportKVPrefix namespaces mission reports; each report is stored at
// missionReportKVPrefix+missionID+":"+reportID, and a mission's reports are
// listed by scanning the prefix missionReportKVPrefix+missionID+":".
//
// It is deliberately a SIBLING of missionKVPrefix, not a child of it (i.e.
// NOT "fleet:mission:report:") — missionKVPrefix ("fleet:mission:") is used
// as a raw LIKE prefix by List() to scan every mission, and nesting reports
// under it would make every report key match that same scan, corrupting the
// mission list with report rows decoded as missions. "fleet:mission_report:"
// does not share "fleet:mission:" as a string prefix (the byte after
// "fleet:mission" differs: '_' vs ':'), so the two prefix scans can never
// collide.
const missionReportKVPrefix = "fleet:mission_report:"

// Mission is the headless interaction model's durable record: a one-line
// intent fired at a declared agent, bound to exactly one session and one
// instance, and bounded by an envelope — a named HITL policy — that governs
// what the unit may do while unattended. It may outlive its session and
// instance and remains listed while open.
//
// LastHeartbeat and LastError are liveness facts, not status: a mission stays
// StatusOpen for its whole run, and these two fields are how a caller tells a
// unit that is quietly working apart from one that has gone dark or is
// erroring, without attaching to its session. Neither is required on create;
// both start zero (LastHeartbeat nil, LastError "") until Heartbeat is
// called.
type Mission struct {
	ID             string     `json:"id"`
	Intent         string     `json:"intent"`
	AgentName      string     `json:"agentName"`
	HITLPolicyName string     `json:"hitlPolicyName"`
	SessionID      string     `json:"sessionId,omitempty"`
	InstanceID     string     `json:"instanceId,omitempty"`
	Status         Status     `json:"status"`
	LastHeartbeat  *time.Time `json:"lastHeartbeat,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// ReportKind is the closed, small set of things a mission report may be. See
// the package doc's "Reports" section for why the set is deliberately closed
// rather than free text.
type ReportKind string

const (
	ReportKindProgress ReportKind = "progress"
	ReportKindFinding  ReportKind = "finding"
	ReportKindBlocker  ReportKind = "blocker"
	ReportKindResult   ReportKind = "result"
)

// Report is a single dispatch from a unit on a mission, filed under a Kind
// that hints how much it matters. Refs is by reference only (file paths,
// URLs, etc.) — a report never carries artifact content.
type Report struct {
	ID        string     `json:"id"`
	MissionID string     `json:"missionId"`
	Kind      ReportKind `json:"kind"`
	Summary   string     `json:"summary"`
	Detail    string     `json:"detail,omitempty"`
	Refs      []string   `json:"refs,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Service exposes validated CRUD over mission records, Bind (which attaches
// this mission's one session and one instance), Heartbeat (unattended
// liveness), and mission reports.
type Service interface {
	Create(ctx context.Context, m *Mission) error
	Get(ctx context.Context, id string) (*Mission, error)
	List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*Mission, error)
	Update(ctx context.Context, m *Mission) error
	Delete(ctx context.Context, id string) error

	// Bind attaches sessionID and/or instanceID to mission id — the
	// one-mission, one-unit invariant mission mode requires. Re-binding the
	// id a mission already carries is an idempotent no-op; binding a
	// DIFFERENT id over one already set is a conflict
	// (apiframework.ErrConflict) rather than an append. An unknown mission
	// id surfaces as libdb.ErrNotFound.
	Bind(ctx context.Context, id string, sessionID, instanceID string) (*Mission, error)

	// Heartbeat records that mission id's unit is still alive: it stamps
	// LastHeartbeat to now and sets LastError to lastErr (empty clears any
	// previously recorded error), then bumps UpdatedAt and persists. An
	// unknown mission id surfaces as libdb.ErrNotFound. Nothing calls this
	// yet — the caller arrives with the mission-tools slice.
	Heartbeat(ctx context.Context, id string, lastErr string) (*Mission, error)

	// AddReport validates report (Kind, Summary), assigns an id and
	// CreatedAt when absent, binds it to missionID, and persists it.
	// missionID must name an existing mission — an unknown one surfaces as
	// libdb.ErrNotFound rather than a silent insert.
	AddReport(ctx context.Context, missionID string, report *Report) error

	// ListReports returns missionID's reports newest-first. The slice is
	// always non-nil, so a mission with no reports yet renders as [].
	ListReports(ctx context.Context, missionID string, limit int) ([]*Report, error)
}

type service struct {
	db libdb.DBManager
}

// New creates a mission service backed by the given database manager.
func New(db libdb.DBManager) Service {
	return &service{db: db}
}

func (s *service) store() runtimetypes.Store {
	return runtimetypes.New(s.db.WithoutTransaction())
}

// Create validates m (intent, envelope, status), assigns an id when absent,
// forces the status to open, stamps timestamps, and persists it. A mission
// with no HITLPolicyName is rejected: the envelope is what bounds an
// unattended unit, so a mission without one is a mission with no bounds,
// which mission mode must not permit.
func (s *service) Create(ctx context.Context, m *Mission) error {
	if m == nil {
		return fmt.Errorf("mission is required")
	}
	m.Status = StatusOpen
	if err := validate(m); err != nil {
		return err
	}
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	return s.put(ctx, m, false)
}

func (s *service) Get(ctx context.Context, id string) (*Mission, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	var m Mission
	if err := s.store().GetKV(ctx, missionKVPrefix+id, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns missions newest-first via the store's prefix scan. The slice is
// always non-nil so an empty fleet renders as [].
func (s *service) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*Mission, error) {
	if limit <= 0 {
		limit = 100
	}
	kvs, err := s.store().ListKVPrefix(ctx, missionKVPrefix, createdAtCursor, limit)
	if err != nil {
		return nil, err
	}
	missions := make([]*Mission, 0, len(kvs))
	for _, kv := range kvs {
		var m Mission
		if err := json.Unmarshal(kv.Value, &m); err != nil {
			return nil, fmt.Errorf("mission %q: %w", kv.Key, err)
		}
		missions = append(missions, &m)
	}
	return missions, nil
}

// Update validates m and persists intent/status/envelope/reference changes to
// an existing mission. An unknown id surfaces as libdb.ErrNotFound. The caller
// owns m's CreatedAt (typically read via Get); UpdatedAt is restamped here.
func (s *service) Update(ctx context.Context, m *Mission) error {
	if m == nil {
		return fmt.Errorf("mission is required")
	}
	if m.ID == "" {
		return fmt.Errorf("id is required for update")
	}
	if err := validate(m); err != nil {
		return err
	}
	m.UpdatedAt = time.Now().UTC()
	return s.put(ctx, m, true)
}

func (s *service) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	return s.store().DeleteKV(ctx, missionKVPrefix+id)
}

// Bind attaches sessionID and/or instanceID to mission id. Mission mode binds
// exactly one session and one instance per mission, so binding is not
// additive: setting an id the mission does not yet carry succeeds,
// re-setting the id it already carries is an idempotent no-op, and setting a
// DIFFERENT id over one already bound is a conflict — a mission does not
// switch which unit it names mid-flight; a caller that wants a different unit
// dispatches a new mission instead of rebinding this one. An unknown mission
// id surfaces as libdb.ErrNotFound.
func (s *service) Bind(ctx context.Context, id string, sessionID, instanceID string) (*Mission, error) {
	if sessionID == "" && instanceID == "" {
		return nil, fmt.Errorf("bind requires a sessionId or instanceId")
	}
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	changed := false
	if sessionID != "" {
		switch m.SessionID {
		case "":
			m.SessionID = sessionID
			changed = true
		case sessionID:
			// Idempotent re-bind of the id already carried: no-op.
		default:
			return nil, apiframework.Conflict(fmt.Sprintf("mission %q is already bound to session %q", id, m.SessionID))
		}
	}
	if instanceID != "" {
		switch m.InstanceID {
		case "":
			m.InstanceID = instanceID
			changed = true
		case instanceID:
			// Idempotent re-bind of the id already carried: no-op.
		default:
			return nil, apiframework.Conflict(fmt.Sprintf("mission %q is already bound to instance %q", id, m.InstanceID))
		}
	}
	if !changed {
		return m, nil
	}

	m.UpdatedAt = time.Now().UTC()
	if err := s.put(ctx, m, true); err != nil {
		return nil, err
	}
	return m, nil
}

// Heartbeat records that mission id's unit is still alive: it stamps
// LastHeartbeat to now, sets LastError to lastErr (empty clears it), bumps
// UpdatedAt, and persists. See the package doc's "Liveness" section for why
// this has to be an explicit, agent-reported fact rather than something a
// human infers. An unknown mission id surfaces as libdb.ErrNotFound.
func (s *service) Heartbeat(ctx context.Context, id string, lastErr string) (*Mission, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	m.LastHeartbeat = &now
	m.LastError = lastErr
	m.UpdatedAt = now
	if err := s.put(ctx, m, true); err != nil {
		return nil, err
	}
	return m, nil
}

// put marshals m and writes it to the KV store. When mustExist is true it uses
// UpdateKV, whose zero-rows-affected result surfaces as libdb.ErrNotFound so an
// update to a missing mission is a not-found rather than a silent insert.
func (s *service) put(ctx context.Context, m *Mission, mustExist bool) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal mission: %w", err)
	}
	if mustExist {
		return s.store().UpdateKV(ctx, missionKVPrefix+m.ID, raw)
	}
	return s.store().SetKV(ctx, missionKVPrefix+m.ID, raw)
}

// AddReport validates report (Kind, Summary), assigns an id and CreatedAt
// when absent, binds it to missionID — overriding whatever MissionID the
// caller supplied, since the argument is authoritative — and persists it.
// missionID is checked against the mission store FIRST, so posting a report
// against an unknown mission surfaces as libdb.ErrNotFound rather than a
// silent insert (the report KV namespace carries no foreign key, so nothing
// else would catch this).
func (s *service) AddReport(ctx context.Context, missionID string, report *Report) error {
	if missionID == "" {
		return fmt.Errorf("missionId is required")
	}
	if report == nil {
		return fmt.Errorf("report is required")
	}
	if _, err := s.Get(ctx, missionID); err != nil {
		return err
	}
	report.MissionID = missionID
	if err := validateReport(report); err != nil {
		return err
	}
	if report.ID == "" {
		report.ID = uuid.NewString()
	}
	report.CreatedAt = time.Now().UTC()

	raw, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return s.store().SetKV(ctx, missionReportKVPrefix+missionID+":"+report.ID, raw)
}

// ListReports returns missionID's reports newest-first via the store's prefix
// scan. The slice is always non-nil so a mission with no reports yet (or an
// unknown missionID) renders as [].
func (s *service) ListReports(ctx context.Context, missionID string, limit int) ([]*Report, error) {
	if missionID == "" {
		return nil, fmt.Errorf("missionId is required")
	}
	if limit <= 0 {
		limit = 100
	}
	kvs, err := s.store().ListKVPrefix(ctx, missionReportKVPrefix+missionID+":", nil, limit)
	if err != nil {
		return nil, err
	}
	reports := make([]*Report, 0, len(kvs))
	for _, kv := range kvs {
		var rep Report
		if err := json.Unmarshal(kv.Value, &rep); err != nil {
			return nil, fmt.Errorf("report %q: %w", kv.Key, err)
		}
		reports = append(reports, &rep)
	}
	return reports, nil
}

func validate(m *Mission) error {
	if strings.TrimSpace(m.Intent) == "" {
		return fmt.Errorf("intent is required")
	}
	if strings.ContainsAny(m.Intent, "\r\n") {
		return fmt.Errorf("intent must be a single line")
	}
	if strings.TrimSpace(m.HITLPolicyName) == "" {
		return fmt.Errorf("hitlPolicyName is required: a mission must name its envelope")
	}
	return validateStatus(m.Status)
}

func validateStatus(status Status) error {
	switch status {
	case StatusOpen, StatusLanded, StatusDerailed, StatusAbandoned:
		return nil
	default:
		return fmt.Errorf("invalid status %q: must be one of open|landed|derailed|abandoned", status)
	}
}

func validateReport(report *Report) error {
	if err := validateReportKind(report.Kind); err != nil {
		return err
	}
	if strings.TrimSpace(report.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if strings.ContainsAny(report.Summary, "\r\n") {
		return fmt.Errorf("summary must be a single line")
	}
	return nil
}

func validateReportKind(kind ReportKind) error {
	switch kind {
	case ReportKindProgress, ReportKindFinding, ReportKindBlocker, ReportKindResult:
		return nil
	default:
		return fmt.Errorf("invalid report kind %q: must be one of progress|finding|blocker|result", kind)
	}
}
