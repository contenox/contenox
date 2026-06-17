//go:build llamanode

// Package chattmpl renders a model's own GGUF chat template (Jinja), including
// optional tool definitions, using the header-only minja engine that llama.cpp
// itself uses. This is the model-native rendering path: the legacy
// llama_chat_apply_template only matches a fixed set of built-in formats and
// cannot execute the GGUF's Jinja tool logic, so tool calls require this engine.
//
// minja is header-only (no llama.cpp linkage), so this package builds and tests
// fast without loading a model. Vendored headers come from
// `make -f Makefile.llamacpp vendor-headers` (CGO_CPPFLAGS=-I.llamacpp-vendor).
package chattmpl

/*
#cgo CXXFLAGS: -std=c++17
#include <stddef.h>
#include <stdlib.h>

char *cx_minja_apply(const char *source, const char *bos, const char *eos,
                     const char *messages_json, const char *tools_json,
                     int add_generation_prompt, char *errbuf, size_t errlen);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Available reports that the minja renderer is compiled into this build.
const Available = true

// Render executes the model's Jinja chat template (source, with the model's bos
// and eos token strings) over the messages and tools JSON, returning the rendered
// prompt. messagesJSON is a JSON array of chat messages; toolsJSON is a JSON array
// of tool definitions (or "" for none). addGenerationPrompt appends the model's
// assistant generation prefix.
func Render(source, bos, eos, messagesJSON, toolsJSON string, addGenerationPrompt bool) (string, error) {
	cSource := C.CString(source)
	defer C.free(unsafe.Pointer(cSource))
	cBos := C.CString(bos)
	defer C.free(unsafe.Pointer(cBos))
	cEos := C.CString(eos)
	defer C.free(unsafe.Pointer(cEos))
	cMsgs := C.CString(messagesJSON)
	defer C.free(unsafe.Pointer(cMsgs))
	cTools := C.CString(toolsJSON)
	defer C.free(unsafe.Pointer(cTools))

	const errLen = 1024
	errbuf := (*C.char)(C.calloc(1, errLen))
	defer C.free(unsafe.Pointer(errbuf))

	add := C.int(0)
	if addGenerationPrompt {
		add = 1
	}

	res := C.cx_minja_apply(cSource, cBos, cEos, cMsgs, cTools, add, errbuf, C.size_t(errLen))
	if res == nil {
		return "", errors.New("chattmpl: " + C.GoString(errbuf))
	}
	defer C.free(unsafe.Pointer(res))
	return C.GoString(res), nil
}
