package vertex

import "strings"

// vertexFamily is the wire-format family a Vertex publisher speaks. Vertex is a
// hosting marketplace, not a unified API: each publisher keeps its own request/
// response schema and endpoint verb. We map publisher -> family and drive the
// chat/stream clients accordingly.
type vertexFamily int

const (
	// familyGemini: Google's generateContent / streamGenerateContent (the only
	// publisher this package originally supported).
	familyGemini vertexFamily = iota
	// familyAnthropic: Anthropic Messages over :rawPredict / :streamRawPredict.
	familyAnthropic
	// familyOpenAICompat: OpenAI chat/completions — Mistral over :rawPredict /
	// :streamRawPredict, Meta/open-model MaaS over /endpoints/openapi/chat/completions.
	familyOpenAICompat
)

func vertexFamilyFor(publisher string) vertexFamily {
	switch publisher {
	case "anthropic":
		return familyAnthropic
	case "meta", "mistralai":
		return familyOpenAICompat
	default:
		return familyGemini
	}
}

// anthropicURL returns the rawPredict / streamRawPredict endpoint for Claude.
func (c *vertexClient) anthropicURL(streaming bool) string {
	if streaming {
		return c.endpoint("streamRawPredict")
	}
	return c.endpoint("rawPredict")
}

// openAICompatURL returns the endpoint for an OpenAI-compatible publisher.
// Mistral uses the publisher rawPredict path; Meta/open-model MaaS uses the
// dedicated OpenAI-compatible openapi path (model goes in the body, streaming
// is a body flag, so the URL does not change between stream and non-stream).
func (c *vertexClient) openAICompatURL(streaming bool) string {
	if c.publisher == "meta" {
		return openAPIChatCompletionsURL(c.baseURL)
	}
	if streaming {
		return c.endpoint("streamRawPredict")
	}
	return c.endpoint("rawPredict")
}

// openAICompatBodyModel returns the model string to place in the request body.
// Meta MaaS requires the publisher-prefixed id (e.g. "meta/llama-...-maas");
// Mistral uses the model id (often with an "@version" suffix) as listed.
func (c *vertexClient) openAICompatBodyModel() string {
	if c.publisher == "meta" {
		return ensurePublisherPrefix("meta", c.modelName)
	}
	return c.modelName
}

func openAPIChatCompletionsURL(base string) string {
	return strings.TrimRight(base, "/") + "/endpoints/openapi/chat/completions"
}

func ensurePublisherPrefix(publisher, model string) string {
	if strings.Contains(model, "/") {
		return model
	}
	return publisher + "/" + model
}
