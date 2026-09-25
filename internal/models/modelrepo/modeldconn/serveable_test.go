package modeldconn

import (
	"testing"
	"time"
)

func resetServeableCache() {
	serveableMu.Lock()
	serveableBackend = ""
	serveableSeenAt = time.Time{}
	serveableMu.Unlock()
}

func TestServeableFrom(t *testing.T) {
	t0 := time.Date(2026, 6, 21, 18, 0, 0, 0, time.UTC)

	t.Run("live backend refreshes cache and is returned", func(t *testing.T) {
		resetServeableCache()
		if got := serveableFrom("llama", t0); got != "llama" {
			t.Fatalf("live backend: got %q, want %q", got, "llama")
		}
	})

	t.Run("within grace returns cached backend after lease drops", func(t *testing.T) {
		resetServeableCache()
		serveableFrom("llama", t0)
		if got := serveableFrom("", t0.Add(serveableGraceWindow-time.Second)); got != "llama" {
			t.Fatalf("within grace: got %q, want %q", got, "llama")
		}
	})

	t.Run("after grace returns empty", func(t *testing.T) {
		resetServeableCache()
		serveableFrom("llama", t0)
		if got := serveableFrom("", t0.Add(serveableGraceWindow+time.Second)); got != "" {
			t.Fatalf("after grace: got %q, want empty", got)
		}
		if got := serveableFrom("", t0.Add(serveableGraceWindow+2*time.Second)); got != "" {
			t.Fatalf("after grace cleared: got %q, want empty", got)
		}
	})

	t.Run("no prior observation returns empty", func(t *testing.T) {
		resetServeableCache()
		if got := serveableFrom("", t0); got != "" {
			t.Fatalf("cold gap: got %q, want empty", got)
		}
	})

	t.Run("live observation re-arms the grace window", func(t *testing.T) {
		resetServeableCache()
		serveableFrom("llama", t0)
		serveableFrom("", t0.Add(10*time.Second))
		serveableFrom("llama", t0.Add(20*time.Second))
		if got := serveableFrom("", t0.Add(20*time.Second).Add(serveableGraceWindow-time.Second)); got != "llama" {
			t.Fatalf("re-armed grace: got %q, want %q", got, "llama")
		}
	})
}
