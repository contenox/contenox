// Package fleetservice is the fleet lifecycle-POLICY layer sitting between the
// agent-instance kernel (runtime/agentinstance) and its consumers. The kernel
// is deliberately policy-free — agentinstance.Manager knows HOW to bring an
// instance up, drive a session, and tear one down, but never WHETHER it
// should: that judgment (refuse a disabled agent, roll back a half-dispatched
// unit, fan a session-less cancel out over every open session) has to live
// somewhere, and scattering it across every caller lets the callers drift
// apart. This package is that one place.
//
// It is the service-package idiom this codebase already uses for the fleet's
// durable half (runtime/missionservice): a validated interface over ctx +
// error, a New() constructor, no HTTP concerns. Today's only consumer is
// runtime/internal/fleetapi (thin REST handlers over this Service); the
// `contenox fleet` CLI (a follow-up slice) mounts on the same interface
// instead of re-deriving the orchestration. See
// docs/development/blueprints/beam/fleet-manager.md for the fleet-manager
// ontology (manifest, dispatch, envelopes, telemetry) this layer implements
// the "dispatch" half of.
package fleetservice

import (
	"context"
	"errors"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/vfs"
)

// DispatchRequest is the input to Dispatch: the declared agent to bring up,
// the intent that becomes its first turn, the HITL policy that bounds it
// while it runs unattended, and an optional session working directory
// (validated against the workspace-root allowlist). It is the single source
// of truth for the shape — fleetapi's wire DTO is a type alias onto this, not
// an independent copy.
//
// Every dispatch is a mission (docs/development/blueprints/acp/
// fleet-consolidation.md, "Mission mode"): there is no headless bring-up that
// is not one, so Intent and HITLPolicyName are both required rather than
// optional extras layered onto a separate prompt. The intent IS the prompt —
// it is sent as the unit's first turn (Dispatch step 4) — so there is no
// separate prompt field to also populate.
type DispatchRequest struct {
	AgentName string `json:"agentName"`
	// Intent is the one-line mission intent: what the unit is being sent to
	// do, and also the content of its first turn. Required — a dispatch with
	// nothing to do is not a dispatch.
	Intent string `json:"intent"`
	// HITLPolicyName names the HITL policy that becomes the mission's
	// envelope — what bounds the unit while it runs unattended (see
	// missionservice.Mission). Required: a mission with no envelope is a
	// mission with no bounds, which mission mode must not permit.
	//
	// Deliberately NOT defaulted from config here: fleetservice has no
	// config dependency, and adding one only to backfill this default would
	// be scope creep for what this slice is doing. A later slice prefills
	// this field in beam's dispatch form instead. Its absence from this
	// struct's defaulting logic is a decision, not an oversight.
	HITLPolicyName string `json:"hitlPolicyName"`
	Cwd            string `json:"cwd,omitempty"`
}

// DispatchResult is Dispatch's output: the ids the dispatch created. MissionID
// is always present — every dispatch is a mission (see DispatchRequest).
type DispatchResult struct {
	InstanceID string `json:"instanceId"`
	SessionID  string `json:"sessionId"`
	MissionID  string `json:"missionId"`
}

// Service is the fleet's operational surface: read the board (List/Get),
// allocate a unit (Dispatch), and end one (Stop/Cancel). Every method takes a
// ctx and returns an error, even where the kernel method it wraps does not,
// so this seam stays uniform regardless of what agentinstance.Manager needs
// under it.
type Service interface {
	// List returns the config+runtime join: every declared agent, annotated
	// with its live instances. A thin passthrough to
	// agentinstance.Manager.List.
	List(ctx context.Context) ([]agentinstance.FleetEntry, error)

	// Get returns one instance's status, or agentinstance.ErrNotFound if
	// instanceID is unknown. A thin passthrough to agentinstance.Manager.Get.
	Get(ctx context.Context, instanceID string) (agentinstance.InstanceStatus, error)

	// Dispatch allocates a unit: it resolves and validates the declared
	// agent (refusing a disabled one), brings up an instance, opens a
	// session, records a mission bound to both ids and carrying the
	// request's envelope, and runs the intent as the unit's first turn on a
	// detached context, returning as soon as the session is open
	// (async-after-OpenSession; the turn's outcome is observable on the
	// board). It is allocation, not operation: no restart policy, no
	// adoption into a beam chat session (a documented v1 limitation). Any
	// failure after Start tears the fresh instance back down so a failed
	// dispatch never leaks a running subprocess.
	Dispatch(ctx context.Context, req DispatchRequest) (DispatchResult, error)

	// Stop tears instanceID down via agentinstance.Manager.Stop, which is
	// idempotent by kernel contract: stopping an unknown or already-stopped
	// id is a no-op returning nil, not an error. Callers (including a
	// DELETE /fleet/{id} handler) may therefore call Stop without a
	// preceding existence check.
	Stop(ctx context.Context, instanceID string) error

	// Cancel cancels an in-flight prompt turn. With sessionID given it
	// cancels exactly that session (agentinstance.Manager.Cancel is safe with
	// no turn in flight, so this is safe to call speculatively). With
	// sessionID empty it fans out over every session InstanceStatus.SessionIDs
	// reports for instanceID and cancels each — "stop everything running on
	// this instance" without the caller having to enumerate sessions itself.
	// Returns agentinstance.ErrNotFound for an unknown instanceID.
	Cancel(ctx context.Context, instanceID, sessionID string) error
}

type service struct {
	instances      agentinstance.Manager
	agents         agentregistryservice.Service
	missions       missionservice.Service
	workspaceRoots *vfs.Factory
	projectRoot    string
	tracker        libtracker.ActivityTracker
}

// New returns a Service driving instances (the kernel) and agents (for the
// Enabled policy check Dispatch enforces). workspaceRoots and projectRoot are
// dispatch-only and may be zero (an absent cwd then defaults to projectRoot
// unvalidated; see resolveCwd). missions is NOT optional: every dispatch is a
// mission (see DispatchRequest), so Dispatch calls into it unconditionally,
// with no per-request check standing in for a wiring problem. A nil registry
// here is the same class of defect as a nil instances or agents would be —
// the caller (contenox serve) always constructs a real one; it is not a
// condition Dispatch validates against a request. A nil tracker degrades to a
// Noop, so the async first-turn outcome is simply not recorded rather than
// panicking.
func New(
	instances agentinstance.Manager,
	agents agentregistryservice.Service,
	missions missionservice.Service,
	workspaceRoots *vfs.Factory,
	projectRoot string,
	tracker libtracker.ActivityTracker,
) Service {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &service{
		instances:      instances,
		agents:         agents,
		missions:       missions,
		workspaceRoots: workspaceRoots,
		projectRoot:    projectRoot,
		tracker:        tracker,
	}
}

func (s *service) List(ctx context.Context) ([]agentinstance.FleetEntry, error) {
	return s.instances.List(ctx)
}

func (s *service) Get(ctx context.Context, instanceID string) (agentinstance.InstanceStatus, error) {
	_ = ctx // agentinstance.Manager.Get is purely in-memory; ctx governs nothing here.
	return s.instances.Get(instanceID)
}

func (s *service) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResult, error) {
	if strings.TrimSpace(req.AgentName) == "" {
		return DispatchResult{}, apiframework.MissingParameter("agentName", "agentName is required")
	}
	// Every dispatch is a mission (see DispatchRequest): the intent becomes
	// the unit's first turn and the HITL policy becomes its envelope, so
	// both are required up front rather than validated in combination with
	// each other or with whether a mission registry happens to be wired.
	if strings.TrimSpace(req.Intent) == "" {
		return DispatchResult{}, apiframework.MissingParameter("intent", "intent is required")
	}
	if strings.TrimSpace(req.HITLPolicyName) == "" {
		return DispatchResult{}, apiframework.MissingParameter("hitlPolicyName", "hitlPolicyName is required: a mission must name its envelope")
	}
	// cwd envelope discipline: a requested cwd must be absolute and must resolve
	// within an allowlisted workspace root; an absent one defaults to the same
	// root the session path uses. The judgement is vfs.ResolveSessionCwd, shared
	// with acpsvc's session paths — see resolveCwd.
	cwd, err := s.resolveCwd(req.Cwd)
	if err != nil {
		return DispatchResult{}, err
	}

	// POLICY: refuse a disabled agent BEFORE bringing anything up, via the
	// ONE shared judgment agentregistryservice.ResolveForSpawn makes for
	// every agent-spawn path (see its doc comment) — this REST path and
	// acpsvc's external bring-up both call it, so the check cannot drift
	// between them. The kernel itself has no concept of Enabled (it spawns
	// whatever record it is handed).
	agent, err := agentregistryservice.ResolveForSpawn(ctx, s.agents, req.AgentName)
	if err != nil {
		if errors.Is(err, agentregistryservice.ErrAgentDisabled) {
			return DispatchResult{}, apiframework.Conflict(err.Error())
		}
		return DispatchResult{}, err
	}

	// 1. Bring up an instance from the record the Enabled check was just made
	// against. StartResolved, not Start(agentName): Start would re-read the same
	// row, which is both a second query per dispatch and a TOCTOU window — an
	// agent disabled between the two reads would still spawn, defeating the check
	// immediately above it.
	instanceID, err := s.instances.StartResolved(ctx, agent)
	if err != nil {
		return DispatchResult{}, err
	}

	// 2. Open a session on the instance. On failure tear the fresh instance
	// down so a failed dispatch never leaks a running subprocess (the acpsvc
	// contract).
	sessionID, err := s.instances.OpenSession(ctx, instanceID, agentinstance.SessionSpec{Cwd: cwd})
	if err != nil {
		_ = s.instances.Stop(instanceID)
		return DispatchResult{}, err
	}

	result := DispatchResult{InstanceID: instanceID, SessionID: string(sessionID)}

	// 3. Record the mission — every dispatch is one — then bind both ids to
	// it. The envelope (HITLPolicyName) is set at creation, not bolted on
	// after: a window between "mission exists" and "mission has bounds" is
	// exactly what mission mode must not allow.
	m := &missionservice.Mission{
		Intent:         req.Intent,
		AgentName:      req.AgentName,
		HITLPolicyName: req.HITLPolicyName,
	}
	if err := s.missions.Create(ctx, m); err != nil {
		_ = s.instances.Stop(instanceID)
		return DispatchResult{}, err
	}
	if _, err := s.missions.Bind(ctx, m.ID, string(sessionID), instanceID); err != nil {
		_ = s.instances.Stop(instanceID)
		return DispatchResult{}, err
	}
	result.MissionID = m.ID

	// 4. The intent runs as the unit's first turn, detached: Dispatch
	// returns as soon as the session is open and the mission is recorded;
	// the turn's outcome is observable on the board and recorded through the
	// tracker (never swallowed). context.WithoutCancel keeps request-scoped
	// values (request id) while surviving the caller's return. Payload
	// discipline: ids and stop reason only, never intent/prompt content.
	detached := context.WithoutCancel(ctx)
	blocks := []libacp.ContentBlock{libacp.NewTextContent(req.Intent)}
	go func() {
		reportErr, reportChange, end := s.tracker.Start(detached, "prompt", "fleet_dispatch",
			"instance_id", instanceID, "session_id", string(sessionID), "agent_name", req.AgentName)
		defer end()
		stop, err := s.instances.Prompt(detached, instanceID, sessionID, blocks)
		if err != nil {
			reportErr(err)
			return
		}
		reportChange(string(sessionID), string(stop))
	}()

	return result, nil
}

func (s *service) Stop(ctx context.Context, instanceID string) error {
	_ = ctx // agentinstance.Manager.Stop is purely in-memory; ctx governs nothing here.
	return s.instances.Stop(instanceID)
}

func (s *service) Cancel(ctx context.Context, instanceID, sessionID string) error {
	_ = ctx // agentinstance.Manager.Cancel/Get take no ctx; kept for interface uniformity.
	if sessionID != "" {
		return s.instances.Cancel(instanceID, libacp.SessionID(sessionID))
	}
	// No session named: cancel every session currently attached on the
	// instance. Safe with no turn in flight (kernel contract), so an
	// instance with zero attached sessions is a no-op returning nil, not an
	// error.
	status, err := s.instances.Get(instanceID)
	if err != nil {
		return err
	}
	var errs []error
	for _, sid := range status.SessionIDs {
		if cerr := s.instances.Cancel(instanceID, libacp.SessionID(sid)); cerr != nil {
			errs = append(errs, cerr)
		}
	}
	return errors.Join(errs...)
}

// resolveCwd maps a requested session cwd onto the concrete root the dispatched
// unit will run in. It does NOT re-derive the rules: it delegates to
// vfs.ResolveSessionCwd — the same implementation the ACP session paths use — and
// owns only the translation of its refusal into this layer's REST error. The
// fallback is the configured project root: unlike the ACP transport, this layer
// HAS a default root, so an absent cwd resolves to it.
//
// Note the tightening this inherits: a relative cwd is refused here, which the
// hand-rolled predecessor did not do. With no allowlist configured it passed a
// relative cwd straight through to OpenSession, where the ACP path would have
// refused it — POST /fleet/dispatch was the one door into session bring-up
// missing the absolute-path guard.
func (s *service) resolveCwd(cwd string) (string, error) {
	resolved, err := vfs.ResolveSessionCwd(s.workspaceRoots, cwd, s.projectRoot)
	if err != nil {
		return "", apiframework.InvalidParameterValue("cwd", err.Error())
	}
	return resolved, nil
}
