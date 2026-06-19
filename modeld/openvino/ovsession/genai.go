//go:build openvino && openvino_genai

package ovsession

/*
#cgo CXXFLAGS: -std=c++17
#include <stdlib.h>
#include "genai.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

// GenAIAvailable reports whether the OpenVINO GenAI session backend was built.
const GenAIAvailable = true

const (
	genAIOutLen = 256 * 1024
	genAIErrLen = 4 * 1024
)

// GenAIConfig controls construction of an OpenVINO GenAI session.
type GenAIConfig struct {
	Device                      string
	KVCachePrecision            string
	CacheSize                   int
	DynamicSplitFuse            *bool
	EnablePrefixCaching         *bool
	UseSparseAttention          *bool
	NumLastDenseTokensInPrefill int
	XAttentionThreshold         float32
	XAttentionBlockSize         int
	XAttentionStride            int
}

// GenerateOptions controls a single GenAI generation call.
type GenerateOptions struct {
	MaxNewTokens     int
	Temperature      *float64
	TopP             *float64
	StructuredOutput StructuredOutput
	ParserProtocols  []string
}

// StructuredOutput selects an OpenVINO structured-output primitive plus its
// payload for this generation call.
type StructuredOutput struct {
	Protocol string
	Payload  string
}

// PipelineMetrics mirrors the OpenVINO GenAI PipelineMetrics fields used by
// the local runtime.
type PipelineMetrics struct {
	Requests          uint64
	ScheduledRequests uint64
	CacheUsage        float32
	MaxCacheUsage     float32
	AvgCacheUsage     float32
	InferenceDuration float32
	CacheSizeInBytes  uint64
}

// DeviceInfo describes one OpenVINO device and any memory telemetry exposed by
// that plugin. CPU devices report zero memory here; modeld uses system RAM for
// CPU capacity planning.
type DeviceInfo struct {
	Index             int
	Name              string
	Description       string
	Type              string
	MemoryFree        uint64
	MemoryTotal       uint64
	SharedWithDisplay bool
}

// RuntimeInfo describes the linked OpenVINO runtime and available devices.
type RuntimeInfo struct {
	RuntimeName        string
	RuntimeDigest      string
	RuntimeSystemInfo  string
	SupportsGPUOffload bool
	Devices            []DeviceInfo
}

// GenAIResult is the generated text plus the pipeline metrics observed for the
// request.
type GenAIResult struct {
	Text       string
	ParsedJSON string
	Metrics    PipelineMetrics
}

// StreamChunk carries a decoded text delta or a terminal stream error.
type StreamChunk struct {
	Text  string
	Error error
}

// GenAISession wraps a single ContinuousBatchingPipeline.
type GenAISession struct {
	mu  sync.Mutex
	ptr *C.cx_genai_session
}

// Runtime reports the linked OpenVINO runtime identity and available devices.
func Runtime() (RuntimeInfo, error) {
	errbuf := C.calloc(1, C.size_t(genAIErrLen))
	if errbuf == nil {
		return RuntimeInfo{}, errors.New("allocate OpenVINO runtime info error buffer")
	}
	defer C.free(errbuf)

	var out C.cx_ov_runtime_info
	if rc := C.cx_ov_runtime_info_get(&out, (*C.char)(errbuf), C.size_t(genAIErrLen)); rc != 0 {
		return RuntimeInfo{}, fmt.Errorf("openvino runtime info: %s", C.GoString((*C.char)(errbuf)))
	}

	info := RuntimeInfo{
		RuntimeName:        cString(out.runtime_name[:]),
		RuntimeDigest:      cString(out.runtime_digest[:]),
		RuntimeSystemInfo:  cString(out.system_info[:]),
		SupportsGPUOffload: out.supports_gpu_offload != 0,
	}
	count := int(out.device_count)
	if count > len(out.devices) {
		count = len(out.devices)
	}
	for i := 0; i < count; i++ {
		info.Devices = append(info.Devices, deviceInfoFromC(out.devices[i]))
	}
	return info, nil
}

// Device returns telemetry for the selected OpenVINO device name, resolving
// aliases such as "GPU" to the first available OpenVINO GPU device.
func Device(device string) (DeviceInfo, error) {
	cDev := C.CString(device)
	defer C.free(unsafe.Pointer(cDev))
	errbuf := C.calloc(1, C.size_t(genAIErrLen))
	if errbuf == nil {
		return DeviceInfo{}, errors.New("allocate OpenVINO device info error buffer")
	}
	defer C.free(errbuf)

	var out C.cx_ov_device_info
	if rc := C.cx_ov_device_info_get(cDev, &out, (*C.char)(errbuf), C.size_t(genAIErrLen)); rc != 0 {
		return DeviceInfo{}, fmt.Errorf("openvino device info: %s", C.GoString((*C.char)(errbuf)))
	}
	return deviceInfoFromC(out), nil
}

func deviceInfoFromC(d C.cx_ov_device_info) DeviceInfo {
	return DeviceInfo{
		Index:             int(d.index),
		Name:              cString(d.name[:]),
		Description:       cString(d.description[:]),
		Type:              cString(d._type[:]),
		MemoryFree:        uint64(d.memory_free),
		MemoryTotal:       uint64(d.memory_total),
		SharedWithDisplay: d.shared_with_display != 0,
	}
}

func cString(buf []C.char) string {
	if len(buf) == 0 {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
}

// NewGenAI creates an OpenVINO GenAI ContinuousBatchingPipeline session.
func NewGenAI(modelDir string, cfg GenAIConfig) (*GenAISession, error) {
	if modelDir == "" {
		return nil, errors.New("openvino GenAI model directory is required")
	}
	device := cfg.Device
	if device == "" {
		device = "CPU"
	}
	cfg = normalizeGenAIConfig(cfg)

	cDir := C.CString(modelDir)
	cDev := C.CString(device)
	cKVPrecision := C.CString(cfg.KVCachePrecision)
	defer C.free(unsafe.Pointer(cDir))
	defer C.free(unsafe.Pointer(cDev))
	defer C.free(unsafe.Pointer(cKVPrecision))

	errbuf := C.calloc(1, C.size_t(genAIErrLen))
	if errbuf == nil {
		return nil, errors.New("allocate OpenVINO GenAI error buffer")
	}
	defer C.free(errbuf)

	cConfig := C.cx_genai_session_config{
		kv_cache_precision:               cKVPrecision,
		cache_size:                       C.size_t(cfg.CacheSize),
		dynamic_split_fuse:               cbool(boolValue(cfg.DynamicSplitFuse, true)),
		enable_prefix_caching:            cbool(boolValue(cfg.EnablePrefixCaching, true)),
		use_sparse_attention:             cbool(boolValue(cfg.UseSparseAttention, true)),
		num_last_dense_tokens_in_prefill: C.size_t(cfg.NumLastDenseTokensInPrefill),
		xattention_threshold:             C.float(cfg.XAttentionThreshold),
		xattention_block_size:            C.size_t(cfg.XAttentionBlockSize),
		xattention_stride:                C.size_t(cfg.XAttentionStride),
	}

	ptr := C.cx_genai_session_new(cDir, cDev, &cConfig, (*C.char)(errbuf), C.size_t(genAIErrLen))
	if ptr == nil {
		return nil, fmt.Errorf("openvino GenAI session new: %s", C.GoString((*C.char)(errbuf)))
	}

	s := &GenAISession{ptr: ptr}
	runtime.SetFinalizer(s, (*GenAISession).mustClose)
	return s, nil
}

// Generate runs one prompt through the GenAI session.
func (s *GenAISession) Generate(ctx context.Context, prompt string, opts GenerateOptions) (GenAIResult, error) {
	if err := ctx.Err(); err != nil {
		return GenAIResult{}, err
	}
	if prompt == "" {
		return GenAIResult{}, errors.New("openvino GenAI prompt is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptr == nil {
		return GenAIResult{}, errors.New("openvino GenAI session is closed")
	}

	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	cStructuredProtocol := C.CString(opts.StructuredOutput.Protocol)
	cStructuredPayload := C.CString(opts.StructuredOutput.Payload)
	cParserProtocols := C.CString(strings.Join(opts.ParserProtocols, "\n"))
	defer C.free(unsafe.Pointer(cStructuredProtocol))
	defer C.free(unsafe.Pointer(cStructuredPayload))
	defer C.free(unsafe.Pointer(cParserProtocols))

	out := C.calloc(1, C.size_t(genAIOutLen))
	if out == nil {
		return GenAIResult{}, errors.New("allocate OpenVINO GenAI output buffer")
	}
	defer C.free(out)
	parsed := C.calloc(1, C.size_t(genAIOutLen))
	if parsed == nil {
		return GenAIResult{}, errors.New("allocate OpenVINO GenAI parsed output buffer")
	}
	defer C.free(parsed)

	errbuf := C.calloc(1, C.size_t(genAIErrLen))
	if errbuf == nil {
		return GenAIResult{}, errors.New("allocate OpenVINO GenAI error buffer")
	}
	defer C.free(errbuf)

	var cmetrics C.cx_genai_metrics
	var temp C.float
	var useTemp C.int
	if opts.Temperature != nil {
		temp = C.float(*opts.Temperature)
		useTemp = 1
	}
	var topP C.float
	var useTopP C.int
	if opts.TopP != nil {
		topP = C.float(*opts.TopP)
		useTopP = 1
	}

	done := make(chan struct{})
	if ctx.Done() != nil {
		ptr := s.ptr
		go func() {
			select {
			case <-ctx.Done():
				C.cx_genai_session_cancel(ptr)
			case <-done:
			}
		}()
	}
	rc := C.cx_genai_generate(
		s.ptr,
		cPrompt,
		C.size_t(max(opts.MaxNewTokens, 0)),
		temp,
		useTemp,
		topP,
		useTopP,
		cStructuredProtocol,
		cStructuredPayload,
		cParserProtocols,
		(*C.char)(out),
		C.size_t(genAIOutLen),
		(*C.char)(parsed),
		C.size_t(genAIOutLen),
		&cmetrics,
		(*C.char)(errbuf),
		C.size_t(genAIErrLen),
	)
	close(done)
	if rc != 0 {
		if rc == 3 {
			if err := ctx.Err(); err != nil {
				return GenAIResult{}, err
			}
			return GenAIResult{}, errors.New("openvino GenAI generation canceled")
		}
		return GenAIResult{}, fmt.Errorf("openvino GenAI generate: %s", C.GoString((*C.char)(errbuf)))
	}
	if err := ctx.Err(); err != nil {
		return GenAIResult{}, err
	}

	return GenAIResult{
		Text:       C.GoString((*C.char)(out)),
		ParsedJSON: C.GoString((*C.char)(parsed)),
		Metrics: PipelineMetrics{
			Requests:          uint64(cmetrics.requests),
			ScheduledRequests: uint64(cmetrics.scheduled_requests),
			CacheUsage:        float32(cmetrics.cache_usage),
			MaxCacheUsage:     float32(cmetrics.max_cache_usage),
			AvgCacheUsage:     float32(cmetrics.avg_cache_usage),
			InferenceDuration: float32(cmetrics.inference_duration),
			CacheSizeInBytes:  uint64(cmetrics.cache_size_in_bytes),
		},
	}, nil
}

// Stream runs one prompt and returns decoded text deltas as GenAI produces them.
func (s *GenAISession) Stream(ctx context.Context, prompt string, opts GenerateOptions) (<-chan StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prompt == "" {
		return nil, errors.New("openvino GenAI prompt is required")
	}

	s.mu.Lock()
	if s.ptr == nil {
		s.mu.Unlock()
		return nil, errors.New("openvino GenAI session is closed")
	}
	ptr := s.ptr

	stream := C.cx_genai_stream_new()
	if stream == nil {
		s.mu.Unlock()
		return nil, errors.New("allocate OpenVINO GenAI stream")
	}

	ch := make(chan StreamChunk, 16)
	generatorDone := make(chan struct{})

	go func() {
		defer close(generatorDone)
		defer s.mu.Unlock()

		cPrompt := C.CString(prompt)
		defer C.free(unsafe.Pointer(cPrompt))

		errbuf := C.calloc(1, C.size_t(genAIErrLen))
		if errbuf == nil {
			msg := C.CString("allocate OpenVINO GenAI stream generator error buffer")
			C.cx_genai_stream_abort(stream, msg)
			C.free(unsafe.Pointer(msg))
			C.cx_genai_session_cancel(ptr)
			return
		}
		defer C.free(errbuf)

		cParserProtocols := C.CString(strings.Join(opts.ParserProtocols, "\n"))
		defer C.free(unsafe.Pointer(cParserProtocols))

		var cmetrics C.cx_genai_metrics
		var temp C.float
		var useTemp C.int
		if opts.Temperature != nil {
			temp = C.float(*opts.Temperature)
			useTemp = 1
		}
		var topP C.float
		var useTopP C.int
		if opts.TopP != nil {
			topP = C.float(*opts.TopP)
			useTopP = 1
		}

		done := make(chan struct{})
		if ctx.Done() != nil {
			go func() {
				select {
				case <-ctx.Done():
					C.cx_genai_session_cancel(ptr)
				case <-done:
				}
			}()
		}
		C.cx_genai_generate_stream(
			ptr,
			cPrompt,
			C.size_t(max(opts.MaxNewTokens, 0)),
			temp,
			useTemp,
			topP,
			useTopP,
			cParserProtocols,
			stream,
			&cmetrics,
			(*C.char)(errbuf),
			C.size_t(genAIErrLen),
		)
		close(done)
	}()

	go func() {
		defer close(ch)
		defer func() {
			<-generatorDone
			C.cx_genai_stream_free(stream)
		}()

		out := C.calloc(1, C.size_t(genAIOutLen))
		if out == nil {
			ch <- StreamChunk{Error: errors.New("allocate OpenVINO GenAI stream output buffer")}
			C.cx_genai_session_cancel(ptr)
			return
		}
		defer C.free(out)

		errbuf := C.calloc(1, C.size_t(genAIErrLen))
		if errbuf == nil {
			ch <- StreamChunk{Error: errors.New("allocate OpenVINO GenAI stream error buffer")}
			C.cx_genai_session_cancel(ptr)
			return
		}
		defer C.free(errbuf)

		for {
			rc := C.cx_genai_stream_next(
				stream,
				(*C.char)(out),
				C.size_t(genAIOutLen),
				(*C.char)(errbuf),
				C.size_t(genAIErrLen),
			)
			switch rc {
			case 0:
				text := C.GoString((*C.char)(out))
				if text == "" {
					continue
				}
				select {
				case ch <- StreamChunk{Text: text}:
				case <-ctx.Done():
					C.cx_genai_session_cancel(ptr)
				}
			case 1:
				return
			case 3:
				if err := ctx.Err(); err != nil {
					ch <- StreamChunk{Error: err}
				} else {
					ch <- StreamChunk{Error: errors.New("openvino GenAI generation canceled")}
				}
				return
			default:
				ch <- StreamChunk{Error: fmt.Errorf("openvino GenAI stream: %s", C.GoString((*C.char)(errbuf)))}
				return
			}
		}
	}()

	return ch, nil
}

// Close releases the native GenAI session.
func (s *GenAISession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptr == nil {
		return nil
	}
	runtime.SetFinalizer(s, nil)
	C.cx_genai_session_free(s.ptr)
	s.ptr = nil
	return nil
}

func (s *GenAISession) mustClose() {
	_ = s.Close()
}

// ChatMessage is one role/content turn for chat-template rendering.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  string // raw JSON array, e.g. [{"id":"...","type":"function",...}]
	ToolCallID string // for role=="tool" result turns
}

// ApplyChatTemplate renders messages with the model's own chat template
// (from tokenizer_config.json) via OpenVINO, returning the prompt string to feed
// to Generate/Stream. This replaces hand-rolled prompt formatting so the model
// sees the format it was trained on.
func (s *GenAISession) ApplyChatTemplate(messages []ChatMessage, toolsJSON string) (string, error) {
	if len(messages) == 0 {
		return "", errors.New("openvino GenAI chat template requires at least one message")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptr == nil {
		return "", errors.New("openvino GenAI session is closed")
	}

	roles := make([]*C.char, len(messages))
	contents := make([]*C.char, len(messages))
	toolCalls := make([]*C.char, len(messages))
	toolCallIDs := make([]*C.char, len(messages))
	for i, m := range messages {
		roles[i] = C.CString(m.Role)
		contents[i] = C.CString(m.Content)
		if m.ToolCalls != "" {
			toolCalls[i] = C.CString(m.ToolCalls)
		}
		if m.ToolCallID != "" {
			toolCallIDs[i] = C.CString(m.ToolCallID)
		}
	}
	defer func() {
		for i := range messages {
			C.free(unsafe.Pointer(roles[i]))
			C.free(unsafe.Pointer(contents[i]))
			if toolCalls[i] != nil {
				C.free(unsafe.Pointer(toolCalls[i]))
			}
			if toolCallIDs[i] != nil {
				C.free(unsafe.Pointer(toolCallIDs[i]))
			}
		}
	}()

	var cTools *C.char
	if toolsJSON != "" {
		cTools = C.CString(toolsJSON)
		defer C.free(unsafe.Pointer(cTools))
	}

	out := C.calloc(1, C.size_t(genAIOutLen))
	if out == nil {
		return "", errors.New("allocate OpenVINO GenAI chat template buffer")
	}
	defer C.free(out)

	errbuf := C.calloc(1, C.size_t(genAIErrLen))
	if errbuf == nil {
		return "", errors.New("allocate OpenVINO GenAI error buffer")
	}
	defer C.free(errbuf)

	rc := C.cx_genai_apply_chat_template(
		s.ptr,
		(**C.char)(unsafe.Pointer(&roles[0])),
		(**C.char)(unsafe.Pointer(&contents[0])),
		(**C.char)(unsafe.Pointer(&toolCalls[0])),
		(**C.char)(unsafe.Pointer(&toolCallIDs[0])),
		C.size_t(len(messages)),
		cTools,
		(*C.char)(out),
		C.size_t(genAIOutLen),
		(*C.char)(errbuf),
		C.size_t(genAIErrLen),
	)
	if rc != 0 {
		return "", fmt.Errorf("openvino GenAI apply chat template: %s", C.GoString((*C.char)(errbuf)))
	}
	return C.GoString((*C.char)(out)), nil
}

// Tokenize encodes prompt text with the model tokenizer owned by the GenAI
// session. It is an observability/correctness primitive for manifest token
// hashes; generation still receives rendered prompt text.
func (s *GenAISession) Tokenize(ctx context.Context, prompt string, addSpecial bool) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prompt == "" {
		return nil, errors.New("openvino GenAI tokenization prompt is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptr == nil {
		return nil, errors.New("openvino GenAI session is closed")
	}

	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))

	errbuf := C.calloc(1, C.size_t(genAIErrLen))
	if errbuf == nil {
		return nil, errors.New("allocate OpenVINO GenAI tokenization error buffer")
	}
	defer C.free(errbuf)

	var required C.size_t
	rc := C.cx_genai_tokenize(
		s.ptr,
		cPrompt,
		cbool(addSpecial),
		nil,
		0,
		&required,
		(*C.char)(errbuf),
		C.size_t(genAIErrLen),
	)
	if rc != 2 && rc != 0 {
		return nil, fmt.Errorf("openvino GenAI tokenize: %s", C.GoString((*C.char)(errbuf)))
	}
	if required == 0 {
		return nil, nil
	}

	buf := make([]C.int64_t, int(required))
	rc = C.cx_genai_tokenize(
		s.ptr,
		cPrompt,
		cbool(addSpecial),
		(*C.int64_t)(unsafe.Pointer(&buf[0])),
		required,
		&required,
		(*C.char)(errbuf),
		C.size_t(genAIErrLen),
	)
	if rc != 0 {
		return nil, fmt.Errorf("openvino GenAI tokenize: %s", C.GoString((*C.char)(errbuf)))
	}
	out := make([]int, len(buf))
	for i, tok := range buf {
		out[i] = int(tok)
	}
	return out, nil
}

func normalizeGenAIConfig(cfg GenAIConfig) GenAIConfig {
	if cfg.KVCachePrecision == "" {
		cfg.KVCachePrecision = "f16"
	}
	if cfg.CacheSize <= 0 {
		cfg.CacheSize = 1
	}
	if cfg.NumLastDenseTokensInPrefill <= 0 {
		cfg.NumLastDenseTokensInPrefill = 10
	}
	if cfg.XAttentionThreshold <= 0 {
		cfg.XAttentionThreshold = 0.9
	}
	if cfg.XAttentionBlockSize <= 0 {
		cfg.XAttentionBlockSize = 128
	}
	if cfg.XAttentionStride <= 0 {
		cfg.XAttentionStride = 16
	}
	return cfg
}

func boolValue(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func cbool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
