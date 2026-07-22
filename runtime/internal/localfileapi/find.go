package localfileapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/agentview"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/vfs"
)

// Streaming-find caps. Like the search caps these are DEFENSIVE bounds, not tuning
// knobs: find is fired from a debounced filter box on every keystroke, so it must
// refuse to buffer a whole walk, stop after a bounded number of matches, and never
// let a pasted blob of patterns dominate.
const (
	// defaultFindMaxResults is the hard cap on emitted file matches. On reaching it
	// the walk stops and the terminal `done` frame carries truncated:true.
	defaultFindMaxResults = 2000
	// defaultFindGlobCount caps how many comma-separated patterns one request may carry.
	defaultFindGlobCount = 16
	// defaultFindGlobBytes caps the raw `glob` parameter — a sanity bound against a
	// pasted blob arriving as a pattern list.
	defaultFindGlobBytes = 256
)

// defaultFindSkipDirs are the noise directories the walk prunes, matching
// local_fs.find_files' default skip set — build output, VCS metadata, vendored
// trees, and editor dirs that would otherwise drown a workspace-wide match.
func defaultFindSkipDirs() map[string]bool {
	return map[string]bool{
		".git": true, "node_modules": true, ".venv": true, "__pycache__": true,
		".next": true, "dist": true, ".cache": true, "vendor": true,
		"target": true, ".idea": true, ".vscode": true,
	}
}

// findDone is the single terminal frame: it ALWAYS closes the stream (whether the
// walk finished, found nothing, or was stopped at the cap). Matches is the count
// emitted; Truncated is true when the hard cap stopped the walk early, so the UI
// can show "showing first N — narrow your filter".
type findDone struct {
	Done      bool `json:"done"`
	Matches   int  `json:"matches"`
	Truncated bool `json:"truncated"`
}

// AddWorkspaceFindRoutes registers GET /workspace/find, the streaming filename
// search — the `find` to AddWorkspaceSearchRoutes' `grep`. It walks a
// vfs-validated workspace root with the recursive localfileservice.Find primitive
// (filepath.Match over file names, per-node boundary re-checked) and pushes each
// matching entry as a Server-Sent Event, so a Beam filter box ("show only *.md")
// gets every match across the tree in ONE request instead of a per-directory
// fan-out. Its `root` is validated through the same *vfs.Factory allowlist as the
// browse and search APIs, and — like the browse API's filter=agent — it can
// annotate each match with the agent's per-path access verdict when hitlFor is
// supplied.
//
// Nil-gated like the other optional route groups: with no workspace-root
// allowlist configured (factory nil), the route registers nothing and 404s.
//
// It mirrors AddWorkspaceSearchRoutes' SSE contract exactly: named events
// (`event: match` / `event: done`) over text/event-stream with an explicit
// http.Flusher, all refusals emitted as ordinary JSON errors BEFORE any SSE byte.
func AddWorkspaceFindRoutes(mux *http.ServeMux, factory *vfs.Factory, hitlFor PolicyEvaluatorFactory) {
	if factory == nil {
		return
	}
	h := &findHandler{
		factory:    factory,
		hitlFor:    hitlFor,
		filters:    defaultFilters(),
		maxResults: defaultFindMaxResults,
		globCount:  defaultFindGlobCount,
		globBytes:  defaultFindGlobBytes,
		skipDirs:   defaultFindSkipDirs(),
		services:   map[string]localfileservice.Service{},
	}
	mux.HandleFunc("GET /workspace/find", h.find)
}

// findHandler carries the caps as fields (not package constants) so a test can
// construct one with a tiny maxResults to exercise truncation without generating
// thousands of matches. AddWorkspaceFindRoutes wires the production defaults.
type findHandler struct {
	factory    *vfs.Factory
	hitlFor    PolicyEvaluatorFactory
	filters    map[string]FileFilter
	maxResults int
	globCount  int
	globBytes  int
	skipDirs   map[string]bool

	mu       sync.Mutex
	services map[string]localfileservice.Service
}

// serviceFor returns a cached localfileservice rooted at the resolved (allowlisted)
// root — the same per-root caching the browse mount uses.
func (h *findHandler) serviceFor(resolvedRoot string) (localfileservice.Service, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if svc, ok := h.services[resolvedRoot]; ok {
		return svc, nil
	}
	svc, err := localfileservice.New(resolvedRoot)
	if err != nil {
		return nil, err
	}
	h.services[resolvedRoot] = svc
	return svc, nil
}

// find streams the file entries under `root` whose names match `glob` as SSE.
func (h *findHandler) find(w http.ResponseWriter, r *http.Request) {
	// @response sse Named SSE events over text/event-stream: `match` frames (one localfileapi.Entry JSON object each), then a terminal `done` frame carrying totals and the truncation flag.
	// All refusals happen BEFORE any SSE header is written, so they are ordinary
	// JSON API errors a client can read a status from — once the stream starts,
	// the only signal left is a terminal `done` frame.
	rawGlob := strings.TrimSpace(apiframework.GetQueryParam(r, "glob", "", "Filename match pattern(s), comma-separated; filepath.Match semantics against the basename, or the root-relative path when a pattern contains '/'. A file matches if ANY pattern matches (e.g. \"*.md\" or \"*.go,*.md\")."))
	if rawGlob == "" {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: glob is required", apiframework.ErrUnprocessableEntity),
			apiframework.ListOperation)
		return
	}
	if len(rawGlob) > h.globBytes {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: glob exceeds the %d-byte limit", apiframework.ErrUnprocessableEntity, h.globBytes),
			apiframework.ListOperation)
		return
	}
	globs := splitGlobs(rawGlob)
	if len(globs) == 0 {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: glob is required", apiframework.ErrUnprocessableEntity),
			apiframework.ListOperation)
		return
	}
	if len(globs) > h.globCount {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: too many glob patterns (max %d)", apiframework.ErrUnprocessableEntity, h.globCount),
			apiframework.ListOperation)
		return
	}
	for _, g := range globs {
		if _, err := filepath.Match(g, ""); err != nil {
			_ = apiframework.Error(w, r,
				fmt.Errorf("%w: bad glob %q", apiframework.ErrUnprocessableEntity, g),
				apiframework.ListOperation)
			return
		}
	}

	// The workspace root is validated through the Factory allowlist — the same
	// authority the /files browse and /workspace/search APIs use.
	root := apiframework.GetQueryParam(r, "root", "", "Workspace root to search; empty or \"/\" uses the configured default. Must be an allowlisted root or a directory under one.")
	resolved, ok := h.factory.Allows(root)
	if !ok {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: workspace root %q is not under any configured workspace root; roots: %s",
				apiframework.ErrUnprocessableEntity, root, h.factory.DescribeRoots()),
			apiframework.ListOperation)
		return
	}

	// The subtree to search, validated (normalized + contained) BEFORE the stream
	// opens so an escape / control-plane / bad path is an honest pre-stream refusal
	// rather than a silent empty result. The bound view is reused for the agent view.
	pathArg := apiframework.GetQueryParam(r, "path", ".", "Directory to search under, relative to the workspace root; defaults to the whole workspace.")
	startRel, err := localfileservice.NormalizeRelPath(pathArg, true)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	view, err := vfs.OpenView(resolved)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ServerOperation)
		return
	}
	if _, err := view.Resolve(startRel); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	// limit clamps DOWN only: a client may ask for fewer results than the hard cap,
	// never more. An absent or unparseable value uses the hard cap.
	limit := h.maxResults
	if raw := strings.TrimSpace(apiframework.GetQueryParam(r, "limit", "", "Maximum matches to stream before truncating; capped at the server maximum.")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n < limit {
			limit = n
		}
	}

	// filter=agent annotates each emitted match with the agent's per-path verdict,
	// exactly like GET /files?filter=agent. Empty or "full" streams raw entries.
	filterName := strings.TrimSpace(apiframework.GetQueryParam(r, "filter", "", "Set to \"agent\" to annotate each match with the agent's per-path access verdict; empty or \"full\" streams raw entries."))
	policyName := strings.TrimSpace(apiframework.GetQueryParam(r, "policy", "", "HITL policy the agent view is evaluated against; empty uses the configured default. Ignored unless filter=agent."))
	var ev *agentview.Evaluator
	if filterName != "" && filterName != "full" {
		if _, known := h.filters[filterName]; !known {
			_ = apiframework.Error(w, r,
				fmt.Errorf("%w: unknown filter %q", apiframework.ErrUnprocessableEntity, filterName),
				apiframework.ListOperation)
			return
		}
		if h.hitlFor == nil {
			_ = apiframework.Error(w, r,
				fmt.Errorf("%w: filter %q is not available on this endpoint", apiframework.ErrUnprocessableEntity, filterName),
				apiframework.ListOperation)
			return
		}
		ev = agentview.NewEvaluator(view, h.hitlFor(policyName), policyName)
	}

	svc, err := h.serviceFor(resolved)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: streaming unsupported", apiframework.ErrInternalServerError),
			apiframework.ServerOperation)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// A leading comment frame flushes the headers immediately (taskeventsapi's pattern).
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	clientGone := false
	result, _ := svc.Find(r.Context(), localfileservice.FindOptions{
		Path:     pathArg,
		Globs:    globs,
		Limit:    limit,
		SkipDirs: h.skipDirs,
	}, func(e localfileservice.Entry) error {
		entry := Entry{Entry: e}
		if ev != nil {
			v := ev.Verdict(r.Context(), e.Path, e.IsDirectory)
			entry.Access = &v
		}
		if err := writeSSEEvent(w, "match", entry); err != nil {
			clientGone = true
			return err
		}
		flusher.Flush()
		return nil
	})

	// A walk error mid-stream (e.g. the root vanished) cannot become an HTTP status
	// now — the stream is open — so it closes with a terminal `done` carrying whatever
	// was emitted, same as a natural finish. Request-fault cases (bad glob/path/root)
	// were already refused pre-stream above.
	if clientGone {
		return
	}
	_ = writeSSEEvent(w, "done", findDone{Done: true, Matches: result.Count, Truncated: result.Truncated})
	flusher.Flush()
}

// splitGlobs splits the comma-separated `glob` parameter into trimmed, non-empty
// patterns.
func splitGlobs(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
