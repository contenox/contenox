// Package sessionkit holds the small backend-neutral helpers shared by the
// modeld transport.Session adapters (llama.cpp in modeld/llama/llamasession and
// OpenVINO in modeld/openvino). Both adapters drive the same
// EnsurePrefix/PrefillSuffix/Decode contract over genuinely different engines,
// so their KV mechanics stay separate — but the surrounding plumbing (cancel-safe
// stream sends, longest-common-prefix reuse, the chat-template role vocabulary)
// is identical and lives here once instead of drifting as per-adapter copies.
package sessionkit

import "context"

// Send delivers v on ch, or reports false if ctx is canceled before a slot is
// free. Decode loops use the bool to stop streaming once the consumer is gone.
func Send[T any](ctx context.Context, ch chan<- T, v T) bool {
	select {
	case ch <- v:
		return true
	case <-ctx.Done():
		return false
	}
}

// TrySend delivers v on ch if a slot is immediately available, otherwise drops
// it. It is the terminal best-effort send used to surface a final error after
// ctx is already done, where blocking is not an option.
func TrySend[T any](ch chan<- T, v T) {
	select {
	case ch <- v:
	default:
	}
}

// CommonPrefixLen returns the length of the longest shared prefix of a and b.
// The adapters use it to find how many already-resident tokens a new prefix can
// reuse before the divergent tail must be (re)prefilled.
func CommonPrefixLen(a, b []int) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// ChatRole maps a manifest segment kind to a chat-template role, or "" for a
// control segment the model's own template renders itself (BOS, the assistant
// generation cue). Centralizing the role vocabulary keeps the two adapters from
// recognizing different role sets.
func ChatRole(kind string) string {
	switch kind {
	case "system", "user", "assistant", "tool":
		return kind
	default:
		return ""
	}
}
