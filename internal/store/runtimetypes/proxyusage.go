package runtimetypes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/google/uuid"
)

// Metering scopes: who a snapshot's cumulative counts. A scope is a value, so a
// new one is another row and never another column.
const (
	UsageScopeClient  = "client"
	UsageScopeSession = "session"
	UsageScopeGlobal  = "global"
)

// Metering windows: the period a snapshot's cumulative counts over.
const (
	UsageWindowFiveHour = "5h"
	UsageWindowWeek     = "week"
	UsageWindowMonth    = "month"
	UsageWindowTotal    = "total"
)

// UsageScope names one metered scope: a client's use of one model over one
// window, or the deployment's.
type UsageScope struct {
	Scope      string
	ScopeID    string
	Model      string
	WindowKind string
}

// UsageSnapshot is a scope's totals through the newest metered turn — the whole
// answer to "how much has this scope spent", read as one row.
type UsageSnapshot struct {
	NID              int64
	PromptTokens     int64
	CompletionTokens int64
	ThinkingTokens   int64
	TotalTokens      int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	EffectiveInput   int64
	EffectiveOutput  int64
	CostMicrodollars int64
	ImageCount       int64
	AudioBytes       int64
	WindowStart      time.Time
	RecordedAt       time.Time
}

// EffectiveTokens is the discounted input and output the burst window meters.
func (u UsageSnapshot) EffectiveTokens() int64 { return u.EffectiveOutput + u.EffectiveInput }

// ProxyUsage is one metered turn: what a client's model call consumed upstream.
// WindowStart is derived from the turn's time and the window being counted, so
// the caller states the turn and nothing about buckets.
type ProxyUsage struct {
	SessionID        string
	KeyHash          string
	ClientID         string
	Model            string
	PromptTokens     int64
	CompletionTokens int64
	ThinkingTokens   int64
	TotalTokens      int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	EffectiveInput   int64
	EffectiveOutput  int64
	CostMicrodollars int64
	ImageCount       int64
	AudioBytes       int64
	DurationMs       int64
	FinishReason     string
	RecordedAt       time.Time
}

// UsageStore is the metering log: an append-only table of cumulative snapshots,
// one row set per metered turn. It holds a DBManager rather than an Exec because
// a turn's sequence bump and its snapshot rows share one transaction.
type UsageStore interface {
	// RecordUsage appends one snapshot per scope the turn is metered into.
	RecordUsage(ctx context.Context, u ProxyUsage) error
	// ReadUsage returns the scope's newest snapshot, or a zero snapshot when the
	// window has rolled. It never aggregates.
	ReadUsage(ctx context.Context, scope UsageScope, now time.Time) (UsageSnapshot, error)
	// UsageByModel is the deployment's all-time totals, one entry per model.
	UsageByModel(ctx context.Context) ([]UsageModelTotal, error)
	// UsageByScope returns lifetime totals by model for a client or session.
	UsageByScope(ctx context.Context, scope, scopeID string) ([]UsageModelTotal, error)
}

// UsageModelTotal is one model's cumulative usage for a scope.
type UsageModelTotal struct {
	Model    string
	Snapshot UsageSnapshot
}

type usageStore struct {
	db libdb.DBManager
}

// NewUsageStore returns the metering log over db.
func NewUsageStore(db libdb.DBManager) UsageStore {
	if db == nil {
		panic("SERVER BUG: runtimetypes.NewUsageStore called with nil db")
	}
	return &usageStore{db: db}
}

// usageScopes is every scope one turn is metered into. A scope earns its row by
// being read: the burst, weekly and monthly ceilings, the per-client lifetime
// total, and the deployment's weekly and lifetime totals.
var usageScopes = []struct{ Scope, WindowKind string }{
	{UsageScopeClient, UsageWindowFiveHour},
	{UsageScopeClient, UsageWindowWeek},
	{UsageScopeClient, UsageWindowMonth},
	{UsageScopeClient, UsageWindowTotal},
	{UsageScopeGlobal, UsageWindowWeek},
	{UsageScopeGlobal, UsageWindowTotal},
}

// RecordUsage appends this turn's snapshot for every scope it counts in. The
// sequence bump, the reads of the previous snapshots and the inserts share one
// transaction, so two concurrent turns cannot both build on the same
// predecessor and lose one of the two.
func (s *usageStore) RecordUsage(ctx context.Context, u ProxyUsage) error {
	if u.KeyHash == "" || u.ClientID == "" || u.Model == "" {
		return errors.New("store: proxy usage requires key_hash, client_id and model")
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if u.RecordedAt.IsZero() {
		u.RecordedAt = time.Now().UTC()
	}
	u.RecordedAt = u.RecordedAt.UTC()

	exec, commit, release, err := s.db.WithTransaction(ctx)
	if err != nil {
		return fmt.Errorf("store: record proxy usage tx: %w", err)
	}
	defer func() { _ = release() }()

	if _, err := exec.ExecContext(ctx, `UPDATE event_nid_seq SET last_nid = last_nid + 1 WHERE id = 1`); err != nil {
		return fmt.Errorf("store: bump proxy usage nid: %w", err)
	}
	var nid int64
	if err := exec.QueryRowContext(ctx, `SELECT last_nid FROM event_nid_seq WHERE id = 1`).Scan(&nid); err != nil {
		return fmt.Errorf("store: read proxy usage nid: %w", err)
	}

	scopes := usageScopes
	if u.SessionID != "" {
		scopes = append(append([]struct{ Scope, WindowKind string }{}, usageScopes...), struct{ Scope, WindowKind string }{UsageScopeSession, UsageWindowTotal})
	}
	for _, scope := range scopes {
		previous, found, err := readUsageSnapshot(ctx, exec, UsageScope{
			Scope:      scope.Scope,
			ScopeID:    usageScopeID(scope.Scope, u),
			Model:      u.Model,
			WindowKind: scope.WindowKind,
		})
		if err != nil {
			return err
		}
		windowStart := UsageWindowStart(scope.WindowKind, u.RecordedAt)
		if found && previous.WindowStart.Equal(windowStart) {
			previous = addUsage(previous, u)
		} else {
			previous = usageFrom(u, windowStart)
		}
		if _, err := exec.ExecContext(ctx, `
			INSERT INTO proxy_usage (
				nid, id, scope, scope_id, key_hash, client_id, model, window_kind, window_start,
				prompt_tokens, completion_tokens, thinking_tokens, total_tokens, cache_read_tokens, cache_write_tokens,
				effective_input, effective_output, cost_microdollars, image_count, audio_bytes, duration_ms, finish_reason,
				cum_prompt_tokens, cum_completion_tokens, cum_thinking_tokens, cum_total_tokens, cum_cache_read_tokens,
				cum_cache_write_tokens, cum_effective_input, cum_effective_output, cum_cost_microdollars, cum_image_count,
				cum_audio_bytes, recorded_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
				$19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34)
		`, nid, uuid.New().String(), scope.Scope, usageScopeID(scope.Scope, u), u.KeyHash, u.ClientID,
			u.Model, scope.WindowKind, windowStart,
			u.PromptTokens, u.CompletionTokens, u.ThinkingTokens, u.TotalTokens, u.CacheReadTokens, u.CacheWriteTokens,
			u.EffectiveInput, u.EffectiveOutput, u.CostMicrodollars, u.ImageCount, u.AudioBytes, u.DurationMs, nullString(u.FinishReason),
			previous.PromptTokens, previous.CompletionTokens, previous.ThinkingTokens, previous.TotalTokens, previous.CacheReadTokens,
			previous.CacheWriteTokens, previous.EffectiveInput, previous.EffectiveOutput, previous.CostMicrodollars, previous.ImageCount,
			previous.AudioBytes, u.RecordedAt); err != nil {
			return fmt.Errorf("store: append proxy usage snapshot: %w", err)
		}
	}
	if err := commit(ctx); err != nil {
		return fmt.Errorf("store: record proxy usage commit: %w", err)
	}
	return nil
}

// ReadUsage returns the scope's newest snapshot. A snapshot whose window is not
// the window now belongs to is stale, so the answer is zero — the window rolled
// and nothing has been counted in the new one yet.
func (s *usageStore) ReadUsage(ctx context.Context, scope UsageScope, now time.Time) (UsageSnapshot, error) {
	snapshot, found, err := readUsageSnapshot(ctx, s.db.WithoutTransaction(), scope)
	if err != nil {
		return UsageSnapshot{}, err
	}
	if !found {
		return UsageSnapshot{WindowStart: UsageWindowStart(scope.WindowKind, now)}, nil
	}
	if !snapshot.WindowStart.Equal(UsageWindowStart(scope.WindowKind, now)) {
		return UsageSnapshot{WindowStart: UsageWindowStart(scope.WindowKind, now)}, nil
	}
	return snapshot, nil
}

// UsageByModel is the deployment's all-time totals, newest snapshot per model.
// The newest row per model is taken in Go rather than by a MAX in SQL: the
// ordering is the index's, and no aggregate belongs in this read either.
func (s *usageStore) UsageByModel(ctx context.Context) ([]UsageModelTotal, error) {
	return s.UsageByScope(ctx, UsageScopeGlobal, "")
}

// UsageByScope returns lifetime totals by model for the requested scope.
func (s *usageStore) UsageByScope(ctx context.Context, scope, scopeID string) ([]UsageModelTotal, error) {
	rows, err := s.db.WithoutTransaction().QueryContext(ctx, `
		SELECT model, `+usageCumulativeColumns+`
		FROM proxy_usage
		WHERE scope = $1 AND scope_id = $2 AND window_kind = $3
		ORDER BY model ASC, nid DESC
	`, scope, scopeID, UsageWindowTotal)
	if err != nil {
		return nil, fmt.Errorf("store: usage by model: %w", err)
	}
	defer rows.Close()
	out := []UsageModelTotal{}
	seen := map[string]bool{}
	for rows.Next() {
		var total UsageModelTotal
		if err := scanUsageCumulative(rows, &total.Model, &total.Snapshot); err != nil {
			return nil, err
		}
		if seen[total.Model] {
			continue
		}
		seen[total.Model] = true
		out = append(out, total)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: usage by model iteration: %w", err)
	}
	return out, nil
}

// UsageWindowStart is the bucket a window's counts accrue in, in UTC. A total
// has no bucket, which is what the zero time says.
//
// The burst window is aligned to the UTC day (00:00, 05:00, 10:00, 15:00,
// 20:00) rather than to an epoch multiple of five hours: an epoch bucket lands
// on clock times that move from day to day, so a quota that refills at 13:00
// today and 08:00 tomorrow is one nobody can predict. The day's last bucket is
// four hours for the same reason — the alternative is a boundary that drifts.
func UsageWindowStart(kind string, now time.Time) time.Time {
	now = now.UTC()
	switch kind {
	case UsageWindowFiveHour:
		day := now.Truncate(24 * time.Hour)
		return day.Add(time.Duration(now.Hour()/5*5) * time.Hour)
	case UsageWindowWeek:
		day := now.Truncate(24 * time.Hour)
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset)
	case UsageWindowMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
}

func usageScopeID(scope string, u ProxyUsage) string {
	if scope == UsageScopeSession {
		return SessionUsageScopeID(u.ClientID, u.SessionID)
	}
	if scope == UsageScopeGlobal {
		return ""
	}
	return u.ClientID
}

const usageCumulativeColumns = `cum_prompt_tokens, cum_completion_tokens, cum_thinking_tokens, cum_total_tokens,
	cum_cache_read_tokens, cum_cache_write_tokens, cum_effective_input, cum_effective_output, cum_cost_microdollars,
	cum_image_count, cum_audio_bytes, window_start, recorded_at, nid`

func readUsageSnapshot(ctx context.Context, exec libdb.Exec, scope UsageScope) (UsageSnapshot, bool, error) {
	var snapshot UsageSnapshot
	err := exec.QueryRowContext(ctx, `
		SELECT `+usageCumulativeColumns+`
		FROM proxy_usage
		WHERE scope = $1 AND scope_id = $2 AND model = $3 AND window_kind = $4
		ORDER BY nid DESC
		LIMIT 1
	`, scope.Scope, scope.ScopeID, scope.Model, scope.WindowKind).
		Scan(&snapshot.PromptTokens, &snapshot.CompletionTokens, &snapshot.ThinkingTokens, &snapshot.TotalTokens,
			&snapshot.CacheReadTokens, &snapshot.CacheWriteTokens, &snapshot.EffectiveInput,
			&snapshot.EffectiveOutput, &snapshot.CostMicrodollars, &snapshot.ImageCount, &snapshot.AudioBytes, &snapshot.WindowStart, &snapshot.RecordedAt, &snapshot.NID)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return UsageSnapshot{}, false, nil
		}
		return UsageSnapshot{}, false, fmt.Errorf("store: read usage snapshot: %w", err)
	}
	snapshot.WindowStart = snapshot.WindowStart.UTC()
	snapshot.RecordedAt = snapshot.RecordedAt.UTC()
	return snapshot, true, nil
}

func scanUsageCumulative(rows *sql.Rows, model *string, into *UsageSnapshot) error {
	if err := rows.Scan(model, &into.PromptTokens, &into.CompletionTokens, &into.ThinkingTokens, &into.TotalTokens,
		&into.CacheReadTokens, &into.CacheWriteTokens, &into.EffectiveInput,
		&into.EffectiveOutput, &into.CostMicrodollars, &into.ImageCount, &into.AudioBytes, &into.WindowStart, &into.RecordedAt, &into.NID); err != nil {
		return fmt.Errorf("store: scan usage snapshot: %w", err)
	}
	into.WindowStart = into.WindowStart.UTC()
	into.RecordedAt = into.RecordedAt.UTC()
	return nil
}

func usageFrom(u ProxyUsage, windowStart time.Time) UsageSnapshot {
	return UsageSnapshot{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		ThinkingTokens:   u.ThinkingTokens,
		TotalTokens:      u.TotalTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		EffectiveInput:   u.EffectiveInput,
		EffectiveOutput:  u.EffectiveOutput,
		CostMicrodollars: u.CostMicrodollars,
		ImageCount:       u.ImageCount,
		AudioBytes:       u.AudioBytes,
		WindowStart:      windowStart,
		RecordedAt:       u.RecordedAt,
	}
}

func addUsage(previous UsageSnapshot, u ProxyUsage) UsageSnapshot {
	previous.PromptTokens += u.PromptTokens
	previous.CompletionTokens += u.CompletionTokens
	previous.ThinkingTokens += u.ThinkingTokens
	previous.TotalTokens += u.TotalTokens
	previous.CacheReadTokens += u.CacheReadTokens
	previous.CacheWriteTokens += u.CacheWriteTokens
	previous.EffectiveInput += u.EffectiveInput
	previous.EffectiveOutput += u.EffectiveOutput
	previous.CostMicrodollars += u.CostMicrodollars
	previous.ImageCount += u.ImageCount
	previous.AudioBytes += u.AudioBytes
	previous.RecordedAt = u.RecordedAt
	return previous
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SessionUsageScopeID separates sessions belonging to different clients.
func SessionUsageScopeID(clientID, sessionID string) string {
	return fmt.Sprintf("%d:%s%s", len(clientID), clientID, sessionID)
}
