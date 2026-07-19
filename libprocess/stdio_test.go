package libprocess_test

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/libprocess"
	"github.com/stretchr/testify/require"
)

// PipeStdio's whole point: the supervised command is a peer to converse with,
// not a job whose output is dumped somewhere. A sink cannot host a protocol.
func TestUnit_Process_PipeStdioIsATwoWayTransport(t *testing.T) {
	cmd, args := shArgs(`while read -r line; do echo "got:$line"; done`)
	p := newProc(t, libprocess.Config{
		Command:      cmd,
		Args:         args,
		PipeStdio:    true,
		GracefulStop: libprocess.CloseStdin,
		StopGrace:    2 * time.Second,
	})

	require.NoError(t, p.Start(t.Context()))
	stdio := p.Stdio()
	require.NotNil(t, stdio)

	r := bufio.NewReader(stdio)
	for _, want := range []string{"one", "two"} {
		_, err := stdio.Write([]byte(want + "\n"))
		require.NoError(t, err)
		line, err := r.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "got:"+want, strings.TrimSpace(line))
	}

	// CloseStdin is the graceful strategy here, so the shell sees EOF on stdin
	// and exits on its own — no signal, no kill.
	require.NoError(t, p.Stop(t.Context()))
	require.Equal(t, libprocess.Stopped, p.State())
}

// Reading a dead peer must terminate, which is only true because the single
// cmd.Wait in watch closes the pipes. A second Wait, or none, would leave a
// reader blocked forever.
func TestUnit_Process_PipeStdioReadsEOFAfterExit(t *testing.T) {
	cmd, args := shArgs(`echo hello`)
	p := newProc(t, libprocess.Config{Command: cmd, Args: args, PipeStdio: true})

	require.NoError(t, p.Start(t.Context()))
	stdio := p.Stdio()
	require.NotNil(t, stdio)

	read := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 64)
		for {
			n, err := stdio.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				read <- sb.String()
				return
			}
		}
	}()

	select {
	case got := <-read:
		require.Equal(t, "hello", strings.TrimSpace(got))
	case <-time.After(3 * time.Second):
		t.Fatal("Read never saw EOF after the command exited")
	}
	<-p.Done()
}

// Sinks and a transport are two different answers to "who owns stdio", and
// either could plausibly be the one meant, so asking for both is rejected at
// construction rather than resolved by a silent precedence rule.
func TestUnit_New_RejectsPipeStdioWithSinks(t *testing.T) {
	_, err := libprocess.New(libprocess.Config{
		Command:   "/bin/true",
		PipeStdio: true,
		Stdout:    &libprocess.LockedBuffer{},
	})
	require.Error(t, err)

	_, err = libprocess.New(libprocess.Config{
		Command:   "/bin/true",
		PipeStdio: true,
		Stdin:     strings.NewReader("x"),
	})
	require.Error(t, err)
}

// CloseStdin has no stdin to close without PipeStdio; Stop must treat that
// like any other undeliverable request and still end the process.
func TestUnit_CloseStdin_WithoutPipeStdioIsAFailedRequestNotAHang(t *testing.T) {
	require.ErrorIs(t, libprocess.CloseStdin(context.Background(), libprocess.Instance{}), libprocess.ErrNoStdio)

	cmd, args := shArgs(`trap "" INT; while true; do sleep 0.05; done`)
	p := newProc(t, libprocess.Config{
		Command:      cmd,
		Args:         args,
		GracefulStop: libprocess.CloseStdin,
		StopGrace:    150 * time.Millisecond,
	})
	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, time.Second, 10*time.Millisecond)

	require.NoError(t, p.Stop(t.Context()), "kill-induced exit is Stop working, not failing")
	require.Equal(t, libprocess.Stopped, p.State())
}

// The supervisor's stderr sink is written by os/exec's copier goroutine while
// the caller reads it from another — the race a bare bytes.Buffer would lose.
func TestUnit_LockedBuffer_CapturesStderrAcrossGoroutines(t *testing.T) {
	buf := &libprocess.LockedBuffer{}
	cmd, args := shArgs(`echo boom >&2; exit 7`)
	p := newProc(t, libprocess.Config{Command: cmd, Args: args, Stderr: buf})

	require.NoError(t, p.Start(t.Context()))

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = buf.String()
				_ = buf.Bytes()
			}
		}()
	}
	wg.Wait()

	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process did not finish in time")
	}
	require.Contains(t, buf.String(), "boom")
}

// Start's context governs the whole supervised lifetime: a cancelled context
// that merely abandoned the supervisor would leave an OS process running that
// nobody holds a handle to any more.
func TestUnit_Process_ContextCancellationShutsTheProcessDown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cmd, args := shArgs(`while true; do sleep 0.05; done`)
	p := newProc(t, libprocess.Config{
		Command:   cmd,
		Args:      args,
		Restart:   libprocess.RestartPolicy{Enabled: true, Always: true},
		StopGrace: time.Second,
	})

	require.NoError(t, p.Start(ctx))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling Start's context left the process running")
	}
	require.Equal(t, 0, p.Pid(), "a supervised process survived its context")
}

// The same must hold while the supervisor is parked in a restart delay: there
// is no process to signal, but the lifetime still has to end.
func TestUnit_Process_ContextCancellationEndsARestartDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cmd, args := shArgs(`exit 1`)
	p := newProc(t, libprocess.Config{
		Command:   cmd,
		Args:      args,
		Restart:   libprocess.RestartPolicy{Enabled: true, Delay: 10 * time.Second},
		StopGrace: 200 * time.Millisecond,
	})

	require.NoError(t, p.Start(ctx))
	require.Eventually(t, func() bool { return p.Pid() == 0 }, 3*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("cancelling Start's context did not interrupt the restart delay")
	}
}

// A classifier replaces the exit-code rules outright: here it restarts a
// *clean* exit that Enabled/Always would never have restarted, and is capped
// only by Limit.
func TestUnit_Process_ShouldRestartClassifierDecidesRestarts(t *testing.T) {
	var mu sync.Mutex
	var seen []error
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{
			Limit: 2,
			ShouldRestart: func(exitErr error) bool {
				mu.Lock()
				seen = append(seen, exitErr)
				mu.Unlock()
				return true
			},
		},
	})

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not reach a terminal state in time")
	}

	require.Equal(t, libprocess.Crashed, p.State(), "the classifier's restarts must still honour Limit")
	require.Equal(t, 2, p.Restarts())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 3)
	require.NoError(t, seen[0], "a clean exit reaches the classifier as a nil error")
}

// A classifier that declines ends the lifetime even though Enabled/Always
// would have restarted forever.
func TestUnit_Process_ShouldRestartClassifierCanDeclineOverridingAlways(t *testing.T) {
	cmd, args := shArgs("exit 1")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{
			Enabled:       true,
			Always:        true,
			ShouldRestart: func(error) bool { return false },
		},
	})

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process did not reach a terminal state in time")
	}
	require.Equal(t, libprocess.Stopped, p.State())
	require.Equal(t, 0, p.Restarts())
}

// A start failure is never routed through the classifier and never retried: a
// retry cannot cure a bad binary.
func TestUnit_Process_StartFailureIsNeverClassifiedOrRetried(t *testing.T) {
	var classified int
	var mu sync.Mutex
	p := newProc(t, libprocess.Config{
		Command: "libprocess-definitely-not-a-real-binary",
		Restart: libprocess.RestartPolicy{
			ShouldRestart: func(error) bool {
				mu.Lock()
				classified++
				mu.Unlock()
				return true
			},
		},
	})

	require.Error(t, p.Start(t.Context()))
	require.Equal(t, libprocess.Crashed, p.State())
	mu.Lock()
	defer mu.Unlock()
	require.Zero(t, classified)
}

// Backoff supersedes the fixed Delay, and receives 1-based attempt numbers so
// a caller can actually grow the delay.
func TestUnit_Process_BackoffSupersedesFixedDelay(t *testing.T) {
	var mu sync.Mutex
	var attempts []int
	cmd, args := shArgs("exit 1")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{
			Enabled: true,
			Limit:   2,
			Delay:   30 * time.Second, // must never be waited
			Backoff: func(attempt int) time.Duration {
				mu.Lock()
				attempts = append(attempts, attempt)
				mu.Unlock()
				return 20 * time.Millisecond
			},
		},
	})

	start := time.Now()
	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Backoff did not supersede Delay: still waiting out the fixed delay")
	}
	require.Less(t, time.Since(start), 5*time.Second)
	require.Equal(t, libprocess.Crashed, p.State())

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int{1, 2}, attempts)
}

// Stop reports what it shut down. A command that died of its own bad status
// is real news even though it happened during our shutdown; one that died of
// our own escalation is not.
func TestUnit_Process_StopSurfacesAGenuineBadExit(t *testing.T) {
	cmd, args := shArgs(`trap 'exit 3' INT; while true; do sleep 0.05; done`)
	p := newProc(t, libprocess.Config{Command: cmd, Args: args, StopGrace: 2 * time.Second})

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // let the shell install its trap

	err := p.Stop(t.Context())
	require.Error(t, err, "a nonzero exit during shutdown must not be swallowed")
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "Stop must surface the command's own exit error")
	require.Equal(t, 3, exitErr.ExitCode())
	require.Equal(t, libprocess.Stopped, p.State())
}
