// Package hitlservice evaluates approval policies for tool calls.
// Policy decisions (allow / deny / approve) are returned to the caller; the
// caller (typically a ToolsRepo decorator like localtools.HITLWrapper) is
// responsible for actually pausing execution and sourcing the human decision.
package hitlservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/google/uuid"

	libdb "github.com/contenox/runtime/libdbexec"
)

type KVReader interface {
	GetKV(ctx context.Context, key string, out interface{}) error
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, toolsName, toolName string, args map[string]any) (EvaluationResult, error)
}

// approvalStore is the durable persistence surface RequestApproval, Respond,
// SweepExpired, and ListPending need: create a pending row, look it up,
// compare-and-swap it into a terminal state, and list rows by state. It is a
// strict subset of runtimetypes.Store (see runtime/runtimetypes/hitl_approvals.go)
// — declared here so hitlservice depends on only the methods it calls, not the
// whole Store.
//
// service.store (the KVReader constructor argument) is type-asserted against
// this interface at construction time. Every production caller
// (contenoxcli/serve_cmd.go, enginesvc.buildTools) already passes a
// runtimetypes.Store there, so it is satisfied automatically; callers that
// pass a bare KVReader fake for Evaluate()-only use (agentview, the /files
// policy filter, most of this package's own policy_test.go-style tests)
// simply never exercise RequestApproval/Respond/SweepExpired/ListPending —
// see the nil checks at the top of each.
type approvalStore interface {
	CreateHITLApproval(ctx context.Context, a *runtimetypes.HITLApproval) error
	GetHITLApproval(ctx context.Context, id string) (*runtimetypes.HITLApproval, error)
	ResolveHITLApproval(ctx context.Context, id string, state runtimetypes.HITLApprovalState, resolution json.RawMessage, resolvedAt time.Time) error
	ListExpiredHITLApprovals(ctx context.Context, asOf time.Time, limit int) ([]*runtimetypes.HITLApproval, error)
	ListHITLApprovals(ctx context.Context, state runtimetypes.HITLApprovalState, createdAtCursor *time.Time, limit int) ([]*runtimetypes.HITLApproval, error)
}

type Service interface {
	PolicyEvaluator

	// RequestApproval durably records req as a pending ask, publishes the
	// approval_requested TaskEvent (unchanged shape/consumers), then blocks
	// until Respond answers it or the wait is bounded away (see
	// DefaultApprovalCeiling / SetApprovalCeiling): a matched rule's own
	// TimeoutS wins when set, otherwise the serve-level ceiling applies so an
	// ask nobody answers cannot block forever.
	RequestApproval(ctx context.Context, req ApprovalRequest, sink taskengine.TaskEventSink) (bool, error)

	// Respond transitions a pending approval to approved/denied and wakes any
	// requester parked on it in this process. It returns ErrApprovalNotFound,
	// ErrApprovalAlreadyResolved, or ErrApprovalExpired instead of a bare
	// false when approvalID cannot be answered — never a silent no-op.
	Respond(ctx context.Context, approvalID string, approved bool) error

	// SweepExpired resolves every pending approval whose deadline has
	// passed, applying its stored OnTimeout (default deny) exactly as
	// localtools.HITLWrapper's own on-timeout branch would. It is the
	// durability backstop for asks whose original requester is gone (a
	// process restart) — serve runs it on an interval; tests can call it
	// directly. Returns the number of rows it resolved.
	SweepExpired(ctx context.Context) (int, error)

	// ListPending is the read half of the durable ask C1 introduced: an ask
	// nobody can see is not answerable (fleet-consolidation.md slice C2,
	// defects D4/D5). It returns pending approvals newest first, bounded by
	// limit (the store's own MAXLIMIT when limit<=0; runtimetypes.
	// ErrLimitParamExceeded when limit is set above it), and always a
	// non-nil slice — a fleet with nothing pending must render empty, not
	// fail. Every field an operator needs to decide rides along on the
	// returned *runtimetypes.HITLApproval rows themselves (tool, args
	// summary, diff, policy name, and matched rule), since ListPending
	// hands back the durable row unprojected rather than a narrower DTO.
	ListPending(ctx context.Context, limit int) ([]*runtimetypes.HITLApproval, error)
}

// Sentinel errors Respond returns instead of a silent false — see Respond's
// doc and the Service interface doc above.
var (
	// ErrApprovalNotFound reports that approvalID does not exist in the store.
	ErrApprovalNotFound = errors.New("hitlservice: approval not found")
	// ErrApprovalAlreadyResolved reports that approvalID was already answered
	// (approved or denied) by an earlier Respond call.
	ErrApprovalAlreadyResolved = errors.New("hitlservice: approval already answered")
	// ErrApprovalExpired reports that approvalID's deadline passed and the
	// sweeper already resolved it via OnTimeout before this Respond landed.
	ErrApprovalExpired = errors.New("hitlservice: approval expired before it was answered")
)

const defaultPolicyName = "hitl-policy-default.json"

// DefaultApprovalCeiling bounds RequestApproval when the matched policy rule
// sets no TimeoutS of its own (Rule.TimeoutS == 0 today means "block
// indefinitely", see policy.go:86-87 — that is the unbounded-hang defect
// this ceiling repairs). An ask nobody answered within an hour is abandoned,
// and a late denial beats an eternal block. Override per-service via
// SetApprovalCeiling — contenoxcli/serve_cmd.go wires it to the
// HITL_APPROVAL_TIMEOUT serve setting.
const DefaultApprovalCeiling = time.Hour

type service struct {
	src            PolicySource
	tenantID       string
	store          KVReader
	tracker        libtracker.ActivityTracker
	fallbackPolicy string
	approvals      approvalStore

	mu              sync.Mutex
	pending         map[string]chan bool
	approvalCeiling time.Duration
}

// New constructs a hitlservice bound to a tenant. The tenantID is forwarded to
// every policy lookup the service performs.
func New(src PolicySource, tenantID string, store KVReader, tracker libtracker.ActivityTracker) Service {
	return NewWithDefaultPolicy(src, tenantID, store, tracker, defaultPolicyName)
}

func NewWithDefaultPolicy(src PolicySource, tenantID string, store KVReader, tracker libtracker.ActivityTracker, fallbackPolicy string) Service {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	if strings.TrimSpace(fallbackPolicy) == "" {
		fallbackPolicy = defaultPolicyName
	}
	svc := &service{
		src:            src,
		tenantID:       tenantID,
		store:          store,
		tracker:        tracker,
		fallbackPolicy: fallbackPolicy,
		pending:        make(map[string]chan bool),
	}
	if as, ok := store.(approvalStore); ok {
		svc.approvals = as
	}
	return svc
}

// SetApprovalCeiling overrides the serve-level approval-wait ceiling
// (DefaultApprovalCeiling otherwise) on svc, when svc was constructed by
// New/NewWithDefaultPolicy in this package. A no-op for any other Service
// implementation and for ceiling <= 0, so callers can pass an
// unparsed/unset config value without an extra branch.
func SetApprovalCeiling(svc Service, ceiling time.Duration) {
	if ceiling <= 0 {
		return
	}
	if s, ok := svc.(*service); ok {
		s.mu.Lock()
		s.approvalCeiling = ceiling
		s.mu.Unlock()
	}
}

func (s *service) ceiling() time.Duration {
	s.mu.Lock()
	d := s.approvalCeiling
	s.mu.Unlock()
	if d <= 0 {
		return DefaultApprovalCeiling
	}
	return d
}

var _ Service = (*service)(nil)

const kvPrefixHITLPolicy = "cli.hitl-policy-name"

// policyNameContextKey scopes an explicit per-request HITL policy name onto a
// context. A single hitlservice is shared across many callers — serve builds ONE
// behind every ACP WebSocket session — so per-session policy differentiation
// cannot live in service state. Instead each ACP prompt turn injects its
// session's resolved policy name into the turn context (see acpsvc/prompt.go),
// and Evaluate prefers it over the process-global cli.hitl-policy-name KV. A
// context WITHOUT this key is unchanged: single-session callers (the CLI,
// `contenox acp`, `contenox chat`) keep reading the global KV.
type policyNameContextKey struct{}

// WithPolicyName returns a context that pins HITL evaluation to policyName for
// this request only. An empty/whitespace policyName returns ctx unchanged, so a
// caller that resolves to "no override" (a defaulting session) leaves the
// existing global-KV/fallback chain intact.
func WithPolicyName(ctx context.Context, policyName string) context.Context {
	policyName = strings.TrimSpace(policyName)
	if policyName == "" {
		return ctx
	}
	return context.WithValue(ctx, policyNameContextKey{}, policyName)
}

// policyNameFromContext returns the context-scoped policy override, or "".
func policyNameFromContext(ctx context.Context) string {
	name, _ := ctx.Value(policyNameContextKey{}).(string)
	return strings.TrimSpace(name)
}

func (s *service) readActivePolicyName(ctx context.Context) string {
	var val string
	if err := s.store.GetKV(ctx, kvPrefixHITLPolicy, &val); err != nil {
		return ""
	}
	return strings.TrimSpace(val)
}

func (s *service) Evaluate(ctx context.Context, toolsName, toolName string, args map[string]any) (EvaluationResult, error) {
	reportErr, reportChange, end := s.tracker.Start(ctx, "hitl", "evaluate", "toolsName", toolsName, "toolName", toolName)
	defer end()
	// A per-request context override (an ACP session's chosen policy) wins over
	// the process-global active-policy KV so concurrent sessions behind ONE shared
	// service gate independently. Absent an override, fall through the existing
	// global-KV -> constructor fallback -> built-in default chain (unchanged for
	// single-session CLI callers).
	policyPath := policyNameFromContext(ctx)
	if policyPath == "" {
		policyPath = s.readActivePolicyName(ctx)
	}
	if policyPath == "" {
		policyPath = s.fallbackPolicy
	}
	if policyPath == "" {
		policyPath = defaultPolicyName
	}
	p, err := loadPolicy(ctx, s.src, s.tenantID, policyPath)
	if err != nil {
		reportErr(fmt.Errorf("hitl: falling back to built-in default policy: %w", err))
		p = defaultPolicy()
	}
	reportChange("policy", policyPath)
	result := evaluate(p, toolsName, toolName, args)
	result.PolicyName = policyPath
	return result, nil
}

func (s *service) RequestApproval(ctx context.Context, req ApprovalRequest, sink taskengine.TaskEventSink) (bool, error) {
	if s.approvals == nil {
		return false, fmt.Errorf("hitlservice: durable approval store not configured; pass a runtimetypes.Store-backed store to New/NewWithDefaultPolicy")
	}
	approvalID := uuid.NewString()
	now := time.Now().UTC()

	onTimeout := req.OnTimeout
	if onTimeout == "" {
		onTimeout = ActionDeny
	}
	// A matched rule's own TimeoutS always wins. Absent one, the row (and the
	// wait below) is bounded by the serve-level ceiling instead of blocking
	// indefinitely — see DefaultApprovalCeiling.
	ruleTimeout := req.TimeoutS > 0
	timeoutDur := s.ceiling()
	if ruleTimeout {
		timeoutDur = time.Duration(req.TimeoutS) * time.Second
	}

	row := &runtimetypes.HITLApproval{
		ID:          approvalID,
		ToolsName:   req.ToolsName,
		ToolName:    req.ToolName,
		ArgsSummary: summarizeApprovalArgs(req.Args),
		PolicyName:  req.PolicyName,
		MatchedRule: req.MatchedRule,
		OnTimeout:   string(onTimeout),
		State:       runtimetypes.HITLApprovalPending,
		CreatedAt:   now,
		ExpiresAt:   now.Add(timeoutDur),
	}
	if req.Diff != "" {
		diff := req.Diff
		row.Diff = &diff
	}
	// Durable pending row FIRST — a restart between here and the answer must
	// still show this ask as pending, not lose it (fleet-consolidation.md
	// slice C1, defect D3).
	if err := s.approvals.CreateHITLApproval(ctx, row); err != nil {
		return false, fmt.Errorf("hitlservice: persist pending approval: %w", err)
	}

	// Buffered (capacity 1): a Respond landing before this goroutine reaches
	// the select below is recorded on the channel instead of discarded by a
	// `default:` arm — that unconditional discard was defect D2 (the old
	// unbuffered send-with-default Respond had zero callers repo-wide and
	// would have dropped answers if wired).
	ch := make(chan bool, 1)
	s.mu.Lock()
	s.pending[approvalID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, approvalID)
		s.mu.Unlock()
	}()

	ev := taskengine.NewTaskEvent(ctx, taskengine.TaskEventApprovalRequested)
	ev.ApprovalID = approvalID
	ev.HookName = req.ToolsName
	ev.ToolName = req.ToolName
	ev.ApprovalArgs = req.Args
	ev.ApprovalDiff = req.Diff
	if err := sink.PublishTaskEvent(ctx, ev); err != nil {
		return false, fmt.Errorf("hitl: publish approval request: %w", err)
	}

	waitCtx := ctx
	if !ruleTimeout {
		// Nothing upstream bounds ctx in this case (localtools.HITLWrapper
		// only wraps its askCtx with a deadline when TimeoutS > 0) — this is
		// exactly the unbounded-hang path (fleet-consolidation.md D1).  Apply
		// the serve-level ceiling ourselves.
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeoutDur)
		defer cancel()
	}

	select {
	case approved := <-ch:
		return approved, nil
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			// The caller's own context ended: process shutdown, client
			// disconnect, or — when the matched rule set TimeoutS>0 — the
			// exact deadline localtools.HITLWrapper.Exec applied to askCtx,
			// which it detects via this same ctx.Err() to apply the rule's
			// OnTimeout itself (unchanged from before this change). The row
			// is left pending here either way: SweepExpired closes it out
			// once expires_at passes, and a restart before that still finds
			// it pending rather than losing it.
			return false, ctx.Err()
		}
		// Only the serve-level ceiling could have fired here (a rule timeout
		// would have made waitCtx == ctx, handled above): nothing upstream
		// bounds this ask, so treat it exactly like an explicit human denial
		// — a late denial beats an eternal block — instead of hanging
		// forever. SweepExpired resolves the row itself on its next tick.
		return false, nil
	}
}

// Respond implements Service.Respond: see its doc on the interface.
func (s *service) Respond(ctx context.Context, approvalID string, approved bool) error {
	if s.approvals == nil {
		return fmt.Errorf("hitlservice: durable approval store not configured; pass a runtimetypes.Store-backed store to New/NewWithDefaultPolicy")
	}
	state := runtimetypes.HITLApprovalApproved
	if !approved {
		state = runtimetypes.HITLApprovalDenied
	}
	now := time.Now().UTC()
	err := s.approvals.ResolveHITLApproval(ctx, approvalID, state, marshalApprovalResolution(approved), now)
	if err != nil {
		if !errors.Is(err, libdb.ErrNotFound) {
			return fmt.Errorf("hitlservice: resolve approval %s: %w", approvalID, err)
		}
		// The compare-and-swap matched zero rows: id either does not exist or
		// is no longer pending. Re-read the row to tell those apart (and,
		// among terminal states, expired from already-answered) rather than
		// collapsing both into one generic failure.
		row, getErr := s.approvals.GetHITLApproval(ctx, approvalID)
		if getErr != nil {
			if errors.Is(getErr, libdb.ErrNotFound) {
				return ErrApprovalNotFound
			}
			return fmt.Errorf("hitlservice: look up approval %s: %w", approvalID, getErr)
		}
		if row.State == runtimetypes.HITLApprovalExpired {
			return ErrApprovalExpired
		}
		return ErrApprovalAlreadyResolved
	}

	// Best-effort in-process wake-up: a requester parked on THIS instance
	// gets the answer immediately over the buffered channel. When none is
	// parked (a different hitlservice instance/process — e.g. after a
	// restart — or the requester already returned via its own timeout), the
	// row transition above is the durable record of the answer; that is
	// precisely the drop this buffered channel + persisted row combination
	// fixes (defect D2).
	s.mu.Lock()
	ch, ok := s.pending[approvalID]
	s.mu.Unlock()
	if ok {
		select {
		case ch <- approved:
		default:
			// Should not happen (the CAS above guarantees at most one winning
			// Respond per row, and the channel is fresh per RequestApproval
			// call), but never block Respond on a full channel.
		}
	}
	return nil
}

// sweepBatchLimit caps how many expired rows one SweepExpired call resolves,
// so a large backlog cannot make a single call block indefinitely; the next
// tick picks up whatever is left.
const sweepBatchLimit = 200

// SweepExpired implements Service.SweepExpired: see its doc on the interface.
func (s *service) SweepExpired(ctx context.Context) (int, error) {
	if s.approvals == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	rows, err := s.approvals.ListExpiredHITLApprovals(ctx, now, sweepBatchLimit)
	if err != nil {
		return 0, fmt.Errorf("hitlservice: list expired approvals: %w", err)
	}
	expired := 0
	for _, row := range rows {
		approved := onTimeoutOutcome(Action(row.OnTimeout))
		err := s.approvals.ResolveHITLApproval(ctx, row.ID, runtimetypes.HITLApprovalExpired, marshalApprovalResolution(approved), now)
		if err != nil {
			if errors.Is(err, libdb.ErrNotFound) {
				continue // already resolved by a racing Respond; nothing to do
			}
			return expired, fmt.Errorf("hitlservice: resolve expired approval %s: %w", row.ID, err)
		}
		expired++
		// Best-effort wake-up, in case a requester is somehow still parked
		// on this id in this process (should not normally happen once its
		// own deadline has passed, but costs nothing to cover).
		s.mu.Lock()
		ch, ok := s.pending[row.ID]
		s.mu.Unlock()
		if ok {
			select {
			case ch <- approved:
			default:
			}
		}
	}
	return expired, nil
}

// ListPending implements Service.ListPending: see its doc on the interface.
// Unlike SweepExpired (a best-effort background tick that silently no-ops
// with nothing configured to sweep), ListPending is a direct, user-facing
// read — like RequestApproval/Respond, it reports the missing durable store
// as an explicit error rather than a quietly empty inbox that could be
// mistaken for "nothing pending".
func (s *service) ListPending(ctx context.Context, limit int) ([]*runtimetypes.HITLApproval, error) {
	if s.approvals == nil {
		return nil, fmt.Errorf("hitlservice: durable approval store not configured; pass a runtimetypes.Store-backed store to New/NewWithDefaultPolicy")
	}
	rows, err := s.approvals.ListHITLApprovals(ctx, runtimetypes.HITLApprovalPending, nil, limit)
	if err != nil {
		return nil, fmt.Errorf("hitlservice: list pending approvals: %w", err)
	}
	if rows == nil {
		rows = []*runtimetypes.HITLApproval{}
	}
	return rows, nil
}

// onTimeoutOutcome mirrors localtools.HITLWrapper.Exec's own on-timeout
// branch (runtime/localtools/hitl.go, the block right after the approval
// select) exactly: only an explicit ActionAllow resolves to an
// auto-approval; every other value denies. In practice a *validated* policy
// can never set on_timeout to "allow" (policy.go's validatePolicy rejects it
// outright, since it would silently bypass approval), so this only ever
// observably returns false — kept as a real branch rather than hardcoded so
// it stays byte-for-byte consistent with what localtools already decided for
// the tool call itself if that constraint ever changes.
func onTimeoutOutcome(onTimeout Action) bool {
	return onTimeout == ActionAllow
}

// approvalResolution is the structured payload written into
// HITLApproval.Resolution. Today the only shape ever written is a boolean
// approve/deny answer — Resolution is not narrowed to a bare boolean column
// because a permission ask is answered yes/no, but a later ask kind answers
// with data instead ("which of these three?", "what value should I use?");
// keeping the stored payload structured now means that shape needs no
// migration later. Respond's own signature stays boolean-only in this slice
// (Approved is simply the only field written); only the storage
// representation is forward-looking.
type approvalResolution struct {
	Approved *bool `json:"approved,omitempty"`
}

// marshalApprovalResolution encodes approved as the current resolution
// payload shape. Marshaling a fixed, trivially-encodable struct cannot fail
// in practice; the fallback exists only so a caller never has to handle an
// error from what is conceptually an infallible encode.
func marshalApprovalResolution(approved bool) json.RawMessage {
	raw, err := json.Marshal(approvalResolution{Approved: &approved})
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"approved":%t}`, approved))
	}
	return raw
}

// summarizeApprovalArgs picks a human-recognizable field out of a tool call's
// args for the durable row's ArgsSummary column, mirroring localtools'
// hitlArgsSummary heuristic. Duplicated (not imported) because localtools
// already imports hitlservice — importing back would cycle.
func summarizeApprovalArgs(args map[string]any) string {
	for _, key := range []string{"path", "command", "url", "pattern"} {
		v, ok := args[key].(string)
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		s := strings.TrimSpace(strings.ReplaceAll(v, "\n", " "))
		if len([]rune(s)) > 96 {
			return string([]rune(s)[:95]) + "..."
		}
		return s
	}
	return ""
}
