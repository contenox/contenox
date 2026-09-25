package runtimetypes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
)

// The capabilities a model may be declared to have. The values are the Ollama
// /api/show capability names verbatim, because that is the wire a gateway client
// parses: a value outside this set would be stored, reported, and then silently
// ignored by every reader, which is a declaration that cannot take effect.
const (
	CapabilityCompletion = "completion"
	CapabilityTools      = "tools"
	CapabilityVision     = "vision"
	CapabilityThinking   = "thinking"
	CapabilityEmbedding  = "embedding"
	CapabilityAudio      = "audio"
)

// ModelCapabilities is every capability a declaration may name.
func ModelCapabilities() []string {
	return []string{
		CapabilityCompletion, CapabilityTools, CapabilityVision,
		CapabilityThinking, CapabilityEmbedding, CapabilityAudio,
	}
}

// ModelPricing is the upstream cost card for one model, in USD per 1M units.
type ModelPricing struct {
	// InputPerMillion is the cost in USD per 1M uncached input (prompt) tokens.
	InputPerMillion float64 `json:"input_per_million,omitempty"`
	// CacheReadPerMillion is the cost in USD per 1M cached prompt tokens.
	CacheReadPerMillion float64 `json:"cache_read_per_million,omitempty"`
	// CacheWritePerMillion is the cost in USD per 1M tokens written to cache.
	CacheWritePerMillion float64 `json:"cache_write_per_million,omitempty"`
	// OutputPerMillion is the cost in USD per 1M completion / thinking tokens.
	OutputPerMillion float64 `json:"output_per_million,omitempty"`
	// PerImage is the cost in USD of one image attachment. It is not per million
	// of anything: a provider that bills images apart from tokens quotes them one
	// at a time, and the count is exact, so there is nothing to amortize.
	PerImage float64 `json:"per_image,omitempty"`
	// PerAudioMebibyte is the cost in USD of one mebibyte of inline audio.
	// Upstream quotes audio by the minute; a mebibyte is the unit this side can
	// measure exactly, and at 128 kbps it is about a minute, which is the
	// conversion an operator states a per-minute rate through.
	PerAudioMebibyte float64 `json:"per_audio_mebibyte,omitempty"`
}

// CalculateCost is the USD cost of one turn under this card. images is the
// number of image attachments the turn carried, which a provider that prices
// images apart from the tokens it reports for them is charged for separately.
func (p ModelPricing) CalculateCost(promptTokens, cacheReadTokens, cacheWriteTokens, completionTokens, images, audioBytes int64) float64 {
	uncachedInput := promptTokens - cacheReadTokens
	if uncachedInput < 0 {
		uncachedInput = 0
	}
	inputCost := (float64(uncachedInput) / 1_000_000.0) * p.InputPerMillion
	cacheReadCost := (float64(cacheReadTokens) / 1_000_000.0) * p.CacheReadPerMillion
	cacheWriteCost := (float64(cacheWriteTokens) / 1_000_000.0) * p.CacheWritePerMillion
	outputCost := (float64(completionTokens) / 1_000_000.0) * p.OutputPerMillion
	imageCost := float64(images) * p.PerImage
	audioCost := (float64(audioBytes) / Mebibyte) * p.PerAudioMebibyte
	return inputCost + cacheReadCost + cacheWriteCost + outputCost + imageCost + audioCost
}

// Mebibyte is what the audio rate is quoted per, and the divisor the meter
// applies to the bytes it counted.
const Mebibyte = 1 << 20

// ModelFacts is what the operator states about one served model that the
// upstream's own model list does not report: its context window, its output
// ceiling, capabilities, and upstream pricing.
type ModelFacts struct {
	// ContextLength is the model's whole context window in tokens, input and
	// output together.
	ContextLength int `json:"context_length,omitempty"`
	// MaxOutputTokens is the largest completion the upstream accepts.
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
	// Capabilities replaces what the upstream was observed to report, because an
	// operator who states a model's capabilities is correcting the observation,
	// not adding to it.
	Capabilities []string `json:"capabilities,omitempty"`
	// Pricing is the upstream provider's rate card for this model.
	Pricing *ModelPricing `json:"pricing,omitempty"`
}

// IsZero reports whether the declaration states nothing.
func (f ModelFacts) IsZero() bool {
	return f.ContextLength == 0 && f.MaxOutputTokens == 0 && len(f.Capabilities) == 0 && f.Pricing == nil
}

// ValidateModelFacts refuses a declaration no reader could honour: a negative
// size, a capability outside the vocabulary, or negative pricing rates.
func ValidateModelFacts(facts ModelFacts) error {
	if facts.ContextLength < 0 {
		return fmt.Errorf("context_length %d is negative", facts.ContextLength)
	}
	if facts.MaxOutputTokens < 0 {
		return fmt.Errorf("max_output_tokens %d is negative", facts.MaxOutputTokens)
	}
	for _, capability := range facts.Capabilities {
		if !ValidModelCapability(capability) {
			return fmt.Errorf("capability %q is not one of %s",
				capability, strings.Join(ModelCapabilities(), ", "))
		}
	}
	if p := facts.Pricing; p != nil {
		if p.InputPerMillion < 0 || p.CacheReadPerMillion < 0 || p.CacheWritePerMillion < 0 || p.OutputPerMillion < 0 || p.PerImage < 0 || p.PerAudioMebibyte < 0 {
			return errors.New("pricing rates must be non-negative")
		}
	}
	return nil
}

// ValidModelCapability reports whether name is a capability a reader honours.
func ValidModelCapability(name string) bool {
	for _, known := range ModelCapabilities() {
		if name == known {
			return true
		}
	}
	return false
}

// SetLLMProviderModelFacts writes one provider's per-model facts, replacing any
// it had. An empty map clears the declaration, which is how an operator returns
// a provider to what the upstream is observed to report.
func (s *store) SetLLMProviderModelFacts(ctx context.Context, providerID string, facts map[string]ModelFacts) error {
	if providerID == "" {
		return libdb.ErrNotFound
	}
	stored := make(map[string]ModelFacts, len(facts))
	for model, modelFacts := range facts {
		if strings.TrimSpace(model) == "" || modelFacts.IsZero() {
			continue
		}
		if err := ValidateModelFacts(modelFacts); err != nil {
			return fmt.Errorf("store: model %s: %w", model, err)
		}
		stored[model] = modelFacts
	}
	if len(stored) == 0 {
		return s.DeleteLLMProviderModelFacts(ctx, providerID)
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("store: encode provider model facts: %w", err)
	}
	_, err = s.ExecContext(ctx, `
		INSERT INTO llm_provider_model_facts (provider_id, facts, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider_id) DO UPDATE SET
			facts = excluded.facts,
			updated_at = excluded.updated_at
	`, providerID, string(raw), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("store: set provider model facts: %w", err)
	}
	return nil
}

// SetLLMProviderModelFact writes one model's facts for one provider, leaving the
// provider's other models declared. Facts that state nothing remove the entry,
// and a provider left declaring nothing loses its row.
func (s *store) SetLLMProviderModelFact(ctx context.Context, providerID, model string, facts ModelFacts) error {
	model = strings.TrimSpace(model)
	if providerID == "" || model == "" {
		return libdb.ErrNotFound
	}
	if facts.IsZero() {
		return s.DeleteLLMProviderModelFact(ctx, providerID, model)
	}
	if err := ValidateModelFacts(facts); err != nil {
		return fmt.Errorf("store: model %s: %w", model, err)
	}
	declared, err := s.GetLLMProviderModelFacts(ctx, providerID)
	if err != nil {
		return err
	}
	declared[model] = facts
	return s.SetLLMProviderModelFacts(ctx, providerID, declared)
}

// DeleteLLMProviderModelFact removes one model's facts from one provider.
func (s *store) DeleteLLMProviderModelFact(ctx context.Context, providerID, model string) error {
	model = strings.TrimSpace(model)
	if providerID == "" || model == "" {
		return libdb.ErrNotFound
	}
	declared, err := s.GetLLMProviderModelFacts(ctx, providerID)
	if err != nil {
		return err
	}
	if _, ok := declared[model]; !ok {
		return nil
	}
	delete(declared, model)
	return s.SetLLMProviderModelFacts(ctx, providerID, declared)
}

// DeleteLLMProviderModelFacts clears a provider's per-model facts.
func (s *store) DeleteLLMProviderModelFacts(ctx context.Context, providerID string) error {
	if providerID == "" {
		return libdb.ErrNotFound
	}
	if _, err := s.ExecContext(ctx, `
		DELETE FROM llm_provider_model_facts WHERE provider_id = $1
	`, providerID); err != nil {
		return fmt.Errorf("store: delete provider model facts: %w", err)
	}
	return nil
}

// GetLLMProviderModelFacts reads one provider's per-model facts. A provider that
// states none returns an empty map, never an invented one.
func (s *store) GetLLMProviderModelFacts(ctx context.Context, providerID string) (map[string]ModelFacts, error) {
	if providerID == "" {
		return nil, libdb.ErrNotFound
	}
	var raw string
	err := s.QueryRowContext(ctx, `
		SELECT facts FROM llm_provider_model_facts WHERE provider_id = $1
	`, providerID).Scan(&raw)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return map[string]ModelFacts{}, nil
		}
		return nil, fmt.Errorf("store: get provider model facts: %w", err)
	}
	facts := map[string]ModelFacts{}
	if raw == "" {
		return facts, nil
	}
	if err := json.Unmarshal([]byte(raw), &facts); err != nil {
		return nil, fmt.Errorf("store: decode provider model facts: %w", err)
	}
	return facts, nil
}
