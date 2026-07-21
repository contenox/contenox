// acp_mission_forward.go makes `/mission` work from a STANDALONE `contenox acp`
// session — the one Zed spawns — by forwarding the dispatch to a running
// `contenox serve` over its REST API, instead of requiring an in-process fleet
// kernel the editor process does not (and should not) have.
//
// acpsvc declares two NARROW interfaces the `/mission` slash command needs
// (acpsvc.MissionDispatcher = Dispatch, acpsvc.MissionAgentResolver = GetByName);
// missionForwarder implements BOTH over the same serveClient the `contenox
// fleet`/`contenox mission` CLIs use, so there is one dispatch path and one
// notion of "what can I fire", not a second hand-rolled client. serve satisfies
// those interfaces with its own in-process fleetservice; a standalone acp
// session satisfies them with this remote forwarder, and acpsvc treats the two
// identically except for the honesty details it keys off Deps.MissionForwarded
// (advertise-only-when-reachable, inbox-routing confirmation, serve-named
// teaching error) — see runtime/acpsvc/mission.go.
//
// # Discovery is lazy and health-probed
//
// Construction contacts nothing. Discovery runs per call, in a fixed order:
//
//	1. CONTENOX_SERVER_URL env — the operator explicitly pointed acp at a serve.
//	2. a live serve's presence row Address over the shared KV store — zero-config
//	   auto-discovery of a serve on a non-default port (the presence-record
//	   extension this slice added; see runtime/presence).
//	3. serve's own loopback bind default (127.0.0.1:32123) — a zero-config local
//	   serve needs no acp configuration either.
//
// The bearer token ALWAYS comes from CONTENOX_SERVER_TOKEN, never from presence
// (which is world-readable): a tokenless loopback serve needs nothing, and a
// token-protected serve is reached only by an operator who set that env on the
// Zed-launched process.
package contenoxcli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/presence"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

const (
	// missionForwardProbeTTL coalesces reachability probes: /mission's
	// advertisement is recomputed every session (and on /help, /mission), and a
	// short cache keeps a burst of session opens from each hitting /health while
	// still letting a serve that stops be noticed within a second on the next
	// fresh session.
	missionForwardProbeTTL = 1 * time.Second
	// missionForwardHealthTimeout bounds one /health probe so a wedged or
	// vanished serve cannot freeze the slash-command menu — a connection refused
	// on loopback returns near-instantly, a hung one at this ceiling.
	missionForwardHealthTimeout = 1500 * time.Millisecond
)

// missionForwarder forwards `/mission` dispatch and agent-name resolution from a
// standalone `contenox acp` session to a running serve. It implements the two
// narrow acpsvc mission interfaces and supplies the Reachable/TargetURL hooks
// acpsvc.MissionForwardConfig needs.
type missionForwarder struct {
	// presenceStore backs discovery order 2; nil disables presence discovery
	// (order 1 and 3 still apply).
	presenceStore *presence.Store
	// getenv is the environment accessor (a seam so tests drive discovery order
	// without mutating the real process environment).
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

// newMissionForwarder builds a forwarder that auto-discovers a serve. presenceStore
// may be nil to disable presence discovery (leaving env + default).
func newMissionForwarder(presenceStore *presence.Store) *missionForwarder {
	return &missionForwarder{
		presenceStore: presenceStore,
		getenv:        os.Getenv,
		httpClient:    &http.Client{Timeout: serveClientTimeout},
		now:           time.Now,
		probeTTL:      missionForwardProbeTTL,
	}
}

// forwardConfig packages the Reachable/TargetURL hooks for acpsvc.Deps.
func (f *missionForwarder) forwardConfig() *acpsvc.MissionForwardConfig {
	return &acpsvc.MissionForwardConfig{
		Reachable: f.Reachable,
		TargetURL: f.TargetURL,
	}
}

// discover resolves (baseURL, token) in the slice's fixed order (see file doc).
func (f *missionForwarder) discover(ctx context.Context) (baseURL, token string) {
	token = strings.TrimSpace(f.getenv(envServeToken))
	if u := strings.TrimSpace(f.getenv(envServeURL)); u != "" {
		return normalizeBaseURL(u), token
	}
	if f.presenceStore != nil {
		if addr := f.serveAddressFromPresence(ctx); addr != "" {
			return "http://" + addr, token
		}
	}
	return fmt.Sprintf("http://%s:%s", defaultServeAddr, defaultServePort), token
}

// serveAddressFromPresence returns the reachable host:port of the freshest live
// serve presence row, or "" when none is discoverable. Stale rows (a serve that
// has likely died) are skipped so discovery does not hand back a dead address.
func (f *missionForwarder) serveAddressFromPresence(ctx context.Context) string {
	entries, err := f.presenceStore.List(ctx)
	if err != nil {
		return ""
	}
	var best *presence.Entry
	for i := range entries {
		e := &entries[i]
		if e.Kind != presence.KindServe || e.Stale || strings.TrimSpace(e.Address) == "" {
			continue
		}
		if best == nil || e.LastSeen.After(best.LastSeen) {
			best = e
		}
	}
	if best == nil {
		return ""
	}
	return normalizeServeHostPort(best.Address)
}

// Reachable reports whether the discovered serve answers /health right now,
// cached for probeTTL. It re-discovers on each uncached probe, so a serve that
// moves ports (a fresh presence Address) or comes back after a stop is picked up.
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
	reach := f.probeHealth(ctx, baseURL)

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

// Dispatch forwards a mission to the discovered serve (POST /fleet/dispatch),
// the same route `contenox mission fire` posts to.
func (f *missionForwarder) Dispatch(ctx context.Context, req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
	baseURL, token := f.discover(ctx)
	return f.clientAt(baseURL, token).dispatchMission(ctx, req)
}

// GetByName resolves a declared agent by name against the discovered serve
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

// normalizeServeHostPort maps a serve's advertised bind address to one a sibling
// on the same host can actually dial: a wildcard bind (0.0.0.0 / ::) is not a
// connect target, so it becomes loopback. A concrete host is returned unchanged.
func normalizeServeHostPort(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return strings.TrimSpace(addr)
	}
	switch strings.Trim(host, "[]") {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
