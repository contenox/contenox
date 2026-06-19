//go:build llamacpp_direct

package llamacppshim

/*
#cgo CFLAGS: -std=c11
#include <stdbool.h>
#include <stdlib.h>
#include "llama.h"
#include "ggml-backend.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/contenox/runtime/modeld/llama/chattmpl"
)

// Available reports whether the direct llama.cpp shim was compiled in.
const Available = true

var backendOnce sync.Once

func initBackend() {
	backendOnce.Do(func() {
		C.ggml_backend_load_all()
		C.llama_backend_init()
	})
}

// SystemInfo returns llama.cpp's linked runtime system information string.
func SystemInfo() string {
	initBackend()
	return C.GoString(C.llama_print_system_info())
}

// SupportsGPUOffload reports the linked llama.cpp runtime's compiled GPU
// offload capability. A true result is not enough to certify actual placement.
func SupportsGPUOffload() bool {
	initBackend()
	return bool(C.llama_supports_gpu_offload())
}

// Device describes a ggml backend device registered in the linked runtime.
type Device struct {
	Index       int
	Name        string
	Description string
	Type        string
	MemoryFree  uint64
	MemoryTotal uint64
}

// Devices returns the ggml backend devices registered after backend init.
func Devices() []Device {
	initBackend()
	count := int(C.ggml_backend_dev_count())
	out := make([]Device, 0, count)
	for i := 0; i < count; i++ {
		dev := C.ggml_backend_dev_get(C.size_t(i))
		if dev == nil {
			continue
		}
		var free, total C.size_t
		C.ggml_backend_dev_memory(dev, &free, &total)
		out = append(out, Device{
			Index:       i,
			Name:        C.GoString(C.ggml_backend_dev_name(dev)),
			Description: C.GoString(C.ggml_backend_dev_description(dev)),
			Type:        deviceType(C.ggml_backend_dev_type(dev)),
			MemoryFree:  uint64(free),
			MemoryTotal: uint64(total),
		})
	}
	return out
}

func deviceType(t C.enum_ggml_backend_dev_type) string {
	switch t {
	case C.GGML_BACKEND_DEVICE_TYPE_CPU:
		return "cpu"
	case C.GGML_BACKEND_DEVICE_TYPE_GPU:
		return "gpu"
	case C.GGML_BACKEND_DEVICE_TYPE_IGPU:
		return "igpu"
	case C.GGML_BACKEND_DEVICE_TYPE_ACCEL:
		return "accel"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// ModelConfig controls model loading for the direct shim.
type ModelConfig struct {
	NumGPULayers int
	TensorSplit  []float32
	UseMmap      bool
	VocabOnly    bool
}

// Model wraps a direct llama_model pointer.
type Model struct {
	ptr   *C.struct_llama_model
	vocab *C.struct_llama_vocab
}

// LoadModel opens a GGUF model through direct llama.cpp.
func LoadModel(path string, cfg ModelConfig) (*Model, error) {
	if path == "" {
		return nil, errors.New("llamacppshim: model path is required")
	}
	initBackend()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	params := C.llama_model_default_params()
	params.n_gpu_layers = C.int32_t(cfg.NumGPULayers)
	params.use_mmap = C.bool(cfg.UseMmap)
	params.vocab_only = C.bool(cfg.VocabOnly)
	if len(cfg.TensorSplit) > 0 {
		var pin runtime.Pinner
		pin.Pin(&cfg.TensorSplit[0])
		defer pin.Unpin()
		params.tensor_split = (*C.float)(unsafe.Pointer(&cfg.TensorSplit[0]))
	}

	ptr := C.llama_model_load_from_file(cpath, params)
	if ptr == nil {
		return nil, fmt.Errorf("llamacppshim: load model %q", path)
	}
	m := &Model{ptr: ptr, vocab: C.llama_model_get_vocab(ptr)}
	runtime.SetFinalizer(m, (*Model).Close)
	return m, nil
}

// Close releases the model.
func (m *Model) Close() {
	if m == nil || m.ptr == nil {
		return
	}
	C.llama_model_free(m.ptr)
	m.ptr = nil
	m.vocab = nil
	runtime.SetFinalizer(m, nil)
}

// Description returns llama.cpp's model description.
func (m *Model) Description() string {
	if m == nil || m.ptr == nil {
		return ""
	}
	buf := make([]byte, 512)
	n := C.llama_model_desc(m.ptr, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if n <= 0 {
		return ""
	}
	if int(n) >= len(buf) {
		n = C.int32_t(len(buf) - 1)
	}
	return string(buf[:int(n)])
}

// ContextTrain returns the model's training context length.
func (m *Model) ContextTrain() int {
	if m == nil || m.ptr == nil {
		return 0
	}
	return int(C.llama_model_n_ctx_train(m.ptr))
}

// EmbeddingLength returns the model embedding dimension.
func (m *Model) EmbeddingLength() int {
	if m == nil || m.ptr == nil {
		return 0
	}
	return int(C.llama_model_n_embd(m.ptr))
}

// NumVocab returns the vocabulary size.
func (m *Model) NumVocab() int {
	if m == nil || m.vocab == nil {
		return 0
	}
	return int(C.llama_vocab_n_tokens(m.vocab))
}

// AddBOS reports the model tokenizer's GGUF add_bos_token policy.
func (m *Model) AddBOS() bool {
	if m == nil || m.vocab == nil {
		return false
	}
	return bool(C.llama_vocab_get_add_bos(m.vocab))
}

// TokenIsEOG reports whether token is an end-of-generation token.
func (m *Model) TokenIsEOG(token int) bool {
	if m == nil || m.vocab == nil {
		return false
	}
	return bool(C.llama_vocab_is_eog(m.vocab, C.llama_token(token)))
}

// TokenToPiece converts a token id to rendered text.
func (m *Model) TokenToPiece(token int) string {
	if m == nil || m.vocab == nil {
		return ""
	}
	n := C.int32_t(32)
	buf := make([]byte, int(n))
	got := C.llama_token_to_piece(m.vocab, C.llama_token(token), (*C.char)(unsafe.Pointer(&buf[0])), n, 0, C.bool(true))
	if got < 0 {
		n = -got
		buf = make([]byte, int(n))
		got = C.llama_token_to_piece(m.vocab, C.llama_token(token), (*C.char)(unsafe.Pointer(&buf[0])), n, 0, C.bool(true))
	}
	if got <= 0 {
		return ""
	}
	return strings.TrimRight(string(buf[:int(got)]), "\x00")
}

// Tokenize tokenizes text with the model's active vocab.
func (m *Model) Tokenize(text string, addSpecial bool, parseSpecial bool) ([]int, error) {
	if m == nil || m.ptr == nil || m.vocab == nil {
		return nil, errors.New("llamacppshim: model is closed")
	}
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))

	need := C.llama_tokenize(
		m.vocab,
		ctext,
		C.int32_t(len(text)),
		nil,
		0,
		C.bool(addSpecial),
		C.bool(parseSpecial),
	)
	if need == 0 {
		return nil, nil
	}
	if need < 0 {
		need = -need
	}
	toks := make([]C.llama_token, int(need))
	got := C.llama_tokenize(
		m.vocab,
		ctext,
		C.int32_t(len(text)),
		&toks[0],
		need,
		C.bool(addSpecial),
		C.bool(parseSpecial),
	)
	if got < 0 {
		return nil, fmt.Errorf("llamacppshim: tokenize failed")
	}
	out := make([]int, int(got))
	for i := range out {
		out[i] = int(toks[i])
	}
	return out, nil
}

// ChatMessage is one role/content turn for chat-template application.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  string
	ToolCallID string
}

// ChatTemplate returns the model's default GGUF chat template.
func (m *Model) ChatTemplate() string {
	if m == nil || m.ptr == nil {
		return ""
	}
	t := C.llama_model_chat_template(m.ptr, nil)
	if t == nil {
		return ""
	}
	return C.GoString(t)
}

func (m *Model) bosText() string {
	if m == nil || m.vocab == nil {
		return ""
	}
	tok := C.llama_vocab_bos(m.vocab)
	if tok < 0 {
		return ""
	}
	txt := C.llama_vocab_get_text(m.vocab, tok)
	if txt == nil {
		return ""
	}
	return C.GoString(txt)
}

func (m *Model) eosText() string {
	if m == nil || m.vocab == nil {
		return ""
	}
	tok := C.llama_vocab_eos(m.vocab)
	if tok < 0 {
		return ""
	}
	txt := C.llama_vocab_get_text(m.vocab, tok)
	if txt == nil {
		return ""
	}
	return C.GoString(txt)
}

// ApplyChatTemplateTools renders the model's Jinja chat template via minja,
// including tool definitions.
func (m *Model) ApplyChatTemplateTools(messagesJSON, toolsJSON string, addAssistant bool) (string, error) {
	source := m.ChatTemplate()
	if source == "" {
		return "", errors.New("llamacppshim: model declares no chat template")
	}
	return chattmpl.Render(source, m.bosText(), m.eosText(), messagesJSON, toolsJSON, addAssistant)
}

// ApplyChatTemplate applies llama.cpp's built-in chat-template matcher. This
// path does not execute arbitrary Jinja tool blocks; use ApplyChatTemplateTools
// when tool definitions are present.
func (m *Model) ApplyChatTemplate(messages []ChatMessage, addAssistant bool) (string, error) {
	tmpl := m.ChatTemplate()
	if tmpl == "" {
		return "", errors.New("llamacppshim: model declares no chat template")
	}
	ctmpl := C.CString(tmpl)
	defer C.free(unsafe.Pointer(ctmpl))

	cmsgs := make([]C.struct_llama_chat_message, len(messages))
	for i, msg := range messages {
		role := C.CString(msg.Role)
		content := C.CString(msg.Content)
		defer C.free(unsafe.Pointer(role))
		defer C.free(unsafe.Pointer(content))
		cmsgs[i].role = role
		cmsgs[i].content = content
	}
	var cmsg *C.struct_llama_chat_message
	if len(cmsgs) > 0 {
		cmsg = (*C.struct_llama_chat_message)(unsafe.Pointer(&cmsgs[0]))
	}
	need := C.llama_chat_apply_template(ctmpl, cmsg, C.size_t(len(cmsgs)), C.bool(addAssistant), nil, 0)
	if need < 0 {
		return "", errors.New("llamacppshim: chat template not supported by llama.cpp")
	}
	buf := make([]byte, int(need)+1)
	got := C.llama_chat_apply_template(ctmpl, cmsg, C.size_t(len(cmsgs)), C.bool(addAssistant), (*C.char)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)))
	if got < 0 {
		return "", errors.New("llamacppshim: chat template apply failed")
	}
	if int(got) > len(buf) {
		got = C.int32_t(len(buf))
	}
	return string(buf[:int(got)]), nil
}

// ContextConfig controls direct llama_context construction.
type ContextConfig struct {
	NumCtx       int
	NumBatch     int
	NumSeqMax    int
	NumThreads   int
	FlashAttn    bool
	KVCacheType  string
	Embeddings   bool
	OffloadKQV   bool
	NoPerf       bool
	PoolingLast  bool
	NonCausalAtt bool
}

// Context wraps a direct llama_context pointer.
type Context struct {
	ptr   *C.struct_llama_context
	model *Model
}

// NewContext creates a direct llama.cpp context for model.
func NewContext(model *Model, cfg ContextConfig) (*Context, error) {
	if model == nil || model.ptr == nil {
		return nil, errors.New("llamacppshim: model is closed")
	}
	params := C.llama_context_default_params()
	if cfg.NumCtx > 0 {
		params.n_ctx = C.uint32_t(cfg.NumCtx)
	}
	if cfg.NumBatch > 0 {
		params.n_batch = C.uint32_t(cfg.NumBatch)
		params.n_ubatch = C.uint32_t(cfg.NumBatch)
	}
	if cfg.NumSeqMax > 0 {
		params.n_seq_max = C.uint32_t(cfg.NumSeqMax)
	} else {
		params.n_seq_max = 1
	}
	if cfg.NumThreads > 0 {
		params.n_threads = C.int32_t(cfg.NumThreads)
		params.n_threads_batch = C.int32_t(cfg.NumThreads)
	}
	if cfg.FlashAttn {
		params.flash_attn_type = C.LLAMA_FLASH_ATTN_TYPE_ENABLED
	} else {
		params.flash_attn_type = C.LLAMA_FLASH_ATTN_TYPE_DISABLED
	}
	params.type_k = kvCacheType(cfg.KVCacheType)
	params.type_v = kvCacheType(cfg.KVCacheType)
	params.embeddings = C.bool(cfg.Embeddings)
	params.offload_kqv = C.bool(cfg.OffloadKQV)
	params.no_perf = C.bool(cfg.NoPerf)
	if cfg.PoolingLast {
		params.pooling_type = C.LLAMA_POOLING_TYPE_LAST
	}
	if cfg.NonCausalAtt {
		params.attention_type = C.LLAMA_ATTENTION_TYPE_NON_CAUSAL
	}

	ptr := C.llama_init_from_model(model.ptr, params)
	if ptr == nil {
		return nil, errors.New("llamacppshim: unable to create llama context")
	}
	ctx := &Context{ptr: ptr, model: model}
	runtime.SetFinalizer(ctx, (*Context).Close)
	return ctx, nil
}

func kvCacheType(s string) C.enum_ggml_type {
	switch strings.ToLower(s) {
	case "q8_0":
		return C.GGML_TYPE_Q8_0
	case "q4_0":
		return C.GGML_TYPE_Q4_0
	default:
		return C.GGML_TYPE_F16
	}
}

// Close releases the context.
func (c *Context) Close() {
	if c == nil || c.ptr == nil {
		return
	}
	C.llama_free(c.ptr)
	c.ptr = nil
	runtime.SetFinalizer(c, nil)
}

// ClearMemory clears llama.cpp memory/KV state.
func (c *Context) ClearMemory(data bool) {
	if c == nil || c.ptr == nil {
		return
	}
	C.llama_memory_clear(C.llama_get_memory(c.ptr), C.bool(data))
}

// MemorySeqRemove removes KV entries for seqID in [p0, p1).
func (c *Context) MemorySeqRemove(seqID, p0, p1 int) bool {
	if c == nil || c.ptr == nil {
		return false
	}
	return bool(C.llama_memory_seq_rm(C.llama_get_memory(c.ptr), C.llama_seq_id(seqID), C.llama_pos(p0), C.llama_pos(p1)))
}

// MemorySeqCopy copies KV state from srcSeqID to dstSeqID.
func (c *Context) MemorySeqCopy(srcSeqID, dstSeqID, p0, p1 int) {
	if c == nil || c.ptr == nil {
		return
	}
	C.llama_memory_seq_cp(C.llama_get_memory(c.ptr), C.llama_seq_id(srcSeqID), C.llama_seq_id(dstSeqID), C.llama_pos(p0), C.llama_pos(p1))
}

// MemorySeqAdd shifts positions for seqID.
func (c *Context) MemorySeqAdd(seqID, p0, p1, delta int) {
	if c == nil || c.ptr == nil {
		return
	}
	C.llama_memory_seq_add(C.llama_get_memory(c.ptr), C.llama_seq_id(seqID), C.llama_pos(p0), C.llama_pos(p1), C.llama_pos(delta))
}

// DecodeStatus preserves llama_decode's exact status class.
type DecodeStatus string

const (
	DecodeOK       DecodeStatus = "ok"
	DecodeNoKVSlot DecodeStatus = "no_kv_slot"
	DecodeAborted  DecodeStatus = "aborted_partial"
	DecodeInvalid  DecodeStatus = "invalid_batch"
	DecodeFatal    DecodeStatus = "fatal"
)

// DecodeResult reports llama_decode status without collapsing warning/fatal
// cases into a generic error.
type DecodeResult struct {
	Status DecodeStatus
	Code   int
	Err    error
}

// Decode runs one batch through llama_decode.
func (c *Context) Decode(batch *Batch) DecodeResult {
	if c == nil || c.ptr == nil {
		return DecodeResult{Status: DecodeFatal, Code: -2, Err: errors.New("llamacppshim: context is closed")}
	}
	if batch == nil {
		return DecodeResult{Status: DecodeInvalid, Code: -1, Err: errors.New("llamacppshim: nil batch")}
	}
	code := int(C.llama_decode(c.ptr, batch.c))
	switch {
	case code == 0:
		return DecodeResult{Status: DecodeOK, Code: code}
	case code == 1:
		return DecodeResult{Status: DecodeNoKVSlot, Code: code, Err: errors.New("llamacppshim: no kv slot")}
	case code == 2:
		return DecodeResult{Status: DecodeAborted, Code: code, Err: errors.New("llamacppshim: decode aborted with partial memory")}
	case code == -1:
		return DecodeResult{Status: DecodeInvalid, Code: code, Err: errors.New("llamacppshim: invalid decode batch")}
	default:
		return DecodeResult{Status: DecodeFatal, Code: code, Err: fmt.Errorf("llamacppshim: fatal decode code %d", code)}
	}
}

// GetEmbeddingsSeq returns pooled embeddings for seqID, if available.
func (c *Context) GetEmbeddingsSeq(seqID int) []float32 {
	if c == nil || c.ptr == nil || c.model == nil {
		return nil
	}
	ptr := C.llama_get_embeddings_seq(c.ptr, C.llama_seq_id(seqID))
	if ptr == nil {
		return nil
	}
	n := c.model.EmbeddingLength()
	out := make([]float32, n)
	copy(out, unsafe.Slice((*float32)(unsafe.Pointer(ptr)), n))
	return out
}

// GetEmbeddingsIth returns token embeddings for output row i, if available.
func (c *Context) GetEmbeddingsIth(i int) []float32 {
	if c == nil || c.ptr == nil || c.model == nil {
		return nil
	}
	ptr := C.llama_get_embeddings_ith(c.ptr, C.int32_t(i))
	if ptr == nil {
		return nil
	}
	n := c.model.EmbeddingLength()
	out := make([]float32, n)
	copy(out, unsafe.Slice((*float32)(unsafe.Pointer(ptr)), n))
	return out
}

// Batch wraps a llama_batch allocated by llama.cpp.
type Batch struct {
	c         C.struct_llama_batch
	batchSize int
	maxSeq    int
	embedSize int
}

// NewBatch creates a llama batch.
func NewBatch(batchSize, maxSeq, embedSize int) (*Batch, error) {
	if batchSize <= 0 {
		return nil, errors.New("llamacppshim: batch size must be positive")
	}
	if maxSeq <= 0 {
		maxSeq = 1
	}
	b := &Batch{
		c:         C.llama_batch_init(C.int32_t(batchSize*maxSeq), C.int32_t(embedSize), C.int32_t(maxSeq)),
		batchSize: batchSize,
		maxSeq:    maxSeq,
		embedSize: embedSize,
	}
	if (embedSize == 0 && b.c.token == nil) || b.c.pos == nil || b.c.n_seq_id == nil || b.c.seq_id == nil || b.c.logits == nil {
		C.llama_batch_free(b.c)
		return nil, errors.New("llamacppshim: unable to allocate batch")
	}
	runtime.SetFinalizer(b, (*Batch).Free)
	return b, nil
}

func (b *Batch) allocSize() int { return b.batchSize * b.maxSeq }

// Clear resets the batch to zero tokens.
func (b *Batch) Clear() {
	if b != nil {
		b.c.n_tokens = 0
	}
}

// Add appends a token or embedding row to the batch.
func (b *Batch) Add(token int, embed []float32, pos int, logits bool, seqIDs ...int) error {
	if b == nil {
		return errors.New("llamacppshim: nil batch")
	}
	i := int(b.c.n_tokens)
	if i >= b.allocSize() {
		return errors.New("llamacppshim: batch is full")
	}
	if b.embedSize == 0 {
		unsafe.Slice(b.c.token, b.allocSize())[i] = C.llama_token(token)
	} else {
		if len(embed) != b.embedSize {
			return fmt.Errorf("llamacppshim: embedding row has %d values, want %d", len(embed), b.embedSize)
		}
		copy(unsafe.Slice((*float32)(b.c.embd), b.allocSize()*b.embedSize)[i*b.embedSize:], embed)
	}
	unsafe.Slice(b.c.pos, b.allocSize())[i] = C.llama_pos(pos)
	unsafe.Slice(b.c.n_seq_id, b.allocSize())[i] = C.int32_t(len(seqIDs))
	for j, seqID := range seqIDs {
		unsafe.Slice(unsafe.Slice(b.c.seq_id, b.allocSize())[i], len(seqIDs))[j] = C.llama_seq_id(seqID)
	}
	if logits {
		unsafe.Slice(b.c.logits, b.allocSize())[i] = 1
	} else {
		unsafe.Slice(b.c.logits, b.allocSize())[i] = 0
	}
	b.c.n_tokens++
	return nil
}

// Free releases the llama batch.
func (b *Batch) Free() {
	if b == nil || b.batchSize == 0 {
		return
	}
	C.llama_batch_free(b.c)
	b.batchSize = 0
	runtime.SetFinalizer(b, nil)
}

// SamplingParams controls the small direct sampler chain used by modeld.
type SamplingParams struct {
	TopK        int
	TopP        float32
	MinP        float32
	Temperature float32
	Seed        uint32
}

// SamplingContext wraps a llama_sampler chain.
type SamplingContext struct {
	ptr *C.struct_llama_sampler
}

// NewSamplingContext creates a minimal sampler chain.
func NewSamplingContext(params SamplingParams) (*SamplingContext, error) {
	cparams := C.llama_sampler_chain_default_params()
	chain := C.llama_sampler_chain_init(cparams)
	if chain == nil {
		return nil, errors.New("llamacppshim: create sampler chain")
	}
	if params.TopK > 0 {
		C.llama_sampler_chain_add(chain, C.llama_sampler_init_top_k(C.int32_t(params.TopK)))
	}
	if params.TopP > 0 && params.TopP < 1 {
		C.llama_sampler_chain_add(chain, C.llama_sampler_init_top_p(C.float(params.TopP), 1))
	}
	if params.MinP > 0 {
		C.llama_sampler_chain_add(chain, C.llama_sampler_init_min_p(C.float(params.MinP), 1))
	}
	if params.Temperature <= 0 {
		C.llama_sampler_chain_add(chain, C.llama_sampler_init_greedy())
	} else {
		C.llama_sampler_chain_add(chain, C.llama_sampler_init_temp(C.float(params.Temperature)))
		C.llama_sampler_chain_add(chain, C.llama_sampler_init_dist(C.uint32_t(params.Seed)))
	}
	s := &SamplingContext{ptr: chain}
	runtime.SetFinalizer(s, (*SamplingContext).Free)
	return s, nil
}

// Sample samples a token from context logits.
func (s *SamplingContext) Sample(ctx *Context, idx int) int {
	if s == nil || s.ptr == nil || ctx == nil || ctx.ptr == nil {
		return -1
	}
	return int(C.llama_sampler_sample(s.ptr, ctx.ptr, C.int32_t(idx)))
}

// Accept accepts a sampled token into the sampler history.
func (s *SamplingContext) Accept(id int) {
	if s == nil || s.ptr == nil {
		return
	}
	C.llama_sampler_accept(s.ptr, C.llama_token(id))
}

// Free releases the sampler chain.
func (s *SamplingContext) Free() {
	if s == nil || s.ptr == nil {
		return
	}
	C.llama_sampler_free(s.ptr)
	s.ptr = nil
	runtime.SetFinalizer(s, nil)
}

// StateSeqGetData returns direct sequence state bytes.
func (c *Context) StateSeqGetData(seqID int) ([]byte, error) {
	if c == nil || c.ptr == nil {
		return nil, errors.New("llamacppshim: context is closed")
	}
	n := C.llama_state_seq_get_size(c.ptr, C.llama_seq_id(seqID))
	if n == 0 {
		return nil, errors.New("llamacppshim: empty sequence state")
	}
	buf := make([]byte, int(n))
	got := C.llama_state_seq_get_data(c.ptr, (*C.uint8_t)(unsafe.Pointer(&buf[0])), n, C.llama_seq_id(seqID))
	if got != n {
		return nil, fmt.Errorf("llamacppshim: copied %d bytes, want %d", uint64(got), uint64(n))
	}
	return buf, nil
}

// StateSeqSetData restores direct sequence state bytes.
func (c *Context) StateSeqSetData(seqID int, data []byte) error {
	if c == nil || c.ptr == nil {
		return errors.New("llamacppshim: context is closed")
	}
	if len(data) == 0 {
		return errors.New("llamacppshim: empty state")
	}
	got := C.llama_state_seq_set_data(c.ptr, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.llama_seq_id(seqID))
	if got == 0 {
		return errors.New("llamacppshim: seq state load failed")
	}
	return nil
}
