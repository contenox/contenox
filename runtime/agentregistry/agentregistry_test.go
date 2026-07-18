package agentregistry

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixture is a trimmed but real sample of the ACP registry catalog (captured
// from https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json),
// covering npx (with and without args), uvx, and binary (with args/sha256/env)
// distribution methods across several platforms.
//
//go:embed testdata/registry.json
var fixture []byte

func loadFixture(t *testing.T) *Registry {
	t.Helper()
	var reg Registry
	require.NoError(t, json.Unmarshal(fixture, &reg))
	return &reg
}

func TestUnit_ParseFixture(t *testing.T) {
	reg := loadFixture(t)
	require.Equal(t, "1.0.0", reg.Version)
	require.Len(t, reg.Agents, 6)

	claude, ok := Find(reg, "claude-acp")
	require.True(t, ok)
	require.Equal(t, "Claude Agent", claude.Name)
	require.Equal(t, "0.59.0", claude.Version)
	require.Contains(t, claude.Distribution, MethodNPX)
}

// ─── Resolve: npx ───────────────────────────────────────────────────────────

func TestUnit_Resolve_NPX_NoArgs_AddsDashY(t *testing.T) {
	reg := loadFixture(t)
	claude, _ := Find(reg, "claude-acp")

	spec, err := Resolve(claude, "linux", "amd64")
	require.NoError(t, err)
	require.Equal(t, MethodNPX, spec.Method)
	require.Equal(t, "npx", spec.Command)
	require.Equal(t, []string{"-y", "@agentclientprotocol/claude-agent-acp@0.59.0"}, spec.Args)
	require.Empty(t, spec.Env)
	require.Empty(t, spec.Note)
}

func TestUnit_Resolve_NPX_WithArgs(t *testing.T) {
	reg := loadFixture(t)
	agora, _ := Find(reg, "agoragentic-acp")

	spec, err := Resolve(agora, "darwin", "arm64")
	require.NoError(t, err)
	require.Equal(t, "npx", spec.Command)
	require.Equal(t, []string{"-y", "agoragentic-mcp@1.3.0", "--acp"}, spec.Args)
}

// ─── Resolve: uvx ───────────────────────────────────────────────────────────

func TestUnit_Resolve_UVX(t *testing.T) {
	reg := loadFixture(t)
	fa, _ := Find(reg, "fast-agent")

	spec, err := Resolve(fa, "linux", "amd64")
	require.NoError(t, err)
	require.Equal(t, MethodUVX, spec.Method)
	require.Equal(t, "uvx", spec.Command)
	require.Equal(t, []string{"fast-agent-acp==0.9.14", "-x"}, spec.Args)
	require.Empty(t, spec.Env)
}

// ─── Resolve: binary ────────────────────────────────────────────────────────

func TestUnit_Resolve_Binary_LinuxAmd64_UsesBasenameAndArgs(t *testing.T) {
	reg := loadFixture(t)
	goose, _ := Find(reg, "goose")

	spec, err := Resolve(goose, "linux", "amd64")
	require.NoError(t, err)
	require.Equal(t, MethodBinary, spec.Method)
	require.Equal(t, "goose", spec.Command, "cmd './goose' resolves to basename 'goose'")
	require.Equal(t, []string{"acp"}, spec.Args)
	require.Contains(t, spec.Note, "goose")
	require.Contains(t, spec.Note, "PATH")
}

func TestUnit_Resolve_Binary_DarwinArm64(t *testing.T) {
	reg := loadFixture(t)
	amp, _ := Find(reg, "amp-acp")

	spec, err := Resolve(amp, "darwin", "arm64")
	require.NoError(t, err)
	require.Equal(t, "amp-acp", spec.Command)
	require.Empty(t, spec.Args)
}

func TestUnit_Resolve_Binary_WindowsBasenameFromBackslashPath(t *testing.T) {
	reg := loadFixture(t)
	goose, _ := Find(reg, "goose")

	// The windows-x86_64 cmd is "./goose-package\\goose.exe"; the basename must
	// be extracted across the backslash separator.
	spec, err := Resolve(goose, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, "goose.exe", spec.Command)
}

func TestUnit_Resolve_Binary_CarriesEnv(t *testing.T) {
	reg := loadFixture(t)
	vt, _ := Find(reg, "vtcode")

	spec, err := Resolve(vt, "linux", "amd64")
	require.NoError(t, err)
	require.Equal(t, "vtcode", spec.Command)
	require.Equal(t, map[string]string{"VT_ACP_ENABLED": "1", "VT_ACP_ZED_ENABLED": "1"}, spec.Env)
}

func TestUnit_Resolve_Binary_PlatformMissError(t *testing.T) {
	reg := loadFixture(t)
	vt, _ := Find(reg, "vtcode")

	// vtcode has no linux-aarch64 entry in the fixture.
	_, err := Resolve(vt, "linux", "arm64")
	require.Error(t, err)
	require.Contains(t, err.Error(), "linux-aarch64")
}

// ─── PlatformKey ────────────────────────────────────────────────────────────

func TestUnit_PlatformKey(t *testing.T) {
	cases := map[[2]string]string{
		{"linux", "amd64"}:   "linux-x86_64",
		{"linux", "arm64"}:   "linux-aarch64",
		{"darwin", "arm64"}:  "darwin-aarch64",
		{"darwin", "amd64"}:  "darwin-x86_64",
		{"windows", "amd64"}: "windows-x86_64",
	}
	for in, want := range cases {
		require.Equal(t, want, PlatformKey(in[0], in[1]), "GOOS=%s GOARCH=%s", in[0], in[1])
	}
}

// ─── Search / Find ──────────────────────────────────────────────────────────

func TestUnit_Search(t *testing.T) {
	reg := loadFixture(t)

	require.Len(t, Search(reg, ""), 6, "empty query returns full catalog")

	byID := Search(reg, "goose")
	require.Len(t, byID, 1)
	require.Equal(t, "goose", byID[0].ID)

	// Case-insensitive match on description.
	byDesc := Search(reg, "MARKETPLACE")
	require.Len(t, byDesc, 1)
	require.Equal(t, "agoragentic-acp", byDesc[0].ID)

	require.Empty(t, Search(reg, "no-such-agent-xyz"))
}

func TestUnit_Find_Miss(t *testing.T) {
	reg := loadFixture(t)
	_, ok := Find(reg, "does-not-exist")
	require.False(t, ok)
}

// ─── Cache read/write + fetch fallback ──────────────────────────────────────

func TestUnit_Fetch_WritesAndReadsCache(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "nested", "agent-registry.json")
	client := &Client{URL: srv.URL, CachePath: cachePath, HTTP: srv.Client()}

	// First fetch: hits the network, writes the cache.
	reg, err := client.Fetch(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, reg.Agents, 6)
	require.Equal(t, 1, hits)
	require.FileExists(t, cachePath)

	// Second fetch without refresh: served from cache, no extra network hit.
	reg2, err := client.Fetch(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, reg2.Agents, 6)
	require.Equal(t, 1, hits, "cached fetch must not hit the network")

	// Refresh forces a re-fetch.
	_, err = client.Fetch(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, 2, hits)
}

func TestUnit_Fetch_FallsBackToCacheOnNetworkError(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "agent-registry.json")
	require.NoError(t, os.WriteFile(cachePath, fixture, 0o644))

	// Unreachable URL forces the network path to fail; cache must save it.
	client := &Client{URL: "http://127.0.0.1:0/registry.json", CachePath: cachePath, HTTP: &http.Client{}}
	reg, err := client.Fetch(context.Background(), true)
	require.NoError(t, err, "must fall back to cache when the network fails")
	require.Len(t, reg.Agents, 6)
}

func TestUnit_Fetch_NetworkErrorNoCacheSurfaces(t *testing.T) {
	client := &Client{
		URL:       "http://127.0.0.1:0/registry.json",
		CachePath: filepath.Join(t.TempDir(), "missing.json"),
		HTTP:      &http.Client{},
	}
	_, err := client.Fetch(context.Background(), true)
	require.Error(t, err, "with no cache to fall back to, the network error must surface")
}

func TestUnit_Fetch_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &Client{
		URL:       srv.URL,
		CachePath: filepath.Join(t.TempDir(), "missing.json"),
		HTTP:      srv.Client(),
	}
	_, err := client.Fetch(context.Background(), true)
	require.Error(t, err)
}
