package acpsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libacp "github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// ErrSessionNotLive reports that no session on this connection maps to a given
// contenox (internal) session id — the firing session has ended, or it was never
// hosted by this transport. It is the signal a mission-report deliverer turns
// into an inbox fallback (mirroring agentinstance.ErrNotFound for the kernel), so
// it is a branchable sentinel.
var ErrSessionNotLive = errors.New("acpsvc: no live session for that contenox id on this connection")

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

// hasMissionCapability reports whether this transport can run /mission at all: a
// dispatcher to fire through (Deps.Fleet) AND an agent resolver to parse the
// two-shape grammar (Deps.Agents). Both are wired together in three shapes — a
// serve-hosted ACP session (serve_cmd.go); a standalone `contenox acp` editor
// that embeds the fleet IN-PROCESS, which is the DEFAULT now (a mission is a
// subagent of THIS process; see docs/development/blueprints/open-work-2026-07-21
// §2 and runtime/contenoxcli/acp_cmd.go); and, only as an explicit opt-in
// (CONTENOX_SERVER_URL set), a FORWARDING pair pointed at a running serve
// (Deps.MissionForwarded).
//
// This single bit gates whether /mission is ADVERTISED (acpCommands, commands.go
// — never advertise what cannot work). It is now UNCONDITIONAL of reachability:
// an editor that embeds the fleet always advertises /mission, and the opt-in
// forwarding pair advertises it too. An unreachable serve is no longer hidden
// from the menu; it is caught at INVOCATION by handleMission, which teaches
// rather than half-firing — a stable menu the operator can rely on, honest at
// the point of use. serve's own in-process kernel and the editor's embedded
// fleet both leave MissionForwarded nil.
func (t *Transport) hasMissionCapability() bool {
	return t.deps.Fleet != nil && t.deps.Agents != nil
}

// handleMission fires a mission FROM this chat session — the `/mission` slash
// command (fleet-consolidation.md M4, "the surface an operator actually reaches
// for"). It sets the mission's ParentSessionID to this session, which is what
// makes the supervision edge real from the chat side: the fired unit's reports
// belong to whoever is driving THIS session.
//
// It dispatches through fleetservice.Dispatch — the same orchestration the REST
// path and `contenox mission fire` use — rather than reimplementing anything, so
// the Enabled gate, the envelope, teardown-on-failure, and the mission record
// are all the shared implementation.
//
// # Where reports go: this session, live (the default)
//
// The governing ontology (open-work-2026-07-21, the Preamble): a mission is a
// SUBAGENT of the process that fired it, and its report notifies exactly the
// parent that fired it. In the default topology the editor embeds the fleet and
// its report router IN-PROCESS (acp_cmd.go), so the fired unit's report is
// DELIVERED live back into THIS session's stream — the supervision edge closes
// inside one process, in the editor, exactly where it was fired. The report
// router reaches this session through DeliverToContenoxSession below; the
// operator inbox is only the fallback for when this session has already ended by
// the time a report lands (never an error — a report is a durable fact).
//
// # The forwarded case (opt-in): reports land in the operator inbox
//
// When CONTENOX_SERVER_URL is explicitly set, the dispatcher is instead a
// FORWARDER pointed at a running serve (Deps.MissionForwarded), so an operator
// can fire onto a bigger box. The dispatch rides over REST, ParentSessionID and
// all — but that parent session id names a session living in THIS acp process,
// which the remote serve's kernel does not own, so serve's report router misses
// on DeliverToSession and falls back to the operator inbox as parent-gone. The
// forwarded confirmation says so plainly rather than promising a session
// delivery it cannot make; live cross-process delivery is a named follow-up.
func (t *Transport) handleMission(ctx context.Context, sess *sessionEntry, args string) (string, error) {
	// Opt-in forwarding (CONTENOX_SERVER_URL set) is the one path whose target can
	// be gone at invocation time. /mission is advertised unconditionally now (see
	// hasMissionCapability), so honesty lives HERE: a serve that has since stopped
	// gets a teaching error naming it and how to bring it back, rather than a raw
	// connection failure from the forwarded dispatch below.
	if f := t.deps.MissionForwarded; f != nil {
		if f.Reachable != nil && !f.Reachable() {
			target := "the configured serve"
			if f.TargetURL != nil {
				if u := strings.TrimSpace(f.TargetURL()); u != "" {
					target = "the serve at " + u
				}
			}
			return "", fmt.Errorf("mission dispatch is unavailable: %s stopped answering — restart it or run `contenox serve`, then try /mission again", target)
		}
	} else if !t.hasMissionCapability() {
		// No fleet is wired in-process: a setup-only editor with no model yet, or a
		// process that is ITSELF a dispatched unit (a subagent does not host its own
		// fleet). /mission is not advertised here, so reaching this is a stale menu or
		// a remembered command. Teach the in-process paths, NOT serve-as-center.
		return "", fmt.Errorf("mission dispatch is unavailable in this session: /mission needs a configured model and the in-process fleet. Configure a model with `contenox config set default-model …` and fire /mission from your editor session, or set CONTENOX_SERVER_URL to fire onto a running serve.")
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
	// The report-routing tail is honest about where reports actually go. The
	// default in-process fleet supervises the mission from THIS session, so its
	// reports arrive here live; the operator inbox is only the fallback for a
	// session that has ended when a report lands. A FORWARDED session (opt-in,
	// CONTENOX_SERVER_URL set) fired at a remote serve whose kernel does not own
	// this session, so its reports fall back to that serve's operator inbox as
	// parent-gone (see this function's doc comment) — the confirmation says so
	// rather than promising a session delivery that will not happen.
	tail := "Reports arrive live in this session as the mission runs; if this session has ended when one lands, it waits in the operator inbox."
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

// DeliverToContenoxSession injects an out-of-band update — a mission report the
// report router routed on the supervision edge — into a LIVE native session on
// THIS stdio connection, addressed by the firing session's contenox (internal)
// session id (the mission's ParentSessionID, which handleMission set above).
//
// It is the in-process editor's half of the supervision edge the ontology
// demands (open-work-2026-07-21, the Preamble): a mission is a subagent of the
// process that fired it, and its report notifies exactly the parent session that
// fired it — which, for a `/mission` fired from the editor, is one of this
// transport's OWN native sessions, not a kernel-owned unit. The report router's
// SessionDeliverer reaches it here (see runtime/contenoxcli/acp_cmd.go): the
// firing session id is mapped to the ACP session id the client knows
// (contenoxToACPID), the notification is re-addressed to it (exactly as the
// kernel's DeliverToSession stamps the owning id), and pushed to the editor as an
// ordinary session/update — carrying the reportrouter's `contenox.missionReport`
// _meta so the editor renders it as a report, not chat text.
//
// Returns ErrSessionNotLive when no session on this connection maps to
// contenoxSessionID (it has ended, or was never here), the signal that routes the
// report to the operator inbox instead — never a fault.
func (t *Transport) DeliverToContenoxSession(ctx context.Context, contenoxSessionID string, n libacp.SessionNotification) error {
	sid, ok := t.acpSessionForContenoxID(contenoxSessionID)
	if !ok {
		return ErrSessionNotLive
	}
	// Re-address to the ACP session id the client learned at session/new; the
	// router built n against the contenox id, which the editor never saw.
	n.SessionID = sid
	t.sendUpdate(ctx, n)
	return nil
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
