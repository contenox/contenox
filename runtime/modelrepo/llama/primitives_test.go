package llama

import (
	"errors"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
)

func TestUnit_LocalNodeErrors_AreTyped(t *testing.T) {
	err := NewContextOverflowError("suffix", 10, 4, 12)
	if !errors.Is(err, ErrContextOverflow) {
		t.Fatalf("context overflow should match ErrContextOverflow: %v", err)
	}
	var overflow *ContextOverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("expected ContextOverflowError, got %T", err)
	}
	if overflow.Stage != "suffix" || overflow.ResidentTokens != 10 || overflow.AdditionalTokens != 4 || overflow.NumCtx != 12 {
		t.Fatalf("unexpected overflow fields: %+v", overflow)
	}

	err = NewUnsupportedFeatureError("tool calls")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("unsupported feature should match ErrUnsupportedFeature: %v", err)
	}
	var unsupported *UnsupportedFeatureError
	if !errors.As(err, &unsupported) || unsupported.Feature != "tool calls" {
		t.Fatalf("unexpected unsupported feature error: %#v", err)
	}

	err = NewManifestMismatchError("profile_id changed")
	if !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("manifest mismatch should match ErrManifestMismatch: %v", err)
	}
	var mismatch *ManifestMismatchError
	if !errors.As(err, &mismatch) || mismatch.Reason != "profile_id changed" {
		t.Fatalf("unexpected manifest mismatch error: %#v", err)
	}

	if !fatalSessionError(ErrSessionFatal) || !fatalSessionError(ErrSessionClosed) {
		t.Fatal("fatalSessionError should catch fatal and closed session states")
	}
}

func TestUnit_LocalNodeNewSession_ReportsUnavailableWhenNoBackendRegistered(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })

	_, err := newSession(modeldconn.ModelRef{Name: "m", Type: "llama", Path: "/tmp/model.gguf"}, Config{})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("newSession error = %v, want ErrSessionUnavailable", err)
	}
}

func TestUnit_LocalNodeSessionCacheKey_IncludesAdapters(t *testing.T) {
	base := Config{NumCtx: 8192, NumBatch: 512}
	ref := modeldconn.ModelRef{Name: "a", Type: "llama", Digest: "digest-a", Path: "/models/a.gguf"}

	withA := ref
	withA.Adapters = []AdapterSpec{{Name: "A", Digest: "adapter-a", Scale: 1}}
	withB := ref
	withB.Adapters = []AdapterSpec{{Name: "B", Digest: "adapter-b", Scale: 1}}
	withAScale2 := ref
	withAScale2.Adapters = []AdapterSpec{{Name: "A", Digest: "adapter-a", Scale: 2}}
	withAB := ref
	withAB.Adapters = []AdapterSpec{{Digest: "adapter-a", Scale: 1}, {Digest: "adapter-b", Scale: 1}}
	withBA := ref
	withBA.Adapters = []AdapterSpec{{Digest: "adapter-b", Scale: 1}, {Digest: "adapter-a", Scale: 1}}

	// base, base+A, base+B, base+A@2.0, [A,B] and [B,A] must all be distinct: a
	// variant must never reuse another variant's (or the base's) warm KV, and
	// adapter order is part of identity.
	labelled := map[string]string{
		"base": sessionCacheKey(ref, base),
		"A":    sessionCacheKey(withA, base),
		"B":    sessionCacheKey(withB, base),
		"A@2":  sessionCacheKey(withAScale2, base),
		"AB":   sessionCacheKey(withAB, base),
		"BA":   sessionCacheKey(withBA, base),
	}
	seen := map[string]string{}
	for label, key := range labelled {
		if other, dup := seen[key]; dup {
			t.Fatalf("cache key collision: %q and %q share a key", label, other)
		}
		seen[key] = label
	}
	// Determinism: an independently-built identical adapter set yields the same key.
	dup := ref
	dup.Adapters = []AdapterSpec{{Name: "A", Digest: "adapter-a", Scale: 1}}
	if sessionCacheKey(withA, base) != sessionCacheKey(dup, base) {
		t.Fatal("same adapter set should produce the same cache key")
	}
}

func TestUnit_RuntimeDigest_IncludesAdapters(t *testing.T) {
	cfg := Config{NumCtx: 8192, NumBatch: 512}
	base := runtimeDigest(cfg, nil)
	a := runtimeDigest(cfg, []AdapterSpec{{Digest: "adapter-a", Scale: 1}})
	b := runtimeDigest(cfg, []AdapterSpec{{Digest: "adapter-b", Scale: 1}})
	aScale2 := runtimeDigest(cfg, []AdapterSpec{{Digest: "adapter-a", Scale: 2}})

	for _, c := range []struct {
		name string
		x, y string
	}{
		{"base vs +A", base, a},
		{"+A vs +B", a, b},
		{"+A scale", a, aScale2},
	} {
		if c.x == c.y {
			t.Fatalf("runtimeDigest must differ across adapter identity: %s", c.name)
		}
	}
	if a != runtimeDigest(cfg, []AdapterSpec{{Digest: "adapter-a", Scale: 1}}) {
		t.Fatal("runtimeDigest must be deterministic for the same adapter set")
	}
}

func TestUnit_LocalNodeSessionCacheKey_IncludesRuntimeIdentity(t *testing.T) {
	base := Config{
		NumCtx:       8192,
		NumBatch:     512,
		NumThreads:   8,
		NumGpuLayers: 35,
		TensorSplit:  []float32{0.25, 0.75},
		FlashAttn:    true,
		KVCacheType:  "q8_0",
	}
	refA := modeldconn.ModelRef{Name: "a", Type: "llama", Digest: "digest-a", Path: "/models/a.gguf"}
	same := base
	if sessionCacheKey(refA, base) != sessionCacheKey(refA, same) {
		t.Fatal("same config should produce same cache key")
	}

	cases := []Config{
		func() Config { c := base; c.NumCtx = 16384; return c }(),
		func() Config { c := base; c.NumBatch = 1024; return c }(),
		func() Config { c := base; c.NumThreads = 16; return c }(),
		func() Config { c := base; c.NumGpuLayers = 0; return c }(),
		func() Config { c := base; c.TensorSplit = []float32{1}; return c }(),
		func() Config { c := base; c.FlashAttn = false; return c }(),
		func() Config { c := base; c.KVCacheType = "q4_0"; return c }(),
		func() Config { c := base; c.PromptFormat = promptFormatLlama3; return c }(),
		func() Config { c := base; c.PromptTemplateDigest = "custom-template"; return c }(),
		func() Config { c := base; c.DisableBOS = true; return c }(),
	}
	seen := map[string]struct{}{sessionCacheKey(refA, base): {}}
	for _, cfg := range cases {
		key := sessionCacheKey(refA, cfg)
		if _, ok := seen[key]; ok {
			t.Fatalf("runtime config was not represented in cache key: %+v", cfg)
		}
		seen[key] = struct{}{}
	}
	refB := refA
	refB.Name = "b"
	if sessionCacheKey(refA, base) == sessionCacheKey(refB, base) {
		t.Fatal("model name should be part of cache key")
	}
	refAd := refA
	refAd.Digest = "digest-b"
	if sessionCacheKey(refA, base) == sessionCacheKey(refAd, base) {
		t.Fatal("model digest should be part of cache key")
	}
}

func TestUnit_LocalNodeDecodeConfig_PropagatesSeedAndClampsMaxTokens(t *testing.T) {
	cfg := &modelrepo.ChatConfig{}
	modelrepo.WithMaxTokens(2048).Apply(cfg)
	modelrepo.WithSeed(123).Apply(cfg)
	modelrepo.WithTopP(0.7).Apply(cfg)

	got := decodeConfig(cfg, 512)
	if got.MaxTokens != 512 {
		t.Fatalf("MaxTokens = %d, want clamp to 512", got.MaxTokens)
	}
	if got.Seed == nil || *got.Seed != 123 {
		t.Fatalf("Seed = %v, want 123", got.Seed)
	}
	if got.TopP == nil || *got.TopP != 0.7 {
		t.Fatalf("TopP = %v, want 0.7", got.TopP)
	}
}

func TestUnit_LocalNodeClient_RejectsToolsWithTypedError(t *testing.T) {
	c := &client{}
	tool := modelrepo.Tool{
		Type: "function",
		Function: &modelrepo.FunctionTool{
			Name:        "read_file",
			Description: "read a file",
		},
	}

	_, err := c.Chat(nil, nil, modelrepo.WithTool(tool))
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("Chat tool error = %v, want ErrUnsupportedFeature", err)
	}
	if !strings.Contains(err.Error(), "tool calls") {
		t.Fatalf("error should name tool calls, got: %v", err)
	}

	_, err = c.Stream(nil, nil, modelrepo.WithTool(tool))
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("Stream tool error = %v, want ErrUnsupportedFeature", err)
	}
}

func TestUnit_LocalNodePromptPlan_BuildsManifestAndStablePrefix(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "system rules"},
		{Role: "user", Content: "do work"},
	}, Config{NumCtx: 4096}, promptIdentity{
		ProfileID:      "coder",
		ModelDigest:    "sha256:model",
		BackendVersion: "v0.17.5",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Stable.Text != "system rules" {
		t.Fatalf("stable prefix = %q, want raw %q", plan.Stable.Text, "system rules")
	}
	if strings.Contains(plan.Stable.Text, "do work") {
		t.Fatalf("user turn leaked into stable prefix: %q", plan.Stable.Text)
	}
	if plan.Volatile.Text != "do work" {
		t.Fatalf("volatile suffix = %q, want raw %q", plan.Volatile.Text, "do work")
	}
	if plan.Stable.Manifest.ProfileID != "coder" ||
		plan.Stable.Manifest.ModelDigest != "sha256:model" ||
		plan.Stable.Manifest.PromptFormat != promptFormatChatML ||
		plan.Stable.Manifest.StableBytes != len(plan.Stable.Text) ||
		plan.Stable.Manifest.TotalBytes != len(plan.Stable.Text)+len(plan.Volatile.Text) ||
		plan.Stable.Manifest.StableByteHash == "" ||
		plan.Stable.Manifest.RuntimeDigest == "" {
		t.Fatalf("manifest not populated: %+v", plan.Stable.Manifest)
	}
	if plan.Stable.Manifest.Digest() == "" {
		t.Fatal("manifest digest should be populated")
	}
}

func TestUnit_LlamaPromptPlan_UserOnlyHasEmptyStablePrefix(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "user", Content: "hello"},
	}, Config{}, promptIdentity{ProfileID: "coder"}, "")
	if err != nil {
		t.Fatal(err)
	}
	// No system turn => empty stable prefix; the user turn is volatile. BOS is no
	// longer a synthetic manifest segment; it is added by the model tokenizer in
	// modeld, proven by the end-to-end test.
	if plan.Stable.Text != "" {
		t.Fatalf("stable text = %q, want empty user-only stable prefix", plan.Stable.Text)
	}
	if plan.Volatile.Text != "hello" {
		t.Fatalf("volatile text = %q, want raw %q", plan.Volatile.Text, "hello")
	}
}

func TestUnit_LlamaPromptPlan_MergesStackedSystemInstructions(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "main rules"},
		{Role: "system", Content: "recovery rules"},
		{Role: "user", Content: "what happened?"},
	}, Config{}, promptIdentity{ProfileID: "coder"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Stable.Text != "main rules\n\nrecovery rules" {
		t.Fatalf("stable text = %q, want merged system instructions", plan.Stable.Text)
	}
	if plan.Volatile.Text != "what happened?" {
		t.Fatalf("volatile text = %q, want user turn", plan.Volatile.Text)
	}
	var roles []string
	for _, seg := range plan.Stable.Manifest.Segments {
		roles = append(roles, seg.Kind)
	}
	if strings.Join(roles, ",") != "system,user" {
		t.Fatalf("roles = %v, want one system followed by user", roles)
	}
}

func TestUnit_LocalNodeManifest_FillsStableAndVolatileTokenRanges(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "hello"},
	}, Config{DisableBOS: true}, promptIdentity{ProfileID: "coder"}, "")
	if err != nil {
		t.Fatal(err)
	}
	tokenize := byteTokenizer
	stableTokens, err := tokenize(plan.Stable.Text, false)
	if err != nil {
		t.Fatal(err)
	}
	stableManifest, err := plan.Stable.Manifest.WithStableTokenization(plan.Stable.Text, stableTokens, tokenize, false)
	if err != nil {
		t.Fatal(err)
	}
	if stableManifest.StableTokenHash == "" {
		t.Fatal("stable token hash should be populated")
	}
	stableCount := 0
	for _, seg := range stableManifest.Segments {
		if seg.Stable {
			stableCount++
			if seg.TokenStart != 0 || seg.TokenEnd != len(stableTokens) || seg.TokenHash == "" {
				t.Fatalf("stable segment token range not populated: %+v", seg)
			}
		}
	}
	if stableCount != 1 {
		t.Fatalf("stable segments = %d, want 1", stableCount)
	}

	volatileTokens, err := tokenize(plan.Volatile.Text, false)
	if err != nil {
		t.Fatal(err)
	}
	fullManifest, err := plan.Volatile.Manifest.WithVolatileTokenization(stableManifest, len(stableTokens), plan.Volatile.Text, volatileTokens, tokenize)
	if err != nil {
		t.Fatal(err)
	}
	if fullManifest.VolatileTokenHash == "" {
		t.Fatal("volatile token hash should be populated")
	}
	for _, seg := range fullManifest.Segments {
		if seg.TokenHash == "" {
			t.Fatalf("segment token hash missing: %+v", seg)
		}
		if seg.TokenEnd < seg.TokenStart {
			t.Fatalf("segment token range is invalid: %+v", seg)
		}
		if !seg.Stable && seg.TokenStart < len(stableTokens) {
			t.Fatalf("volatile segment token range should be offset by prefix tokens: %+v", seg)
		}
	}
}

func TestUnit_LlamaManifest_RejectsSplitTokenizationThatDiffersFromColdFullPrompt(t *testing.T) {
	tokenize := func(text string, addSpecial bool) ([]int, error) {
		switch text {
		case "a":
			return []int{1}, nil
		case "b":
			return []int{2}, nil
		case "ab":
			return []int{3}, nil
		default:
			return byteTokenizer(text, addSpecial)
		}
	}
	err := (ContextManifest{}).ValidateSplitTokenization("a", "b", []int{1}, []int{2}, tokenize, false)
	if !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("expected split tokenization mismatch, got %v", err)
	}
}

func TestUnit_LocalNodeManifest_RejectsUnalignedTokenBoundary(t *testing.T) {
	m := ContextManifest{
		Backend:        backendName,
		PromptFormat:   promptFormatChatML,
		RuntimeDigest:  "runtime",
		StableBytes:    2,
		TotalBytes:     2,
		StableByteHash: hashString("ab"),
		Segments: []ManifestSegment{
			{Kind: "a", Stable: true, ByteStart: 0, ByteEnd: 1, ByteHash: hashString("a")},
			{Kind: "b", Stable: true, ByteStart: 1, ByteEnd: 2, ByteHash: hashString("b")},
		},
	}
	mergeTokenizer := func(text string, addSpecial bool) ([]int, error) {
		switch text {
		case "a":
			return []int{1}, nil
		case "ab":
			return []int{2}, nil
		default:
			return byteTokenizer(text, addSpecial)
		}
	}
	_, err := m.WithStableTokenization("ab", []int{2}, mergeTokenizer, false)
	if !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("expected boundary mismatch, got %v", err)
	}
}

func TestUnit_LocalNodeManifest_RejectsVolatileTokenizationWithoutStableRanges(t *testing.T) {
	stableText := "s"
	suffixText := "v"
	m := ContextManifest{
		Backend:        backendName,
		PromptFormat:   promptFormatChatML,
		RuntimeDigest:  "runtime",
		StableBytes:    len(stableText),
		TotalBytes:     len(stableText) + len(suffixText),
		StableByteHash: hashString(stableText),
		Segments: []ManifestSegment{
			{Kind: "system", Stable: true, ByteStart: 0, ByteEnd: len(stableText), ByteHash: hashString(stableText)},
			{Kind: "user", Stable: false, ByteStart: len(stableText), ByteEnd: len(stableText) + len(suffixText), ByteHash: hashString(suffixText)},
		},
	}
	suffixTokens, err := byteTokenizer(suffixText, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.WithVolatileTokenization(m, 1, suffixText, suffixTokens, byteTokenizer)
	if !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("expected missing stable token range mismatch, got %v", err)
	}
}

func TestUnit_LocalNodePromptPlan_SendsRawContentForModelTemplate(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "hello"},
	}, Config{PromptFormat: promptFormatLlama3}, promptIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The runtime sends RAW content keyed by role; modeld applies the model's own
	// chat template (from the GGUF), so no format tokens appear in the plan here.
	if plan.Stable.Text != "rules" {
		t.Fatalf("stable (raw) = %q, want %q", plan.Stable.Text, "rules")
	}
	if plan.Volatile.Text != "hello" {
		t.Fatalf("volatile (raw) = %q, want %q", plan.Volatile.Text, "hello")
	}
	// Roles ride in the manifest segments for modeld to reconstruct the turns.
	roles := map[string]bool{}
	for _, seg := range plan.Stable.Manifest.Segments {
		roles[seg.Kind] = true
	}
	if !roles[segmentSystem] || !roles[segmentUser] {
		t.Fatalf("segments missing system/user roles: %+v", plan.Stable.Manifest.Segments)
	}
}

func TestUnit_LocalNodePromptPlan_RejectsUnsupportedPrompt(t *testing.T) {
	_, err := buildPromptPlan(nil, Config{PromptFormat: "unknown"}, promptIdentity{}, "")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("unsupported prompt format error = %v, want ErrUnsupportedFeature", err)
	}
}

func TestUnit_LocalNodePromptPlan_PropagatesToolHistory(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "assistant", Content: "thinking", ToolCalls: []modelrepo.ToolCall{{ID: "call_123", Type: "function"}}},
		{Role: "tool", Content: "result", ToolCallID: "call_123"},
	}, Config{}, promptIdentity{}, `[{"type":"function"}]`)
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

func TestUnit_LocalNodePromptPlan_TextOnlyCallsFlattenToolHistoryAndAlternateTurns(t *testing.T) {
	plan, err := buildPromptPlan([]modelrepo.Message{
		{Role: "system", Content: "main rules"},
		{Role: "system", Content: "summary rules"},
		{Role: "user", Content: "start"},
		{Role: "user", Content: "extra user detail"},
		{Role: "assistant", Content: "calling", ToolCalls: []modelrepo.ToolCall{{ID: "call_123", Type: "function"}}},
		{Role: "tool", Content: "result", ToolCallID: "call_123"},
		{Role: "assistant", Content: "final note"},
	}, Config{}, promptIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}

	if plan.Stable.Text != "main rules\n\nsummary rules" {
		t.Fatalf("stable text = %q, want merged systems", plan.Stable.Text)
	}
	if strings.Contains(plan.Volatile.Text, "tool_calls") {
		t.Fatalf("text-only volatile prompt leaked OpenAI tool_calls JSON field: %q", plan.Volatile.Text)
	}
	wantParts := []string{"start", "extra user detail", "Assistant requested tool calls", "Tool result for call_123", "final note"}
	for _, want := range wantParts {
		if !strings.Contains(plan.Volatile.Text, want) {
			t.Fatalf("volatile text %q missing %q", plan.Volatile.Text, want)
		}
	}
	var roles []string
	for _, seg := range plan.Stable.Manifest.Segments {
		roles = append(roles, seg.Kind)
	}
	if strings.Join(roles, ",") != "system,user,assistant,user,assistant" {
		t.Fatalf("roles = %v, want strict alternating text turns", roles)
	}
	for _, seg := range plan.Stable.Manifest.Segments {
		if seg.ToolCallsJSON != "" || seg.ToolCallID != "" {
			t.Fatalf("text-only segment retained tool metadata: %+v", seg)
		}
	}
}

func TestUnit_LocalNodeManifest_CompatibilityIgnoresStableByteChangeOnly(t *testing.T) {
	base := ContextManifest{
		ProfileID:            "coder",
		Backend:              backendName,
		BackendVersion:       "v1",
		ModelDigest:          "m1",
		PromptFormat:         promptFormatChatML,
		PromptTemplateDigest: "t1",
		RuntimeDigest:        "r1",
		AddBOS:               true,
		StableByteHash:       "s1",
	}
	next := base
	next.StableByteHash = "s2"
	if ok, reason := base.CompatibleRuntime(next); !ok {
		t.Fatalf("stable byte changes should still allow token LCP reuse, reason=%q", reason)
	}

	next = base
	next.PromptTemplateDigest = "t2"
	if ok, reason := base.CompatibleRuntime(next); ok || !strings.Contains(reason, "prompt_template_digest") {
		t.Fatalf("template changes should be incompatible, ok=%t reason=%q", ok, reason)
	}
}

func byteTokenizer(text string, addSpecial bool) ([]int, error) {
	out := make([]int, 0, len(text)+1)
	if addSpecial {
		out = append(out, 1)
	}
	for _, b := range []byte(text) {
		out = append(out, int(b)+100)
	}
	return out, nil
}
