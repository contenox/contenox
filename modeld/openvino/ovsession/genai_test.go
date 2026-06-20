//go:build openvino && openvino_genai

package ovsession

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystem_OpenVINOGenAI_SessionGenerateAndClose(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	s, err := NewGenAI(modelDir, GenAIConfig{Device: device})
	require.NoError(t, err)

	got, err := s.Generate(context.Background(), "def add(a, b):", GenerateOptions{MaxNewTokens: 8})
	require.NoError(t, err)
	require.NotEmpty(t, got.Text)
	require.Equal(t, uint64(1), got.Metrics.Requests)
	require.Equal(t, uint64(1), got.Metrics.ScheduledRequests)
	require.Greater(t, got.Metrics.CacheSizeInBytes, uint64(0))

	require.NoError(t, s.Close())
	require.NoError(t, s.Close())

	_, err = s.Generate(context.Background(), "def sub(a, b):", GenerateOptions{MaxNewTokens: 1})
	require.ErrorContains(t, err, "closed")
}

func TestSystem_OpenVINOGenAI_ColdKVCapability(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	s, err := NewGenAI(modelDir, GenAIConfig{Device: device})
	require.NoError(t, err)
	defer s.Close()

	require.True(t, s.SupportsColdKV(), "GenAI bridge should expose cold KV import/export hooks")
}

func TestSystem_OpenVINOGenAI_TokenPrefillAndGenerate(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	s, err := NewGenAI(modelDir, GenAIConfig{Device: device})
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	tokens, err := s.Tokenize(ctx, "def add(a, b):", true)
	require.NoError(t, err)
	require.NotEmpty(t, tokens)

	require.NoError(t, s.PrefillTokens(ctx, tokens))
	got, err := s.GenerateTokens(ctx, tokens, GenerateOptions{MaxNewTokens: 8})
	require.NoError(t, err)
	require.NotEmpty(t, got.Text)
}

func TestSystem_OpenVINOGenAI_ContextCanceledBeforeGenerate(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	s, err := NewGenAI(modelDir, GenAIConfig{Device: device})
	require.NoError(t, err)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Generate(ctx, "def add(a, b):", GenerateOptions{MaxNewTokens: 8})
	require.ErrorIs(t, err, context.Canceled)
}

// TestSystem_OpenVINOGenAI_StreamCanceledInFlight cancels generation *after* it is
// demonstrably underway (we have already received decoded tokens) and asserts the
// native cancel hook stops the stream early with context.Canceled, rather than
// running the full token budget. This is the deterministic in-flight counterpart
// to the pre-canceled-context case above.
func TestSystem_OpenVINOGenAI_StreamCanceledInFlight(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	s, err := NewGenAI(modelDir, GenAIConfig{Device: device})
	require.NoError(t, err)
	defer s.Close()

	const budget = 1024
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := s.Stream(ctx, "Write a very long Go program:", GenerateOptions{MaxNewTokens: budget})
	require.NoError(t, err)

	var got int
	var sawCancel bool
	for chunk := range ch {
		if chunk.Error != nil {
			require.ErrorIs(t, chunk.Error, context.Canceled)
			sawCancel = true
			continue // keep draining until the channel closes
		}
		got++
		if got == 2 {
			cancel() // generation is in-flight; cut it off
		}
	}

	require.True(t, sawCancel, "expected an in-flight context.Canceled stream error")
	require.Greater(t, got, 0, "expected some tokens before cancellation")
	require.Less(t, got, budget, "stream should stop well before the full token budget")
}
