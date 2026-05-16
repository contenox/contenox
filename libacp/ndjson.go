package libacp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	defaultScanBuf = 64 * 1024
	maxScanBuf     = 16 * 1024 * 1024
)

var (
	wireMu  sync.Mutex
	wireOut io.Writer
)

func init() {
	if p := os.Getenv("CONTENOX_ACP_WIRE_LOG"); p != "" {
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			wireOut = f
		}
	}
}

func wireDump(dir string, b []byte) {
	if wireOut == nil {
		return
	}
	wireMu.Lock()
	defer wireMu.Unlock()
	fmt.Fprintf(wireOut, "%s %s %s\n", time.Now().Format(time.RFC3339Nano), dir, b)
}

type ndjsonReader struct {
	scanner *bufio.Scanner
}

func newNDJSONReader(r io.Reader) *ndjsonReader {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, defaultScanBuf), maxScanBuf)
	return &ndjsonReader{scanner: s}
}

func (r *ndjsonReader) Next() ([]byte, error) {
	for {
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		line := r.scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		out := make([]byte, len(line))
		copy(out, line)
		wireDump("<-", out)
		return out, nil
	}
}

type ndjsonWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newNDJSONWriter(w io.Writer) *ndjsonWriter {
	return &ndjsonWriter{w: w}
}

func (w *ndjsonWriter) Write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("libacp: marshal: %w", err)
	}
	wireDump("->", data)
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.w.Write(data); err != nil {
		return err
	}
	if _, err := w.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}
