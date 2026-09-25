package app

import (
	"testing"

	"github.com/contenox/contenox/internal/surfaces/beam/enginebridge"
	"github.com/contenox/contenox/internal/surfaces/beam/input"
)

// askEvent is one mission ask arriving in the session on screen.
func askEvent(askID string) enginebridge.MissionAsk {
	return enginebridge.MissionAsk{
		SessionID: testSession,
		MissionID: "mis-1",
		AskID:     askID,
		AgentName: "scout",
		Summary:   "may I widen the search?",
		Text:      "may I widen the search?",
		MessageID: "mission-ask-" + askID,
	}
}

// TestUnit_Answer_BareAnswerCompletesWithThePendingAsk pins the one thing the
// TUI can do that no CLI verb can: name the ask on screen, whose id is a uuid
// the operator cannot read. A lone /answer fills the id in and hands the line
// back, because the reply itself is the operator's to type.
func TestUnit_Answer_BareAnswerCompletesWithThePendingAsk(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.deliver(askEvent("ask-7"))

	h.typeText("/answer")
	h.press(input.KeyEnter)

	if got, want := h.a.comp.Draft(), "/answer ask-7 "; got != want {
		t.Fatalf("the draft was not completed with the pending ask: got %q want %q", got, want)
	}
	requireNotContains(t, h.calls(), "SubmitPrompt", "completing must not send the command")

	// Finishing the line is what actually answers the unit.
	h.typeText("yes, widen it")
	h.press(input.KeyEnter)
	requireContains(t, h.calls(), `SubmitPrompt(beam-test-session, "/answer ask-7 yes, widen it")`,
		"the completed answer reaches the core")
}

// TestUnit_Answer_NamesTheNewestAsk pins that a second ask supersedes the
// first, since the card the operator is looking at is the newest one.
func TestUnit_Answer_NamesTheNewestAsk(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.deliver(askEvent("ask-1"), askEvent("ask-2"))

	h.typeText("/answer")
	h.press(input.KeyEnter)

	if got, want := h.a.comp.Draft(), "/answer ask-2 "; got != want {
		t.Fatalf("the draft names the wrong ask: got %q want %q", got, want)
	}
}

// TestUnit_Answer_WithoutAPendingAskIsTheAgentsCommand pins the guard: with
// nothing waiting, a lone /answer is the core's own listing command and must
// reach the agent untouched rather than being rewritten.
func TestUnit_Answer_WithoutAPendingAskIsTheAgentsCommand(t *testing.T) {
	h := newHarness(t)
	h.start()

	h.typeText("/answer")
	h.press(input.KeyEnter)

	if got := h.a.comp.Draft(); got != "" {
		t.Fatalf("the draft was rewritten or kept: %q", got)
	}
	requireContains(t, h.calls(), `SubmitPrompt(beam-test-session, "/answer")`,
		"a lone /answer with nothing waiting belongs to the agent")
}

// TestUnit_Answer_AnAnsweredAskDoesNotOutliveItsCard pins that beam does not
// keep offering an id once the operator has supplied their own: a typed
// /answer <id> text is never rewritten, and an id-less /answer after that is
// still the operator's line, not silently re-pointed at the old ask.
func TestUnit_Answer_AnAnsweredAskDoesNotOutliveItsCard(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.deliver(askEvent("ask-9"))

	h.typeText("/answer ask-9 keep going")
	h.press(input.KeyEnter)

	requireContains(t, h.calls(), `SubmitPrompt(beam-test-session, "/answer ask-9 keep going")`,
		"a fully typed answer is sent verbatim")
}

// TestUnit_Answer_TheCardNamesTheCommand pins that the ask card itself teaches
// the command, since the completion is useless if the operator never learns
// the ask is answerable from here at all.
func TestUnit_Answer_TheCardNamesTheCommand(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.deliver(askEvent("ask-3"))

	requireContains(t, h.scrollback(), "/answer ask-3",
		"the ask card carries the reply command")
	requireContains(t, h.scrollback(), "is waiting on you", "the card still reads as an ask")
}

// TestUnit_Answer_CompletionSurvivesAnEmptyAskID pins that an ask carrying no
// id is remembered as nothing: there is no id to complete, so a lone /answer
// stays the agent's command rather than completing to an empty argument.
func TestUnit_Answer_CompletionSurvivesAnEmptyAskID(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.deliver(askEvent(""))

	h.typeText("/answer")
	h.press(input.KeyEnter)

	requireContains(t, h.calls(), `SubmitPrompt(beam-test-session, "/answer")`,
		"an id-less ask must not complete the command")
}
