package libacp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

const (
	defaultScanBuf = 64 * 1024
	maxScanBuf     = 16 * 1024 * 1024
)

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
