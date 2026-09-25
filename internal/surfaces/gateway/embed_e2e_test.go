package gateway_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/require"
)

const embedModel = "nomic-embed-text"

func embedBackend(t *testing.T) gatewaySeed {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "embed.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "nomic-embed-text",
  "capabilities": {"chat": true, "embed": true},
  "turns": [{"text": "ok"}]
}`), 0o600))

	return func(ctx context.Context, store runtimetypes.Store) {
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: "scripted", Name: "scripted",
			Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
		}))
	}
}

func postEmbed(t *testing.T, url, bearer, body string) (int, gateway.EmbedResponse) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/api/embed", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var decoded gateway.EmbedResponse
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	}
	return resp.StatusCode, decoded
}

func TestSystem_GatewayEmbedsAndMetersForAnAllowedKey(t *testing.T) {
	authority, svc := newGatewayAuthority(t, embedBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)

	status, resp := postEmbed(t, server.URL, token, `{"model":"`+embedModel+`","input":"hello world"}`)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, embedModel, resp.Model)
	require.Len(t, resp.Embeddings, 1)
	require.Len(t, resp.Embeddings[0], 64)
	require.Positive(t, resp.PromptEvalCount, "the caller is told what it spent")

	snapshot, err := authority.usage.ReadUsage(context.Background(), runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop-alex",
		Model: embedModel, WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, resp.PromptEvalCount, snapshot.PromptTokens)
	require.Zero(t, snapshot.CompletionTokens, "an embedding is input through and through")
	require.EqualValues(t, snapshot.PromptTokens, snapshot.EffectiveInput,
		"no cache was read, so the whole prompt counts against the input ceiling")
}

func TestSystem_GatewayEmbedsABatchInOrder(t *testing.T) {
	authority, svc := newGatewayAuthority(t, embedBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)

	single, one := postEmbed(t, server.URL, token, `{"model":"`+embedModel+`","input":"alpha"}`)
	require.Equal(t, http.StatusOK, single)

	status, batch := postEmbed(t, server.URL, token, `{"model":"`+embedModel+`","input":["alpha","beta"]}`)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, batch.Embeddings, 2)
	require.Equal(t, one.Embeddings[0], batch.Embeddings[0], "the vectors come back in the order they were sent")
	require.NotEqual(t, batch.Embeddings[0], batch.Embeddings[1])
}

func TestSystem_GatewayRefusesEmbeddingForAModelTheKeyDoesNotName(t *testing.T) {
	authority, svc := newGatewayAuthority(t, embedBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mintFor(t, "laptop-bob", "qwen3:8b", true)

	status, _ := postEmbed(t, server.URL, token, `{"model":"`+embedModel+`","input":"hello"}`)
	require.Equal(t, http.StatusForbidden, status)
}

func TestSystem_GatewayRefusesAnEmbeddingOnAModelThatCannotEmbed(t *testing.T) {
	authority, svc := newGatewayAuthority(t, func(ctx context.Context, store runtimetypes.Store) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "chat.json")
		require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "coder",
  "capabilities": {"chat": true, "embed": false},
  "turns": [{"text": "ok"}]
}`), 0o600))
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: "scripted", Name: "scripted",
			Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
		}))
	})
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)

	status, _ := postEmbed(t, server.URL, token, `{"model":"coder","input":"hello"}`)
	require.Equal(t, http.StatusNotFound, status,
		"the resolver only offers models observed to embed, so this never reaches the provider")
}

func TestSystem_GatewayRefusesAMalformedEmbedInput(t *testing.T) {
	authority, svc := newGatewayAuthority(t, embedBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)

	for _, body := range []string{
		`{"model":"` + embedModel + `","input":""}`,
		`{"model":"` + embedModel + `","input":[]}`,
		`{"model":"` + embedModel + `","input":[1,2]}`,
		`{"model":"` + embedModel + `"}`,
		`{"model":"` + embedModel + `","input":"hello","dimensions":256}`,
	} {
		status, _ := postEmbed(t, server.URL, token, body)
		require.Equal(t, http.StatusBadRequest, status, body)
	}
}

func TestSystem_GatewayHoldsAnEmbeddingToTheKeysInputAllowance(t *testing.T) {
	authority, svc := newGatewayAuthority(t, embedBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mintFor(t, "laptop-alex", embedModel, true, func(claims *liblicense.Claims) {
		claims.SetInt64(liblicense.InputAllowanceKey(embedModel), 1)
	})

	status, first := postEmbed(t, server.URL, token, `{"model":"`+embedModel+`","input":"hello world"}`)
	require.Equal(t, http.StatusOK, status, "the ceiling is soft: the turn that crosses it runs")
	require.Positive(t, first.PromptEvalCount)

	status, _ = postEmbed(t, server.URL, token, `{"model":"`+embedModel+`","input":"hello again"}`)
	require.Equal(t, http.StatusTooManyRequests, status,
		"the input the embedding spent is the input the key's ceiling is measured on")
}

// The record for one turn carries the image count the request had, the count the
// weekly ceiling is measured on, and the cost the rate card charges for it.
func TestSystem_GatewayCountsAndChargesImages(t *testing.T) {
	authority, svc := newGatewayAuthority(t, func(ctx context.Context, store runtimetypes.Store) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "vision.json")
		require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "qwen2.5vl",
  "capabilities": {"chat": true, "vision": true},
  "turns": [{"text": "a cat", "usage": {"prompt_tokens": 100, "completion_tokens": 5}}]
}`), 0o600))
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: "scripted", Name: "scripted",
			Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
		}))
		require.NoError(t, store.SetLLMProviderModelFact(ctx, "scripted", "qwen2.5vl", runtimetypes.ModelFacts{
			Pricing: &runtimetypes.ModelPricing{InputPerMillion: 1, OutputPerMillion: 1, PerImage: 0.02},
		}))
	})
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)

	body := `{"model":"qwen2.5vl","stream":false,"messages":[{"role":"user","content":"what is this?","images":["` +
		base64.StdEncoding.EncodeToString([]byte("not-really-a-png")) + `","` +
		base64.StdEncoding.EncodeToString([]byte("nor-this")) + `"]}]}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/chat", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	snapshot, err := authority.usage.ReadUsage(context.Background(), runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop-alex",
		Model: "qwen2.5vl", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 2, snapshot.ImageCount, "both attachments are counted from the request")
	require.InDelta(t, 0.04, float64(snapshot.CostMicrodollars)/1_000_000, 0.001,
		"two images at the declared per-image rate")
}

func TestSystem_GatewayHoldsATurnToTheKeysImageAllowance(t *testing.T) {
	authority, svc := newGatewayAuthority(t, func(ctx context.Context, store runtimetypes.Store) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "vision.json")
		require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "qwen2.5vl",
  "capabilities": {"chat": true, "vision": true},
  "turns": [
    {"text": "a cat", "usage": {"prompt_tokens": 100, "completion_tokens": 5}},
    {"text": "another", "usage": {"prompt_tokens": 100, "completion_tokens": 5}}
  ]
}`), 0o600))
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: "scripted", Name: "scripted",
			Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
		}))
	})
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mintFor(t, "laptop-alex", "qwen2.5vl", true, func(claims *liblicense.Claims) {
		claims.SetInt64(liblicense.ImageAllowanceKey("qwen2.5vl"), 1)
	})

	one := `{"model":"qwen2.5vl","stream":false,"messages":[{"role":"user","content":"what is this?","images":["` +
		base64.StdEncoding.EncodeToString([]byte("an-image")) + `"]}]}`

	post := func(body string) int {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/chat", bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	require.Equal(t, http.StatusOK, post(one), "the ceiling is soft: the turn that crosses it runs")
	require.Equal(t, http.StatusTooManyRequests, post(one), "the second image is over the key's weekly allowance")
}

// A provider that reports no token accounting still spent an image, and the
// meter records what it can count rather than dropping the turn.
func TestSystem_GatewayRecordsAnImageTurnWithNoReportedTokens(t *testing.T) {
	authority, svc := newGatewayAuthority(t, func(ctx context.Context, store runtimetypes.Store) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "silent.json")
		require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "qwen2.5vl",
  "capabilities": {"chat": true, "vision": true},
  "turns": [{"text": "ok"}]
}`), 0o600))
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: "scripted", Name: "scripted",
			Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
		}))
		require.NoError(t, store.SetLLMProviderModelFact(ctx, "scripted", "qwen2.5vl", runtimetypes.ModelFacts{
			Pricing: &runtimetypes.ModelPricing{PerImage: 0.05},
		}))
	})
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)
	body := `{"model":"qwen2.5vl","stream":false,"messages":[{"role":"user","content":"look","images":["` +
		base64.StdEncoding.EncodeToString([]byte("an-image")) + `"]}]}`

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/chat", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	snapshot, err := authority.usage.ReadUsage(context.Background(), runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop-alex",
		Model: "qwen2.5vl", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 1, snapshot.ImageCount)
	require.InDelta(t, 0.05, float64(snapshot.CostMicrodollars)/1_000_000, 0.0001,
		"the image is charged even though the provider reported no tokens for it")
}

func wavBytes(payload string) []byte {
	header := []byte("RIFF")
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(36+len(payload)))
	header = append(header, size...)
	header = append(header, []byte("WAVEfmt ")...)
	return append(header, []byte(payload)...)
}

func audioBackend(t *testing.T) gatewaySeed {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "audio.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "gemma3n",
  "capabilities": {"chat": true, "audio": true},
  "turns": [
    {"text": "transcribed", "usage": {"prompt_tokens": 50, "completion_tokens": 4}},
    {"text": "again", "usage": {"prompt_tokens": 50, "completion_tokens": 4}}
  ]
}`), 0o600))

	return func(ctx context.Context, store runtimetypes.Store) {
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: "scripted", Name: "scripted",
			Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
		}))
		require.NoError(t, store.SetLLMProviderModelFact(ctx, "scripted", "gemma3n", runtimetypes.ModelFacts{
			Pricing: &runtimetypes.ModelPricing{PerAudioMebibyte: 1.50},
		}))
	}
}

func chatWithAudio(t *testing.T, url, bearer string, audio []byte) int {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"model":  "gemma3n",
		"stream": false,
		"messages": []map[string]any{{
			"role":    "user",
			"content": "transcribe this",
			"audios":  []string{base64.StdEncoding.EncodeToString(audio)},
		}},
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url+"/api/chat", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestSystem_GatewayAcceptsAudioAndMetersItsBytes(t *testing.T) {
	authority, svc := newGatewayAuthority(t, audioBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)
	audio := wavBytes(strings.Repeat("a", 2*runtimetypes.Mebibyte))

	require.Equal(t, http.StatusOK, chatWithAudio(t, server.URL, token, audio))

	snapshot, err := authority.usage.ReadUsage(context.Background(), runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop-alex",
		Model: "gemma3n", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, len(audio), snapshot.AudioBytes, "the bytes that crossed the wire are what is recorded")
	require.InDelta(t, 3.00, float64(snapshot.CostMicrodollars)/1_000_000, 0.01,
		"two mebibytes at the declared per-mebibyte rate")
}

func TestSystem_GatewayRefusesAudioThatIsNotWAV(t *testing.T) {
	authority, svc := newGatewayAuthority(t, audioBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mint(t, "laptop-alex", true)

	require.Equal(t, http.StatusBadRequest, chatWithAudio(t, server.URL, token, []byte("this is not a wav file")),
		"the contract identifies audio by its magic bytes, so anything else is refused rather than mislabelled")
}

func TestSystem_GatewayHoldsATurnToTheKeysAudioAllowance(t *testing.T) {
	authority, svc := newGatewayAuthority(t, audioBackend(t))
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	token := authority.mintFor(t, "laptop-alex", "gemma3n", true, func(claims *liblicense.Claims) {
		claims.SetInt64(liblicense.AudioAllowanceKey("gemma3n"), 1)
	})
	audio := wavBytes(strings.Repeat("a", runtimetypes.Mebibyte))

	require.Equal(t, http.StatusOK, chatWithAudio(t, server.URL, token, audio), "the ceiling is soft: the turn that crosses it runs")
	require.Equal(t, http.StatusTooManyRequests, chatWithAudio(t, server.URL, token, audio), "the second mebibyte is over the key's weekly allowance")
}

// A cutoff raised because one model's week ran out is about that model: the
// durable check is per model, so the instant one has to agree, or a client is
// refused the models it still has budget for.
func TestSystem_BroadcastCutoffIsScopedToItsModel(t *testing.T) {
	authority, svc := newGatewayAuthority(t, func(ctx context.Context, store runtimetypes.Store) {
		for _, backend := range []struct{ id, model string }{
			{"scripted-a", "nomic-embed-text"},
			{"scripted-b", "bge-m3"},
		} {
			dir := t.TempDir()
			path := filepath.Join(dir, backend.model+".json")
			require.NoError(t, os.WriteFile(path, []byte(`{
  "model": "`+backend.model+`",
  "capabilities": {"chat": true, "embed": true},
  "turns": [{"text": "ok", "usage": {"prompt_tokens": 5, "completion_tokens": 1}}]
}`), 0o600))
			require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
				ID: backend.id, Name: backend.id,
				Type: modelrepo.ScriptedTestBackendType, BaseURL: path,
			}))
		}
	})
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, svc.SubscribeControlPlane(ctx))

	token := authority.mint(t, "laptop-alex", true)
	embed := func(model string) int {
		status, _ := postEmbed(t, server.URL, token, `{"model":"`+model+`","input":"hello"}`)
		return status
	}
	require.Equal(t, http.StatusOK, embed("bge-m3"), "both models start usable")

	event, err := json.Marshal(gateway.ProxyControlRevocationEvent{
		ClientID:  "laptop-alex",
		Model:     "nomic-embed-text",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		Reason:    "weekly output allowance exhausted",
	})
	require.NoError(t, err)
	require.NoError(t, authority.bus.Publish(ctx, gateway.SubjectProxyControlRevocation, event))

	require.Eventually(t, func() bool {
		return embed("nomic-embed-text") == http.StatusTooManyRequests
	}, 5*time.Second, 50*time.Millisecond, "the exhausted model is cut off at once")

	require.Equal(t, http.StatusOK, embed("bge-m3"),
		"the model with budget left is not collateral damage")
}
