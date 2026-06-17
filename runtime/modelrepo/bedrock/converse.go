package bedrock

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// buildConverseInput maps neutral messages + config into a Converse request.
// Adjacent same-role messages are merged (Bedrock requires alternating roles).
func buildConverseInput(modelName string, messages []modelrepo.Message, cfg *modelrepo.ChatConfig, maxOutputTokens int) *bedrockruntime.ConverseInput {
	in := &bedrockruntime.ConverseInput{ModelId: aws.String(modelName)}

	var system []types.SystemContentBlock
	var msgs []types.Message

	appendBlocks := func(role types.ConversationRole, blocks []types.ContentBlock) {
		if len(blocks) == 0 {
			return
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Content = append(msgs[n-1].Content, blocks...)
			return
		}
		msgs = append(msgs, types.Message{Role: role, Content: blocks})
	}

	for _, m := range messages {
		switch m.Role {
		case "system":
			if m.Content != "" {
				system = append(system, &types.SystemContentBlockMemberText{Value: m.Content})
			}
		case "tool":
			appendBlocks(types.ConversationRoleUser, []types.ContentBlock{
				&types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
					ToolUseId: aws.String(m.ToolCallID),
					Content:   []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{Value: m.Content}},
				}},
			})
		case "assistant", "model":
			var blocks []types.ContentBlock
			if m.Content != "" {
				blocks = append(blocks, &types.ContentBlockMemberText{Value: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
					ToolUseId: aws.String(tc.ID),
					Name:      aws.String(tc.Function.Name),
					Input:     jsonStringToDocument(tc.Function.Arguments),
				}})
			}
			appendBlocks(types.ConversationRoleAssistant, blocks)
		default: // "user"
			if m.Content != "" {
				appendBlocks(types.ConversationRoleUser, []types.ContentBlock{&types.ContentBlockMemberText{Value: m.Content}})
			}
		}
	}

	in.Messages = msgs
	if len(system) > 0 {
		in.System = system
	}

	if cfg != nil {
		ic := &types.InferenceConfiguration{}
		set := false
		if cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
			effective, _ := modelrepo.ClampMaxOutputTokens(*cfg.MaxTokens, maxOutputTokens)
			const maxInt32 = int64(1<<31 - 1)
			if int64(effective) > maxInt32 {
				effective = int(maxInt32)
			}
			v := int32(effective)
			ic.MaxTokens = &v
			set = true
		}
		if cfg.Temperature != nil {
			v := float32(*cfg.Temperature)
			ic.Temperature = &v
			set = true
		}
		if cfg.TopP != nil {
			v := float32(*cfg.TopP)
			ic.TopP = &v
			set = true
		}
		if set {
			in.InferenceConfig = ic
		}

		var tools []types.Tool
		for _, t := range cfg.Tools {
			if strings.ToLower(t.Type) != "function" || t.Function == nil {
				continue
			}
			tools = append(tools, &types.ToolMemberToolSpec{Value: types.ToolSpecification{
				Name:        aws.String(t.Function.Name),
				Description: aws.String(t.Function.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(t.Function.Parameters)},
			}})
		}
		if len(tools) > 0 {
			in.ToolConfig = &types.ToolConfiguration{Tools: tools}
		}
	}

	return in
}

// decodeConverse maps a Converse response into a neutral ChatResult.
func decodeConverse(out *bedrockruntime.ConverseOutput) (modelrepo.ChatResult, error) {
	if out == nil || out.Output == nil {
		return modelrepo.ChatResult{}, fmt.Errorf("bedrock: empty converse output")
	}
	msgOut, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return modelrepo.ChatResult{}, fmt.Errorf("bedrock: unexpected converse output type %T", out.Output)
	}

	var text strings.Builder
	var toolCalls []modelrepo.ToolCall
	for _, cb := range msgOut.Value.Content {
		switch v := cb.(type) {
		case *types.ContentBlockMemberText:
			text.WriteString(v.Value)
		case *types.ContentBlockMemberToolUse:
			toolCalls = append(toolCalls, newToolCall(
				aws.ToString(v.Value.ToolUseId),
				aws.ToString(v.Value.Name),
				documentToJSONString(v.Value.Input),
			))
		}
	}

	if text.Len() == 0 && len(toolCalls) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("bedrock: no text or tool calls in response")
	}
	return modelrepo.ChatResult{
		Message:   modelrepo.Message{Role: "assistant", Content: text.String()},
		ToolCalls: toolCalls,
	}, nil
}

func newToolCall(id, name, args string) modelrepo.ToolCall {
	tc := modelrepo.ToolCall{ID: id, Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}
