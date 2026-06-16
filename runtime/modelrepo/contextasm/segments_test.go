package contextasm

import "testing"

func TestUnit_ContextasmAssembleManifest_StableHashIgnoresVolatileChange(t *testing.T) {
	id := ManifestIdentity{
		ProfileID:            "coder",
		Backend:              "test",
		ModelDigest:          "model",
		PromptFormat:         "template",
		PromptTemplateDigest: "template-digest",
		RuntimeDigest:        "runtime",
	}
	a := []Segment{
		{Kind: KindSystem, Content: "rules"},
		{Kind: KindUserTurn, Content: "turn a"},
	}
	b := []Segment{
		{Kind: KindUserTurn, Content: "turn b"},
		{Kind: KindSystem, Content: "rules"},
	}

	pa, ma := AssembleManifest(a, id)
	pb, mb := AssembleManifest(b, id)

	if pa == pb {
		t.Fatal("full logical prompt should include volatile change")
	}
	if ma.StableByteHash != mb.StableByteHash {
		t.Fatalf("stable hash changed for volatile-only edit: %s != %s", ma.StableByteHash, mb.StableByteHash)
	}
	if ma.ProfileID != id.ProfileID || ma.Backend != id.Backend || ma.RuntimeDigest != id.RuntimeDigest {
		t.Fatalf("identity was not copied into manifest: %+v", ma)
	}
	if len(ma.Segments) != 2 || !ma.Segments[0].Stable || ma.Segments[1].Stable {
		t.Fatalf("segments not classified stable/volatile: %+v", ma.Segments)
	}
}

func TestUnit_ContextasmManifest_CompatibilityIgnoresStableByteHash(t *testing.T) {
	base := ContextManifest{
		Backend:              "test",
		ModelDigest:          "model",
		PromptFormat:         "format",
		PromptTemplateDigest: "template",
		RuntimeDigest:        "runtime",
		StableByteHash:       "stable-a",
	}
	next := base
	next.StableByteHash = "stable-b"
	if ok, reason := base.CompatibleRuntime(next); !ok {
		t.Fatalf("stable hash change should not block runtime compatibility: %s", reason)
	}

	next = base
	next.ModelDigest = "other"
	if ok, reason := base.CompatibleRuntime(next); ok || reason == "" {
		t.Fatalf("model digest change should block compatibility, ok=%t reason=%q", ok, reason)
	}
}
