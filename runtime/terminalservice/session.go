package terminalservice

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

type session struct {
	id                string
	tty               *os.File
	cmd               *exec.Cmd
	busy              atomic.Bool
	lastActivityNanos atomic.Int64
}

func (s *session) touch() {
	s.lastActivityNanos.Store(time.Now().UnixNano())
}

func (s *session) lastActivity() time.Time {
	return time.Unix(0, s.lastActivityNanos.Load())
}

func (s *session) shutdown(_ context.Context) error {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.tty != nil {
		_ = s.tty.Close()
	}
	return nil
}
