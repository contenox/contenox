package acpsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// MissionDispatcher is the narrow slice of fleetservice.Service the /mission
// slash command needs: fire a mission and get back its ids. Kept as a local
// interface (not the whole Service) so this package depends only on the one
// method it uses and a fake is trivial in tests — the house rule that a command
// accepts the narrowest interface it actually needs. serve satisfies it with the
// real fleetservice.Service; the stdio `contenox acp` path leaves it nil, and
// /mission then reports that dispatch is unavailable rather than half-firing.
type MissionDispatcher interface {
	Dispatch(ctx context.Context, req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error)
}

// MissionAgentResolver is the narrow slice of agentregistryservice.Service used
// to disambiguate /mission's two shape-identical grammar forms — is the first
// token an agent name, or the first word of the intent? A hit means the named
// form. Only GetByName is needed here, so that is all this interface asks for.
type MissionAgentResolver interface {
	GetByName(ctx context.Context, name string) (*runtimetypes.Agent, error)
}

// hasMissionCapability reports whether this transport can actually run
// /mission: a fleet kernel to dispatch through (Deps.Fleet) AND an agent
// resolver to parse its two-shape grammar (Deps.Agents). Both are wired
// together in a serve-hosted ACP session (see serve_cmd.go) and now ALSO in a
// standalone `contenox acp` session as a FORWARDING pair pointed at a running
// serve (see runtime/contenoxcli mission forwarding, marked by
// Deps.MissionForwarded). This single bit gates two things that must agree:
// whether /mission is advertised at all (acpCommands, in commands.go — never
// advertise what cannot work) and whether handleMission below treats the
// command as impossible outright, rather than degrading quietly. A caller
// hitting the impossible case anyway — stale client menu state, a command typed
// from memory — gets a teaching error instead of a crash or a silent no-op.
//
// For the forwarding case there is a third, LIVE requirement: the target serve
// must answer right now (MissionForwarded.Reachable). This is what makes the
// advertisement honest across a serve that starts, stops, or moves after this
// process launched — advertisement is recomputed per session, so a fresh
// session sees /mission exactly when a dispatch would land. serve's own
// in-process kernel leaves MissionForwarded nil and is always available.
func (t *Transport) hasMissionCapability() bool {
	if t.deps.Fleet == nil || t.deps.Agents == nil {
		return false
	}
	if f := t.deps.MissionForwarded; f != nil && f.Reachable != nil {
		return f.Reachable()
	}
	return true
}

// handleMission fires a mission FROM this chat session — the `/mission` slash
// command (fleet-consolidation.md M4, "the surface an operator actually reaches
// for"). It sets the mission's ParentSessionID to this session, which is what
// makes the supervision edge real from the chat side: the fired unit's reports
// belong to whoever is driving THIS session, not to an operator inbox nobody is
// reading. Consumption of that edge (report routing back into the session) is a
// sibling slice; this only records it.
//
// It dispatches through fleetservice.Dispatch — the same orchestration the REST
// path and `contenox mission fire` use — rather than reimplementing anything, so
// the Enabled gate, the envelope, teardown-on-failure, and the mission record
// are all the shared implementation.
//
// # The forwarded case: reports land in the operator inbox, not this session
//
// A standalone `contenox acp` session (Deps.MissionForwarded set) fires by
// FORWARDING the dispatch to a running serve over REST, ParentSessionID and all.
// But that parent session id names a session living in THIS acp process, which
// serve's kernel does not own — so when serve's report router tries to deliver a
// report back on the supervision edge (DeliverToSession(ParentSessionID)), the
// kernel Manager returns ErrNotFound and the router falls back to the operator
// inbox as parent-gone. That is the DESIGNED fallback, never an error (see
// runtime/reportrouter): a report is a durable fact and always readable; only
// the live delivery-into-the-editor-session half is missing. The forwarded
// confirmation says so plainly ("reports land in the operator inbox") rather
// than promising session delivery it cannot make. Cross-process report delivery
// back into the firing editor session is a NAMED FOLLOW-UP, not wired here.
func (t *Transport) handleMission(ctx context.Context, sess *sessionEntry, args string) (string, error) {
	// /mission is never advertised without this capability (acpCommands), so
	// reaching here means a client sent it anyway — stale menu state, someone
	// typing a remembered command, or (forwarded case) a serve that answered at
	// advertisement time but has since stopped. The teaching error names the
	// paths that work; for the forwarded case it names the specific serve that
	// went silent and how to bring it back.
	if !t.hasMissionCapability() {
		if f := t.deps.MissionForwarded; f != nil {
			target := "the configured serve"
			if f.TargetURL != nil {
				if u := strings.TrimSpace(f.TargetURL()); u != "" {
					target = "the serve at " + u
				}
			}
			return "", fmt.Errorf("mission dispatch is unavailable: %s stopped answering — restart it or run `contenox serve`, then try /mission again", target)
		}
		return "", fmt.Errorf("mission dispatch is unavailable in this session: /mission needs the serve-hosted fleet kernel, which a standalone 'contenox acp' session doesn't have. Fire it from a Beam session (or any ACP client connected to a running 'contenox serve'), or run `contenox mission fire` against that serve instead")
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return "", fmt.Errorf("usage: /mission <intent>   or   /mission <agent-name> <intent>")
	}

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())

	agentName, intent, named := t.resolveMissionAgentAndIntent(ctx, store, args)
	if strings.TrimSpace(agentName) == "" {
		return "", fmt.Errorf("no mission agent: name one as `/mission <agent-name> <intent>`, or set a default with `contenox config set default-mission-agent <name>`")
	}
	policy := strings.TrimSpace(clikv.Read(ctx, store, "default-mission-policy"))
	if policy == "" {
		return "", fmt.Errorf("no mission envelope: set one with `contenox config set default-mission-policy <policy>` — a mission must name the HITL policy that bounds it")
	}

	res, err := t.deps.Fleet.Dispatch(ctx, fleetservice.DispatchRequest{
		AgentName:      agentName,
		Intent:         intent,
		HITLPolicyName: policy,
		// The supervision edge: this session FIRED the mission, so its upstream
		// contenox session id is the parent. Empty only if the session somehow
		// carries no internal id, which routes reports to the operator inbox — the
		// same fallback as an operator firing directly.
		ParentSessionID: sess.InternalSessionID,
	})
	if err != nil {
		return "", err
	}

	// The two grammar forms are indistinguishable by shape, so the confirmation
	// states PLAINLY which agent was chosen (default vs named) and echoes the
	// intent verbatim — the blueprint's chosen mitigation for the ambiguity,
	// making a misread visible in the transcript the instant it happens rather
	// than letting a first intent word that happens to match an agent name change
	// meaning silently.
	agentRole := "default mission agent"
	if named {
		agentRole = "named agent"
	}
	// The report-routing tail is honest about where reports actually go. A
	// serve-hosted session supervises the mission directly, so reports return to
	// it. A FORWARDED session (standalone acp) fired at a remote serve whose
	// kernel does not own this session, so its reports fall back to the operator
	// inbox as parent-gone (see this function's doc comment) — the confirmation
	// says so rather than promising a session delivery that will not happen.
	tail := "Reports return to this session."
	if t.deps.MissionForwarded != nil {
		tail = "Reports land in the operator inbox on that serve (read them with `contenox approvals`, or `contenox mission show`); live delivery back into this editor session is a named follow-up."
	}
	return fmt.Sprintf(
		"Mission fired at %s %q under envelope %q.\nIntent: %s\nMission %s (instance %s, session %s). %s",
		agentRole, agentName, policy, intent, res.MissionID, res.InstanceID, res.SessionID, tail,
	), nil
}

// resolveMissionAgentAndIntent decides the mission's agent and intent from the
// command's arguments, resolving the grammar ambiguity fleet-consolidation.md M4
// flags: `/mission <intent>` and `/mission <agent-name> <intent>` are the same
// shape. The rule (deliberate, per the blueprint): resolve the FIRST token
// against the declared-agent registry — a hit is the named form (agent = token,
// intent = the rest); a miss means the whole line is the intent for the
// configured default agent. named reports which branch was taken so the caller's
// confirmation can name it. With no resolver wired, or a single-token input,
// only the default form is possible.
func (t *Transport) resolveMissionAgentAndIntent(ctx context.Context, store runtimetypes.Store, args string) (agentName, intent string, named bool) {
	first, rest := splitFirstToken(args)
	if rest != "" && t.deps.Agents != nil {
		if a, err := t.deps.Agents.GetByName(ctx, first); err == nil && a != nil {
			return a.Name, rest, true
		}
	}
	return strings.TrimSpace(clikv.Read(ctx, store, "default-mission-agent")), args, false
}

// splitFirstToken splits args into its first whitespace-delimited token and the
// trimmed remainder. A single-token input yields an empty remainder.
func splitFirstToken(args string) (first, rest string) {
	args = strings.TrimSpace(args)
	if i := strings.IndexFunc(args, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }); i >= 0 {
		return args[:i], strings.TrimSpace(args[i+1:])
	}
	return args, ""
}
