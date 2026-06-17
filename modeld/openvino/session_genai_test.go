package openvino

import (
	"context"
	"errors"
	"testing"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/transport"
)

// fakeGenAIBackend is a string-prompt GenAI stand-in: it records the prompt it
// is asked to stream and tokenizes by rune count, so the adapter's text mapping
// can be tested without the CGO OpenVINO backend.
type fakeGenAIBackend struct {
	streamPrompts []string
	emit          []string
	closed        bool
}

func (f *fakeGenAIBackend) Stream(_ context.Context, prompt string, _ ovsession.GenerateOptions) (<-chan ovsession.StreamChunk, error) {
	f.streamPrompts = append(f.streamPrompts, prompt)
	ch := make(chan ovsession.StreamChunk, len(f.emit))
	for _, t := range f.emit {
		ch <- ovsession.StreamChunk{Text: t}
	}
	close(ch)
	return ch, nil
}

func (f *fakeGenAIBackend) Tokenize(_ context.Context, prompt string, _ bool) ([]int, error) {
	return make([]int, len([]rune(prompt))), nil
}

func (f *fakeGenAIBackend) ApplyChatTemplate(messages []ovsession.ChatMessage, _ string) (string, error) {
	out := ""
	for _, m := range messages {
		out += "<|" + m.Role + "|>" + m.Content
	}
	return out, nil
}

func (f *fakeGenAIBackend) Close() error { f.closed = true; return nil }

func ovManifest(stableHash, runtimeDigest string) contextasm.ContextManifest {
	return contextasm.ContextManifest{
		Backend:              "openvino",
		ModelDigest:          "model-d1",
		PromptFormat:         "openvino_chat_template",
		PromptTemplateDigest: "model-d1",
		RuntimeDigest:        runtimeDigest,
		StableByteHash:       stableHash,
	}
}

func TestGenaiSessionDecodeConcatenatesStableAndSuffix(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{"hel", "lo"}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "SYSTEM", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 8})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var out string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		out += chunk.Text
	}
	if out != "hello" {
		t.Errorf("decoded text = %q, want %q", out, "hello")
	}
	if len(fake.streamPrompts) != 1 || fake.streamPrompts[0] != "SYSTEMUSER" {
		t.Errorf("streamed prompt = %v, want one prompt %q", fake.streamPrompts, "SYSTEMUSER")
	}
}

func TestGenaiSessionWarmReuseReportedOnRepeatedPrefix(t *testing.T) {
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	cold, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix cold: %v", err)
	}
	if cold.ReusedTokens != 0 || cold.PrefilledTokens != len("STABLE") {
		t.Errorf("cold status = %+v, want reused=0 prefilled=%d", cold, len("STABLE"))
	}
	warm, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix warm: %v", err)
	}
	if warm.ReusedTokens != len("STABLE") || warm.PrefilledTokens != 0 {
		t.Errorf("warm status = %+v, want reused=%d prefilled=0", warm, len("STABLE"))
	}
}

func TestGenaiSessionIncompatibleManifestDropsPrefix(t *testing.T) {
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	ctx := context.Background()

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r1")}); err != nil {
		t.Fatalf("EnsurePrefix first: %v", err)
	}
	// Different runtime digest => incompatible => the resident string prefix must
	// be dropped, not reused.
	got, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r2")})
	if err != nil {
		t.Fatalf("EnsurePrefix second: %v", err)
	}
	if got.ReusedTokens != 0 {
		t.Errorf("reused tokens = %d across an incompatible runtime, want 0", got.ReusedTokens)
	}
	if got.DroppedTokens != len("STABLE") {
		t.Errorf("dropped tokens = %d, want %d", got.DroppedTokens, len("STABLE"))
	}
}

func TestGenaiSessionSuffixManifestMismatchRejected(t *testing.T) {
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	ctx := context.Background()

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r1")}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	_, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: ovManifest("hash-AAA", "r2")})
	if !errors.Is(err, contextasm.ErrManifestMismatch) {
		t.Fatalf("PrefillSuffix mismatch error = %v, want ErrManifestMismatch", err)
	}
}

func TestGenaiSessionCloseStopsUse(t *testing.T) {
	fake := &fakeGenAIBackend{}
	s := newGenaiSession(fake, 4096)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fake.closed {
		t.Error("Close did not reach the backend")
	}
	if _, err := s.EnsurePrefix(context.Background(), transport.PrefixInput{Text: "x"}); !errors.Is(err, transport.ErrSessionClosed) {
		t.Fatalf("EnsurePrefix after close = %v, want ErrSessionClosed", err)
	}
}
