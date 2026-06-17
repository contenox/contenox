//go:build openvino && openvino_genai

package ovsession_test

import (
	"context"
	"os"
	"testing"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
)

func TestSystem_OpenVINOGenAI_ReasoningStreaming(t *testing.T) {
	path := os.Getenv("CONTENOX_OPENVINO_REASONING_MODEL")
	if path == "" {
		t.Skip("set CONTENOX_OPENVINO_REASONING_MODEL to run reasoning parser tests")
	}

	session, err := ovsession.NewGenAI(path, ovsession.GenAIConfig{Device: "CPU"})
	if err != nil {
		t.Fatalf("NewGenAI: %v", err)
	}
	defer session.Close()

	ctx := context.Background()

	// deepseek-r1 model test
	messages := []ovsession.ChatMessage{
		{Role: "user", Content: "Why is the sky blue?"},
	}
	prompt, err := session.ApplyChatTemplate(messages, "")
	if err != nil {
		t.Fatalf("ApplyChatTemplate: %v", err)
	}
	t.Logf("Formatted prompt: %q", prompt)

	opts := ovsession.GenerateOptions{
		MaxNewTokens: 128,
	}

	ch, err := session.Stream(ctx, prompt, opts)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var text string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("Chunk error: %v", chunk.Error)
		}
		t.Logf("Chunk: %q", chunk.Text)
		text += chunk.Text
	}

	if text == "" {
		t.Fatal("Expected some output, got empty")
	}

	t.Logf("Filtered streamed output: %s", text)
}
