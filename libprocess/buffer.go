package libprocess

import (
	"bytes"
	"sync"
)

// LockedBuffer is a concurrency-safe io.Writer around a bytes.Buffer, meant
// for Config.Stderr (or Config.Stdout): the supervised command's output is
// written by os/exec's own copier goroutine while the caller reads String()
// from another, typically only once something has already gone wrong and the
// command's stderr is wanted for a failure message. A bare bytes.Buffer in
// that position is a data race that a passing test will not show you, because
// the reading side usually only runs on the failure path.
//
// It is deliberately unbounded: it is for short-lived commands and diagnostic
// tails, not for streaming the output of a long-running daemon. Wire an
// io.Pipe or a ring buffer for that.
type LockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the buffer.
func (b *LockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns everything written so far.
func (b *LockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Bytes returns a copy of everything written so far. It copies rather than
// exposing the buffer's own slice, which a concurrent Write may reallocate or
// overwrite underneath the caller.
func (b *LockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// Reset discards the buffered output.
func (b *LockedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
