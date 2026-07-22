package localfileapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/vfs"
)

// Streaming-search caps. They are DEFENSIVE bounds, not tuning knobs: a workspace
// search is fired from a debounced search box on every keystroke, so the endpoint
// must refuse to buffer a whole scan, must stop after a screenful of results, and
// must never let one giant file or one pathological query dominate.
const (
	// defaultSearchMaxResults is the hard total cap on emitted matches — theia's
	// default. On reaching it the server kills rg and emits a truncated `done`.
	defaultSearchMaxResults = 500
	// defaultSearchPerFileCap is rg's -m (max matching LINES per file), so one
	// enormous file cannot fill the whole result budget on its own.
	defaultSearchPerFileCap = 100
	// defaultSearchPreviewBytes caps the context preview (rg's matched line text)
	// so a minified/one-line file cannot ship a multi-kilobyte "line".
	defaultSearchPreviewBytes = 200
	// defaultSearchQueryBytes caps the query itself — a sanity bound against a
	// pasted blob arriving as a search term.
	defaultSearchQueryBytes = 512
	// searchScanTokenBytes bounds one line of rg --json output the scanner will
	// hold. rg emits one JSON object per line and a matched line can be long
	// (minified source), so this is generous but bounded.
	searchScanTokenBytes = 1 << 20
)

// searchMatch is one streamed match frame: the wire shape the Beam search panel
// renders. Column is the 0-based BYTE offset of the match within Preview (rg's
// submatch start), Length its byte length, so the client highlights the substring
// without re-searching. Preview is rg's matched line text, trailing newline
// stripped and truncated to defaultSearchPreviewBytes on a rune boundary — Column
// stays aligned to Preview for any match inside the cap. Path is relative to the
// workspace root (the leading "./" rg prints is stripped), matching the /files
// endpoints' root-relative paths so a hit routes straight into the diff view.
type searchMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Length  int    `json:"length"`
	Preview string `json:"preview"`
}

// searchDone is the single terminal frame: it ALWAYS closes the stream (whether
// rg finished, found nothing, or was killed at the cap). Matches is the count
// emitted; Truncated is true when the hard cap stopped the scan early, so the UI
// can show "showing first N — refine your search".
type searchDone struct {
	Done      bool `json:"done"`
	Matches   int  `json:"matches"`
	Truncated bool `json:"truncated"`
}

// AddWorkspaceSearchRoutes registers GET /workspace/search, the streaming
// workspace-wide search that shells to `rg --json` under a vfs-validated
// workspace root and pushes matches as Server-Sent Events (Arc 2 of
// docs/development/blueprints/beam/ide-workflows.md; component-roadmap Tier 2
// item 5). It is a SIBLING of AddWorkspaceRootsRoutes and mounted right after it:
// both take the same *vfs.Factory, and search's root MUST be validated through
// that Factory's allowlist so the scan can never reach outside a configured
// workspace — the allowlist is the authority, exactly as it is for the /files
// browse API. A request whose `root` is not allowlisted is refused with 422
// through the same localfileapi register the /files handlers use, before any
// process is spawned.
//
// Nil-gated like the other optional route groups: with no workspace-root
// allowlist configured (factory nil — the stdio/legacy path), the route
// registers nothing and 404s, matching AddWorkspaceRootsRoutes.
//
// # Why SSE (and why named events)
//
// The codebase's one established HTTP-streaming precedent is
// runtime/internal/taskeventsapi, which streams a single homogeneous event type
// as plain `data:` frames over text/event-stream with an explicit http.Flusher.
// This endpoint follows that precedent for the transport (SSE + Flush, never
// buffering the whole scan) but streams TWO shapes — many `match` frames then one
// `done` frame — so it uses NAMED SSE events (`event: match` / `event: done`)
// rather than plain `data:` frames. That lets an EventSource client demux with
// addEventListener('match') / addEventListener('done') instead of sniffing a
// discriminator field, and the terminal frame additionally carries `done:true`
// so a client that ignores event names can still recognize it. The divergence
// from taskeventsapi is deliberate and is the whole reason the two differ.
func AddWorkspaceSearchRoutes(mux *http.ServeMux, factory *vfs.Factory) {
	if factory == nil {
		return
	}
	h := &searchHandler{
		factory:    factory,
		maxResults: defaultSearchMaxResults,
		perFileCap: defaultSearchPerFileCap,
		previewCap: defaultSearchPreviewBytes,
		queryCap:   defaultSearchQueryBytes,
		lookPath:   exec.LookPath,
	}
	mux.HandleFunc("GET /workspace/search", h.search)
}

// searchHandler carries the caps as fields (not package constants) so a test can
// construct one with a tiny maxResults to exercise truncation without generating
// hundreds of matches, and can stub lookPath to exercise the rg-missing branch on
// a host that HAS rg. AddWorkspaceSearchRoutes wires the production defaults.
type searchHandler struct {
	factory    *vfs.Factory
	maxResults int
	perFileCap int
	previewCap int
	queryCap   int
	// lookPath resolves the rg binary; exec.LookPath in production, stubbable in
	// tests. An error here is the "ripgrep not installed" teaching case.
	lookPath func(string) (string, error)
}

// search streams matches for `q` under `root` as Server-Sent Events.
func (h *searchHandler) search(w http.ResponseWriter, r *http.Request) {
	// @response sse Named SSE events over text/event-stream: `match` frames (one searchMatch JSON object each), then a terminal `done` frame carrying totals and the truncation flag.
	// All refusals happen BEFORE any SSE header is written, so they are ordinary
	// JSON API errors a client can read a status from — once the stream starts,
	// the only signal left is a terminal `done` frame.
	q := strings.TrimSpace(apiframework.GetQueryParam(r, "q", "", "Search query, matched as a literal substring (smart-case)."))
	if q == "" {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: q is required", apiframework.ErrUnprocessableEntity),
			apiframework.ListOperation)
		return
	}
	if len(q) > h.queryCap {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: q exceeds the %d-byte limit", apiframework.ErrUnprocessableEntity, h.queryCap),
			apiframework.ListOperation)
		return
	}

	// The workspace root is validated through the Factory allowlist — the same
	// authority the /files browse API uses. "" and "/" resolve to the default
	// root; anything else must be an allowlisted member, or the scan is refused.
	root := apiframework.GetQueryParam(r, "root", "", "Workspace root to search; empty or \"/\" uses the configured default. Must be an allowlisted root or a directory under one.")
	resolved, ok := h.factory.Allows(root)
	if !ok {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: workspace root %q is not under any configured workspace root; roots: %s",
				apiframework.ErrUnprocessableEntity, root, h.factory.DescribeRoots()),
			apiframework.ListOperation)
		return
	}

	// limit clamps DOWN only: a client may ask for fewer results than the hard
	// cap, never more. An absent or unparseable value uses the hard cap.
	limit := h.maxResults
	if raw := strings.TrimSpace(apiframework.GetQueryParam(r, "limit", "", "Maximum matches to stream before truncating; capped at the server maximum.")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n < limit {
			limit = n
		}
	}

	rgPath, err := h.lookPath("rg")
	if err != nil {
		writeSearchDependencyMissing(w)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: streaming unsupported", apiframework.ErrInternalServerError),
			apiframework.ServerOperation)
		return
	}

	// A cancelable context derived from the request so BOTH client disconnect and
	// hitting the result cap kill rg (exec.CommandContext sends SIGKILL on cancel).
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// --fixed-strings: the query is a literal, not a regex. This is the safe
	//   default for a search-as-you-type box — a half-typed "foo(" is a valid
	//   literal, where as a regex it would be a syntax error rg exits non-zero on,
	//   after the SSE stream already started (no way to report it cleanly).
	// --smart-case: case-insensitive unless the query has an uppercase letter.
	// -m: per-file matched-line cap (see defaultSearchPerFileCap).
	// cwd=resolved + target ".": rg prints paths relative to the root ("./x"), so
	//   matches carry root-relative paths the /files endpoints can consume, and rg
	//   physically cannot escape the validated root (no -L, so symlinks are not
	//   followed out of it). .gitignore is respected (theia's default), so build
	//   artifacts and vendored trees don't drown the results.
	// Control-plane carveout for the RECURSION, not just the root: the root was
	// validated above (a denied dir can never BE the root), but rg descends into
	// subdirectories — a search rooted at a granted PARENT of ~/.contenox would
	// otherwise read the runtime's own config/policies/DB into results. Same
	// invariant vfs enforces on every resolve path, applied to rg's walk via
	// per-denied-dir glob excludes (relative to the root when contained in it).
	args := []string{"--json", "--fixed-strings", "--smart-case",
		"-m", strconv.Itoa(h.perFileCap)}
	for _, denied := range vfs.ControlPlaneDenied() {
		if rel, err := filepath.Rel(resolved, denied); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			args = append(args, "--glob", "!"+filepath.ToSlash(rel)+"/**", "--glob", "!"+filepath.ToSlash(rel))
		}
	}
	args = append(args, "--", q, ".")
	cmd := exec.CommandContext(ctx, rgPath, args...)
	cmd.Dir = resolved
	cmd.Stderr = io.Discard

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ServerOperation)
		return
	}
	if err := cmd.Start(); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ServerOperation)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// A leading comment frame flushes the headers immediately, so a client's
	// EventSource opens before the first match arrives (taskeventsapi's pattern).
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		cancel()
		_ = cmd.Wait()
		return
	}
	flusher.Flush()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), searchScanTokenBytes)

	matches := 0
	truncated := false
	clientGone := false

scan:
	for scanner.Scan() {
		frames, isMatch := parseRipgrepMatch(scanner.Bytes(), h.previewCap)
		if !isMatch {
			continue
		}
		for _, m := range frames {
			if matches >= limit {
				truncated = true
				break scan
			}
			if err := writeSSEEvent(w, "match", m); err != nil {
				clientGone = true
				break scan
			}
			flusher.Flush()
			matches++
		}
	}

	// Stop rg (a natural finish already closed stdout; a cap/error break kills it)
	// and reap it so no process is left behind.
	cancel()
	_ = cmd.Wait()

	if clientGone {
		return
	}
	_ = writeSSEEvent(w, "done", searchDone{Done: true, Matches: matches, Truncated: truncated})
	flusher.Flush()
}

// ripgrepEnvelope is the SUBSET of rg's --json event schema this endpoint reads:
// the `match` events (type=="match") carry the path, the matched line text, its
// line number, and the per-occurrence submatch offsets. `begin`/`end`/`summary`
// events are ignored — the terminal `done` frame is computed server-side from the
// emitted count, not from rg's summary, so a killed-at-the-cap scan (which never
// emits a summary) still terminates cleanly.
type ripgrepEnvelope struct {
	Type string `json:"type"`
	Data struct {
		Path       ripgrepText       `json:"path"`
		Lines      ripgrepText       `json:"lines"`
		LineNumber int               `json:"line_number"`
		Submatches []ripgrepSubmatch `json:"submatches"`
	} `json:"data"`
}

// ripgrepText is rg's {"text": "..."} wrapper. For non-UTF8 paths/lines rg emits
// a {"bytes": "..."} form instead, which decodes to an empty Text here — such a
// match is skipped (binary/again-non-text content is not what workspace search is
// for), rather than shipping a garbled preview.
type ripgrepText struct {
	Text string `json:"text"`
}

type ripgrepSubmatch struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// parseRipgrepMatch turns one line of rg --json output into zero or more match
// frames. It returns (nil, false) for any line that is not a `match` event (or is
// unparseable, or is a non-text match), and (frames, true) for a match line —
// one frame per submatch, since a single line can contain several occurrences and
// each is an independently navigable hit. It is pure and allocation-local so the
// streaming loop can call it per line and the JSON-parsing contract is unit
// testable without a live rg.
func parseRipgrepMatch(line []byte, previewCap int) ([]searchMatch, bool) {
	var env ripgrepEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, false
	}
	if env.Type != "match" {
		return nil, false
	}
	path := strings.TrimPrefix(env.Data.Path.Text, "./")
	if path == "" {
		return nil, false
	}
	preview := trimPreview(env.Data.Lines.Text, previewCap)
	frames := make([]searchMatch, 0, len(env.Data.Submatches))
	for _, sm := range env.Data.Submatches {
		frames = append(frames, searchMatch{
			Path:    path,
			Line:    env.Data.LineNumber,
			Column:  sm.Start,
			Length:  sm.End - sm.Start,
			Preview: preview,
		})
	}
	if len(frames) == 0 {
		return nil, false
	}
	return frames, true
}

// trimPreview strips the trailing newline rg includes on the matched line and
// caps the preview at previewCap bytes, backing off to a rune boundary so a
// multibyte character is never split (which would corrupt the JSON string).
func trimPreview(s string, previewCap int) string {
	s = strings.TrimRight(s, "\r\n")
	if len(s) <= previewCap {
		return s
	}
	s = s[:previewCap]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// writeSSEEvent marshals payload and writes one named SSE frame
// ("event: <name>\ndata: <json>\n\n"). A write error means the client hung up;
// the caller stops streaming.
func writeSSEEvent(w io.Writer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

// writeSearchDependencyMissing answers the rg-absent case with a 501 Not
// Implemented and a teaching message that NAMES the missing dependency, in the
// same {error:{message,type,code}} envelope apiframework.Error emits — so a
// client parses it uniformly. There is no apiframework sentinel that maps to 501
// (its table tops out at 500), so this is written directly rather than through
// apiframework.Error; the endpoint is genuinely un-implementable on a host
// without ripgrep, which is a configuration fact, not a request error.
func writeSearchDependencyMissing(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": "workspace search requires the ripgrep (rg) binary, which is not installed on this host; install ripgrep to enable GET /workspace/search",
			"type":    "api_error",
			"code":    "dependency_missing",
		},
	})
}
