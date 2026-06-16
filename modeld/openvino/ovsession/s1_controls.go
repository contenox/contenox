//go:build openvino && openvino_genai

package ovsession

/*
#cgo CXXFLAGS: -std=c++17
#include <stdlib.h>
#include "s1_probe.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	s1ReportLen = 16 * 1024
	s1ErrLen    = 4 * 1024
)

func runS1GenAIProbe(modelDir, device string) (string, error) {
	cModelDir := C.CString(modelDir)
	cDevice := C.CString(device)
	defer C.free(unsafe.Pointer(cModelDir))
	defer C.free(unsafe.Pointer(cDevice))

	report := C.calloc(1, C.size_t(s1ReportLen))
	if report == nil {
		return "", fmt.Errorf("allocate S1 report buffer")
	}
	defer C.free(report)

	errbuf := C.calloc(1, C.size_t(s1ErrLen))
	if errbuf == nil {
		return "", fmt.Errorf("allocate S1 error buffer")
	}
	defer C.free(errbuf)

	rc := C.cx_s1_genai_probe(
		cModelDir,
		cDevice,
		(*C.char)(report),
		C.size_t(s1ReportLen),
		(*C.char)(errbuf),
		C.size_t(s1ErrLen),
	)
	if rc != 0 {
		return "", fmt.Errorf("%s", C.GoString((*C.char)(errbuf)))
	}
	return C.GoString((*C.char)(report)), nil
}
