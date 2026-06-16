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
