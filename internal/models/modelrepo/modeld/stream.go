package modeld

import (
	"context"
	"fmt"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/transport"
	"github.com/contenox/contenox/libtracker"
)

// ModeldStreamClient forwards native decode events and accounting.
type ModeldStreamClient struct {
	modelName       string
	backendID       string
	maxOutputTokens int
	supportsThink   bool
	tracker         libtracker.ActivityTracker
}

func (c *ModeldStreamClient) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	_, _, end := c.tracker.Start(ctx, "stream", "modeld", "model", c.modelName)

	if err := modelrepo.RefuseAudioInput("modeld", c.modelName, messages); err != nil {
		end()
		return nil, err
	}

	config := &modelrepo.ChatConfig{}
	for _, arg := range args {
		arg.Apply(config)
	}

	target := resolveTarget(c.backendID)
	tConfig := transport.Config{}

	sess, err := openSessionWithAutoPull(ctx, target, c.modelName, tConfig, c.tracker)
	if err != nil {
		end()
		return nil, fmt.Errorf("modeld open session for %s: %w", c.modelName, err)
	}

	prefixInput, suffixInput, err := splitMessagesToInputs(messages, config)
	if err != nil {
		sess.Close()
		end()
		return nil, err
	}

	prefixStatus, err := sess.EnsurePrefix(ctx, prefixInput)
	if err != nil {
		sess.Close()
		end()
		return nil, fmt.Errorf("modeld ensure prefix: %w", err)
	}

	suffixStatus, err := sess.PrefillSuffix(ctx, suffixInput)
	if err != nil {
		sess.Close()
		end()
		return nil, fmt.Errorf("modeld prefill suffix: %w", err)
	}

	decodeCfg := transport.DecodeConfig{
		MaxTokens:   c.maxOutputTokens,
		Temperature: config.Temperature,
		TopP:        config.TopP,
		Seed:        config.Seed,
	}
	if config.MaxTokens != nil && *config.MaxTokens > 0 {
		decodeCfg.MaxTokens = *config.MaxTokens
		if c.maxOutputTokens > 0 && decodeCfg.MaxTokens > c.maxOutputTokens {
			decodeCfg.MaxTokens = c.maxOutputTokens
		}
	}

	chunks, err := sess.Decode(ctx, decodeCfg)
	if err != nil {
		sess.Close()
		end()
		return nil, fmt.Errorf("modeld decode: %w", err)
	}

	out := make(chan *modelrepo.StreamParcel)
	go func() {
		defer close(out)
		defer sess.Close()
		defer end()

		usage := modelrepo.TokenUsage{PromptTokens: prefixStatus.PrefixTokens + suffixStatus.SuffixTokens, CacheReadTokens: prefixStatus.ReusedTokens}
		usage.TotalTokens = usage.PromptTokens
		send := func(p *modelrepo.StreamParcel) bool {
			select {
			case out <- p:
				return true
			case <-ctx.Done():
				return false
			}
		}
		initialUsage := usage
		if !send(&modelrepo.StreamParcel{Usage: &initialUsage}) {
			return
		}
		toolIndex := 0
		finish := ""
		for {
			var chunk transport.StreamChunk
			var ok bool
			select {
			case <-ctx.Done():
				return
			case chunk, ok = <-chunks:
			}
			if !ok {
				break
			}
			if chunk.Usage != nil {
				usage.Merge(&modelrepo.TokenUsage{PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens, ThinkingTokens: chunk.Usage.ThinkingTokens, CacheReadTokens: chunk.Usage.CacheReadTokens})
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
				copyUsage := usage
				if !send(&modelrepo.StreamParcel{Usage: &copyUsage}) {
					return
				}
			}
			if chunk.Error != nil {
				send(&modelrepo.StreamParcel{Error: chunk.Error})
				return
			}
			if chunk.FinishReason != "" {
				finish = chunk.FinishReason
			}
			if chunk.Text != "" && !send(&modelrepo.StreamParcel{Data: chunk.Text}) {
				return
			}
			if chunk.Thinking != "" && !send(&modelrepo.StreamParcel{Thinking: chunk.Thinking}) {
				return
			}
			for _, tc := range chunk.ToolCalls {
				if !send(&modelrepo.StreamParcel{ToolCall: &modelrepo.ToolCallDelta{Index: toolIndex, ID: tc.ID, Type: tc.Type, Name: tc.Function.Name, ArgsFragment: tc.Function.Arguments}}) {
					return
				}
				toolIndex++
			}
		}
		if toolIndex > 0 && (finish == "" || finish == "stop") {
			finish = "tool_calls"
		}
		send(&modelrepo.StreamParcel{Terminal: &modelrepo.StreamTerminal{FinishReason: finish, Usage: &usage}})
	}()
	return out, nil
}

var _ modelrepo.LLMStreamClient = (*ModeldStreamClient)(nil)
