package ollama

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
	"github.com/ollama/ollama/api"
)

type OllamaPromptClient struct {
	ollamaClient *api.Client
	modelName    string
	backendURL   string
}

// Prompt implements serverops.LLMPromptClient.
func (o *OllamaPromptClient) Prompt(ctx context.Context, systeminstruction string, temperature float32, prompt string) (string, error) {
	stream := false
	think := api.ThinkValue{
		Value: false,
	}
	req := &api.GenerateRequest{
		Model:  o.modelName,
		Prompt: prompt,
		System: systeminstruction,
		Stream: &stream,
		Options: map[string]any{
			"temperature": temperature,
		},
		Think: &think,
	}

	var (
		content       string
		finalResponse api.GenerateResponse
	)

	err := o.ollamaClient.Generate(ctx, req, func(gr api.GenerateResponse) error {
		content += gr.Response
		if gr.Done {
			finalResponse = gr
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("ollama generate API call failed for model %s: %w", o.modelName, err)
	}

	if !finalResponse.Done {
		return "", fmt.Errorf("no completion received from ollama for model %s", o.modelName)
	}

	switch finalResponse.DoneReason {
	case "error":
		return "", fmt.Errorf("ollama generation error for model %s: %s", o.modelName, content)
	case "length":
		return "", fmt.Errorf("token limit reached for model %s (partial response: %q)", o.modelName, content)
	case "stop":
		if content == "" {
			return "", fmt.Errorf("empty content from model %s despite normal completion", o.modelName)
		}
	default:
		return "", fmt.Errorf("unexpected completion reason %q for model %s", finalResponse.DoneReason, o.modelName)
	}

	return content, nil
}

var _ modelrepo.LLMPromptExecClient = (*OllamaPromptClient)(nil)
