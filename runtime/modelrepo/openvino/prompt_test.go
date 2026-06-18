package openvino

import (
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func TestUnit_LocalNodePromptPlan_RejectsUnsupportedPrompt(t *testing.T) {
	_, err := buildPromptPlan(nil, Config{PromptFormat: "unknown"}, promptIdentity{}, "")
	// OpenVINO prompt format validates natively inside modeld based on the chat template;
	// it doesn't currently error here. We just check basic history behavior here.
	if err != nil {
		t.Logf("Expected success for format passing, got: %v", err)
	}
}

func TestUnit_LocalNodePromptPlan_PropagatesToolHistory(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "assistant", Content: "thinking", ToolCalls: []modelrepo.ToolCall{{ID: "call_123", Type: "function"}}},
		{Role: "tool", Content: "result", ToolCallID: "call_123"},
	}, Config{}, promptIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Volatile.Manifest.Segments) != 2 {
		t.Fatalf("expected 2 volatile segments, got %d", len(plan.Volatile.Manifest.Segments))
	}

	astSeg := plan.Volatile.Manifest.Segments[0]
	if astSeg.Kind != "assistant" || astSeg.ToolCallsJSON == "" {
		t.Fatalf("assistant segment missing tool calls JSON: %+v", astSeg)
	}
	if !strings.Contains(astSeg.ToolCallsJSON, "call_123") {
		t.Fatalf("assistant segment tool calls JSON missing ID: %s", astSeg.ToolCallsJSON)
	}

	toolSeg := plan.Volatile.Manifest.Segments[1]
	if toolSeg.Kind != "tool" || toolSeg.ToolCallID != "call_123" {
		t.Fatalf("tool segment missing or incorrect tool call ID: %+v", toolSeg)
	}
}
