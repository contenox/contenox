package jseval

import (
	"sync"
	"time"
)

// ExecLogEntry is a single line in the JS execution trace.
type ExecLogEntry struct {
	Timestamp time.Time      `json:"ts"`
	Kind      string         `json:"kind"`            // "console", "sendEvent", "executeTask", "executeTaskChain", ...
	Level     string         `json:"level,omitempty"` // for console logs, e.g. "log", "error"
	Name      string         `json:"name,omitempty"`  // builtin / function name (sendEvent, executeTask, etc)
	Message   string         `json:"message,omitempty"`
	Args      []any          `json:"args,omitempty"` // raw args for console / calls
	Meta      map[string]any `json:"meta,omitempty"` // arbitrary extra data (event_type, chain_id, etc)
	Error     string         `json:"error,omitempty"`
}

// Collector accumulates logs and call records for a single JS execution.
type Collector struct {
	mu   sync.Mutex
	logs []ExecLogEntry
}

func NewCollector() *Collector {
	return &Collector{
		logs: make([]ExecLogEntry, 0, 16),
	}
}

func (c *Collector) Add(entry ExecLogEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logs = append(c.logs, entry)
}

func (c *Collector) Logs() []ExecLogEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ExecLogEntry, len(c.logs))
	copy(out, c.logs)
	return out
}
