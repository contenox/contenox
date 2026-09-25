package acpsvc

import (
	"context"
	"sort"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/libacp"
)

// ModelUsage is the per-model slice of a session's usage record. Provider is
// the backend that served the calls; tokens are provider-reported.
type ModelUsage struct {
	Provider string `json:"provider,omitempty"`
	Calls    int64  `json:"calls"`

	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	ThinkingTokens   int64 `json:"thinkingTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
}

// SessionUsage is one session's lifetime usage, reduced from the engine event
// stream. It is the accounting record later surfaces and the EE proxy's
// per-user billing read from: every token field is provider-reported, never
// estimated, and zero means the provider did not report it.
type SessionUsage struct {
	SessionID string `json:"sessionId"`

	// Turns counts user-prompt chain runs; Steps counts tool executions.
	Turns int64 `json:"turns"`
	Steps int64 `json:"steps"`

	// ModelCalls counts model calls; LlmMs is wall time inside model calls,
	// ToolMs wall time inside tool-execution steps (including approval waits).
	ModelCalls int64 `json:"modelCalls"`
	LlmMs      int64 `json:"llmMs"`
	ToolMs     int64 `json:"toolMs"`

	// TtftSamples/TtftMs yield the average first-token latency per model call
	// (ms/sample); calls whose stream died before a first chunk contribute no
	// sample. StreamMs is the summed output-stream duration, so output
	// throughput is OutputTokens/StreamMs.
	TtftSamples int64 `json:"ttftSamples"`
	TtftMs      int64 `json:"ttftMs"`
	StreamMs    int64 `json:"streamMs"`

	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	ThinkingTokens   int64 `json:"thinkingTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`

	// ByModel splits the token totals per model for later per-tier export.
	ByModel map[string]ModelUsage `json:"byModel,omitempty"`

	stepStart  time.Time
	stepOpen   bool
	firstChunk time.Time
	model      string
	provider   string
	toolOpen   bool
	toolStart  time.Time
}

// NewSessionUsage returns an empty usage record for one session.
func NewSessionUsage(sessionID string) *SessionUsage {
	return &SessionUsage{SessionID: sessionID, ByModel: map[string]ModelUsage{}}
}

// Observe folds one engine event into the record. The reducer is a state
// machine over the engine's ordering contract (step_started … step_chunk …
// step_stream_end … step_completed, tool steps between), so events must be
// fed in the order the engine emitted them.
func (u *SessionUsage) Observe(ev taskengine.TaskEvent) {
	switch ev.Kind {
	case taskengine.TaskEventChainStarted:
		u.Turns++
	case taskengine.TaskEventStepStarted:
		switch {
		case isToolRunHandler(ev.TaskHandler):
			u.toolOpen = true
			u.toolStart = ev.Timestamp
		case isModelStreamHandler(ev.TaskHandler):
			u.stepOpen = true
			u.stepStart = ev.Timestamp
			u.firstChunk = time.Time{}
		}
	case taskengine.TaskEventStepChunk:
		if u.stepOpen && u.firstChunk.IsZero() {
			u.firstChunk = ev.Timestamp
		}
	case taskengine.TaskEventStepStreamEnd:
		u.closeModelCall(ev)
	case taskengine.TaskEventToolCallPending:
		u.Steps++
	case taskengine.TaskEventStepCompleted, taskengine.TaskEventStepFailed:
		if u.toolOpen {
			u.ToolMs += msBetween(u.toolStart, ev.Timestamp)
			u.toolOpen = false
		}
		u.resetStep()
	}
}

// isModelStreamHandler reports handlers whose step streams a model response
// (chat and route steps both end with a step_stream_end carrying usage).
func isModelStreamHandler(handler string) bool {
	switch taskengine.TaskHandler(handler) {
	case taskengine.HandleChatCompletion, taskengine.HandleRoute:
		return true
	}
	return false
}

// isToolRunHandler reports the one step handler that exists to run tools.
func isToolRunHandler(handler string) bool {
	return taskengine.TaskHandler(handler) == taskengine.HandleExecuteToolCalls
}

func (u *SessionUsage) closeModelCall(ev taskengine.TaskEvent) {
	if !u.stepOpen {
		return
	}
	u.ModelCalls++
	u.LlmMs += msBetween(u.stepStart, ev.Timestamp)
	if !u.firstChunk.IsZero() {
		u.TtftSamples++
		u.TtftMs += msBetween(u.stepStart, u.firstChunk)
		u.StreamMs += msBetween(u.firstChunk, ev.Timestamp)
	}
	u.resetStep()
	if ev.Usage == nil {
		return
	}
	u.InputTokens += int64(ev.Usage.Prompt)
	u.OutputTokens += int64(ev.Usage.Completion)
	u.ThinkingTokens += int64(ev.Usage.Thinking)
	u.CacheReadTokens += int64(ev.Usage.CacheRead)
	u.CacheWriteTokens += int64(ev.Usage.CacheWrite)
	mu := u.ByModel[ev.ModelName]
	if mu.Calls == 0 {
		mu.Provider = ev.ProviderType
	}
	mu.Calls++
	mu.InputTokens += int64(ev.Usage.Prompt)
	mu.OutputTokens += int64(ev.Usage.Completion)
	mu.ThinkingTokens += int64(ev.Usage.Thinking)
	mu.CacheReadTokens += int64(ev.Usage.CacheRead)
	mu.CacheWriteTokens += int64(ev.Usage.CacheWrite)
	u.ByModel[ev.ModelName] = mu
}

func (u *SessionUsage) resetStep() {
	u.stepOpen = false
	u.stepStart = time.Time{}
	u.firstChunk = time.Time{}
}

func msBetween(from, to time.Time) int64 {
	if to.IsZero() || from.IsZero() {
		return 0
	}
	d := to.Sub(from)
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}

// Snapshot renders the record in the ACP wire shape every client consumes.
// ByModel is sorted by model name so snapshots are deterministic.
func (u *SessionUsage) Snapshot() libacp.SessionStats {
	s := libacp.SessionStats{
		SessionID:        u.SessionID,
		Turns:            u.Turns,
		Steps:            u.Steps,
		ModelCalls:       u.ModelCalls,
		LlmMs:            u.LlmMs,
		ToolMs:           u.ToolMs,
		TtftSamples:      u.TtftSamples,
		TtftMs:           u.TtftMs,
		StreamMs:         u.StreamMs,
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		ThinkingTokens:   u.ThinkingTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
	}
	if len(u.ByModel) == 0 {
		return s
	}
	models := make([]string, 0, len(u.ByModel))
	for m := range u.ByModel {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		mu := u.ByModel[m]
		s.ByModel = append(s.ByModel, libacp.SessionModelStats{
			Model:            m,
			Provider:         mu.Provider,
			Calls:            mu.Calls,
			InputTokens:      mu.InputTokens,
			OutputTokens:     mu.OutputTokens,
			ThinkingTokens:   mu.ThinkingTokens,
			CacheReadTokens:  mu.CacheReadTokens,
			CacheWriteTokens: mu.CacheWriteTokens,
		})
	}
	return s
}

// observeAndEmitUsage folds one engine event into the session's usage record
// and pushes a usage_stats snapshot after each model call, tool step, or
// chain completion, so clients stay live without per-chunk chatter.
func (t *Transport) observeAndEmitUsage(ctx context.Context, sid libacp.SessionID, ev taskengine.TaskEvent) {
	t.usageMu.Lock()
	if t.usage == nil {
		// A bare Transport (unit-test construction) has no constructor-run
		// map initialisation; publishing must never panic on it.
		t.usage = make(map[libacp.SessionID]*SessionUsage)
	}
	u := t.usage[sid]
	if u == nil {
		u = NewSessionUsage(string(sid))
		t.usage[sid] = u
	}
	u.Observe(ev)
	t.usageMu.Unlock()

	switch ev.Kind {
	case taskengine.TaskEventStepStreamEnd, taskengine.TaskEventStepCompleted, taskengine.TaskEventChainCompleted:
		stats := u.Snapshot()
		t.sendUpdate(ctx, libacp.SessionNotification{
			SessionID: sid,
			Update: libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateUsageStats,
				Stats:         &stats,
			},
		})
	}
}
