package openvino

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnit_OpenVINOAssembleContext_OrderIndependentAndDeterministic(t *testing.T) {
	a := []Segment{
		{Kind: KindUserTurn, Content: "fix the bug"},
		{Kind: KindSystem, Content: "you are a coding agent"},
		{Kind: KindPinned, Content: "package main"},
		{Kind: KindTools, Content: "tool: grep"},
	}
	// Same segments, shuffled input order.
	b := []Segment{
		{Kind: KindTools, Content: "tool: grep"},
		{Kind: KindPinned, Content: "package main"},
		{Kind: KindSystem, Content: "you are a coding agent"},
		{Kind: KindUserTurn, Content: "fix the bug"},
	}

	pa, ha := AssembleContext(a)
	pb, hb := AssembleContext(b)

	require.Equal(t, pa, pb, "input order must not change the rendered prompt")
	require.Equal(t, ha, hb, "input order must not change the stable-prefix hash")
}

func TestUnit_OpenVINOAssembleContext_StableBeforeVolatile(t *testing.T) {
	prompt, _ := AssembleContext([]Segment{
		{Kind: KindUserTurn, Content: "Q"},
		{Kind: KindSystem, Content: "S"},
	})
	sysIdx := strings.Index(prompt, "<|seg:system|>")
	userIdx := strings.Index(prompt, "<|seg:user|>")
	require.GreaterOrEqual(t, sysIdx, 0)
	require.GreaterOrEqual(t, userIdx, 0)
	require.Less(t, sysIdx, userIdx, "stable system segment must render before the volatile user segment")
}

func TestUnit_OpenVINOAssembleContext_VolatileChangeKeepsStablePrefix(t *testing.T) {
	stable := []Segment{
		{Kind: KindSystem, Content: "you are a coding agent"},
		{Kind: KindRepoMap, Content: "pkg/a.go pkg/b.go"},
	}
	turn1 := append(append([]Segment{}, stable...), Segment{Kind: KindUserTurn, Content: "question one"})
	turn2 := append(append([]Segment{}, stable...), Segment{Kind: KindUserTurn, Content: "a completely different question two"})

	p1, h1 := AssembleContext(turn1)
	p2, h2 := AssembleContext(turn2)

	require.Equal(t, h1, h2, "changing only a volatile segment must keep the stable-prefix hash (cache should hit)")
	require.NotEqual(t, p1, p2, "the full prompt should still differ")
	require.True(t, strings.HasPrefix(p1, p2[:strings.Index(p2, "<|seg:user|>")]),
		"the byte-identical stable prefix is what the cache reuses")
}

func TestUnit_OpenVINOAssembleContext_StableChangeChangesPrefixHash(t *testing.T) {
	base := []Segment{
		{Kind: KindSystem, Content: "you are a coding agent"},
		{Kind: KindPinned, Content: "package main // v1"},
		{Kind: KindUserTurn, Content: "same question"},
	}
	changed := []Segment{
		{Kind: KindSystem, Content: "you are a coding agent"},
		{Kind: KindPinned, Content: "package main // v2 EDITED"},
		{Kind: KindUserTurn, Content: "same question"},
	}

	_, hBase := AssembleContext(base)
	_, hChanged := AssembleContext(changed)

	require.NotEqual(t, hBase, hChanged, "editing a stable segment must change the stable-prefix hash (cache miss for the tail)")
}
