package llama

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type modelDigestKey struct {
	path    string
	size    int64
	modTime time.Time
}

var modelDigestCache = struct {
	sync.Mutex
	m map[modelDigestKey]string
}{m: map[modelDigestKey]string{}}

func modelFileDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("llama model stat %s: %w", path, err)
	}
	key := modelDigestKey{path: path, size: info.Size(), modTime: info.ModTime()}
	modelDigestCache.Lock()
	if digest, ok := modelDigestCache.m[key]; ok {
		modelDigestCache.Unlock()
		return digest, nil
	}
	modelDigestCache.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("llama model open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("llama model hash %s: %w", path, err)
	}
	digest := hex.EncodeToString(h.Sum(nil))

	modelDigestCache.Lock()
	modelDigestCache.m[key] = digest
	modelDigestCache.Unlock()
	return digest, nil
}
