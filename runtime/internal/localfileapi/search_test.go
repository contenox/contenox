package localfileapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/require"
)

// ─── rg --json parsing (no live rg) ─────────────────────────────────────────

// parseRipgrepMatch turns exactly the `match` events into frames, one per
// submatch, with root-relative paths and a trimmed preview; every other rg event
// (and any junk) yields (nil, false).
func TestUnit_ParseRipgrepMatch(t *testing.T) {
	const cap = 200

	t.Run("single submatch", func(t *testing.T) {
		line := []byte(`{"type":"match","data":{"path":{"text":"./a.txt"},"lines":{"text":"hello world\n"},"line_number":1,"absolute_offset":0,"submatches":[{"match":{"text":"hello"},"start":0,"end":5}]}}`)
		frames, ok := parseRipgrepMatch(line, cap)
		require.True(t, ok)
		require.Len(t, frames, 1)
		require.Equal(t, "a.txt", frames[0].Path, "the leading ./ is stripped to a root-relative path")
		require.Equal(t, 1, frames[0].Line)
		require.Equal(t, 0, frames[0].Column)
		require.Equal(t, 5, frames[0].Length)
		require.Equal(t, "hello world", frames[0].Preview, "the trailing newline is stripped")
	})

	t.Run("multiple submatches on one line yield one frame each", func(t *testing.T) {
		line := []byte(`{"type":"match","data":{"path":{"text":"./b.txt"},"lines":{"text":"foo foo\n"},"line_number":3,"submatches":[{"match":{"text":"foo"},"start":0,"end":3},{"match":{"text":"foo"},"start":4,"end":7}]}}`)
		frames, ok := parseRipgrepMatch(line, cap)
		require.True(t, ok)
		require.Len(t, frames, 2, "each occurrence is an independently navigable hit")
		require.Equal(t, 0, frames[0].Column)
		require.Equal(t, 4, frames[1].Column)
		require.Equal(t, 3, frames[1].Length)
	})

	t.Run("non-match events are ignored", func(t *testing.T) {
		for _, line := range []string{
			`{"type":"begin","data":{"path":{"text":"./a.txt"}}}`,
			`{"type":"end","data":{"path":{"text":"./a.txt"},"stats":{}}}`,
			`{"type":"summary","data":{"stats":{}}}`,
		} {
			_, ok := parseRipgrepMatch([]byte(line), cap)
			require.False(t, ok, "only match events produce frames: %s", line)
		}
	})

	t.Run("garbage and non-text matches are skipped", func(t *testing.T) {
		_, ok := parseRipgrepMatch([]byte("not json at all"), cap)
		require.False(t, ok)
		// A bytes-form (non-UTF8) path decodes to an empty Text and is dropped.
		_, ok = parseRipgrepMatch([]byte(`{"type":"match","data":{"path":{"bytes":"AA=="},"lines":{"text":"x\n"},"line_number":1,"submatches":[{"match":{"text":"x"},"start":0,"end":1}]}}`), cap)
		require.False(t, ok)
	})
}

// trimPreview strips the trailing newline, caps at the byte limit, and never
// splits a multibyte rune.
func TestUnit_TrimPreview(t *testing.T) {
	require.Equal(t, "hello world", trimPreview("hello world\n", 200))
	require.Equal(t, "hello", trimPreview("hello world", 5), "capped at the byte limit")

	// A cap that would land mid-rune backs off to the previous boundary rather
	// than emitting invalid UTF-8.
	got := trimPreview("héllo", 2) // 'é' is 2 bytes; cap 2 lands mid-é
	require.Equal(t, "h", got)
	require.True(t, len(got) <= 2)
}

// ─── refusals, all BEFORE any stream starts ─────────────────────────────────

func newSearchMux(t *testing.T, root string) *http.ServeMux {
	t.Helper()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)
	mux := http.NewServeMux()
	AddWorkspaceSearchRoutes(mux, factory)
	return mux
}

// A root that is not on the allowlist is refused with 422 — the allowlist is the
// authority, exactly as it is for the /files browse API — and no stream begins.
func TestUnit_Search_RootValidationRefusal(t *testing.T) {
	allowed := t.TempDir()
	disallowed := t.TempDir() // exists, but never handed to the Factory
	mux := newSearchMux(t, allowed)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/workspace/search?q=hello&root=" + disallowed)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "a non-allowlisted root is refused before any scan")
}

// An empty or oversized query is refused with 422 before any process spawns.
func TestUnit_Search_QueryValidation(t *testing.T) {
	mux := newSearchMux(t, t.TempDir())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/workspace/search?q=")
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "an empty query is rejected")

	resp, err = http.Get(srv.URL + "/workspace/search?q=" + strings.Repeat("x", 600))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "an oversized query is rejected")
}

// rg absent → a 501 teaching error that names the dependency, deterministically
// exercised by stubbing lookPath (rg IS installed in CI, so we cannot rely on it
// being absent).
func TestUnit_Search_RipgrepMissing(t *testing.T) {
	factory, err := vfs.NewFactory(t.TempDir())
	require.NoError(t, err)
	h := &searchHandler{
		factory:    factory,
		maxResults: defaultSearchMaxResults,
		perFileCap: defaultSearchPerFileCap,
		previewCap: defaultSearchPreviewBytes,
		queryCap:   defaultSearchQueryBytes,
		lookPath:   func(string) (string, error) { return "", errors.New("executable file not found in $PATH") },
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /workspace/search", h.search)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/workspace/search?q=hello")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotImplemented, resp.StatusCode)

	var body struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Contains(t, body.Error.Message, "ripgrep", "the error names the missing dependency")
	require.Equal(t, "dependency_missing", body.Error.Code)
}

// ─── live SSE stream (skips cleanly when rg is absent) ──────────────────────

type sseEvent struct {
	Event string
	Data  string
}

// parseSSE splits a text/event-stream body into named events, dropping comment
// frames (": ...").
func parseSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var out []sseEvent
	for _, block := range strings.Split(strings.TrimRight(body, "\n"), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, ":") {
			continue
		}
		var ev sseEvent
		for _, ln := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(ln, "event:"):
				ev.Event = strings.TrimSpace(strings.TrimPrefix(ln, "event:"))
			case strings.HasPrefix(ln, "data:"):
				ev.Data = strings.TrimSpace(strings.TrimPrefix(ln, "data:"))
			}
		}
		if ev.Event != "" {
			out = append(out, ev)
		}
	}
	return out
}

// A real scan over a fixture directory streams a `match` frame per hit and closes
// with a single `done` frame (truncated=false when everything fit).
func TestUnit_Search_StreamsMatchesThenDone(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep (rg) not installed; skipping live SSE search test")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello world\nnothing here\nhello again\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("say hello\n"), 0o644))

	mux := newSearchMux(t, root)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/workspace/search?q=hello")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	buf, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	events := parseSSE(t, string(buf))

	var matches []searchMatch
	var done *searchDone
	for _, ev := range events {
		switch ev.Event {
		case "match":
			var m searchMatch
			require.NoError(t, json.Unmarshal([]byte(ev.Data), &m))
			matches = append(matches, m)
		case "done":
			var d searchDone
			require.NoError(t, json.Unmarshal([]byte(ev.Data), &d))
			done = &d
		}
	}

	require.Len(t, matches, 3, "three lines contain hello")
	require.NotNil(t, done, "the stream always terminates with a done frame")
	require.True(t, done.Done)
	require.Equal(t, 3, done.Matches)
	require.False(t, done.Truncated)

	// Paths are root-relative and previews carry the matched line.
	paths := map[string]string{}
	for _, m := range matches {
		paths[m.Path] = m.Preview
	}
	require.Contains(t, paths, "a.txt")
	require.Contains(t, paths, filepath.Join("sub", "b.txt"))
	require.Contains(t, paths["sub/b.txt"], "hello")
}

// When the total exceeds the hard cap, the scan stops at the cap and the done
// frame reports truncated=true. A tiny cap makes this cheap to prove.
func TestUnit_Search_TruncatesAtCap(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep (rg) not installed; skipping live SSE truncation test")
	}
	root := t.TempDir()
	// Five matching lines, cap of two.
	require.NoError(t, os.WriteFile(filepath.Join(root, "many.txt"),
		[]byte("hit 1\nhit 2\nhit 3\nhit 4\nhit 5\n"), 0o644))

	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)
	h := &searchHandler{
		factory:    factory,
		maxResults: 2,
		perFileCap: defaultSearchPerFileCap,
		previewCap: defaultSearchPreviewBytes,
		queryCap:   defaultSearchQueryBytes,
		lookPath:   exec.LookPath,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /workspace/search", h.search)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/workspace/search?q=hit")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	buf, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	events := parseSSE(t, string(buf))

	matchCount := 0
	var done *searchDone
	for _, ev := range events {
		switch ev.Event {
		case "match":
			matchCount++
		case "done":
			var d searchDone
			require.NoError(t, json.Unmarshal([]byte(ev.Data), &d))
			done = &d
		}
	}
	require.Equal(t, 2, matchCount, "exactly the cap of matches is streamed")
	require.NotNil(t, done)
	require.True(t, done.Truncated, "the done frame flags truncation")
	require.Equal(t, 2, done.Matches)
}
