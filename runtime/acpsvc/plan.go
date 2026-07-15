package acpsvc

import (
	libacp "github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/taskengine"
)

// planTracker mirrors one prompt turn's chain execution as an ACP plan: every
// chain task is one entry, statuses advance as step events arrive, and each
// change re-sends the complete plan (ACP plan updates replace the previous
// plan wholesale). This is the protocol-native equivalent of Beam's timeline
// rail — any conformant client (Zed renders plans natively) gets the chain's
// progress without a bespoke endpoint.
type planTracker struct {
	entries []libacp.PlanEntry
	index   map[string]int // chain task ID → entries index
	started bool           // at least one step event was applied
}

// newPlanTracker builds the pending plan from the chain definition. Chains
// with fewer than two tasks return nil: a one-step "plan" is noise.
func newPlanTracker(chain *taskengine.TaskChainDefinition) *planTracker {
	if chain == nil || len(chain.Tasks) < 2 {
		return nil
	}
	p := &planTracker{
		entries: make([]libacp.PlanEntry, 0, len(chain.Tasks)),
		index:   make(map[string]int, len(chain.Tasks)),
	}
	for _, task := range chain.Tasks {
		content := task.Description
		if content == "" {
			content = task.ID
		}
		p.index[task.ID] = len(p.entries)
		p.entries = append(p.entries, libacp.PlanEntry{
			Content:  content,
			Priority: libacp.PlanPriorityMedium,
			Status:   libacp.PlanStatusPending,
		})
	}
	return p
}

// apply advances the plan from a task event and reports whether the plan
// changed (i.e. a plan update should be sent).
func (p *planTracker) apply(ev taskengine.TaskEvent) bool {
	if p == nil {
		return false
	}
	switch ev.Kind {
	case taskengine.TaskEventStepStarted:
		return p.setStatus(ev.TaskID, libacp.PlanStatusInProgress)
	case taskengine.TaskEventStepCompleted, taskengine.TaskEventStepFailed:
		// ACP plan entries know pending/in_progress/completed; a failed step
		// still ended — the failure itself rides the tool-call card.
		return p.setStatus(ev.TaskID, libacp.PlanStatusCompleted)
	case taskengine.TaskEventChainCompleted, taskengine.TaskEventChainFailed:
		// The turn is over: tasks on branches that were never taken stay
		// "pending" forever otherwise, so the final plan keeps only the
		// entries that actually ran.
		return p.pruneUnstarted()
	}
	return false
}

func (p *planTracker) setStatus(taskID string, status libacp.PlanEntryStatus) bool {
	i, ok := p.index[taskID]
	if !ok {
		return false
	}
	p.started = true
	if p.entries[i].Status == status {
		return false
	}
	p.entries[i].Status = status
	return true
}

// pruneUnstarted drops entries still pending at chain end. Returns false when
// nothing ran at all (no plan was ever shown as active, so no final update).
func (p *planTracker) pruneUnstarted() bool {
	if !p.started {
		return false
	}
	kept := p.entries[:0]
	pruned := false
	for _, e := range p.entries {
		if e.Status == libacp.PlanStatusPending {
			pruned = true
			continue
		}
		kept = append(kept, e)
	}
	p.entries = kept
	return pruned
}

// snapshot returns a copy safe to hand to the wire layer.
func (p *planTracker) snapshot() []libacp.PlanEntry {
	if p == nil {
		return nil
	}
	out := make([]libacp.PlanEntry, len(p.entries))
	copy(out, p.entries)
	return out
}
