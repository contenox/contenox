package libsandbox

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Only names in allow survive, and HOME is forced to the scoped dir. This is the
// whole credential-leak fix in one assertion: the parent's secrets are gone, its
// real HOME is replaced, and nothing rides along that was not named.
func TestUnit_scrubEnv_OnlyAllowedNamesPass(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"TERM=xterm",
		"AWS_SECRET_ACCESS_KEY=shhh",
		"HOME=/home/real",
		"CONTENOX_TOKEN=leak",
	}

	got := scrubEnv(parent, []string{"PATH", "TERM"}, nil, "/scoped/home")

	require.Equal(t, []string{
		"HOME=/scoped/home",
		"PATH=/usr/bin",
		"TERM=xterm",
	}, got)
}

// A credential-shaped variable that is not in allow must not appear anywhere in
// the output — neither its name nor its value.
func TestUnit_scrubEnv_CredentialVarNotInAllowIsDropped(t *testing.T) {
	parent := []string{"AWS_SECRET_ACCESS_KEY=shhh", "PATH=/usr/bin"}

	got := scrubEnv(parent, []string{"PATH"}, nil, "/h")

	for _, kv := range got {
		require.NotContains(t, kv, "AWS_SECRET_ACCESS_KEY")
		require.NotContains(t, kv, "shhh")
	}
}

// HOME is authoritative: neither the inherited value (even when HOME is in
// allow) nor an explicit EnvSet HOME can override the scoped home.
func TestUnit_scrubEnv_HomeForcedOverInheritedAndSet(t *testing.T) {
	parent := []string{"HOME=/home/real"}

	got := scrubEnv(parent, []string{"HOME"}, map[string]string{"HOME": "/set/home"}, "/scoped/home")

	require.Contains(t, got, "HOME=/scoped/home")
	require.NotContains(t, got, "HOME=/home/real")
	require.NotContains(t, got, "HOME=/set/home")
}

// set overrides an allow-copied value and can add variables not present in the
// parent at all.
func TestUnit_scrubEnv_SetOverridesAllowCopiedAndAddsExtras(t *testing.T) {
	parent := []string{"FOO=parent", "PATH=/usr/bin"}

	got := scrubEnv(parent, []string{"FOO", "PATH"}, map[string]string{"FOO": "explicit", "EXTRA": "x"}, "/h")

	require.Contains(t, got, "FOO=explicit")
	require.NotContains(t, got, "FOO=parent")
	require.Contains(t, got, "EXTRA=x")
	require.Contains(t, got, "PATH=/usr/bin")
}

// With nothing allowed and nothing set, the confined process still gets exactly
// one variable: the forced scoped HOME.
func TestUnit_scrubEnv_EmptyAllowYieldsOnlyHome(t *testing.T) {
	parent := []string{"PATH=/usr/bin", "SECRET=x"}

	got := scrubEnv(parent, nil, nil, "/scoped")

	require.Equal(t, []string{"HOME=/scoped"}, got)
}

// The output is sorted by name and stable across identical calls: a pure,
// deterministic function is what makes the emitted environment auditable.
func TestUnit_scrubEnv_DeterministicSorted(t *testing.T) {
	parent := []string{"BETA=2", "ALPHA=1", "GAMMA=3"}
	allow := []string{"BETA", "ALPHA", "GAMMA"}

	got := scrubEnv(parent, allow, nil, "/h")

	names := make([]string, len(got))
	for i, kv := range got {
		names[i] = kv[:strings.IndexByte(kv, '=')]
	}
	require.True(t, sort.StringsAreSorted(names), "env names must be sorted: %v", names)
	require.Equal(t, got, scrubEnv(parent, allow, nil, "/h"))
}

// A malformed parent entry (no "=") has no name to match and is dropped.
func TestUnit_scrubEnv_SkipsMalformedParentEntry(t *testing.T) {
	parent := []string{"NOTAKEYVALUE", "PATH=/usr/bin"}

	got := scrubEnv(parent, []string{"PATH"}, nil, "/h")

	require.Equal(t, []string{"HOME=/h", "PATH=/usr/bin"}, got)
}
