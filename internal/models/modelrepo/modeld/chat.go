package modeld

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/contenox/internal/modeld/contextasm"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/modelrepo/modeldconn"
	"github.com/contenox/contenox/internal/transport"
	"github.com/contenox/contenox/libtracker"
)

// ModeldChatClient assembles responses from the native session transport.
type ModeldChatClient struct {
	modelName       string
	backendID       string
	maxOutputTokens int
	supportsThink   bool
	tracker         libtracker.ActivityTracker
}

// Chat returns the assembled native stream, including usage reported before an error.
func (c *ModeldChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	client := &ModeldStreamClient{modelName: c.modelName, backendID: c.backendID, maxOutputTokens: c.maxOutputTokens, supportsThink: c.supportsThink, tracker: c.tracker}
	parcels, err := client.Stream(ctx, messages, args...)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	assembler := modelrepo.NewStreamAssembler("modeld", c.modelName)
	var usage modelrepo.TokenUsage
	sawUsage := false
	for p := range parcels {
		if p.Usage != nil {
			usage.Merge(p.Usage)
			sawUsage = true
		}
		if p.Terminal != nil && p.Terminal.Usage != nil {
			usage.Merge(p.Terminal.Usage)
			sawUsage = true
		}
		_ = assembler.Consume(p)
	}
	result, err := assembler.Result()
	if err != nil && sawUsage {
		result.Usage = &usage
	}
	return modelrepo.ChatResult{Message: modelrepo.Message{Role: "assistant", Content: result.Content, Thinking: result.Thinking}, ToolCalls: result.ToolCalls, Usage: result.Usage, FinishReason: result.FinishReason}, err
}

func resolveTarget(backendID string) modeldconn.ModeldTarget {
	b := strings.TrimSpace(backendID)
	if b == "" || b == "local" || b == modeldconn.LocalSentinel {
		return modeldconn.ModeldTarget{}
	}
	return modeldconn.ModeldTarget{
		BackendID: b,
		Endpoint:  b,
	}
}

func splitMessagesToInputs(messages []modelrepo.Message, config *modelrepo.ChatConfig) (transport.PrefixInput, transport.SuffixInput, error) {
	var stable, volatile strings.Builder
	var segments []contextasm.ManifestSegment
	var images []transport.ImagePart
	prefixDone := false
	for i, msg := range messages {
		stableHistory := config.CacheHints != nil && i < config.CacheHints.StableHistoryLen
		isStable := !prefixDone && (msg.Role == "system" || stableHistory) && len(msg.Images) == 0
		if !isStable {
			prefixDone = true
		}
		text := msg.Content
		for _, img := range msg.Images {
			text += transport.MediaMarker
			images = append(images, transport.ImagePart{Data: img.Data, MimeType: img.MimeType})
		}
		begin := stable.Len() + volatile.Len()
		if isStable {
			stable.WriteString(text)
		} else {
			volatile.WriteString(text)
		}
		seg := contextasm.ManifestSegment{Kind: msg.Role, Stable: isStable, ByteStart: begin, ByteEnd: begin + len(text), ToolCallID: msg.ToolCallID}
		if len(msg.ToolCalls) > 0 {
			b, err := json.Marshal(msg.ToolCalls)
			if err != nil {
				return transport.PrefixInput{}, transport.SuffixInput{}, err
			}
			seg.ToolCallsJSON = string(b)
		}
		segments = append(segments, seg)
	}
	manifest, err := contextasm.BuildSplitManifest(stable.String(), volatile.String(), segments, contextasm.ManifestIdentity{})
	if err != nil {
		return transport.PrefixInput{}, transport.SuffixInput{}, err
	}
	prefix := transport.PrefixInput{Text: stable.String(), Manifest: manifest}
	suffix := transport.SuffixInput{Text: volatile.String(), Manifest: manifest, Images: images}
	if len(config.Tools) > 0 {
		b, err := json.Marshal(config.Tools)
		if err != nil {
			return prefix, suffix, fmt.Errorf("marshal tools: %w", err)
		}
		prefix.Tools = string(b)
	}
	if config.Think != nil && *config.Think != "" && *config.Think != "auto" {
		enabled := *config.Think != "off"
		suffix.EnableThinking = &enabled
		if enabled {
			suffix.ReasoningEffort = *config.Think
		}
	}
	return prefix, suffix, nil
}

var _ modelrepo.LLMChatClient = (*ModeldChatClient)(nil)
