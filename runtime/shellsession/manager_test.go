//go:build !windows

package shellsession

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/runtime/runtimetypes"
)

func newTestManager(t *testing.T, idle time.Duration) *manager {
	t.Helper()
	root := t.TempDir()
	m := NewManager(Config{
		CwdResolver: func(context.Context) string { return root },
		DefaultRoot: root,
		IdleTimeout: idle,
	}).(*manager)
	t.Cleanup(m.Shutdown)
	return m
}

func ctxWithSession(id string) context.Context {
	return context.WithValue(context.Background(), runtimetypes.SessionIDContextKey, id)
}

// waitFor polls until cond is true or the deadline elapses.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func TestManager_RunThenReadReturnsOutput(t *testing.T) {
	m := newTestManager(t, time.Minute)
	ctx := ctxWithSession("sess-1")

	res, err := m.Run(ctx, "sess-1", "echo hallo-welt")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Started {
		t.Fatalf("first run should have created a shell")
	}

	ok := waitFor(t, 3*time.Second, func() bool {
		r := m.Read("sess-1", 0, 0)
		return r.Exists && strings.Contains(r.Content, "hallo-welt")
	})
	if !ok {
		t.Fatalf("scrollback never contained the command output: %q", m.Read("sess-1", 0, 0).Content)
	}
}

func TestManager_ReadUnknownSession(t *testing.T) {
	m := newTestManager(t, time.Minute)
	r := m.Read("nope", 0, 0)
	if r.Exists {
		t.Fatalf("Read on a session with no shell must report Exists=false")
	}
}

func TestManager_SubscribeReceivesOutput(t *testing.T) {
	m := newTestManager(t, time.Minute)
	ctx := ctxWithSession("sess-sub")

	var mu sync.Mutex
	var sawReset bool
	var buf strings.Builder
	cancel := m.Subscribe("sess-sub", func(c Chunk) {
		mu.Lock()
		defer mu.Unlock()
		if c.Reset {
			sawReset = true
			buf.Reset()
		}
		buf.WriteString(c.Data)
	})
	defer cancel()

	// The initial Reset snapshot arrives even before any shell exists.
	if !waitFor(t, time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return sawReset }) {
		t.Fatalf("subscriber never received the initial reset snapshot")
	}

	if _, err := m.Run(ctx, "sess-sub", "echo streamed-line"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ok := waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(buf.String(), "streamed-line")
	})
	if !ok {
		t.Fatalf("subscriber never received the streamed output")
	}
}

func TestManager_KillRemovesShell(t *testing.T) {
	m := newTestManager(t, time.Minute)
	ctx := ctxWithSession("sess-kill")
	if _, err := m.Run(ctx, "sess-kill", "echo alive"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !m.Read("sess-kill", 0, 0).Exists {
		t.Fatalf("shell should exist after run")
	}
	m.Kill("sess-kill")
	if m.Read("sess-kill", 0, 0).Exists {
		t.Fatalf("shell should be gone after Kill")
	}
}

func TestManager_IdleTimeoutReapsShell(t *testing.T) {
	m := newTestManager(t, 150*time.Millisecond)
	ctx := ctxWithSession("sess-idle")
	if _, err := m.Run(ctx, "sess-idle", "echo hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !m.Read("sess-idle", 0, 0).Exists {
		t.Fatalf("shell should exist right after run")
	}
	ok := waitFor(t, 3*time.Second, func() bool {
		return !m.Read("sess-idle", 0, 0).Exists
	})
	if !ok {
		t.Fatalf("idle shell was never reaped")
	}
}
