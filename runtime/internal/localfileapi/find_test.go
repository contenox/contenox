package localfileapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/require"
)

func newFindMux(t *testing.T, root string) *http.ServeMux {
	t.Helper()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)
	mux := http.NewServeMux()
	AddWorkspaceFindRoutes(mux, factory, nil) // nil hitlFor: agent filter unavailable
	return mux
}

// findFrame is one parsed SSE frame from the find stream.
type findFrame struct {
	event string
	data  string
}

// parseFindStream splits an SSE response body into (match entries, done frame).
func parseFindStream(t *testing.T, body string) ([]Entry, *findDone) {
	t.Helper()
	var matches []Entry
	var done *findDone
	for _, seg := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(seg) == "" {
			continue
		}
		var fr findFrame
		for _, line := range strings.Split(seg, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				fr.event = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				fr.data = strings.TrimSpace(line[len("data:"):])
			}
		}
		switch fr.event {
		case "match":
			var e Entry
			require.NoError(t, json.Unmarshal([]byte(fr.data), &e))
			matches = append(matches, e)
		case "done":
			var d findDone
			require.NoError(t, json.Unmarshal([]byte(fr.data), &d))
			done = &d
		}
	}
	return matches, done
}

func seedFindTree(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o750))
	for _, f := range []string{"README.md", "docs/intro.md", "docs/app.ts"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644))
	}
}

func TestUnit_Find_StreamsMatches(t *testing.T) {
	root := t.TempDir()
	seedFindTree(t, root)
	mux := newFindMux(t, root)

	req := httptest.NewRequest(http.MethodGet, "/workspace/find?glob=*.md", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	matches, done := parseFindStream(t, rec.Body.String())

	var paths []string
	for _, m := range matches {
		paths = append(paths, m.Path)
		require.Nil(t, m.Access, "no agent filter requested → no access annotation")
	}
	require.ElementsMatch(t, []string{"README.md", "docs/intro.md"}, paths)
	require.NotNil(t, done)
	require.True(t, done.Done)
	require.Equal(t, 2, done.Matches)
	require.False(t, done.Truncated)
}

func TestUnit_Find_MissingGlobRefused(t *testing.T) {
	mux := newFindMux(t, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/workspace/find", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "glob is required, refused before any stream")
	require.NotEqual(t, "text/event-stream", rec.Header().Get("Content-Type"))
}

func TestUnit_Find_BadGlobRefused(t *testing.T) {
	mux := newFindMux(t, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/workspace/find?glob=%5B", nil) // "["
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "a malformed pattern is refused pre-stream")
}

func TestUnit_Find_DisallowedRootRefused(t *testing.T) {
	allowed := t.TempDir()
	disallowed := t.TempDir()
	mux := newFindMux(t, allowed)
	req := httptest.NewRequest(http.MethodGet, "/workspace/find?glob=*.md&root="+disallowed, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "a non-allowlisted root is refused before any walk")
}

func TestUnit_Find_AgentFilterUnavailableWithoutHITL(t *testing.T) {
	root := t.TempDir()
	seedFindTree(t, root)
	mux := newFindMux(t, root) // registered with nil hitlFor
	req := httptest.NewRequest(http.MethodGet, "/workspace/find?glob=*.md&filter=agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "filter=agent needs a policy engine; refused when none is wired")
}

func TestUnit_Find_LimitTruncates(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "c.md"), []byte("x"), 0o644))
	mux := newFindMux(t, root)

	req := httptest.NewRequest(http.MethodGet, "/workspace/find?glob=*.md&limit=2", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	matches, done := parseFindStream(t, rec.Body.String())
	require.Len(t, matches, 2)
	require.NotNil(t, done)
	require.True(t, done.Truncated)
}

// TestUnit_Find_ExcludesControlPlaneRecursion mirrors the search control-plane
// test: a denied dir can never BE the root (the Factory refuses it), but the walk
// recurses, so the per-node re-resolution in localfileservice.Find must prune it.
func TestUnit_Find_ExcludesControlPlaneRecursion(t *testing.T) {
	root := t.TempDir()
	cpDir := filepath.Join(root, "cp")
	lookalike := filepath.Join(root, "cp2")
	require.NoError(t, os.MkdirAll(cpDir, 0o750))
	require.NoError(t, os.MkdirAll(lookalike, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "secret.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lookalike, "open.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plain.md"), []byte("x"), 0o644))

	require.NoError(t, vfs.SetControlPlaneDenied(cpDir))
	t.Cleanup(func() { require.NoError(t, vfs.SetControlPlaneDenied()) })

	mux := newFindMux(t, root)
	req := httptest.NewRequest(http.MethodGet, "/workspace/find?glob=*.md", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	matches, _ := parseFindStream(t, rec.Body.String())
	var paths []string
	for _, m := range matches {
		paths = append(paths, m.Path)
	}
	require.Contains(t, paths, "plain.md")
	require.Contains(t, paths, "cp2/open.md", "sibling-named lookalike stays reachable — the skip is segment-exact")
	require.NotContains(t, paths, "cp/secret.md", "the walk must not descend into the denied control-plane subtree")
}
