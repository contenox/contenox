package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
)

type codexCatalog struct {
	opts    modelrepo.CatalogOptions
	baseURL string
}

// codexClientVersion selects the upstream catalog compatibility level, not the Contenox release version.
const codexClientVersion = "0.158.0"

func init() {
	modelrepo.RegisterCatalogProvider(modelauth.ProviderType, func(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		if spec.BaseURL != modelauth.BaseURL || spec.APIKey != "" {
			return nil, fmt.Errorf("openai-codex requires the ChatGPT endpoint and device login, not an API key")
		}
		if opts.Authorize == nil {
			return nil, modelauth.ErrLoginRequired
		}
		client := *opts.HTTPClient
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		opts.HTTPClient = &client
		return &codexCatalog{opts: opts, baseURL: spec.BaseURL}, nil
	})
}

func (c *codexCatalog) Type() string { return modelauth.ProviderType }

func (c *codexCatalog) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	headers, err := c.opts.Authorize(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers.Clone()
	req.Header.Set("originator", "contenox")
	req.Header.Set("User-Agent", "contenox")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	resp, err := c.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, codexResponseError(resp, headers)
	}
	return resp, nil
}

func (c *codexCatalog) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
	ctx, cancel := modelrepo.NonStreamingContext(ctx)
	defer cancel()
	resp, err := c.request(ctx, http.MethodGet, "/models?client_version="+codexClientVersion, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var wire struct {
		Models []struct {
			Slug            string            `json:"slug"`
			Visibility      string            `json:"visibility"`
			Context         int               `json:"context_window"`
			Reasoning       []json.RawMessage `json:"supported_reasoning_levels"`
			InputModalities []string          `json:"input_modalities"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&wire); err != nil {
		return nil, fmt.Errorf("invalid ChatGPT model catalog: %w", err)
	}
	var models []modelrepo.ObservedModel
	for _, m := range wire.Models {
		if m.Slug == "" || m.Visibility != "list" {
			continue
		}
		models = append(models, modelrepo.ObservedModel{Name: m.Slug, ContextLength: m.Context, CapabilityConfig: modelrepo.CapabilityConfig{ContextLength: m.Context, CanChat: true, CanPrompt: true, CanStream: true, CanThink: len(m.Reasoning) > 0, CanVision: slices.Contains(m.InputModalities, "image")}})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("ChatGPT model discovery returned no selectable models for client compatibility %s", codexClientVersion)
	}
	return models, nil
}

func (c *codexCatalog) ProviderFor(m modelrepo.ObservedModel) modelrepo.Provider {
	base := NewOpenAIProvider("", m.Name, []string{c.baseURL}, m.CapabilityConfig, c.opts.HTTPClient, c.opts.Tracker).(*OpenAIProvider)
	base.id = "openai-codex-" + m.Name
	base.codex = c
	return base
}

func codexResponseError(resp *http.Response, headers http.Header) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var wire struct {
		Detail json.RawMessage `json:"detail"`
		Error  struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &wire)
	message := wire.Error.Message
	if message == "" {
		_ = json.Unmarshal(wire.Detail, &message)
	}
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	clean := func(value string) string {
		for _, secret := range []string{headers.Get("Authorization"), strings.TrimPrefix(headers.Get("Authorization"), "Bearer "), headers.Get("Chatgpt-Account-Id")} {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[redacted]")
			}
		}
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, value)
		return truncateString(value, 2048)
	}
	var err error = &modelrepo.HTTPError{StatusCode: resp.StatusCode, Code: clean(wire.Error.Code), Message: clean(message), RequestID: clean(resp.Header.Get("X-Request-Id"))}
	if readErr != nil {
		err = fmt.Errorf("%w: read error response: %w", err, readErr)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		err = fmt.Errorf("%w: %w", modelauth.ErrLoginRequired, err)
	}
	return modelrepo.ClassifyProviderError(err, resp.StatusCode, wire.Error.Code, message)
}
