// Package missionservice stores mission records — the durable manifest half of
// the fleet manager (see docs/development/blueprints/beam/fleet-manager.md, slice
// F4). A mission is a one-line intent bound to work: the declared agent it runs
// on and the sessions/instances it spawned, plus a lifecycle status. Missions
// carry identity and references only, never content or artifacts (those hang off
// a mission by reference in a later slice).
//
// Storage is runtimetypes KV records keyed by mission id under a shared prefix
// (pattern of acpsvc's acp:session_* keys, runtime/acpsvc/external.go), listed
// server-side via the store's prefix scan — zero migration. The validated-CRUD
// shape mirrors runtime/agentregistryservice so the two registries stay easy to
// compare.
package missionservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// Mission is a durable note bound to fleet work. It may outlive its sessions and
// instances and remains listed while open.
type Mission struct {
	ID          string    `json:"id"`
	Intent      string    `json:"intent"`
	AgentName   string    `json:"agentName"`
	SessionIDs  []string  `json:"sessionIds"`
	InstanceIDs []string  `json:"instanceIds"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Service exposes validated CRUD over mission records plus Bind, which attaches a
// session and/or instance to an existing mission.
type Service interface {
	Create(ctx context.Context, m *Mission) error
	Get(ctx context.Context, id string) (*Mission, error)
	List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*Mission, error)
	Update(ctx context.Context, m *Mission) error
	Delete(ctx context.Context, id string) error
	Bind(ctx context.Context, id string, sessionID, instanceID string) (*Mission, error)
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

// Create validates m (intent, status), assigns an id when absent, forces the
// status to open, stamps timestamps, and persists it.
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
	if m.SessionIDs == nil {
		m.SessionIDs = []string{}
	}
	if m.InstanceIDs == nil {
		m.InstanceIDs = []string{}
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

// Update validates m and persists intent/status/reference changes to an existing
// mission. An unknown id surfaces as libdb.ErrNotFound. The caller owns m's
// CreatedAt (typically read via Get); UpdatedAt is restamped here.
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

// Bind appends a sessionID and/or instanceID to an existing mission, ignoring
// values already present, and returns the updated mission. At least one id is
// required. An unknown mission id surfaces as libdb.ErrNotFound.
func (s *service) Bind(ctx context.Context, id string, sessionID, instanceID string) (*Mission, error) {
	if sessionID == "" && instanceID == "" {
		return nil, fmt.Errorf("bind requires a sessionId or instanceId")
	}
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if sessionID != "" {
		m.SessionIDs = appendUnique(m.SessionIDs, sessionID)
	}
	if instanceID != "" {
		m.InstanceIDs = appendUnique(m.InstanceIDs, instanceID)
	}
	m.UpdatedAt = time.Now().UTC()
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

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

func validate(m *Mission) error {
	if strings.TrimSpace(m.Intent) == "" {
		return fmt.Errorf("intent is required")
	}
	if strings.ContainsAny(m.Intent, "\r\n") {
		return fmt.Errorf("intent must be a single line")
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
