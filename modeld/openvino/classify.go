package openvino

import (
	"fmt"
	"strings"

	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/contextasm"
)

type classifiedChatContext struct {
	Messages         []modeld.Message
	Segments         []Segment
	Manifest         contextasm.ContextManifest
	StablePrefixHash string
}

func classifyChatContext(messages []modeld.Message, toolsJSON string) classifiedChatContext {
	return classifyChatContextWithIdentity(messages, toolsJSON, contextasm.ManifestIdentity{})
}

func classifyChatContextWithIdentity(messages []modeld.Message, toolsJSON string, identity contextasm.ManifestIdentity) classifiedChatContext {
	ordered := make([]modeld.Message, 0, len(messages))
	segments := make([]Segment, 0, len(messages)+1)
	seenVolatile := false

	for _, m := range messages {
		if !seenVolatile && isSystemRole(m.Role) {
			ordered = append(ordered, m)
			segments = append(segments, Segment{Kind: KindSystem, Content: messageSegmentContent(m)})
			continue
		}
		seenVolatile = true
		ordered = append(ordered, m)
		segments = append(segments, Segment{Kind: volatileSegmentKind(m.Role), Content: messageSegmentContent(m)})
	}

	if strings.TrimSpace(toolsJSON) != "" {
		segments = append(segments, Segment{Kind: KindTools, Content: toolsJSON})
	}
	_, manifest := contextasm.AssembleManifest(segments, identity)
	return classifiedChatContext{
		Messages:         ordered,
		Segments:         segments,
		Manifest:         manifest,
		StablePrefixHash: manifest.StableByteHash,
	}
}

func isSystemRole(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "system")
}

func volatileSegmentKind(role string) SegmentKind {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "tool":
		return KindTerminal
	default:
		return KindUserTurn
	}
}

func messageSegmentContent(m modeld.Message) string {
	return fmt.Sprintf("role=%s\ncontent=%s\n", strings.TrimSpace(m.Role), m.Content)
}
