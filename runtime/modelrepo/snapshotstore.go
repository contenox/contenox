package modelrepo

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tunables for the on-disk snapshot store (package vars so tests and deployments
// can override). SnapshotMaxBytes bounds the total on-disk snapshot budget across
// all keys; the store evicts least-recently-used snapshots to stay under it.
// SnapshotTTL drops snapshots not read within the window. SnapshotTimeout bounds
// a single capture/restore round-trip to the daemon.
var (
	SnapshotMaxBytes = int64(4) << 30 // 4 GiB total on-disk snapshot budget
	SnapshotTTL      = 24 * time.Hour
	SnapshotTimeout  = 10 * time.Second
	// SnapshotMaxBlobBytes skips capturing any single snapshot larger than this
	// (0 = no per-snapshot limit). It bounds the latency and bandwidth of one
	// capture: a very large resident KV state can cost more to serialize, ship
	// off the daemon, and write than a cold prefill would — the low-bandwidth
	// case the runtime targets. When a blob is skipped the next open is simply
	// cold, exactly as if snapshot survival were off for that key.
	SnapshotMaxBlobBytes = int64(0)
)

// SnapshotStore persists opaque session-snapshot blobs keyed by a warm-cache key.
// It is a best-effort durability layer for warm KV: a Save that never lands, or a
// Load that misses, only costs a cold prefill on the next open — it never
// corrupts a session. Implementations must be safe for concurrent use.
type SnapshotStore interface {
	// Save persists blob under key, replacing any previous blob for that key.
	Save(key string, blob []byte)
	// Load returns the blob previously saved under key, or ok=false on a miss.
	Load(key string) (blob []byte, ok bool)
	// Delete removes any blob stored under key. It is a no-op on a miss.
	Delete(key string)
}

// MemSnapshotStore is an in-process SnapshotStore. It survives a modeld daemon
// restart (the runtime process outlives it) but not a runtime restart; use it as
// a test double or when on-disk durability is disabled.
type MemSnapshotStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

// NewMemSnapshotStore returns an empty in-process snapshot store.
func NewMemSnapshotStore() *MemSnapshotStore { return &MemSnapshotStore{m: map[string][]byte{}} }

func (s *MemSnapshotStore) Save(key string, blob []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = append([]byte(nil), blob...)
}

func (s *MemSnapshotStore) Load(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), b...), true
}

func (s *MemSnapshotStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

var _ SnapshotStore = (*MemSnapshotStore)(nil)

// snapshotFileMagic tags a snapshot file so a stray or foreign file in the
// snapshot dir is never decoded as a snapshot.
var snapshotFileMagic = [4]byte{'c', 'x', 's', '1'}

const snapshotFileExt = ".snap"

// snapshotTempPrefix names the atomic-write temp files (Save writes here then
// renames). A crash between create and rename leaves one orphaned; gcLocked
// reaps them. Kept distinct from the final "<hash>.snap" names so the two never
// collide.
const snapshotTempPrefix = ".snaptmp-"

// DiskSnapshotStore is a durable SnapshotStore: one file per key under a
// directory, surviving a full runtime restart. It bounds total on-disk bytes
// (LRU eviction by access time) and drops snapshots idle past a TTL. The
// directory is resolved lazily per operation via dirFn, so a store constructed at
// package-init time picks up a data root configured later (see
// modeldconn.SnapshotDir).
//
// The file records the exact key alongside the blob; a sha256 filename collision
// or a reused directory therefore yields a clean miss instead of restoring the
// wrong session's KV. Layered atop the manifest-compatibility gate in
// Session.Restore, a mismatched or corrupt snapshot can only ever cause a safe
// cold-prefill fallback.
type DiskSnapshotStore struct {
	dirFn    func() string
	maxBytes int64
	ttl      time.Duration
	now      func() time.Time
	mu       sync.Mutex
}

// NewDiskSnapshotStore returns a disk-backed store rooted at dirFn(). maxBytes<=0
// disables the size cap; ttl<=0 disables idle expiry.
func NewDiskSnapshotStore(dirFn func() string, maxBytes int64, ttl time.Duration) *DiskSnapshotStore {
	return &DiskSnapshotStore{dirFn: dirFn, maxBytes: maxBytes, ttl: ttl, now: time.Now}
}

var _ SnapshotStore = (*DiskSnapshotStore)(nil)

func (s *DiskSnapshotStore) path(dir, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+snapshotFileExt)
}

// encodeSnapshotFile frames a snapshot as magic | keyLen | key | blob so Load can
// confirm the file truly holds this key's snapshot.
func encodeSnapshotFile(key string, blob []byte) []byte {
	out := make([]byte, 0, len(snapshotFileMagic)+4+len(key)+len(blob))
	out = append(out, snapshotFileMagic[:]...)
	var kl [4]byte
	binary.BigEndian.PutUint32(kl[:], uint32(len(key)))
	out = append(out, kl[:]...)
	out = append(out, key...)
	out = append(out, blob...)
	return out
}

// decodeSnapshotFile validates the frame and returns the blob only when the file
// belongs to key.
func decodeSnapshotFile(key string, data []byte) ([]byte, bool) {
	if len(data) < len(snapshotFileMagic)+4 {
		return nil, false
	}
	if [4]byte{data[0], data[1], data[2], data[3]} != snapshotFileMagic {
		return nil, false
	}
	kl := int(binary.BigEndian.Uint32(data[4:8]))
	if kl < 0 || 8+kl > len(data) {
		return nil, false
	}
	if string(data[8:8+kl]) != key {
		return nil, false
	}
	return data[8+kl:], true
}

func (s *DiskSnapshotStore) Save(key string, blob []byte) {
	if len(blob) == 0 {
		return
	}
	dir := s.dirFn()
	if dir == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	final := s.path(dir, key)
	tmp, err := os.CreateTemp(dir, snapshotTempPrefix+"*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(encodeSnapshotFile(key, blob)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	// Stamp the just-written file as most-recently-used so LRU eviction below
	// never reclaims it, then reconcile the directory against TTL and size cap.
	now := s.now()
	_ = os.Chtimes(final, now, now)
	s.gcLocked(dir, now)
}

func (s *DiskSnapshotStore) Load(key string) ([]byte, bool) {
	dir := s.dirFn()
	if dir == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	final := s.path(dir, key)
	info, err := os.Stat(final)
	if err != nil {
		return nil, false
	}
	now := s.now()
	if s.ttl > 0 && now.Sub(info.ModTime()) > s.ttl {
		_ = os.Remove(final)
		return nil, false
	}
	data, err := os.ReadFile(final)
	if err != nil {
		return nil, false
	}
	blob, ok := decodeSnapshotFile(key, data)
	if !ok {
		_ = os.Remove(final)
		return nil, false
	}
	// Reading is a use: refresh access time so a live snapshot is the last thing
	// LRU eviction reclaims.
	_ = os.Chtimes(final, now, now)
	return append([]byte(nil), blob...), true
}

func (s *DiskSnapshotStore) Delete(key string) {
	dir := s.dirFn()
	if dir == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(s.path(dir, key))
}

// gcLocked drops TTL-expired snapshots and then evicts least-recently-used
// snapshots until the directory fits the size cap. Callers hold s.mu.
func (s *DiskSnapshotStore) gcLocked(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type snapFile struct {
		path  string
		size  int64
		mtime time.Time
	}
	var files []snapFile
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Reap orphaned atomic-write temp files from a crashed Save. Safe here:
		// gcLocked runs under s.mu after the current Save has already renamed its
		// temp away, so any remaining temp is a dead leftover.
		if strings.HasPrefix(e.Name(), snapshotTempPrefix) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		if filepath.Ext(e.Name()) != snapshotFileExt {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if s.ttl > 0 && now.Sub(info.ModTime()) > s.ttl {
			_ = os.Remove(p)
			continue
		}
		files = append(files, snapFile{path: p, size: info.Size(), mtime: info.ModTime()})
		total += info.Size()
	}
	if s.maxBytes <= 0 || total <= s.maxBytes {
		return
	}
	// Evict oldest-accessed first until under the cap.
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
	for _, f := range files {
		if total <= s.maxBytes {
			break
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
}
