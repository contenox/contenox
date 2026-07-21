// acp_mission_forward.go is the EXPLICIT OPT-IN forwarding path for `/mission`
// in a standalone `contenox acp` process: when — and only when — the operator
// sets CONTENOX_SERVER_URL, the dispatch is forwarded to a running `contenox
// serve` over its REST API instead of running on the editor's own in-process
// fleet.
//
// This used to be the DEFAULT (and auto-discovered a serve via a presence row or
// the loopback bind), built on the false premise that a standalone `contenox acp`
// has no fleet kernel of its own — it does; the kernel is an embeddable library
// (runtime/agentinstance), which the acp process now embeds so `/mission` runs
// IN-PROCESS and its reports come back live into the firing editor session (the
// ontology: a mission is a subagent of the process that fired it — see
// docs/development/blueprints/open-work-2026-07-21 §2, and acp_cmd.go's embedding).
//
// Forwarding survives ONLY as the explicit opt-in an operator reaches for to fire
// a mission onto a bigger box: presence-based auto-discovery is gone, and the
// target is the CONTENOX_SERVER_URL the operator set, nothing else. acpsvc keys
// its forwarding-honesty details off Deps.MissionForwarded — an unreachable serve
// teaches at invocation (not a hidden menu), and the confirmation states reports
// land in that serve's OPERATOR INBOX as parent-gone (the remote serve's kernel
// does not own this process's firing session) — see runtime/acpsvc/mission.go.
//
// The bearer token comes from CONTENOX_SERVER_TOKEN: a tokenless loopback serve
// needs nothing, a token-protected serve is reached only by an operator who set
// that env on the launched process.
package contenoxcli

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

const (
	// missionForwardProbeTTL coalesces reachability probes: /mission's
	// invocation guard consults Reachable, and a short cache keeps a burst from
	// each hitting /health while still noticing a serve that stops within a second.
	missionForwardProbeTTL = 1 * time.Second
	// missionForwardHealthTimeout bounds one /health probe so a wedged or
	// vanished serve cannot freeze the slash-command flow — a connection refused
	// on loopback returns near-instantly, a hung one at this ceiling.
	missionForwardHealthTimeout = 1500 * time.Millisecond
)

// missionForwarder forwards `/mission` dispatch and agent-name resolution from a
// standalone `contenox acp` session to the serve named by CONTENOX_SERVER_URL. It
// implements the two narrow acpsvc mission interfaces and supplies the
// Reachable/TargetURL hooks acpsvc.MissionForwardConfig needs.
type missionForwarder struct {
	// getenv is the environment accessor (a seam so tests drive discovery without
	// mutating the real process environment).
	getenv     func(string) string
	httpClient *http.Client
	now        func() time.Time
	probeTTL   time.Duration

	mu          sync.Mutex
	cachedReach bool
	cachedAt    time.Time
	haveCache   bool
	lastTarget  string
}

var (
	_ acpsvc.MissionDispatcher    = (*missionForwarder)(nil)
	_ acpsvc.MissionAgentResolver = (*missionForwarder)(nil)
)

// newMissionForwarder builds a forwarder targeting CONTENOX_SERVER_URL. It is
// constructed only on the explicit opt-in (acp_cmd.go builds it solely when that
// env is set), so discovery is env-only — no presence auto-discovery, no loopback
// default.
func newMissionForwarder() *missionForwarder {
	return &missionForwarder{
		getenv:     os.Getenv,
		httpClient: &http.Client{Timeout: serveClientTimeout},
		now:        time.Now,
		probeTTL:   missionForwardProbeTTL,
	}
}

// forwardConfig packages the Reachable/TargetURL hooks for acpsvc.Deps.
func (f *missionForwarder) forwardConfig() *acpsvc.MissionForwardConfig {
	return &acpsvc.MissionForwardConfig{
		Reachable: f.Reachable,
		TargetURL: f.TargetURL,
	}
}

// discover resolves (baseURL, token) from the environment only:
// CONTENOX_SERVER_URL is the target (the caller guarantees it is set) and
// CONTENOX_SERVER_TOKEN authenticates. No presence row, no loopback default —
// forwarding is the operator's explicit choice, pointed at exactly one serve.
func (f *missionForwarder) discover(_ context.Context) (baseURL, token string) {
	token = strings.TrimSpace(f.getenv(envServeToken))
	return normalizeBaseURL(strings.TrimSpace(f.getenv(envServeURL))), token
}

// Reachable reports whether the configured serve answers /health right now,
// cached for probeTTL. It re-discovers on each uncached probe, so a serve that
// comes back after a stop is picked up.
func (f *missionForwarder) Reachable() bool {
	f.mu.Lock()
	if f.haveCache && f.now().Sub(f.cachedAt) < f.probeTTL {
		reach := f.cachedReach
		f.mu.Unlock()
		return reach
	}
	f.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), missionForwardHealthTimeout)
	defer cancel()
	baseURL, _ := f.discover(ctx)
	reach := baseURL != "" && f.probeHealth(ctx, baseURL)

	f.mu.Lock()
	f.cachedReach, f.cachedAt, f.haveCache, f.lastTarget = reach, f.now(), true, baseURL
	f.mu.Unlock()
	return reach
}

// probeHealth GETs baseURL+"/health" (an open route, no auth) and reports a 200.
func (f *missionForwarder) probeHealth(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// TargetURL returns the serve URL last discovered (for acpsvc's teaching error),
// discovering once if no probe has run yet.
func (f *missionForwarder) TargetURL() string {
	f.mu.Lock()
	if f.lastTarget != "" {
		t := f.lastTarget
		f.mu.Unlock()
		return t
	}
	f.mu.Unlock()

	baseURL, _ := f.discover(context.Background())
	f.mu.Lock()
	f.lastTarget = baseURL
	f.mu.Unlock()
	return baseURL
}

// Dispatch forwards a mission to the configured serve (POST /fleet/dispatch),
// the same route `contenox mission fire` posts to.
func (f *missionForwarder) Dispatch(ctx context.Context, req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
	baseURL, token := f.discover(ctx)
	return f.clientAt(baseURL, token).dispatchMission(ctx, req)
}

// GetByName resolves a declared agent by name against the configured serve
// (GET /agents/by-name/{name}) so `/mission`'s two-shape grammar can tell a
// named agent apart from the first word of an intent. Any error reads as "not a
// named agent" to the caller (resolveMissionAgentAndIntent), so a serve blip
// during resolution simply falls the line through to the default agent.
func (f *missionForwarder) GetByName(ctx context.Context, name string) (*runtimetypes.Agent, error) {
	baseURL, token := f.discover(ctx)
	return f.clientAt(baseURL, token).getAgentByName(ctx, name)
}

// clientAt builds a serveClient bound to an explicit URL+token — the forwarder
// discovers the target itself rather than reading cobra flags, so it cannot use
// newServeClient.
func (f *missionForwarder) clientAt(baseURL, token string) *serveClient {
	return &serveClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    f.httpClient,
	}
}

// getAgentByName fetches GET /agents/by-name/{name}. A 404 (unknown agent)
// surfaces as a *ServeError, which the /mission resolver treats as "not a named
// agent" — the whole line is then the intent for the default agent.
func (c *serveClient) getAgentByName(ctx context.Context, name string) (*runtimetypes.Agent, error) {
	var out runtimetypes.Agent
	if err := c.get(ctx, "/agents/by-name/"+url.PathEscape(name), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// normalizeBaseURL trims a configured server URL and defaults a missing scheme to
// http:// so a bare host:port from CONTENOX_SERVER_URL still resolves.
func normalizeBaseURL(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" {
		return u
	}
	if !strings.Contains(u, "://") {
		return "http://" + u
	}
	return u
}
