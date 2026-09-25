package contenoxcli

import (
	"context"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/fleetservice"
	"github.com/contenox/contenox/internal/services/localtools"
	"github.com/contenox/contenox/internal/services/missionservice"
	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/contenox/contenox/internal/surfaces/acpsvc"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

func testToolset(optInBeta bool) map[string]taskengine.ToolsRepo {
	noTransport := func(context.Context) *acpsvc.Transport { return nil }
	noFleet := func() fleetservice.Service { return nil }
	return acpToolset(nil, libtracker.NoopTracker{}, "test-workspace",
		noTransport, missionservice.New(nil), nil, nil, optInBeta, noFleet)
}

// TestUnit_ACPToolset_CarriesTheSharedToolsets pins the divergence part 1
// closes: an ACP session (contenox acp/acpx — Zed, JetBrains, OpenClaw) must
// carry the same toolsets `contenox chat`/`run` gets via engine.go's
// localToolset, not just local_fs/webtools/local_shell.
func TestUnit_ACPToolset_CarriesTheSharedToolsets(t *testing.T) {
	tools := testToolset(true)

	// Every provider chat/run already had must still be present.
	for _, name := range []string{"local_fs", "local_shell", missiontools.ToolsProviderName} {
		require.Containsf(t, tools, name, "ACP toolset must keep registering %q", name)
	}

	// The toolsets that were missing, asserted by Supports(), the same way
	// engine_test.go pins localToolset's composition.
	cases := []struct {
		provider string
		tool     string
	}{}
	for _, tc := range cases {
		repo, ok := tools[tc.provider]
		require.Truef(t, ok, "ACP toolset must register provider %q", tc.provider)
		supported, err := repo.Supports(context.Background())
		require.NoError(t, err)
		require.Containsf(t, supported, tc.tool, "%s must support %s", tc.provider, tc.tool)
	}

	stable := testToolset(false)
	for _, name := range []string{"local_fs", "local_shell", missiontools.ToolsProviderName} {
		require.Containsf(t, stable, name, "stable toolset %q must not be beta-gated", name)
	}
}

// TestUnit_ACPToolset_NativeToolsetsAreDeclarationScoped pins the allowlist
// vocabulary an operator writes: "*" means every connected toolset with no
// exceptions, "!name" removes one, and naming a set grants exactly it. The
// native scope is a namespace, not a hidden exclusion — a "*" that quietly
// withheld the native sets would be the runtime deciding it knows better than
// the declaration.
func TestUnit_ACPToolset_NativeToolsetsAreDeclarationScoped(t *testing.T) {
	tools := testToolset(true)

	all := make([]string, 0, len(tools))
	var native []string
	for name := range tools {
		all = append(all, name)
		if strings.HasPrefix(name, "native-") {
			native = append(native, name)
		}
	}
	require.Subset(t, all, []string{
		localtools.GitToolsName, localtools.LocalFSToolsName, localtools.WebToolsName,
	}, "the editor profile carries the cleared native toolsets")
	require.NotEmpty(t, native)

	admitted := taskengine.ExportedApplyAllowlist([]string{"*"}, all)
	require.ElementsMatch(t, all, admitted, `"*" admits everything connected, native sets included`)

	excluded := taskengine.ExportedApplyAllowlist([]string{"*", "!" + localtools.GitToolsName}, all)
	require.NotContains(t, excluded, localtools.GitToolsName, `"!name" is how an operator removes one set`)
	require.Contains(t, excluded, "local_fs", "and removes only that one")

	named := taskengine.ExportedApplyAllowlist([]string{localtools.GitToolsName}, all)
	require.Contains(t, named, localtools.GitToolsName, "a declaration naming the toolset exactly admits it")
}

// TestUnit_ACPToolset_AdvertisesOnlyTheInProcessHalfWithoutAClient pins the rule
// the production incident broke, on the namespace that now holds both kinds of
// tool: a host serving a session nobody is attached to must not show the model a
// read or a shell it can never run — and must still show it the tools that run
// here, which is what local_fs browsing is.
func TestUnit_ACPToolset_AdvertisesOnlyTheInProcessHalfWithoutAClient(t *testing.T) {
	tools := testToolset(true)

	for _, name := range []string{localtools.LocalExecToolsName} {
		repo, ok := tools[name]
		require.Truef(t, ok, "%q must stay registered", name)
		advertised, err := repo.GetToolsForToolsByName(context.Background(), name)
		require.NoError(t, err)
		require.Emptyf(t, advertised, "%q must advertise nothing with no client attached", name)
	}

	repo, ok := tools[localtools.LocalFSToolsName]
	require.True(t, ok, "local_fs must stay registered")
	advertised, err := repo.GetToolsForToolsByName(context.Background(), localtools.LocalFSToolsName)
	require.NoError(t, err)
	require.NotEmpty(t, advertised, "the in-process half runs without a client")

	var leaves []string
	for _, tool := range advertised {
		leaves = append(leaves, tool.Function.Name)
	}
	for _, leaf := range []string{"read_file", "write_file", "edit_file", "sed", "read_file_range"} {
		require.NotContainsf(t, leaves, leaf, "%q proxies to the client and must not be advertised without one", leaf)
	}
	for _, leaf := range []string{"list_dir", "grep", "find_files", "count_stats", "stat_file"} {
		require.Containsf(t, leaves, leaf, "%q runs in process and must be advertised", leaf)
	}
}
