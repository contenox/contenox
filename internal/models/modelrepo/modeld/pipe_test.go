package modeld

import (
	"context"
	"errors"
	"github.com/contenox/contenox/internal/modeld/slot"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
)

type pipeService struct {
	transport.Service
	transport.NodeAdmin
	session *pipeSession
}

func (s *pipeService) OpenSession(_ context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if req.Type != "llama" {
		return nil, transport.ErrUnsupportedModelType
	}
	return s.session, nil
}

func (s *pipeService) ListModels(context.Context) ([]transport.NodeModel, error) {
	return []transport.NodeModel{{Name: "test", Type: "llama"}}, nil
}

type pipeSession struct {
	transport.Session
	prefix transport.PrefixInput
	suffix transport.SuffixInput
	fail   bool
	config transport.DecodeConfig
}

func (s *pipeSession) EnsurePrefix(_ context.Context, p transport.PrefixInput) (transport.PrefixStatus, error) {
	s.prefix = p
	return transport.PrefixStatus{PrefixTokens: 10, ReusedTokens: 7}, nil
}
func (s *pipeSession) PrefillSuffix(_ context.Context, p transport.SuffixInput) (transport.SuffixStatus, error) {
	s.suffix = p
	return transport.SuffixStatus{SuffixTokens: 20}, nil
}
func (s *pipeSession) Close() error { return nil }
func (s *pipeSession) Decode(_ context.Context, c transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	s.config = c
	out := make(chan transport.StreamChunk, 4)
	out <- transport.StreamChunk{Thinking: "reason", Text: "answer"}
	call := transport.ToolCall{ID: "next", Type: "function"}
	call.Function.Name = "lookup"
	call.Function.Arguments = `{"q":"x"}`
	out <- transport.StreamChunk{ToolCalls: []transport.ToolCall{call}}
	out <- transport.StreamChunk{Usage: &transport.TokenUsage{CompletionTokens: 11, ThinkingTokens: 4}, FinishReason: "stop"}
	if s.fail {
		out <- transport.StreamChunk{Error: errors.New("decode failed")}
	}
	close(out)
	return out, nil
}

func TestUnit_ModeldPipeRoundTrip(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sess := &pipeSession{fail: fail}
			service := &pipeService{Service: transport.NewMemoryService(), session: sess}
			done := make(chan error, 1)
			go func() { done <- transportgrpc.Serve(ctx, listener, service, "owner", "llama") }()
			defer func() { cancel(); <-done }()
			provider := NewModeldProvider("test", []string{listener.Addr().String()}, modelrepo.CapabilityConfig{CanChat: true, CanStream: true}, nil)
			client, err := provider.GetChatConnection(ctx, listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			call := modelrepo.ToolCall{ID: "prior", Type: "function"}
			call.Function.Name = "lookup"
			call.Function.Arguments = `{"q":"old"}`
			result, err := client.Chat(ctx, []modelrepo.Message{{Role: "system", Content: "rules"}, {Role: "user", Content: "ask"}, {Role: "assistant", ToolCalls: []modelrepo.ToolCall{call}}, {Role: "tool", ToolCallID: "prior", Content: "found"}}, modelrepo.WithThink("off"))
			if (err != nil) != fail {
				t.Fatalf("error=%v fail=%v", err, fail)
			}
			if result.Usage == nil || result.Usage.PromptTokens != 30 || result.Usage.CompletionTokens != 11 || result.Usage.ThinkingTokens != 4 || result.Usage.CacheReadTokens != 7 || result.Usage.TotalTokens != 41 {
				t.Fatalf("usage=%+v", result.Usage)
			}
			if !fail && (result.Message.Content != "answer" || result.Message.Thinking != "reason" || len(result.ToolCalls) != 1 || result.FinishReason != "tool_calls") {
				t.Fatalf("result=%+v", result)
			}
			segs := sess.suffix.Manifest.Segments
			if len(segs) != 4 || segs[2].ToolCallsJSON == "" || segs[3].ToolCallID != "prior" {
				t.Fatalf("lost tool history: %+v", segs)
			}
			if sess.suffix.EnableThinking == nil || *sess.suffix.EnableThinking {
				t.Fatal("explicit thinking off lost")
			}
		})
	}
}

func TestUnit_ModeldManifestStableHistoryAndImages(t *testing.T) {
	messages := []modelrepo.Message{{Role: "system", Content: "rules"}, {Role: "user", Content: "prior"}, {Role: "assistant", Content: "answer"}, {Role: "user", Content: "inspect", Images: []modelrepo.ImagePart{{Data: []byte("image"), MimeType: "image/png"}}}}
	prefix, suffix, err := splitMessagesToInputs(messages, &modelrepo.ChatConfig{CacheHints: &modelrepo.CacheHints{StableHistoryLen: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if prefix.Text != "rulesprioranswer" || suffix.Text != "inspect"+transport.MediaMarker || len(suffix.Images) != 1 {
		t.Fatalf("prefix=%+v suffix=%+v", prefix, suffix)
	}
	if prefix.Manifest.Segments[3].Stable {
		t.Fatal("image promoted to unsupported stable prefix")
	}
	if _, _, err := splitMessagesToInputs(messages, &modelrepo.ChatConfig{Tools: []modelrepo.Tool{{Function: &modelrepo.FunctionTool{Parameters: make(chan int)}}}}); err == nil {
		t.Fatal("invalid tool schema accepted")
	}
}

func (s *pipeSession) ExplainContext() transport.ContextReport {
	return transport.ContextReport{NumCtx: 4096}
}

func TestUnit_ModeldPipeResolvesTypedModelThroughNativeSlot(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "test")
	if err := os.MkdirAll(modelDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	backend := &pipeService{Service: transport.NewMemoryService(), session: &pipeSession{}}
	service := slot.New(backend, slot.WithBackend("llama"), slot.WithModelsDir(dir))
	done := make(chan error, 1)
	go func() { done <- transportgrpc.Serve(ctx, lis, service, "typed-owner", "llama") }()
	defer func() { cancel(); <-done }()
	provider := NewModeldProvider("test", nil, modelrepo.CapabilityConfig{CanChat: true}, nil)
	client, err := provider.GetChatConnection(ctx, lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Chat(ctx, []modelrepo.Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "answer" {
		t.Fatalf("result=%+v", result)
	}
}
