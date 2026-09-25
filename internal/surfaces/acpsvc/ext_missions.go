package acpsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/services/missionservice"
	"github.com/contenox/contenox/internal/services/operatorinbox"
	libacp "github.com/contenox/contenox/libacp"
	libdb "github.com/contenox/contenox/libdbexec"
)

// The extension reads answer rows whose JSON field names are the contract every
// client renders; renaming one is a breaking wire change.

const (
	extMethodMissionsList    = "_contenox/missions/list"
	extMethodMissionsGet     = "_contenox/missions/get"
	extMethodMissionsReports = "_contenox/missions/reports"
	extMethodInboxList       = "_contenox/inbox/list"
)

// MissionRow is one durable mission's wire shape.
type MissionRow struct {
	ID              string        `json:"id"`
	Intent          string        `json:"intent"`
	AgentName       string        `json:"agentName"`
	HITLPolicyName  string        `json:"hitlPolicyName"`
	SessionID       string        `json:"sessionId,omitempty"`
	InstanceID      string        `json:"instanceId,omitempty"`
	ParentSessionID string        `json:"parentSessionId,omitempty"`
	Status          string        `json:"status"`
	StatusReason    string        `json:"statusReason,omitempty"`
	Plan            MissionPlan   `json:"plan"`
	PlanRevisions   []PlanSummary `json:"planRevisions,omitempty"`
	LastHeartbeat   *time.Time    `json:"lastHeartbeat,omitempty"`
	LastError       string        `json:"lastError,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

// MissionPlan is a mission's living plan: an ordered entry list owned by one
// planner, held as a reviewable record.
type MissionPlan struct {
	Entries     []PlanEntry `json:"entries"`
	Revision    int         `json:"revision"`
	Explanation string      `json:"explanation,omitempty"`
}

// PlanEntry is one plan step.
type PlanEntry struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// PlanSummary is one plan-revision summary.
type PlanSummary struct {
	Revision    int       `json:"revision"`
	Explanation string    `json:"explanation,omitempty"`
	Added       int       `json:"added"`
	Removed     int       `json:"removed"`
	Pending     int       `json:"pending"`
	InProgress  int       `json:"inProgress"`
	Completed   int       `json:"completed"`
	At          time.Time `json:"at"`
}

// ReportRow is one mission report's wire shape.
type ReportRow struct {
	ID        string    `json:"id"`
	MissionID string    `json:"missionId"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Detail    string    `json:"detail,omitempty"`
	Refs      []string  `json:"refs,omitempty"`
	Handover  *Handover `json:"handover,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Handover is the structured hand-off a mission attaches to a report.
type Handover struct {
	Outcome         string   `json:"outcome,omitempty"`
	Artifacts       []string `json:"artifacts,omitempty"`
	HandoverForNext string   `json:"handoverForNext,omitempty"`
	Caveats         string   `json:"caveats,omitempty"`
}

// OperatorItemRow is one operator-inbox item's wire shape.
type OperatorItemRow struct {
	ID              string     `json:"id"`
	MissionID       string     `json:"missionId"`
	AgentName       string     `json:"agentName,omitempty"`
	Intent          string     `json:"intent,omitempty"`
	ParentSessionID string     `json:"parentSessionId,omitempty"`
	Reason          string     `json:"reason"`
	Report          ReportRow  `json:"report"`
	CreatedAt       time.Time  `json:"createdAt"`
	Acked           bool       `json:"acked,omitempty"`
	AckedAt         *time.Time `json:"ackedAt,omitempty"`
}

func missionRow(m missionservice.Mission) MissionRow {
	entries := make([]PlanEntry, 0, len(m.Plan.Entries))
	for _, e := range m.Plan.Entries {
		entries = append(entries, PlanEntry{
			ID: e.ID, Content: e.Content,
			Status: string(e.Status), Priority: string(e.Priority),
		})
	}
	revs := make([]PlanSummary, 0, len(m.PlanRevisions))
	for _, r := range m.PlanRevisions {
		revs = append(revs, PlanSummary{
			Revision: r.Revision, Explanation: r.Explanation,
			Added: r.Added, Removed: r.Removed, Pending: r.Pending,
			InProgress: r.InProgress, Completed: r.Completed, At: r.At,
		})
	}
	return MissionRow{
		ID:              m.ID,
		Intent:          m.Intent,
		AgentName:       m.AgentName,
		HITLPolicyName:  m.HITLPolicyName,
		SessionID:       m.SessionID,
		InstanceID:      m.InstanceID,
		ParentSessionID: m.ParentSessionID,
		Status:          string(m.Status),
		StatusReason:    m.StatusReason,
		Plan:            MissionPlan{Entries: entries, Revision: m.Plan.Revision, Explanation: m.Plan.Explanation},
		PlanRevisions:   revs,
		LastHeartbeat:   m.LastHeartbeat,
		LastError:       m.LastError,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func reportRow(r missionservice.Report) ReportRow {
	row := ReportRow{
		ID: r.ID, MissionID: r.MissionID, Kind: string(r.Kind),
		Summary: r.Summary, Detail: r.Detail, Refs: r.Refs, CreatedAt: r.CreatedAt,
	}
	if r.Handover != nil {
		row.Handover = &Handover{
			Outcome: r.Handover.Outcome, Artifacts: r.Handover.Artifacts,
			HandoverForNext: r.Handover.HandoverForNext, Caveats: r.Handover.Caveats,
		}
	}
	return row
}

func operatorItemRow(it operatorinbox.Item) OperatorItemRow {
	return OperatorItemRow{
		ID: it.ID, MissionID: it.MissionID, AgentName: it.AgentName,
		Intent: it.Intent, ParentSessionID: it.ParentSessionID,
		Reason: string(it.Reason), Report: reportRow(it.Report),
		CreatedAt: it.CreatedAt, Acked: it.Acked, AckedAt: it.AckedAt,
	}
}

// parseCursor parses the optional created-at cursor (RFC 3339) for mission
// listing; an empty string means "start at the newest page".
func parseMissionCursor(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *Transport) handleMissionsList(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	if t.deps.Missions == nil {
		return nil, libacp.MethodNotFound(extMethodMissionsList + " is not enabled on this server")
	}
	var p struct {
		Limit           int    `json:"limit"`
		CreatedAtCursor string `json:"createdAtCursor"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, libacp.InvalidParams("missions/list: " + err.Error())
		}
	}
	cursor, err := parseMissionCursor(p.CreatedAtCursor)
	if err != nil {
		return nil, libacp.InvalidParams("missions/list: " + err.Error())
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 100
	}
	items, err := t.deps.Missions.List(ctx, cursor, limit)
	if err != nil {
		return nil, libacp.InternalError("missions/list: " + err.Error())
	}
	rows := make([]MissionRow, 0, len(items))
	for _, it := range items {
		if it != nil {
			rows = append(rows, missionRow(*it))
		}
	}
	out, _ := json.Marshal(rows)
	return out, nil
}

func (t *Transport) handleMissionsGet(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	if t.deps.Missions == nil {
		return nil, libacp.MethodNotFound(extMethodMissionsGet + " is not enabled on this server")
	}
	var p struct {
		MissionID string `json:"missionId"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, libacp.InvalidParams("missions/get: " + err.Error())
		}
	}
	if strings.TrimSpace(p.MissionID) == "" {
		return nil, libacp.InvalidParams("missions/get: missionId is required")
	}
	m, err := t.deps.Missions.Get(ctx, p.MissionID)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return nil, libacp.NewError(libacp.ErrResourceNotFound, "missions/get: "+err.Error())
		}
		return nil, libacp.InternalError("missions/get: " + err.Error())
	}
	out, _ := json.Marshal(missionRow(*m))
	return out, nil
}

func (t *Transport) handleMissionsReports(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	if t.deps.Missions == nil {
		return nil, libacp.MethodNotFound(extMethodMissionsReports + " is not enabled on this server")
	}
	var p struct {
		MissionID string `json:"missionId"`
		Limit     int    `json:"limit"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, libacp.InvalidParams("missions/reports: " + err.Error())
		}
	}
	if strings.TrimSpace(p.MissionID) == "" {
		return nil, libacp.InvalidParams("missions/reports: missionId is required")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 100
	}
	items, err := t.deps.Missions.ListReports(ctx, p.MissionID, limit)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return nil, libacp.NewError(libacp.ErrResourceNotFound, "missions/reports: "+err.Error())
		}
		return nil, libacp.InternalError("missions/reports: " + err.Error())
	}
	rows := make([]ReportRow, 0, len(items))
	for _, it := range items {
		if it != nil {
			rows = append(rows, reportRow(*it))
		}
	}
	out, _ := json.Marshal(rows)
	return out, nil
}

func (t *Transport) handleInboxList(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	if t.deps.Inbox == nil {
		return nil, libacp.MethodNotFound(extMethodInboxList + " is not enabled on this server")
	}
	var p struct {
		Limit int `json:"limit"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, libacp.InvalidParams("inbox/list: " + err.Error())
		}
	}
	items, err := t.deps.Inbox.List(ctx, p.Limit)
	if err != nil {
		return nil, libacp.InternalError("inbox/list: " + err.Error())
	}
	rows := make([]OperatorItemRow, 0, len(items))
	for _, it := range items {
		if it != nil {
			rows = append(rows, operatorItemRow(*it))
		}
	}
	out, _ := json.Marshal(rows)
	return out, nil
}
