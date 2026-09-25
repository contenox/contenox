//go:build llamanode && llamacpp_direct

package llamasession

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/llama"
)

func TestSystem_RecurrentSession_DecodeAndChangedPrefix(t *testing.T) {
	path := os.Getenv("CONTENOX_LLAMA_RECURRENT_GGUF")
	if path == "" {
		t.Skip("CONTENOX_LLAMA_RECURRENT_GGUF is not set")
	}
	sess, err := New(path, llama.Config{NumCtx: 512, NumBatch: 64, NumThreads: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	native := sess.(*session)
	if !native.model.HasRecurrentMemory() {
		t.Fatal("fixture has no recurrent state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for turn, stable := range []string{"system\nYou are helpful.\n", "system\nYou are concise.\n"} {
		suffix := "user\nHello\nassistant\n"
		manifest := tinyManifest(stable, suffix)
		manifest.Segments = nil
		prefix, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: manifest})
		if err != nil {
			t.Fatal(err)
		}
		if turn > 0 && prefix.ReusedTokens != 0 {
			t.Fatalf("recomputed recurrent prefix reported %d reused tokens", prefix.ReusedTokens)
		}
		if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: suffix, Manifest: manifest}); err != nil {
			t.Fatal(err)
		}
		if _, err := decodeOne(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
}
