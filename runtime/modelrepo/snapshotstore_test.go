package modelrepo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDiskStore(t *testing.T, maxBytes int64, ttl time.Duration) (*DiskSnapshotStore, string) {
	t.Helper()
	dir := t.TempDir()
	return NewDiskSnapshotStore(func() string { return dir }, maxBytes, ttl), dir
}

// TestUnit_DiskSnapshotStore_RoundTrip proves a saved blob comes back byte-exact
// under a fresh store instance rooted at the same directory — the case that
// matters for runtime-restart durability: the process is gone, only the file
// remains.
func TestUnit_DiskSnapshotStore_RoundTrip(t *testing.T) {
	s, dir := newTestDiskStore(t, 0, 0)
	want := []byte("resident-kv-blob")
	s.Save("model-a", want)

	// Simulate a runtime restart: a brand-new store instance, no in-memory state,
	// pointed at the same directory.
	fresh := NewDiskSnapshotStore(func() string { return dir }, 0, 0)
	got, ok := fresh.Load("model-a")
	if !ok {
		t.Fatal("expected a hit after simulated restart")
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestUnit_DiskSnapshotStore_MissReturnsFalse proves an unknown key is a clean
// miss, not an error or a zero-value blob.
func TestUnit_DiskSnapshotStore_MissReturnsFalse(t *testing.T) {
	s, _ := newTestDiskStore(t, 0, 0)
	if _, ok := s.Load("nope"); ok {
		t.Fatal("expected a miss for an unwritten key")
	}
}

// TestUnit_DiskSnapshotStore_DeleteRemovesBlob proves Delete makes a subsequent
// Load miss, and is a safe no-op when the key was never saved.
func TestUnit_DiskSnapshotStore_DeleteRemovesBlob(t *testing.T) {
	s, _ := newTestDiskStore(t, 0, 0)
	s.Save("k", []byte("v"))
	s.Delete("k")
	if _, ok := s.Load("k"); ok {
		t.Fatal("expected a miss after Delete")
	}
	s.Delete("never-existed") // must not panic or error
}

// TestUnit_DiskSnapshotStore_KeyMismatchIsAMiss proves a file that hashes to the
// same path but was written for a different key (or a foreign/corrupt file
// dropped in the directory) never gets decoded as if it belonged to the
// requested key. This is the safety property that keeps a snapshot restore from
// ever attaching one session's KV to a different model's identity.
func TestUnit_DiskSnapshotStore_KeyMismatchIsAMiss(t *testing.T) {
	s, dir := newTestDiskStore(t, 0, 0)
	s.Save("key-a", []byte("payload-a"))

	// Directly overwrite the on-disk file with a frame claiming a different key,
	// simulating a hash collision or a tampered/foreign file.
	path := s.path(dir, "key-a")
	forged := encodeSnapshotFile("key-b", []byte("payload-b"))
	if err := os.WriteFile(path, forged, 0o600); err != nil {
		t.Fatalf("write forged file: %v", err)
	}

	if _, ok := s.Load("key-a"); ok {
		t.Fatal("expected a miss: file belongs to a different key")
	}
	// The corrupt/mismatched file must have been cleaned up by the miss.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected the key-mismatched file to be removed")
	}
}

// TestUnit_DiskSnapshotStore_GarbageFileIsAMiss proves a file with no valid magic
// header (e.g. a stray non-snapshot file placed in the directory) is rejected
// instead of crashing or silently returning garbage.
func TestUnit_DiskSnapshotStore_GarbageFileIsAMiss(t *testing.T) {
	s, dir := newTestDiskStore(t, 0, 0)
	path := s.path(dir, "key-a")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not a snapshot file"), 0o600); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	if _, ok := s.Load("key-a"); ok {
		t.Fatal("expected a miss for a non-snapshot file")
	}
}

// TestUnit_DiskSnapshotStore_TTLExpiresEntries proves a snapshot older than the
// TTL is treated as a miss and removed, rather than restoring stale KV
// indefinitely.
func TestUnit_DiskSnapshotStore_TTLExpiresEntries(t *testing.T) {
	s, dir := newTestDiskStore(t, 0, time.Minute)
	clock := time.Now()
	s.now = func() time.Time { return clock }
	s.Save("k", []byte("v"))

	clock = clock.Add(2 * time.Minute)
	if _, ok := s.Load("k"); ok {
		t.Fatal("expected a miss: snapshot is past its TTL")
	}
	if _, err := os.Stat(s.path(dir, "k")); !os.IsNotExist(err) {
		t.Fatal("expected the expired file to be removed")
	}
}

// TestUnit_DiskSnapshotStore_SizeCapEvictsLRU proves that once the total on-disk
// budget is exceeded, the store evicts the least-recently-touched snapshots
// first (by access time), keeping the more recently used ones.
func TestUnit_DiskSnapshotStore_SizeCapEvictsLRU(t *testing.T) {
	blob := make([]byte, 100)
	// Cap fits ~2.x blobs (plus per-file framing overhead), forcing eviction once
	// a 4th distinct key is saved.
	s, dir := newTestDiskStore(t, 250, 0)
	clock := time.Now()
	s.now = func() time.Time { return clock }

	s.Save("a", blob)
	clock = clock.Add(time.Second)
	s.Save("b", blob)
	clock = clock.Add(time.Second)
	s.Save("c", blob) // over cap now -> LRU ("a") must be evicted

	if _, ok := s.Load("a"); ok {
		t.Fatal("expected 'a' (least recently touched) to have been evicted")
	}
	if _, ok := s.Load("b"); !ok {
		t.Fatal("expected 'b' to still be present")
	}
	if _, ok := s.Load("c"); !ok {
		t.Fatal("expected 'c' to still be present")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files on disk after eviction, got %d", len(entries))
	}
}

// TestUnit_DiskSnapshotStore_SnapshotDirDisabled proves that when dirFn returns
// "" (the SnapshotDir-disabled contract), Save/Load/Delete are all safe no-ops —
// the store degrades to "no snapshot survival" rather than erroring.
func TestUnit_DiskSnapshotStore_SnapshotDirDisabled(t *testing.T) {
	s := NewDiskSnapshotStore(func() string { return "" }, 0, 0)
	s.Save("k", []byte("v")) // must not panic
	if _, ok := s.Load("k"); ok {
		t.Fatal("expected a miss when snapshot dir is disabled")
	}
	s.Delete("k") // must not panic
}

// TestUnit_DiskSnapshotStore_AtomicWriteLeavesNoTempFiles proves Save cleans up
// its temp file on the success path (rename, not copy).
func TestUnit_DiskSnapshotStore_AtomicWriteLeavesNoTempFiles(t *testing.T) {
	s, dir := newTestDiskStore(t, 0, 0)
	s.Save("k", []byte("v"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != snapshotFileExt {
			t.Fatalf("unexpected leftover file: %s", e.Name())
		}
	}
}

// TestUnit_DiskSnapshotStore_ReapsOrphanedTempFiles proves a temp file left by a
// Save that crashed before rename (simulated here by dropping one in the dir) is
// reaped by the next Save's GC, so crashes do not leak disk indefinitely.
func TestUnit_DiskSnapshotStore_ReapsOrphanedTempFiles(t *testing.T) {
	s, dir := newTestDiskStore(t, 0, 0)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orphan := filepath.Join(dir, snapshotTempPrefix+"leftover")
	if err := os.WriteFile(orphan, []byte("half-written"), 0o600); err != nil {
		t.Fatalf("write orphan: %v", err)
	}

	s.Save("k", []byte("v")) // triggers gcLocked

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("expected the orphaned temp file to be reaped by GC")
	}
	if _, ok := s.Load("k"); !ok {
		t.Fatal("the real snapshot must survive the temp-file reap")
	}
}
