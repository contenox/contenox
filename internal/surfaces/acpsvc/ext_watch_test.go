package acpsvc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/services/localfileservice"
	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

func TestUnit_fsScanStopsOnCanceledContext(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fsScan(ctx, root)
	require.ErrorIs(t, err, context.Canceled)
}

func TestUnit_fsScanSkipsControlPlaneDirs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "keep.txt"), []byte("x"), 0o644))
	cp := filepath.Join(root, "cp")
	require.NoError(t, os.MkdirAll(cp, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cp, "secret.txt"), []byte("x"), 0o644))

	require.NoError(t, vfs.SetControlPlaneDenied(cp))
	t.Cleanup(func() { _ = vfs.SetControlPlaneDenied() })

	snap, err := fsScan(context.Background(), root)
	require.NoError(t, err)
	_, ok := snap["keep.txt"]
	require.True(t, ok)
	_, ok = snap[filepath.ToSlash(filepath.Join("cp", "secret.txt"))]
	require.False(t, ok, "control-plane files must never surface as watch events")
}

func TestUnit_fsDiffSnap(t *testing.T) {
	changed := fsEntryLight{Size: 2}
	created := fsEntryLight{Size: 1}
	prev := fsSnapshot{"a.txt": fsEntryLight{Size: 1}}
	next := fsSnapshot{"a.txt": changed, "b.txt": created}
	events := fsDiffSnap(prev, next)
	require.Equal(t, []fsEvent{
		{Path: "a.txt", Kind: "changed"},
		{Path: "b.txt", Kind: "created"},
	}, events)

	events = fsDiffSnap(next, prev)
	require.Equal(t, []fsEvent{
		{Path: "a.txt", Kind: "changed"},
		{Path: "b.txt", Kind: "deleted"},
	}, events)
}

func TestUnit_WatchUnwatchBookkeeping(t *testing.T) {
	root := t.TempDir()
	svc, err := localfileservice.New(root)
	require.NoError(t, err)

	connCtx, connCancel := context.WithCancel(context.Background())
	defer connCancel()
	tr := &Transport{deps: Deps{Files: svc}, connCtx: connCtx, connCancel: connCancel}

	_, rpcErr := tr.handleFSWatch(context.Background(), nil)
	require.Nil(t, rpcErr)
	require.Equal(t, 1, tr.fsWatchCount)
	require.NotNil(t, tr.fsWatchCancel)

	_, rpcErr = tr.handleFSUnwatch(context.Background(), nil)
	require.Nil(t, rpcErr)
	require.Equal(t, 0, tr.fsWatchCount)
	require.Nil(t, tr.fsWatchCancel)
}

func TestExtWatch_EventsFlowAndUnwatchStopsThem(t *testing.T) {
	origInterval := fsWatchInterval
	fsWatchInterval = 20 * time.Millisecond
	t.Cleanup(func() { fsWatchInterval = origInterval })

	root := t.TempDir()
	svc, err := localfileservice.New(root)
	require.NoError(t, err)

	_, client, notifs := startExtWire(t, Deps{Files: svc})

	_, err = extCall(t, client, extMethodFSWatch, nil)
	require.NoError(t, err)

	// Let the first snapshot land, then create a file: one "created" event.
	time.Sleep(3 * fsWatchInterval)
	require.NoError(t, os.WriteFile(filepath.Join(root, "new.txt"), []byte("x"), 0o644))

	require.Eventually(t, func() bool {
		for {
			select {
			case n := <-notifs:
				if n.Method != fsEventMethod {
					continue
				}
				var ev fsEvent
				if json.Unmarshal(n.Params, &ev) != nil {
					continue
				}
				return ev.Kind == "created" && ev.Path == "new.txt"
			default:
				return false
			}
		}
	}, 5*time.Second, 10*time.Millisecond, "created event for new.txt")

	_, err = extCall(t, client, extMethodFSUnwatch, nil)
	require.NoError(t, err)
	time.Sleep(5 * fsWatchInterval)
	drainNotifications(notifs)

	require.NoError(t, os.WriteFile(filepath.Join(root, "after-unwatch.txt"), []byte("x"), 0o644))
	time.Sleep(10 * fsWatchInterval)
	require.Empty(t, notifs, "no events may flow after unwatch")
}

func drainNotifications(ch chan extNotification) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestExtWatch_EditorShapeCannotSubscribe(t *testing.T) {
	_, client, _ := startExtWire(t, Deps{})
	_, err := extCall(t, client, extMethodFSWatch, nil)
	extWireError(t, err, libacp.ErrMethodNotFound)
}
