package llama

import (
	"errors"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
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

	_, err := newSession("/tmp/model.gguf", Config{})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("newSession error = %v, want ErrSessionUnavailable", err)
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
	same := base
	if sessionCacheKey("/models/a.gguf", "digest-a", base) != sessionCacheKey("/models/a.gguf", "digest-a", same) {
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
	seen := map[string]struct{}{sessionCacheKey("/models/a.gguf", "digest-a", base): {}}
	for _, cfg := range cases {
		key := sessionCacheKey("/models/a.gguf", "digest-a", cfg)
		if _, ok := seen[key]; ok {
			t.Fatalf("runtime config was not represented in cache key: %+v", cfg)
		}
		seen[key] = struct{}{}
	}
	if sessionCacheKey("/models/a.gguf", "digest-a", base) == sessionCacheKey("/models/b.gguf", "digest-a", base) {
		t.Fatal("model path should be part of cache key")
	}
	if sessionCacheKey("/models/a.gguf", "digest-a", base) == sessionCacheKey("/models/a.gguf", "digest-b", base) {
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
	// longer a synthetic manifest segment — it is added by the model tokenizer in
	// modeld (llamaabi.AddBOS), proven by the end-to-end test.
	if plan.Stable.Text != "" {
		t.Fatalf("stable text = %q, want empty user-only stable prefix", plan.Stable.Text)
	}
	if plan.Volatile.Text != "hello" {
		t.Fatalf("volatile text = %q, want raw %q", plan.Volatile.Text, "hello")
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

func TestUnit_LocalNodePromptPlan_RejectsUnsupportedPromptAndToolHistory(t *testing.T) {
	_, err := buildPromptPlan(nil, Config{PromptFormat: "unknown"}, promptIdentity{}, "")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("unsupported prompt format error = %v, want ErrUnsupportedFeature", err)
	}

	_, err = buildPromptPlan([]modelrepo.Message{{Role: "tool", Content: "{}"}}, Config{}, promptIdentity{}, "")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("tool history error = %v, want ErrUnsupportedFeature", err)
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
