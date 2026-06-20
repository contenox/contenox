package grpc_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/contenox/runtime/modeld/slot"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// startServer serves a MemoryService over gRPC on a loopback port and returns a
// fenced client dialed at the given expectedOwner.
func startServer(t *testing.T, ownerFence, expectedOwner string) *transportgrpc.Client {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svc := transport.NewMemoryService(transport.WithOwnerFence(ownerFence))
	return startServerWithService(t, svc, lis, ownerFence, expectedOwner)
}

func startServerWithService(t *testing.T, svc transport.Service, lis net.Listener, ownerFence, expectedOwner string) *transportgrpc.Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = transportgrpc.Serve(ctx, lis, svc, ownerFence, "test") }()

	client, err := transportgrpc.DialLeader(lis.Addr().String(), expectedOwner)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(); cancel() })
	return client
}

type embeddingService struct {
	*transport.MemoryService

	mu   sync.Mutex
	last transport.EmbedRequest
}

func (s *embeddingService) Embed(ctx context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	if _, err := s.MemoryService.Embed(ctx, req); errors.Is(err, transport.ErrStaleFence) {
		return transport.EmbedResult{}, err
	}
	s.mu.Lock()
	s.last = req
	s.mu.Unlock()
	return transport.EmbedResult{Vector: []float32{1, 2.5, -3}}, nil
}

func (s *embeddingService) lastRequest() transport.EmbedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

type decodeRecordingService struct {
	*transport.MemoryService
	sess transport.Session
}

func (s *decodeRecordingService) OpenSession(context.Context, transport.OpenSessionRequest) (transport.Session, error) {
	return s.sess, nil
}

type canceledOpenService struct {
	*transport.MemoryService
}

func (s *canceledOpenService) OpenSession(context.Context, transport.OpenSessionRequest) (transport.Session, error) {
	return nil, context.Canceled
}

type describeService struct {
	*transport.MemoryService
	info transport.ModelInfo
}

func (s *describeService) Describe(context.Context, transport.OpenSessionRequest) (transport.ModelInfo, error) {
	return s.info, nil
}

type streamErrorSession struct {
	decodeRecordingSession
	err error
}

func (s *streamErrorSession) Decode(context.Context, transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	ch := make(chan transport.StreamChunk, 1)
	ch <- transport.StreamChunk{Error: s.err}
	close(ch)
	return ch, nil
}

type decodeRecordingSession struct {
	mu     sync.Mutex
	config transport.DecodeConfig
}

func transportToolCall(id, name, arguments string) transport.ToolCall {
	var tc transport.ToolCall
	tc.ID = id
	tc.Type = "function"
	tc.Function.Name = name
	tc.Function.Arguments = arguments
	return tc
}

func (s *decodeRecordingSession) EnsurePrefix(context.Context, transport.PrefixInput) (transport.PrefixStatus, error) {
	return transport.PrefixStatus{}, nil
}

func (s *decodeRecordingSession) PrefillSuffix(context.Context, transport.SuffixInput) (transport.SuffixStatus, error) {
	return transport.SuffixStatus{}, nil
}

func (s *decodeRecordingSession) Decode(_ context.Context, cfg transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	ch := make(chan transport.StreamChunk, 2)
	ch <- transport.StreamChunk{Thinking: "think"}
	ch <- transport.StreamChunk{Text: "answer", ToolCalls: []transport.ToolCall{transportToolCall("call_1", "lookup", `{"q":"x"}`)}}
	close(ch)
	return ch, nil
}

func (s *decodeRecordingSession) ExplainContext() transport.ContextReport {
	return transport.ContextReport{}
}
func (s *decodeRecordingSession) Snapshot(context.Context) (transport.SessionSnapshot, error) {
	return transport.SessionSnapshot{}, nil
}
func (s *decodeRecordingSession) Restore(context.Context, transport.SessionSnapshot) error {
	return nil
}
func (s *decodeRecordingSession) Close() error { return nil }

func (s *decodeRecordingSession) lastConfig() transport.DecodeConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config
}

func manifest(stable string) contextasm.ContextManifest {
	return contextasm.ContextManifest{
		Backend:              "mem",
		ModelDigest:          "d1",
		PromptFormat:         "f1",
		PromptTemplateDigest: "t1",
		RuntimeDigest:        "r1",
		StableByteHash:       contextasm.HashString(stable),
	}
}

func TestDecodeMetadataRoundTripOverWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	rec := &decodeRecordingSession{}
	svc := &decodeRecordingService{
		MemoryService: transport.NewMemoryService(transport.WithOwnerFence("owner-1")),
		sess:          rec,
	}
	client := startServerWithService(t, svc, lis, "owner-1", "owner-1")
	sess, err := client.OpenSession(context.Background(), transport.OpenSessionRequest{
		Fence: transport.Fence{OwnerInstanceID: "owner-1"},
	})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	ch, err := sess.Decode(context.Background(), transport.DecodeConfig{
		MaxTokens:       4,
		ParserProtocols: []string{"openvino:deepseek_r1_reasoning_incremental_parser"},
		ReasoningFormat: "deepseek",
		StructuredOutput: transport.StructuredOutputConfig{
			Protocol: "openvino:json_schema_tool_calls",
			Payload:  `{"type":"object"}`,
			ToolName: "echo.echo",
		},
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var text, thinking string
	var calls []transport.ToolCall
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("decode chunk error: %v", chunk.Error)
		}
		text += chunk.Text
		thinking += chunk.Thinking
		calls = append(calls, chunk.ToolCalls...)
	}
	if text != "answer" || thinking != "think" {
		t.Fatalf("stream text/thinking = %q/%q", text, thinking)
	}
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments != `{"q":"x"}` {
		t.Fatalf("stream tool calls = %+v", calls)
	}
	cfg := rec.lastConfig()
	if cfg.MaxTokens != 4 || len(cfg.ParserProtocols) != 1 || cfg.ParserProtocols[0] != "openvino:deepseek_r1_reasoning_incremental_parser" ||
		cfg.ReasoningFormat != "deepseek" ||
		cfg.StructuredOutput.Protocol != "openvino:json_schema_tool_calls" ||
		cfg.StructuredOutput.Payload != `{"type":"object"}` ||
		cfg.StructuredOutput.ToolName != "echo.echo" {
		t.Fatalf("decode config over wire = %+v", cfg)
	}
}

func TestRoundTripContractOverWire(t *testing.T) {
	client := startServer(t, "owner-1", "owner-1")
	ctx := context.Background()

	sess, err := client.OpenSession(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: "owner-1"},
		ModelName: "m",
		Path:      "m",
		Config:    transport.Config{NumCtx: 100},
	})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	m := manifest("hello")
	cold, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: "hello", Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix cold: %v", err)
	}
	if cold.ReusedTokens != 0 || cold.PrefilledTokens != 5 || cold.PrefixTokens != 5 {
		t.Fatalf("cold status over wire = %+v", cold)
	}
	warm, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: "hello", Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix warm: %v", err)
	}
	if warm.ReusedTokens != 5 || warm.PrefilledTokens != 0 {
		t.Fatalf("warm status over wire = %+v", warm)
	}

	if _, err := sess.PrefillSuffix(ctx, transport.SuffixInput{Text: " world", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}

	ch, err := sess.Decode(ctx, transport.DecodeConfig{MaxTokens: 3})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var n int
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("decode streamed %d chunks, want 3", n)
	}

	report := sess.ExplainContext()
	if report.ResidentTokens != 11 { // "hello"(5) + " world"(6)
		t.Fatalf("ExplainContext resident = %d, want 11", report.ResidentTokens)
	}
	snap, err := sess.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.ResidentTokens != 11 || snap.PrefixTokens != 5 || snap.Manifest.Digest() != m.Digest() {
		t.Fatalf("snapshot over wire = %+v, want resident=11 prefix=5 manifest digest %q", snap, m.Digest())
	}
	if _, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: "other", Manifest: manifest("other")}); err != nil {
		t.Fatalf("EnsurePrefix other: %v", err)
	}
	if err := sess.Restore(ctx, snap); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	report = sess.ExplainContext()
	if report.ResidentTokens != 11 || report.ManifestDigest != m.Digest() {
		t.Fatalf("ExplainContext after restore = %+v, want restored snapshot", report)
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// After close the handle is gone: the sentinel must survive the wire.
	if _, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: "x", Manifest: manifest("x")}); !errors.Is(err, transport.ErrSessionClosed) {
		t.Fatalf("EnsurePrefix after close = %v, want ErrSessionClosed", err)
	}
}

func TestStaleFenceRejectedOverWire(t *testing.T) {
	client := startServer(t, "owner-1", "wrong-owner")
	_, err := client.OpenSession(context.Background(), transport.OpenSessionRequest{ModelName: "m", Path: "m"})
	if !errors.Is(err, transport.ErrStaleFence) {
		t.Fatalf("OpenSession with wrong owner = %v, want ErrStaleFence", err)
	}
}

func TestContextOverflowSentinelOverWire(t *testing.T) {
	client := startServer(t, "owner-1", "owner-1")
	ctx := context.Background()
	sess, err := client.OpenSession(ctx, transport.OpenSessionRequest{
		Fence:  transport.Fence{OwnerInstanceID: "owner-1"},
		Config: transport.Config{NumCtx: 4},
	})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	_, err = sess.EnsurePrefix(ctx, transport.PrefixInput{Text: "too many tokens", Manifest: manifest("too many tokens")})
	if !errors.Is(err, transport.ErrContextOverflow) {
		t.Fatalf("EnsurePrefix overflow = %v, want ErrContextOverflow", err)
	}
}

func TestContextCanceledSentinelOverWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svc := &canceledOpenService{MemoryService: transport.NewMemoryService(transport.WithOwnerFence("owner-1"))}
	client := startServerWithService(t, svc, lis, "owner-1", "owner-1")

	_, err = client.OpenSession(context.Background(), transport.OpenSessionRequest{Fence: transport.Fence{OwnerInstanceID: "owner-1"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenSession canceled over wire = %v, want context.Canceled", err)
	}
}

func TestDescribeContextBudgetRoundTripOverWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	want := transport.ModelInfo{
		ModelMaxContext:              32768,
		EffectiveContext:             8192,
		MemoryContextTokens:          12000,
		HotContextTokens:             8192,
		PlannerEffectiveContext:      8192,
		KVBytesPerToken:              2048,
		FreeBytes:                    64 << 20,
		WeightsBytes:                 4 << 20,
		UsableBytes:                  32 << 20,
		RequiredBytes:                20 << 20,
		Clamped:                      true,
		Reason:                       "request_exceeds_memory_budget",
		DeviceKind:                   "gpu",
		DeviceID:                     "GPU.0",
		SparseAttention:              true,
		SlidingWindowAttentionTokens: 4096,
	}
	svc := &describeService{
		MemoryService: transport.NewMemoryService(transport.WithOwnerFence("owner-1")),
		info:          want,
	}
	client := startServerWithService(t, svc, lis, "owner-1", "owner-1")

	got, err := client.Describe(context.Background(), transport.OpenSessionRequest{
		Fence:  transport.Fence{OwnerInstanceID: "owner-1"},
		Config: transport.Config{NumCtx: 8192},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got.ModelMaxContext != want.ModelMaxContext ||
		got.EffectiveContext != want.EffectiveContext ||
		got.MemoryContextTokens != want.MemoryContextTokens ||
		got.HotContextTokens != want.HotContextTokens ||
		got.PlannerEffectiveContext != want.PlannerEffectiveContext ||
		got.KVBytesPerToken != want.KVBytesPerToken ||
		got.RequiredBytes != want.RequiredBytes ||
		got.Reason != want.Reason ||
		got.DeviceKind != want.DeviceKind ||
		got.DeviceID != want.DeviceID ||
		got.SparseAttention != want.SparseAttention ||
		got.SlidingWindowAttentionTokens != want.SlidingWindowAttentionTokens {
		t.Fatalf("Describe over wire = %+v, want %+v", got, want)
	}
}

func TestEmbedRoundTripOverWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svc := &embeddingService{MemoryService: transport.NewMemoryService(transport.WithOwnerFence("owner-1"))}
	client := startServerWithService(t, svc, lis, "owner-1", "owner-1")

	res, err := client.Embed(context.Background(), transport.EmbedRequest{
		Fence:     transport.Fence{OwnerInstanceID: "owner-1"},
		ModelName: "embedder",
		Type:      "openvino",
		Digest:    "sha256:test",
		Path:      "/models/embedder",
		Text:      "query text",
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(res.Vector) != 3 || res.Vector[0] != 1 || res.Vector[1] != 2.5 || res.Vector[2] != -3 {
		t.Fatalf("Embed vector = %+v", res.Vector)
	}
	req := svc.lastRequest()
	if req.ModelName != "embedder" || req.Type != "openvino" || req.Digest != "sha256:test" || req.Path != "/models/embedder" {
		t.Fatalf("Embed request identity = %+v", req)
	}
	if req.Text != "query text" {
		t.Fatalf("Embed text = %q", req.Text)
	}
}

func TestSlotControlRoundTripOverWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svc := slot.New(
		transport.NewMemoryService(transport.WithOwnerFence("owner-1")),
		slot.WithOwner("owner-1"),
		slot.WithBackend("llama"),
	)
	client := startServerWithService(t, svc, lis, "owner-1", "owner-1")
	ctx := context.Background()

	active, err := client.LoadModel(ctx, transport.LoadModelRequest{
		Fence:     transport.Fence{OwnerInstanceID: "owner-1"},
		ModelName: "a",
		Type:      "llama",
		Digest:    "digest-a",
		Path:      "/models/a.gguf",
		Config:    transport.Config{NumCtx: 100},
	})
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if active.ModelName != "a" || active.Generation == 0 {
		t.Fatalf("active over wire = %+v", active)
	}

	st, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State != transport.SlotReady || st.Active == nil || st.Active.Generation != active.Generation {
		t.Fatalf("status over wire = %+v", st)
	}

	sess, err := client.OpenSession(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: "owner-1"},
		ModelName: "a",
		Type:      "llama",
		Digest:    "digest-a",
		Path:      "/models/a.gguf",
		Config:    transport.Config{NumCtx: 100},
	})
	if err != nil {
		t.Fatalf("OpenSession active: %v", err)
	}
	_, err = client.LoadModel(ctx, transport.LoadModelRequest{
		Fence:     transport.Fence{OwnerInstanceID: "owner-1"},
		ModelName: "b",
		Type:      "llama",
		Digest:    "digest-b",
		Path:      "/models/b.gguf",
		Config:    transport.Config{NumCtx: 100},
	})
	if !errors.Is(err, transport.ErrModelBusy) {
		t.Fatalf("LoadModel while session held = %v, want ErrModelBusy", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := client.UnloadModel(ctx, transport.UnloadModelRequest{ExpectedGeneration: active.Generation}); err != nil {
		t.Fatalf("UnloadModel: %v", err)
	}
}

func TestStreamChunkSentinelErrorOverWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svc := &decodeRecordingService{
		MemoryService: transport.NewMemoryService(transport.WithOwnerFence("owner-1")),
		sess:          &streamErrorSession{err: transport.ErrSlotGenerationStale},
	}
	client := startServerWithService(t, svc, lis, "owner-1", "owner-1")
	sess, err := client.OpenSession(context.Background(), transport.OpenSessionRequest{
		Fence: transport.Fence{OwnerInstanceID: "owner-1"},
	})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	ch, err := sess.Decode(context.Background(), transport.DecodeConfig{})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	chunk, ok := <-ch
	if !ok {
		t.Fatal("Decode stream closed without chunk")
	}
	if !errors.Is(chunk.Error, transport.ErrSlotGenerationStale) {
		t.Fatalf("stream chunk error = %v, want ErrSlotGenerationStale", chunk.Error)
	}
}
