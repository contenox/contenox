package terminalservice

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

type session struct {
	id                    string
	tty                   *os.File
	input                 *os.File
	output                *os.File
	cmd                   *exec.Cmd
	pseudoConsole         uintptr
	processHandle         uintptr
	waitOwnsProcessHandle bool
	busy                  atomic.Bool
	lastActivityNanos     atomic.Int64
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
	if s.processHandle != 0 {
		_ = terminateProcessHandle(s.processHandle)
		if !s.waitOwnsProcessHandle {
			_ = closePlatformHandle(s.processHandle)
		}
		s.processHandle = 0
	}
	if s.pseudoConsole != 0 {
		closePseudoConsoleHandle(s.pseudoConsole)
		s.pseudoConsole = 0
	}
	if s.tty != nil {
		_ = s.tty.Close()
	}
	if s.input != nil {
		_ = s.input.Close()
	}
	if s.output != nil {
		_ = s.output.Close()
	}
	return nil
}
