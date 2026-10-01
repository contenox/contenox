package palette_test

import (
	"encoding/json"
	"testing"

	"github.com/contenox/contenox/internal/surfaces/beam/comp/palette"
	"github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

func TestUnit_MissionArgumentCompletions(t *testing.T) {
	meta, err := json.Marshal(libacp.CommandCompletionMeta{Completions: map[string]libacp.CommandCompletion{
		"":                           {Values: []string{"--policy", "codex"}, Hint: "Choose an agent"},
		"--policy":                   {Values: []string{"strict.json"}, Hint: "Choose an envelope"},
		"--policy strict.json":       {Values: []string{"codex"}},
		"codex":                      {Hint: "Type the intent"},
		"--policy strict.json codex": {Hint: "Type the intent"},
	}})
	require.NoError(t, err)
	p := palette.New()
	p.SetRemote([]libacp.AvailableCommand{{Name: "mission", Meta: meta}})
	for _, tc := range []struct{ draft, want string }{
		{"/mission", "/mission --policy "},
		{"/mission co", "/mission codex "},
		{"/mission --policy st", "/mission --policy strict.json "},
		{"/mission --policy strict.json co", "/mission --policy strict.json codex "},
	} {
		p.Open(tc.draft)
		got, ok := p.CompleteValueText()
		require.True(t, ok, tc.draft)
		require.Equal(t, tc.want, got)
	}
	p.Open("/mission codex ")
	require.True(t, p.AwaitingArgument())
	hint, ok := p.ArgHint("/mission codex ")
	require.True(t, ok)
	require.Equal(t, "Type the intent", hint)
	p.Open("/mission codex Review the changes")
	require.False(t, p.AwaitingArgument())
	_, ok = p.CompleteValueText()
	require.False(t, ok, "completion must not replace the intent")
	p.SetRemote([]libacp.AvailableCommand{{Name: "mission"}})
	p.Open("/mission ")
	require.Nil(t, p.FilteredValues(), "session switches must discard old choices")
}
