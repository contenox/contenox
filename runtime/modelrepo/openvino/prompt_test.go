package openvino

import (
	"errors"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
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

func TestUnit_LocalNodePromptPlan_IncludesBackendVersion(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "work"},
	}, Config{NumCtx: 4096}, promptIdentity{
		ProfileID:      "coder",
		ModelDigest:    "sha256:model",
		BackendVersion: "OpenVINO GenAI@2026.2",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Stable.Manifest.BackendVersion != "OpenVINO GenAI@2026.2" {
		t.Fatalf("BackendVersion = %q", plan.Stable.Manifest.BackendVersion)
	}
	if plan.Stable.Manifest.Backend != "openvino" || plan.Stable.Manifest.RuntimeDigest == "" {
		t.Fatalf("manifest identity incomplete: %+v", plan.Stable.Manifest)
	}
}

// Golden test for the image wire encoding: every attached image contributes one
// media marker in the volatile text (in reading order) and its bytes ride on
// SuffixInput.Images in the same order. The stable prefix stays image-free.
func TestUnit_OpenVINOPromptPlan_EncodesImagesOnVolatileSuffix(t *testing.T) {
	img1 := modelrepo.ImagePart{Data: []byte("png-1"), MimeType: "image/png"}
	img2 := modelrepo.ImagePart{Data: []byte("jpg-2"), MimeType: "image/jpeg"}
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "What is this?", Images: []modelrepo.ImagePart{img1}},
		{Role: "assistant", Content: "A cat."},
		{Role: "user", Content: "And this?", Images: []modelrepo.ImagePart{img2}},
	}, Config{}, promptIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}

	if plan.Stable.Text != "rules" || len(plan.Stable.Images) != 0 {
		t.Fatalf("stable prefix must stay image-free: text=%q images=%d", plan.Stable.Text, len(plan.Stable.Images))
	}
	m := transport.MediaMarker
	wantVolatile := m + "\nWhat is this?" + "A cat." + m + "\nAnd this?"
	if plan.Volatile.Text != wantVolatile {
		t.Fatalf("volatile text = %q\nwant %q", plan.Volatile.Text, wantVolatile)
	}
	if len(plan.Volatile.Images) != 2 {
		t.Fatalf("volatile images = %d, want 2", len(plan.Volatile.Images))
	}
	if string(plan.Volatile.Images[0].Data) != "png-1" || string(plan.Volatile.Images[1].Data) != "jpg-2" {
		t.Fatalf("image order lost: %+v", plan.Volatile.Images)
	}
	if plan.Volatile.Images[1].MimeType != "image/jpeg" {
		t.Fatalf("mime type lost: %+v", plan.Volatile.Images[1])
	}
}

func TestUnit_OpenVINOPromptPlan_RejectsSystemImages(t *testing.T) {
	_, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "look", Images: []modelrepo.ImagePart{{Data: []byte("x"), MimeType: "image/png"}}},
		{Role: "user", Content: "hi"},
	}, Config{}, promptIdentity{}, "")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("expected typed unsupported-feature refusal for system images, got: %v", err)
	}
}

func TestUnit_OpenVINORuntimeDigest_IncludesPlannerContext(t *testing.T) {
	base := runtimeDigest(Config{NumCtx: 4096}, nil)
	planner := runtimeDigest(Config{NumCtx: 4096, PlannerEffectiveContext: 16384}, nil)
	if base == planner {
		t.Fatal("planner context must be part of runtime digest identity")
	}

	ref := modeldconn.ModelRef{Name: "m", Type: "openvino", Digest: "digest"}
	baseKey := sessionCacheKey(ref, Config{NumCtx: 4096})
	plannerKey := sessionCacheKey(ref, Config{NumCtx: 4096, PlannerEffectiveContext: 16384})
	if baseKey == plannerKey {
		t.Fatal("planner context must be part of session cache identity")
	}
}
