//go:build !openvino || !openvino_genai

package ovsession

import (
	"context"
	"errors"
)

// GenAIAvailable reports whether the OpenVINO GenAI session backend was built.
const GenAIAvailable = false

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

// GenAISession is unavailable without the openvino and openvino_genai build
// tags.
type GenAISession struct{}

// NewGenAI reports that the native GenAI backend is not compiled in.
func NewGenAI(_ string, _ GenAIConfig) (*GenAISession, error) {
	return nil, errors.New("openvino GenAI backend is not compiled in")
}

// Generate reports that the native GenAI backend is not compiled in.
func (s *GenAISession) Generate(_ context.Context, _ string, _ GenerateOptions) (GenAIResult, error) {
	return GenAIResult{}, errors.New("openvino GenAI backend is not compiled in")
}

// Stream reports that the native GenAI backend is not compiled in.
func (s *GenAISession) Stream(_ context.Context, _ string, _ GenerateOptions) (<-chan StreamChunk, error) {
	return nil, errors.New("openvino GenAI backend is not compiled in")
}

// Close is a no-op for the stub.
func (s *GenAISession) Close() error {
	return nil
}

// ChatMessage is one role/content turn for chat-template rendering.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCall   string `json:",omitempty"`
	ToolCallID string `json:",omitempty"`
}

// ApplyChatTemplate reports that the native GenAI backend is not compiled in.
func (s *GenAISession) ApplyChatTemplate(_ []ChatMessage, _ string) (string, error) {
	return "", errors.New("openvino GenAI backend is not compiled in")
}

// Tokenize reports that the native GenAI backend is not compiled in.
func (s *GenAISession) Tokenize(_ context.Context, _ string, _ bool) ([]int, error) {
	return nil, errors.New("openvino GenAI backend is not compiled in")
}
