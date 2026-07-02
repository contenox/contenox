package llama

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnit_LocalNodeProfile_DefaultContextFeedsCapabilities(t *testing.T) {
	p := modelProfile{}

	cfg := p.config()
	caps := p.capabilityConfig()

	// An omitted context is "auto", not a placeholder: forcing a concrete
	// default here would ride to the real OpenSession as a hard Request
	// ceiling and permanently override modeld's live capacity computation.
	if cfg.NumCtx != 0 {
		t.Fatalf("default NumCtx = %d, want 0 (auto — modeld resolves the window live)", cfg.NumCtx)
	}
	if caps.ContextLength != 0 {
		t.Fatalf("default ContextLength = %d, want 0 (unknown until modeld describes it)", caps.ContextLength)
	}
}

func TestUnit_LocalNodeProfile_RuntimeContextFeedsCapabilities(t *testing.T) {
	p := modelProfile{Runtime: runtimeProfile{NumCtx: 16384}}

	caps := p.capabilityConfig()

	if caps.ContextLength != 16384 {
		t.Fatalf("ContextLength = %d, want runtime num_ctx", caps.ContextLength)
	}
}

func TestUnit_LocalNodeProfile_ReasoningProtocolFeedsCanThink(t *testing.T) {
	p := modelProfile{Reasoning: reasoningProfile{Protocol: reasoningProtocolCommonChat, Format: "deepseek"}}

	caps := p.capabilityConfig()

	if !caps.CanThink {
		t.Fatal("reasoning protocol should advertise CanThink")
	}
}

func TestUnit_LocalNodeProfile_PromptSettingsFeedConfig(t *testing.T) {
	addBOS := false
	p := modelProfile{
		Prompt: promptProfile{
			Format:         promptFormatLlama3,
			TemplateDigest: "template-digest",
			AddBOS:         &addBOS,
		},
	}

	cfg := p.config()

	if cfg.PromptFormat != promptFormatLlama3 {
		t.Fatalf("PromptFormat = %q, want llama3", cfg.PromptFormat)
	}
	if cfg.PromptTemplateDigest != "template-digest" {
		t.Fatalf("PromptTemplateDigest = %q", cfg.PromptTemplateDigest)
	}
	if !cfg.DisableBOS {
		t.Fatal("DisableBOS should reflect prompt.add_bos=false")
	}
}

func TestUnit_LocalNodeProfile_ExplicitContextWinsOverRuntime(t *testing.T) {
	p := modelProfile{
		ContextLength: 32768,
		Runtime:       runtimeProfile{NumCtx: 16384},
	}

	caps := p.capabilityConfig()

	if caps.ContextLength != 32768 {
		t.Fatalf("ContextLength = %d, want explicit context_length", caps.ContextLength)
	}
}

func TestUnit_LocalNodeProfile_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{"runtime":{"num_ctx":4096},"typo":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadModelProfile(dir); err == nil {
		t.Fatal("expected unknown profile fields to be rejected")
	}
}

func TestUnit_LocalNodeProfile_RejectsUnknownNestedPromptFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{"prompt":{"format":"llama3","typo":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadModelProfile(dir); err == nil {
		t.Fatal("expected unknown nested prompt fields to be rejected")
	}
}

func TestUnit_LocalNodeProfile_RejectsUnsupportedPromptFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{"prompt":{"format":"made_up"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadModelProfile(dir); !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("expected unsupported prompt format error, got %v", err)
	}
}

func TestUnit_LocalNodeProfile_RejectsUnsupportedReasoningProtocol(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{"reasoning":{"protocol":"llama:nope"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadModelProfile(dir); !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("expected unsupported reasoning protocol error, got %v", err)
	}
}

func TestUnit_LocalNodeProfile_RejectsReasoningProtocolWithoutFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{"reasoning":{"protocol":"llama:common_chat_reasoning_parser"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadModelProfile(dir); !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("expected unsupported reasoning profile error, got %v", err)
	}
}

func TestUnit_LocalNodeProfile_AcceptsCommonChatToolProtocol(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`{"tool_calls":{"protocol":"` + toolParserProtocolCommonChat + `"}}`)
	if err := os.WriteFile(filepath.Join(dir, profileFileName), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModelProfile(dir); err != nil {
		t.Fatalf("common chat tool protocol should be accepted: %v", err)
	}
}

func TestUnit_LocalNodeProfile_RejectsLegacyToolProtocolAliases(t *testing.T) {
	for _, protocol := range []string{"qwen", "hermes"} {
		dir := t.TempDir()
		body := []byte(`{"tool_calls":{"protocol":"` + protocol + `"}}`)
		if err := os.WriteFile(filepath.Join(dir, profileFileName), body, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadModelProfile(dir); err == nil {
			t.Fatalf("legacy protocol alias %q should be rejected", protocol)
		}
	}
}
