package contextasm

import (
	"sort"
	"strings"
)

// SegmentKind classifies a context segment by how stable it is across turns.
// Lower kinds are more stable and render first so backend prefix caches can
// reuse their KV; kinds at or after KindDiff are volatile and render last.
type SegmentKind int

const (
	KindSystem    SegmentKind = iota // system / developer instructions
	KindTools                        // tool schemas
	KindRepoRules                    // AGENTS.md / project conventions
	KindRepoMap                      // repo / symbol map
	KindPinned                       // pinned files
	KindDiff                         // current diff / working changes (volatile)
	KindTerminal                     // terminal / test output (volatile)
	KindUserTurn                     // the user's message (volatile)
)

const volatileFrom = KindDiff

// Segment is one deterministic, hashable piece of workspace context.
type Segment struct {
	Kind    SegmentKind
	Content string
}

// ManifestIdentity is the backend/profile/runtime identity attached to a
// manifest built from assembled context segments.
type ManifestIdentity struct {
	ProfileID            string
	Backend              string
	BackendVersion       string
	ModelDigest          string
	PromptFormat         string
	PromptTemplateDigest string
	RuntimeDigest        string
	AddBOS               bool
}

// AssembleContext renders segments into a single prompt whose stable prefix is
// byte-identical across turns when the stable segments are unchanged.
func AssembleContext(segs []Segment) (prompt string, stablePrefixHash string) {
	prompt, manifest := AssembleManifest(segs, ManifestIdentity{})
	return prompt, manifest.StableByteHash
}

// AssembleManifest renders segments canonically and returns a manifest over the
// rendered logical context. Backends that render through an opaque model-native
// chat template may still use this as cache identity, but must not claim exact
// template-token segment ranges until they can prove boundaries under that
// template.
func AssembleManifest(segs []Segment, id ManifestIdentity) (prompt string, manifest ContextManifest) {
	sorted := make([]Segment, len(segs))
	copy(sorted, segs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Kind < sorted[j].Kind })

	var b strings.Builder
	stableLen := 0
	manifestSegments := make([]ManifestSegment, 0, len(sorted))
	for _, s := range sorted {
		text := renderSegment(s)
		start := b.Len()
		b.WriteString(text)
		end := b.Len()
		stable := s.Kind < volatileFrom
		if stable {
			stableLen = end
		}
		manifestSegments = append(manifestSegments, ManifestSegment{
			Kind:      s.Kind.Tag(),
			Stable:    stable,
			ByteStart: start,
			ByteEnd:   end,
			ByteHash:  HashString(text),
		})
	}

	full := b.String()
	return full, ContextManifest{
		ProfileID:            id.ProfileID,
		Backend:              id.Backend,
		BackendVersion:       id.BackendVersion,
		ModelDigest:          id.ModelDigest,
		PromptFormat:         id.PromptFormat,
		PromptTemplateDigest: id.PromptTemplateDigest,
		RuntimeDigest:        id.RuntimeDigest,
		AddBOS:               id.AddBOS,
		StableBytes:          stableLen,
		TotalBytes:           len(full),
		StableByteHash:       HashString(full[:stableLen]),
		Segments:             manifestSegments,
	}
}

func renderSegment(s Segment) string {
	return "<|seg:" + s.Kind.Tag() + "|>\n" + s.Content + "\n"
}

// Tag returns the stable string form used in manifests.
func (k SegmentKind) Tag() string {
	switch k {
	case KindSystem:
		return "system"
	case KindTools:
		return "tools"
	case KindRepoRules:
		return "repo_rules"
	case KindRepoMap:
		return "repo_map"
	case KindPinned:
		return "pinned"
	case KindDiff:
		return "diff"
	case KindTerminal:
		return "terminal"
	case KindUserTurn:
		return "user"
	default:
		return "unknown"
	}
}
