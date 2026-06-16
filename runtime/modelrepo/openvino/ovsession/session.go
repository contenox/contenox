//go:build openvino

package ovsession

/*
#cgo CXXFLAGS: -std=c++17
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

// Available reports whether the native OpenVINO backend was compiled in.
const Available = true

const errLen = 1024

// Session wraps a single OpenVINO InferRequest and its KV cursor.
type Session struct {
	ptr *C.cx_session
}

// New creates a stateful OpenVINO session for an IR model directory.
func New(modelDir, device string) (*Session, error) {
	if modelDir == "" {
		return nil, errors.New("openvino model directory is required")
	}
	if device == "" {
		device = "CPU"
	}

	cDir := C.CString(modelDir)
	cDev := C.CString(device)
	defer C.free(unsafe.Pointer(cDir))
	defer C.free(unsafe.Pointer(cDev))

	errbuf := make([]byte, errLen)
	ptr := C.cx_session_new(cDir, cDev, cptr(errbuf), C.size_t(len(errbuf)))
	if ptr == nil {
		return nil, fmt.Errorf("openvino session new: %s", goErr(errbuf))
	}
	s := &Session{ptr: ptr}
	runtime.SetFinalizer(s, (*Session).mustClose)
	return s, nil
}

// Close releases the native session.
func (s *Session) Close() error {
	if s == nil || s.ptr == nil {
		return nil
	}
	runtime.SetFinalizer(s, nil)
	C.cx_session_free(s.ptr)
	s.ptr = nil
	return nil
}

func (s *Session) mustClose() {
	_ = s.Close()
}

// Prefill feeds prompt token IDs in one pass and stores the first predicted
// token for subsequent DecodeNext calls.
func (s *Session) Prefill(tokens []int64) error {
	if len(tokens) == 0 {
		return errors.New("openvino prefill requires at least one token")
	}
	if err := s.requireOpen(); err != nil {
		return err
	}
	errbuf := make([]byte, errLen)
	rc := C.cx_prefill(s.ptr, (*C.int64_t)(unsafe.Pointer(&tokens[0])), C.size_t(len(tokens)), cptr(errbuf), C.size_t(len(errbuf)))
	if rc != 0 {
		return fmt.Errorf("openvino prefill: %s", goErr(errbuf))
	}
	return nil
}

// DecodeNext greedily decodes one token ID.
func (s *Session) DecodeNext() (int64, error) {
	if err := s.requireOpen(); err != nil {
		return -1, err
	}
	errbuf := make([]byte, errLen)
	id := C.cx_decode_next(s.ptr, cptr(errbuf), C.size_t(len(errbuf)))
	if id < 0 {
		return -1, fmt.Errorf("openvino decode next: %s", goErr(errbuf))
	}
	return int64(id), nil
}

// SnapshotSave persists the current KV state and decode cursor.
func (s *Session) SnapshotSave(path string) error {
	if path == "" {
		return errors.New("openvino snapshot path is required")
	}
	if err := s.requireOpen(); err != nil {
		return err
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	errbuf := make([]byte, errLen)
	if C.cx_snapshot_save(s.ptr, cPath, cptr(errbuf), C.size_t(len(errbuf))) != 0 {
		return fmt.Errorf("openvino snapshot save: %s", goErr(errbuf))
	}
	return nil
}

// SnapshotRestore restores a KV snapshot into this session.
func (s *Session) SnapshotRestore(path string) error {
	if path == "" {
		return errors.New("openvino snapshot path is required")
	}
	if err := s.requireOpen(); err != nil {
		return err
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	errbuf := make([]byte, errLen)
	if C.cx_snapshot_restore(s.ptr, cPath, cptr(errbuf), C.size_t(len(errbuf))) != 0 {
		return fmt.Errorf("openvino snapshot restore: %s", goErr(errbuf))
	}
	return nil
}

func (s *Session) requireOpen() error {
	if s == nil || s.ptr == nil {
		return errors.New("openvino session is closed")
	}
	return nil
}

func cptr(b []byte) *C.char {
	return (*C.char)(unsafe.Pointer(&b[0]))
}

func goErr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
