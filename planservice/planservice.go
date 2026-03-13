// Package planservice manages AI-generated execution plans.
// Each plan is a named, ordered list of steps created from a free-text goal.
//
// Chains are passed per-call so the caller decides which planner / executor
// to use at runtime (selected by the API client).
package planservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/contenox/contenox/execservice"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/planstore"
	"github.com/contenox/contenox/taskengine"
	"github.com/contenox/contenox/vfsservice"
	"github.com/google/uuid"
)

// Service is the contract for managing plans.
type Service interface {
	// New generates a plan from goal using plannerChain, saves it as active.
	New(ctx context.Context, goal string, plannerChain *taskengine.TaskChainDefinition) (*planstore.Plan, []*planstore.PlanStep, string, error)

	// Replan replaces remaining pending steps using plannerChain.
	Replan(ctx context.Context, plannerChain *taskengine.TaskChainDefinition) ([]*planstore.PlanStep, string, error)

	// Next executes the next pending step using executorChain.
	Next(ctx context.Context, args Args, executorChain *taskengine.TaskChainDefinition) (string, string, error)

	// Retry puts a failed/skipped step back to pending (ordinal is 1-based).
	Retry(ctx context.Context, ordinal int) (string, error)

	// Skip marks a step as intentionally bypassed (ordinal is 1-based).
	Skip(ctx context.Context, ordinal int) (string, error)

	// Active returns the current active plan and its steps.
	Active(ctx context.Context) (*planstore.Plan, []*planstore.PlanStep, error)

	// Show returns the active plan rendered as Markdown.
	Show(ctx context.Context) (string, error)

	// List returns all plans oldest-first.
	List(ctx context.Context) ([]*planstore.Plan, error)

	// SetActive makes the named plan active (archives the previous active one).
	SetActive(ctx context.Context, planName string) error

	// Delete permanently removes a plan by name.
	Delete(ctx context.Context, planName string) error

	// Clean removes all completed or archived plans; returns count removed.
	Clean(ctx context.Context) (int, error)
}

// Args controls Next execution behaviour.
type Args struct {
	WithShell bool
	WithAuto  bool
}

type service struct {
	db     libdb.DBManager
	engine execservice.TasksEnvService
	vfs    vfsservice.Service
}

// New creates a Service. vfs may be nil (plan markdown writing is skipped).
func New(db libdb.DBManager, engine execservice.TasksEnvService, vfs vfsservice.Service) Service {
	return &service{db: db, engine: engine, vfs: vfs}
}

var _ Service = (*service)(nil)

// ── helpers ──────────────────────────────────────────────────────────────────

// activePlan returns the active plan and its steps using a single targeted query.
func (s *service) activePlan(ctx context.Context) (*planstore.Plan, []*planstore.PlanStep, error) {
	st := planstore.New(s.db.WithoutTransaction())
	plan, err := st.GetActivePlan(ctx)
	if errors.Is(err, planstore.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	steps, err := st.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		return nil, nil, err
	}
	return plan, steps, nil
}

// callPlanner calls plannerChain with goal and returns the parsed step list.
// It uses taskengine.ExtractJSONArray to robustly extract the JSON array from
// the LLM response regardless of preamble text or Markdown code fences.
func (s *service) callPlanner(ctx context.Context, goal string, chain *taskengine.TaskChainDefinition) ([]string, error) {
	out, outType, _, err := s.engine.Execute(ctx, chain, goal, taskengine.DataTypeString)
	if err != nil {
		return nil, fmt.Errorf("plannerChain execute: %w", err)
	}
	var raw string
	switch outType {
	case taskengine.DataTypeString:
		raw, _ = out.(string)
	case taskengine.DataTypeJSON:
		b, _ := json.Marshal(out)
		raw = string(b)
	default:
		raw = fmt.Sprintf("%v", out)
	}
	// Extract the outermost [...] block from the raw response.
	// This handles preamble text, code fences, and trailing commentary.
	extracted := taskengine.ExtractJSONArray(raw)
	var steps []string
	if err := json.Unmarshal([]byte(extracted), &steps); err != nil {
		return nil, fmt.Errorf("plannerChain output is not a JSON string array: %w (raw: %.500s)", err, raw)
	}
	return steps, nil
}

// callExecutor calls executorChain with the step description and returns result text.
func (s *service) callExecutor(ctx context.Context, step string, chain *taskengine.TaskChainDefinition) (string, error) {
	out, _, _, err := s.engine.Execute(ctx, chain, step, taskengine.DataTypeString)
	if err != nil {
		return "", fmt.Errorf("executorChain execute: %w", err)
	}
	switch v := out.(type) {
	case string:
		return v, nil
	default:
		b, _ := json.Marshal(v)
		return string(b), nil
	}
}

// renderMarkdown produces a Markdown checklist for the plan.
func renderMarkdown(plan *planstore.Plan, steps []*planstore.PlanStep) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Plan: %s\n\n", plan.Name))
	sb.WriteString(fmt.Sprintf("**Goal:** %s\n\n", plan.Goal))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", plan.Status))
	sb.WriteString("## Steps\n\n")
	for _, st := range steps {
		var marker string
		switch st.Status {
		case planstore.StepStatusCompleted:
			marker = "x"
		case planstore.StepStatusFailed:
			marker = "!"
		case planstore.StepStatusSkipped:
			marker = "-"
		default:
			marker = " "
		}
		sb.WriteString(fmt.Sprintf("- [%s] %d. %s\n", marker, st.Ordinal, st.Description))
		// Fix 11: TrimSpace prevents dangling empty blockquote lines.
		if result := strings.TrimSpace(st.ExecutionResult); result != "" {
			for _, line := range strings.Split(result, "\n") {
				sb.WriteString(fmt.Sprintf("  > %s\n", line))
			}
		}
	}
	return sb.String()
}

// writePlanVFS writes (or updates) plans/{name}.md in the VFS.
func (s *service) writePlanVFS(ctx context.Context, plan *planstore.Plan, steps []*planstore.PlanStep) {
	if s.vfs == nil {
		return
	}
	md := renderMarkdown(plan, steps)
	fileName := plan.Name + ".md"
	// Try to find existing file.
	existing, err := s.vfs.GetFilesByPath(ctx, fileName)
	if err == nil && len(existing) > 0 {
		f := existing[0]
		if _, err := s.vfs.UpdateFile(ctx, &vfsservice.File{
			ID:          f.ID,
			Data:        []byte(md),
			ContentType: "text/markdown",
		}); err != nil {
			log.Printf("planservice: vfs update %s: %v", fileName, err)
		}
		return
	}
	if _, err := s.vfs.CreateFile(ctx, &vfsservice.File{
		Name:        fileName,
		Data:        []byte(md),
		ContentType: "text/markdown",
		ParentID:    "",
	}); err != nil {
		log.Printf("planservice: vfs create %s: %v", fileName, err)
	}
}

// ── Service implementation ────────────────────────────────────────────────────

func (s *service) New(ctx context.Context, goal string, plannerChain *taskengine.TaskChainDefinition) (*planstore.Plan, []*planstore.PlanStep, string, error) {
	if goal == "" {
		return nil, nil, "", fmt.Errorf("goal is required")
	}
	if plannerChain == nil {
		return nil, nil, "", fmt.Errorf("plannerChain is required")
	}
	stepDescs, err := s.callPlanner(ctx, goal, plannerChain)
	if err != nil {
		return nil, nil, "", err
	}
	if len(stepDescs) == 0 {
		return nil, nil, "", fmt.Errorf("planner returned no steps")
	}

	planID := uuid.NewString()
	now := time.Now().UTC()
	plan := &planstore.Plan{
		ID:        planID,
		Name:      "plan-" + uuid.NewString()[:8],
		Goal:      goal,
		Status:    planstore.PlanStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var stepSlice []*planstore.PlanStep
	for i, desc := range stepDescs {
		stepSlice = append(stepSlice, &planstore.PlanStep{
			ID:          uuid.NewString(),
			PlanID:      planID,
			Ordinal:     i + 1,
			Description: desc,
			Status:      planstore.StepStatusPending,
		})
	}

	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	defer rTx()
	st := planstore.New(tx)
	if err := st.CreatePlan(ctx, plan); err != nil {
		return nil, nil, "", fmt.Errorf("create plan: %w", err)
	}
	if err := st.CreatePlanSteps(ctx, stepSlice...); err != nil {
		return nil, nil, "", fmt.Errorf("create steps: %w", err)
	}
	if err := commit(ctx); err != nil {
		return nil, nil, "", err
	}
	md := renderMarkdown(plan, stepSlice)
	s.writePlanVFS(ctx, plan, stepSlice)
	return plan, stepSlice, md, nil
}

// ── checkAndComplete (Fix 3 + 4) ─────────────────────────────────────────────

// checkAndComplete inspects allSteps and marks the plan as completed when all
// steps are done and none have failed. A plan with a failed step stays active
// so the user can call Retry or Skip.
// Must be called inside an open transaction (txSt).
func checkAndComplete(ctx context.Context, txSt planstore.Store, plan *planstore.Plan, allSteps []*planstore.PlanStep) error {
	allDone, hasFailed := true, false
	for _, step := range allSteps {
		switch step.Status {
		case planstore.StepStatusPending, planstore.StepStatusRunning:
			allDone = false
		case planstore.StepStatusFailed:
			hasFailed = true
		}
	}
	if allDone && !hasFailed {
		if err := txSt.UpdatePlanStatus(ctx, plan.ID, planstore.PlanStatusCompleted); err != nil {
			return fmt.Errorf("complete plan: %w", err)
		}
		plan.Status = planstore.PlanStatusCompleted
	}
	return nil
}

func (s *service) Replan(ctx context.Context, plannerChain *taskengine.TaskChainDefinition) ([]*planstore.PlanStep, string, error) {
	if plannerChain == nil {
		return nil, "", fmt.Errorf("plannerChain is required")
	}
	plan, steps, err := s.activePlan(ctx)
	if err != nil {
		return nil, "", err
	}
	if plan == nil {
		return nil, "", fmt.Errorf("no active plan")
	}

	// Fix 6a: include failed steps (and their error) so the LLM understands what went wrong.
	// Fix 6b: maxOrdinal only counts completed/skipped — pending steps will be deleted.
	var sb strings.Builder
	sb.WriteString(plan.Goal)
	sb.WriteString("\n\nProgress so far:\n")
	maxOrdinal := 0
	for _, st := range steps {
		switch st.Status {
		case planstore.StepStatusCompleted:
			if st.Ordinal > maxOrdinal {
				maxOrdinal = st.Ordinal
			}
			sb.WriteString(fmt.Sprintf("- [done] %d. %s\n", st.Ordinal, st.Description))
		case planstore.StepStatusSkipped:
			if st.Ordinal > maxOrdinal {
				maxOrdinal = st.Ordinal
			}
			sb.WriteString(fmt.Sprintf("- [skipped] %d. %s\n", st.Ordinal, st.Description))
		case planstore.StepStatusFailed:
			sb.WriteString(fmt.Sprintf("- [FAILED] %d. %s\n", st.Ordinal, st.Description))
			if st.ExecutionResult != "" {
				sb.WriteString(fmt.Sprintf("  Error: %s\n", strings.TrimSpace(st.ExecutionResult)))
			}
		}
	}
	sb.WriteString("\nGenerate only the remaining steps needed to achieve the goal.")

	newDescs, err := s.callPlanner(ctx, sb.String(), plannerChain)
	if err != nil {
		return nil, "", err
	}

	var newSteps []*planstore.PlanStep
	for i, desc := range newDescs {
		newSteps = append(newSteps, &planstore.PlanStep{
			ID:          uuid.NewString(),
			PlanID:      plan.ID,
			Ordinal:     maxOrdinal + i + 1,
			Description: desc,
			Status:      planstore.StepStatusPending,
		})
	}

	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, "", err
	}
	defer rTx()
	st := planstore.New(tx)
	if err := st.DeletePendingPlanSteps(ctx, plan.ID); err != nil {
		return nil, "", fmt.Errorf("delete pending: %w", err)
	}
	if err := st.CreatePlanSteps(ctx, newSteps...); err != nil {
		return nil, "", fmt.Errorf("create new steps: %w", err)
	}
	if err := commit(ctx); err != nil {
		return nil, "", err
	}

	// Reload all steps for markdown.
	allSteps, err := planstore.New(s.db.WithoutTransaction()).ListPlanSteps(ctx, plan.ID)
	if err != nil {
		return nil, "", err
	}
	md := renderMarkdown(plan, allSteps)
	s.writePlanVFS(ctx, plan, allSteps)
	return newSteps, md, nil
}

func (s *service) Next(ctx context.Context, args Args, executorChain *taskengine.TaskChainDefinition) (string, string, error) {
	if executorChain == nil {
		return "", "", fmt.Errorf("executorChain is required")
	}

	// 1. Read active plan (no lock: just a SELECT LIMIT 1).
	st := planstore.New(s.db.WithoutTransaction())
	plan, err := st.GetActivePlan(ctx)
	if errors.Is(err, planstore.ErrNotFound) {
		return "", "", fmt.Errorf("no active plan")
	}
	if err != nil {
		return "", "", err
	}

	// 2. Atomically claim the next pending step (FOR UPDATE SKIP LOCKED).
	pending, err := st.ClaimNextPendingStep(ctx, plan.ID)
	if errors.Is(err, planstore.ErrNotFound) {
		return "", "", fmt.Errorf("no pending steps remaining")
	}
	if err != nil {
		return "", "", err
	}

	// 3. Execute LLM outside any transaction (can be long-running).
	result, execErr := s.callExecutor(ctx, pending.Description, executorChain)

	finalStatus := planstore.StepStatusCompleted
	finalResult := result
	if execErr != nil {
		finalStatus = planstore.StepStatusFailed
		finalResult = execErr.Error()
		result = ""
	}

	// Fix 1: Use WithoutCancel so cleanup always succeeds even if the caller's
	// context was canceled during the (potentially long) LLM call.
	cleanupCtx := context.WithoutCancel(ctx)
	tx, commit, rTx, txErr := s.db.WithTransaction(cleanupCtx)
	if txErr != nil {
		return "", "", txErr
	}
	defer rTx()
	txSt := planstore.New(tx)
	if err := txSt.UpdatePlanStepStatus(cleanupCtx, pending.ID, finalStatus, finalResult); err != nil {
		return "", "", fmt.Errorf("update step: %w", err)
	}
	allSteps, err := txSt.ListPlanSteps(cleanupCtx, plan.ID)
	if err != nil {
		return "", "", fmt.Errorf("list steps: %w", err)
	}
	// Fix 3: use checkAndComplete — plan stays active if any step failed.
	if err := checkAndComplete(cleanupCtx, txSt, plan, allSteps); err != nil {
		return "", "", err
	}
	if err := commit(cleanupCtx); err != nil {
		return "", "", err
	}
	md := renderMarkdown(plan, allSteps)
	s.writePlanVFS(ctx, plan, allSteps)
	if execErr != nil {
		return "", md, execErr
	}
	return result, md, nil
}

func (s *service) Retry(ctx context.Context, ordinal int) (string, error) {
	plan, steps, err := s.activePlan(ctx)
	if err != nil {
		return "", err
	}
	if plan == nil {
		return "", fmt.Errorf("no active plan")
	}
	var target *planstore.PlanStep
	for _, st := range steps {
		if st.Ordinal == ordinal {
			target = st
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("step %d not found", ordinal)
	}
	// Fix 5: wrap in transaction to keep step + plan updated_at in sync.
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return "", err
	}
	defer rTx()
	txSt := planstore.New(tx)
	if err := txSt.UpdatePlanStepStatus(ctx, target.ID, planstore.StepStatusPending, ""); err != nil {
		return "", err
	}
	if err := commit(ctx); err != nil {
		return "", err
	}
	// Fix 9: propagate errors, don't silently use a wrong plan's steps.
	allSteps, err := planstore.New(s.db.WithoutTransaction()).ListPlanSteps(ctx, plan.ID)
	if err != nil {
		return "", err
	}
	md := renderMarkdown(plan, allSteps)
	s.writePlanVFS(ctx, plan, allSteps)
	return md, nil
}

func (s *service) Skip(ctx context.Context, ordinal int) (string, error) {
	plan, steps, err := s.activePlan(ctx)
	if err != nil {
		return "", err
	}
	if plan == nil {
		return "", fmt.Errorf("no active plan")
	}
	var target *planstore.PlanStep
	for _, st := range steps {
		if st.Ordinal == ordinal {
			target = st
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("step %d not found", ordinal)
	}
	// Fix 5: wrap in transaction.
	// Fix 4: run checkAndComplete in same tx to handle last-step-skip.
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return "", err
	}
	defer rTx()
	txSt := planstore.New(tx)
	if err := txSt.UpdatePlanStepStatus(ctx, target.ID, planstore.StepStatusSkipped, "skipped"); err != nil {
		return "", err
	}
	allSteps, err := txSt.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		return "", err
	}
	if err := checkAndComplete(ctx, txSt, plan, allSteps); err != nil {
		return "", err
	}
	if err := commit(ctx); err != nil {
		return "", err
	}
	// Fix 9: propagate errors.
	md := renderMarkdown(plan, allSteps)
	s.writePlanVFS(ctx, plan, allSteps)
	return md, nil
}

func (s *service) Active(ctx context.Context) (*planstore.Plan, []*planstore.PlanStep, error) {
	return s.activePlan(ctx)
}

func (s *service) Show(ctx context.Context) (string, error) {
	plan, steps, err := s.activePlan(ctx)
	if err != nil {
		return "", err
	}
	if plan == nil {
		return "", fmt.Errorf("no active plan")
	}
	return renderMarkdown(plan, steps), nil
}

func (s *service) List(ctx context.Context) ([]*planstore.Plan, error) {
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer rTx()
	plans, err := planstore.New(tx).ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	if err := commit(ctx); err != nil {
		return nil, err
	}
	return plans, nil
}

func (s *service) SetActive(ctx context.Context, planName string) error {
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return err
	}
	defer rTx()
	st := planstore.New(tx)
	// Fix 8: single UPDATE instead of load-all + N individual UPDATEs.
	if err := st.ArchiveActivePlans(ctx); err != nil {
		return err
	}
	target, err := st.GetPlanByName(ctx, planName)
	if err != nil {
		return fmt.Errorf("plan %q not found: %w", planName, err)
	}
	if err := st.UpdatePlanStatus(ctx, target.ID, planstore.PlanStatusActive); err != nil {
		return err
	}
	return commit(ctx)
}

func (s *service) Delete(ctx context.Context, planName string) error {
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return err
	}
	defer rTx()
	st := planstore.New(tx)
	plan, err := st.GetPlanByName(ctx, planName)
	if err != nil {
		return fmt.Errorf("plan %q not found: %w", planName, err)
	}
	if err := st.DeletePlan(ctx, plan.ID); err != nil {
		return err
	}
	return commit(ctx)
}

func (s *service) Clean(ctx context.Context) (int, error) {
	// Fix 7: return 0 on error (not a misleading partial count).
	// Fix 8: single DELETE IN ('completed','archived') RETURNING instead of N+1 loops.
	n, err := planstore.New(s.db.WithoutTransaction()).DeleteFinishedPlans(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}
