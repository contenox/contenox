package openai

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
)

type OpenAIEmbedClient struct {
	openAIClient
}

type openAIEmbedRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
}

type openAIEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *OpenAIEmbedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	request := openAIEmbedRequest{
		Model:          c.modelName,
		Input:          prompt,
		EncodingFormat: "float",
	}

	var response openAIEmbedResponse
	if err := c.sendRequest(ctx, "/embeddings", request, &response); err != nil {
		return nil, err
	}

	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embedding data returned from OpenAI for model %s", c.modelName)
	}
	return response.Data[0].Embedding, nil
}

var _ modelrepo.LLMEmbedClient = (*OpenAIEmbedClient)(nil)
