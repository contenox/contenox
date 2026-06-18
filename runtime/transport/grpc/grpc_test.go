package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"

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
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = transportgrpc.Serve(ctx, lis, svc, ownerFence, "test") }()

	client, err := transportgrpc.DialLeader(lis.Addr().String(), expectedOwner)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(); cancel() })
	return client
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
