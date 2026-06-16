//go:build llamanode && llama_unsafe_abi

// Package llamaabi is a quarantined, isolated discovery spike to prove that we
// can access upstream llama.cpp state and lifecycle APIs that are missing from
// the vendored Ollama wrapper (v0.17.5).
//
// Do NOT use this package in production. If the L0/L2 spikes prove the cache
// economics are viable, the production path is a Contenox-owned binding, not
// this unsafe shim.
package llamaabi

/*
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>

// Opaque struct matching the C side.
struct llama_context;

// Basic integer types used in the C API.
typedef int32_t llama_seq_id;
typedef int32_t llama_token;

// Extern declarations mapping to the upstream symbols linked via the ollama wrapper.
extern void llama_free(struct llama_context * ctx);
extern size_t llama_state_seq_get_size(struct llama_context * ctx, llama_seq_id seq_id);
extern size_t llama_state_seq_get_data(struct llama_context * ctx, uint8_t * dst, size_t size, llama_seq_id seq_id);
extern size_t llama_state_seq_set_data(struct llama_context * ctx, const uint8_t * src, size_t size, llama_seq_id dest_seq_id);
extern size_t llama_state_seq_save_file(struct llama_context * ctx, const char * filepath, llama_seq_id seq_id, const llama_token * tokens, size_t n_token_count);
extern size_t llama_state_seq_load_file(struct llama_context * ctx, const char * filepath, llama_seq_id dest_seq_id, llama_token * tokens_out, size_t n_token_capacity, size_t * n_token_count_out);
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"unsafe"

	"github.com/ollama/ollama/llama"
)

// contextPrefix matches the exact memory layout of the beginning of the unexported
// context struct inside github.com/ollama/ollama/llama (v0.17.5).
type contextPrefix struct {
	c unsafe.Pointer
}

func contextPtr(ctx *llama.Context) *C.struct_llama_context {
	if ctx == nil {
		return nil
	}
	return (*C.struct_llama_context)((*contextPrefix)(unsafe.Pointer(ctx)).c)
}

func clearContextPtr(ctx *llama.Context) {
	(*contextPrefix)(unsafe.Pointer(ctx)).c = nil
}

// FreeContext exposes llama_free to cleanly destroy a context and avoid the leak.
func FreeContext(ctx *llama.Context) error {
	c := contextPtr(ctx)
	if c == nil {
		return nil
	}
	C.llama_free(c)
	clearContextPtr(ctx)
	runtime.KeepAlive(ctx)
	return nil
}

// StateSeqGetData reads the raw sequence state for snapshotting.
func StateSeqGetData(ctx *llama.Context, seqID int) ([]byte, error) {
	c := contextPtr(ctx)
	if c == nil {
		return nil, errors.New("llamaabi: nil context")
	}

	n := C.llama_state_seq_get_size(c, C.llama_seq_id(seqID))
	if n == 0 {
		runtime.KeepAlive(ctx)
		return nil, errors.New("llamaabi: empty sequence state")
	}
	if uint64(n) > uint64(math.MaxInt) {
		runtime.KeepAlive(ctx)
		return nil, fmt.Errorf("llamaabi: state too large: %d bytes", uint64(n))
	}

	buf := make([]byte, int(n))
	got := C.llama_state_seq_get_data(c, (*C.uint8_t)(unsafe.Pointer(&buf[0])), n, C.llama_seq_id(seqID))
	runtime.KeepAlive(ctx)

	if got != n {
		return nil, fmt.Errorf("llamaabi: copied %d bytes, want %d", uint64(got), uint64(n))
	}
	return buf, nil
}

// StateSeqSetData writes the raw sequence state from a snapshot.
func StateSeqSetData(ctx *llama.Context, seqID int, data []byte) error {
	c := contextPtr(ctx)
	if c == nil {
		return errors.New("llamaabi: nil context")
	}
	if len(data) == 0 {
		return errors.New("llamaabi: empty state")
	}

	got := C.llama_state_seq_set_data(c, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.llama_seq_id(seqID))
	runtime.KeepAlive(ctx)

	// Sequence set_data documents positive = OK, zero = failed.
	if got == 0 {
		return errors.New("llamaabi: seq state load failed")
	}
	return nil
}

// StateSeqSaveFile directly dumps the KV seq state to disk via the C API.
func StateSeqSaveFile(ctx *llama.Context, filepath string, seqID int, tokens []int32) (int, error) {
	c := contextPtr(ctx)
	if c == nil {
		return 0, errors.New("llamaabi: nil context")
	}

	// Convert int32 slice to C.llama_token slice format
	cpath := C.CString(filepath)
	defer C.free(unsafe.Pointer(cpath))

	var cTokens *C.llama_token
	var nTokens C.size_t = 0

	if len(tokens) > 0 {
		nTokens = C.size_t(len(tokens))
		cTokens = (*C.llama_token)(unsafe.Pointer(&tokens[0]))
	}

	written := C.llama_state_seq_save_file(c, cpath, C.llama_seq_id(seqID), cTokens, nTokens)
	runtime.KeepAlive(ctx)

	if written == 0 {
		return 0, errors.New("llamaabi: failed to save seq state file")
	}

	return int(written), nil
}

// StateSeqLoadFile directly loads the KV seq state from disk via the C API.
func StateSeqLoadFile(ctx *llama.Context, filepath string, destSeqID int, maxTokens int) ([]int32, int, error) {
	c := contextPtr(ctx)
	if c == nil {
		return nil, 0, errors.New("llamaabi: nil context")
	}

	cpath := C.CString(filepath)
	defer C.free(unsafe.Pointer(cpath))

	tokensOut := make([]int32, maxTokens)
	var cTokensOut *C.llama_token
	if maxTokens > 0 {
		cTokensOut = (*C.llama_token)(unsafe.Pointer(&tokensOut[0]))
	}

	var nTokenCountOut C.size_t

	read := C.llama_state_seq_load_file(c, cpath, C.llama_seq_id(destSeqID), cTokensOut, C.size_t(maxTokens), &nTokenCountOut)
	runtime.KeepAlive(ctx)

	if read == 0 {
		return nil, 0, errors.New("llamaabi: failed to load seq state file")
	}

	return tokensOut[:int(nTokenCountOut)], int(read), nil
}
