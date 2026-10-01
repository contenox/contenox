package app

import (
	"encoding/json"
	"testing"

	"github.com/contenox/contenox/internal/surfaces/beam/enginebridge"
	"github.com/contenox/contenox/internal/surfaces/beam/input"
	"github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

func TestUnit_MissionPickerDoesNotDispatchSelections(t *testing.T) {
	h := newHarness(t).start()
	meta, err := json.Marshal(libacp.CommandCompletionMeta{Completions: map[string]libacp.CommandCompletion{
		"":                           {Values: []string{"--policy default.json", "--policy strict.json"}},
		"--policy":                   {Values: []string{"strict.json"}},
		"--policy strict.json":       {Values: []string{"codex"}},
		"--policy strict.json codex": {Hint: "Type the intent"},
	}})
	require.NoError(t, err)
	h.deliver(enginebridge.CommandsUpdated{SessionID: testSession, Commands: []libacp.AvailableCommand{{Name: "mission", Meta: meta}}})
	h.typeText("/mission")
	require.Equal(t, []string{"--policy default.json", "--policy strict.json"}, h.a.pal.FilteredValues())
	h.press(input.KeyDown)
	h.press(input.KeyEnter)
	require.Equal(t, "/mission --policy strict.json ", h.a.comp.Draft())
	h.press(input.KeyEnter)
	require.Equal(t, "/mission --policy strict.json codex ", h.a.comp.Draft())
	h.press(input.KeyEnter)
	requireNotContains(t, h.calls(), "SubmitPrompt", "selections and empty intent must not dispatch")
	h.typeText("Review the changes")
	h.press(input.KeyEnter)
	requireContains(t, h.calls(), `SubmitPrompt(beam-test-session, "/mission --policy strict.json codex Review the changes")`, "mission dispatch")
}
