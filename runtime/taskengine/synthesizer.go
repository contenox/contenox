package taskengine

import (
	"fmt"
	"time"
)

// SynthesizeHistory rebuilds a conversation transcript from a chain run by
// walking the captured step stream. It exists so that hard-failed turns
// (errors, timeouts, cancellations, denied/timed-out HITL gates) make it
// into the persisted ChatHistory — the chain's returned ChatHistory only
// contains messages from steps that completed successfully.
//
// prior is the session history that was sent into the chain.
// units is the captured step stream from Inspector.GetExecutionHistory().
// chainErr is the error returned by the chain runner, if any.
//
// The result is a candidate []Message ready for chatservice.PersistDiff —
// PersistDiff handles dedupe against already-stored messages by ID, so the
// synthesizer is free to emit overlapping prefixes between runs.
func SynthesizeHistory(prior []Message, units []CapturedStateUnit, chainErr error) []Message {
	out := make([]Message, 0, len(prior)+len(units))
	out = append(out, prior...)

	seenIDs := make(map[string]bool)
	for _, m := range prior {
		if m.ID != "" {
			seenIDs[m.ID] = true
		}
	}

	lastUnitErrored := false
	for _, unit := range units {
		appendedFromOutput := false

		if unit.OutputType == DataTypeChatHistory {
			if outHist, ok := unit.Output.(ChatHistory); ok {
				startIdx := 0
				if inHist, ok := unit.Input.(ChatHistory); ok {
					startIdx = len(inHist.Messages)
				}
				if startIdx > len(outHist.Messages) {
					startIdx = len(outHist.Messages)
				}
				for _, msg := range outHist.Messages[startIdx:] {
					if msg.ID != "" {
						if seenIDs[msg.ID] {
							continue
						}
						seenIDs[msg.ID] = true
					}
					out = append(out, msg)
					appendedFromOutput = true
				}
			}
		}

		if unit.Error.Error != "" {
			out = append(out, failureAnnotation(unit))
			lastUnitErrored = true
		} else if appendedFromOutput {
			lastUnitErrored = false
		}
	}

	if chainErr != nil && !lastUnitErrored {
		out = append(out, Message{
			Role:      "assistant",
			Content:   fmt.Sprintf("[chain failed: %s]", chainErr.Error()),
			Timestamp: time.Now().UTC(),
		})
	}

	return out
}

func failureAnnotation(unit CapturedStateUnit) Message {
	var content string
	switch {
	case unit.Cancelled:
		content = fmt.Sprintf("[step %q (%s) was cancelled before completion]", unit.TaskID, unit.TaskHandler)
	case unit.TimedOut:
		content = fmt.Sprintf("[step %q (%s) timed out]", unit.TaskID, unit.TaskHandler)
	default:
		content = fmt.Sprintf("[step %q (%s) failed: %s]", unit.TaskID, unit.TaskHandler, unit.Error.Error)
	}
	return Message{
		Role:      "assistant",
		Content:   content,
		Timestamp: time.Now().UTC(),
	}
}
