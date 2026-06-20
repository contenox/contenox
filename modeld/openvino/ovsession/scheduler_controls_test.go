//go:build openvino && openvino_genai

package ovsession

import (
	"os"
	"strings"
	"testing"
)

func TestSystem_OpenVINOGenAI_SchedulerControlsReachable(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	got, err := runGenAISchedulerProbe(modelDir, device)
	if err != nil {
		t.Fatalf("GenAI scheduler probe failed: %s", err)
	}
	t.Log(got)
	for _, want := range []string{
		"cache_size: 1",
		"dynamic_split_fuse: true",
		"enable_prefix_caching: true",
		"use_sparse_attention: true",
		"sparseAttentionMode: XATTENTION",
		"xattention_threshold: 0.9",
		"xattention_block_size: 128",
		"xattention_stride: 16",
		"PipelineMetrics",
		"GenerationResultCount: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("scheduler report missing %q\n%s", want, got)
		}
	}
}
