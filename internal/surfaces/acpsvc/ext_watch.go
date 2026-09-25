package acpsvc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/libacp"
)

const (
	extMethodFSWatch   = "_contenox/fs/watch"
	extMethodFSUnwatch = "_contenox/fs/unwatch"
	fsEventMethod      = "_contenox/fs/event"
)

type fsEvent struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // created | changed | deleted
}

type fsEntryLight struct {
	Size    int64     `json:"-"`
	ModTime time.Time `json:"-"`
}

type fsSnapshot map[string]fsEntryLight

// fsWatchInterval is the poll cadence of a watch loop, a variable so tests can
// shorten it.
var fsWatchInterval = time.Second

// fsScan snapshots every file under root (relative keys), stopping early when
// ctx is done and never descending into a control-plane directory, whose
// changes must not surface as events.
func fsScan(ctx context.Context, root string) (fsSnapshot, error) {
	snap := fsSnapshot{}
	denied := vfs.ControlPlaneDenied()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if _, deniedHere := vfs.WithinControlPlane(denied, path); deniedHere {
				return filepath.SkipDir
			}
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		snap[rel] = fsEntryLight{Size: fi.Size(), ModTime: fi.ModTime()}
		return nil
	})
	return snap, err
}

// fsDiffSnap returns the change events between two snapshots, ordered by path
// so a diff is deterministic for its consumer.
func fsDiffSnap(prev, next fsSnapshot) []fsEvent {
	var events []fsEvent
	for p, e := range next {
		old, ok := prev[p]
		if !ok {
			events = append(events, fsEvent{Path: p, Kind: "created"})
		} else if old.Size != e.Size || !old.ModTime.Equal(e.ModTime) {
			events = append(events, fsEvent{Path: p, Kind: "changed"})
		}
	}
	for p := range prev {
		if _, ok := next[p]; !ok {
			events = append(events, fsEvent{Path: p, Kind: "deleted"})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Path < events[j].Path })
	return events
}

func (t *Transport) handleFSWatch(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSWatch + " requires the host workspace")
	}
	root := svc.Root()

	t.fsWatchMu.Lock()
	first := t.fsWatchCount == 0
	t.fsWatchCount++
	t.fsWatchMu.Unlock()

	if first {
		loopCtx, cancel := context.WithCancel(t.connCtx)
		t.fsWatchCancel = cancel
		go t.fsWatchLoop(loopCtx, root)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (t *Transport) handleFSUnwatch(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	t.fsWatchMu.Lock()
	if t.fsWatchCount > 0 {
		t.fsWatchCount--
		if t.fsWatchCount == 0 && t.fsWatchCancel != nil {
			t.fsWatchCancel()
			t.fsWatchCancel = nil
		}
	}
	t.fsWatchMu.Unlock()
	return json.RawMessage(`{"ok":true}`), nil
}

func (t *Transport) fsWatchLoop(ctx context.Context, root string) {
	prev, _ := fsScan(ctx, root)
	ticker := time.NewTicker(fsWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next, err := fsScan(ctx, root)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			events := fsDiffSnap(prev, next)
			prev = next
			for _, ev := range events {
				raw, _ := json.Marshal(ev)
				if t.conn == nil || t.conn.SendExtNotification(fsEventMethod, raw) != nil {
					// A dead connection cannot be told; the loop stops and the
					// transport's own shutdown tears the watch state down.
					return
				}
			}
		}
	}
}
