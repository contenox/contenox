//go:build integration

package local

import (
	"context"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/runtime/modelrepo"
)

const (
	tinyModelURL  = "https://huggingface.co/Hjgugugjhuhjggg/FastThink-0.5B-Tiny-Q2_K-GGUF/resolve/main/fastthink-0.5b-tiny-q2_k.gguf"
	tinyModelName = "fastthink-0.5b-tiny-q2_k.gguf"

	// all-MiniLM-L6-v2: a small (~24MB Q5), symmetric, general-purpose
	// embedding model — no task prefix, queries and documents encoded the same
	// way — with MEAN pooling, so it exercises the pooled GetEmbeddingsSeq path.
	embedModelURL  = "https://huggingface.co/second-state/All-MiniLM-L6-v2-Embedding-GGUF/resolve/main/all-MiniLM-L6-v2-Q5_K_M.gguf"
	embedModelName = "all-MiniLM-L6-v2-Q5_K_M.gguf"

	// nomic-embed-text-v1.5: the production default (Makefile EMBED_MODEL).
	// Unlike all-MiniLM it is a RoPE-based nomic-bert architecture and is
	// asymmetric — trained with search_query:/search_document: task prefixes —
	// so it exercises the exact path the EE harness disputed.
	nomicModelURL  = "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q4_K_M.gguf"
	nomicModelName = "nomic-embed-text-v1.5.Q4_K_M.gguf"
)

// tinyModelPath returns a cached path to the tiny test model, downloading it if necessary.
func tinyModelPath(t *testing.T) string {
	t.Helper()
	return cachedModelPath(t, tinyModelURL, tinyModelName)
}

func cachedModelPath(t *testing.T, url, name string) string {
	t.Helper()
	cacheDir := filepath.Join(os.TempDir(), "contenox-test-models")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))
	dest := filepath.Join(cacheDir, name)
	if _, err := os.Stat(dest); err == nil {
		return dest
	}
	t.Logf("downloading test model to %s ...", dest)
	resp, err := http.Get(url) //nolint:gosec
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	f, err := os.Create(dest)
	require.NoError(t, err)
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	require.NoError(t, err)
	return dest
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func l2Norm(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

func userMsg(content string) modelrepo.Message {
	return modelrepo.Message{Role: "user", Content: content}
}

func TestSystem_Local_Chat(t *testing.T) {
	path := tinyModelPath(t)
	client := &localChatClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.Chat(ctx, []modelrepo.Message{userMsg("Say hello in one word.")})
	require.NoError(t, err)
	assert.Equal(t, "assistant", result.Message.Role)
	assert.NotEmpty(t, result.Message.Content)
	t.Logf("Chat response: %q", result.Message.Content)
}

func TestSystem_Local_Prompt(t *testing.T) {
	path := tinyModelPath(t)
	client := &localPromptClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	out, err := client.Prompt(ctx, "You are a helpful assistant.", 0.5, "Say hello in one word.")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	t.Logf("Prompt response: %q", out)
}

func TestSystem_Local_Stream(t *testing.T) {
	path := tinyModelPath(t)
	client := &localStreamClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ch, err := client.Stream(ctx, []modelrepo.Message{userMsg("Say hello in one word.")})
	require.NoError(t, err)

	var sb strings.Builder
	for p := range ch {
		require.NoError(t, p.Error, "unexpected stream error parcel")
		sb.WriteString(p.Data)
	}

	assert.NotEmpty(t, sb.String())
	t.Logf("Stream response: %q", sb.String())
}

func TestSystem_Local_Embed(t *testing.T) {
	path := tinyModelPath(t)
	client := &localEmbedClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	emb, err := client.Embed(ctx, "hello world")
	require.NoError(t, err)
	assert.NotEmpty(t, emb)

	allZero := true
	for _, v := range emb {
		if v != 0 {
			allZero = false
			break
		}
	}
	assert.False(t, allZero, "embedding vector should not be all zeros")
	// FastThink is a decoder model (no pooling metadata), so this also exercises
	// the mean-pool fallback in extractEmbedding. Either path must return a
	// unit-length vector after l2Normalize.
	assert.InDelta(t, 1.0, l2Norm(emb), 1e-6, "embedding should be L2-normalized")
	t.Logf("Embedding dim=%d, norm=%.6f, first 5 values: %v", len(emb), l2Norm(emb), emb[:min(5, len(emb))])
}

// TestSystem_Local_Embed_SemanticRanking is the gate that proves the local
// embedding provider is semantically sane in isolation — no Vald, no Ollama.
// Near-synonymous sentences must embed closer (higher cosine) than an unrelated
// sentence, by a clear margin. A pass means pooling resolved correctly and the
// vectors carry real meaning; a fail means a genuine provider bug to chase.
func TestSystem_Local_Embed_SemanticRanking(t *testing.T) {
	path := cachedModelPath(t, embedModelURL, embedModelName)
	client := &localEmbedClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	embed := func(text string) []float64 {
		v, err := client.Embed(ctx, text)
		require.NoError(t, err)
		require.NotEmpty(t, v)
		require.InDelta(t, 1.0, l2Norm(v), 1e-6, "embedding should be L2-normalized")
		return v
	}

	anchor := embed("A dog is running through the park.")
	near := embed("A canine sprints across the grassy field.")
	far := embed("The quarterly stock market report was released today.")

	simNear := cosine(anchor, near)
	simFar := cosine(anchor, far)
	t.Logf("cosine(anchor, near)=%.4f  cosine(anchor, far)=%.4f", simNear, simFar)

	assert.Greater(t, simNear, simFar,
		"near-synonym should be closer to the anchor than the unrelated sentence")
	assert.Greater(t, simNear-simFar, 0.1,
		"the semantic gap should be clear, not marginal")
}

// TestSystem_Local_Embed_NomicRanking runs the production default model
// (nomic-embed-text-v1.5) through the fixed provider in isolation. nomic is the
// model this whole investigation was about, and it differs from all-MiniLM on
// the two axes that matter: it is RoPE-based nomic-bert (not vanilla BERT), and
// it is asymmetric (trained with search_query:/search_document: prefixes).
//
// The hard assertion is on the prefixed (correct-usage) ranking: a clean pass
// proves the nomic-bert embedding extraction path is sound, killing the
// "llama.cpp's nomic path != Ollama's" hypothesis. The unprefixed ranking is
// measured and logged — not asserted — to reveal whether missing prefixes (a
// real caller-side usage requirement) were a second, independent cause of the
// EE harness's poor results, distinct from the normalization fix here.
func TestSystem_Local_Embed_NomicRanking(t *testing.T) {
	path := cachedModelPath(t, nomicModelURL, nomicModelName)
	client := &localEmbedClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	embed := func(text string) []float64 {
		v, err := client.Embed(ctx, text)
		require.NoError(t, err)
		require.NotEmpty(t, v)
		require.InDelta(t, 1.0, l2Norm(v), 1e-6, "embedding should be L2-normalized")
		return v
	}

	const (
		query   = "A dog is running through the park."
		nearDoc = "A canine sprints across the grassy field."
		farDoc  = "The quarterly stock market report was released today."
	)

	// Correct asymmetric usage: query gets search_query:, documents get
	// search_document:.
	pq := embed("search_query: " + query)
	pNear := embed("search_document: " + nearDoc)
	pFar := embed("search_document: " + farDoc)
	prefixedNear := cosine(pq, pNear)
	prefixedFar := cosine(pq, pFar)

	// Same inputs, no prefixes — how the EE harness used it.
	uq := embed(query)
	uNear := embed(nearDoc)
	uFar := embed(farDoc)
	plainNear := cosine(uq, uNear)
	plainFar := cosine(uq, uFar)

	t.Logf("prefixed:   cosine(near)=%.4f  cosine(far)=%.4f  gap=%.4f", prefixedNear, prefixedFar, prefixedNear-prefixedFar)
	t.Logf("no prefix:  cosine(near)=%.4f  cosine(far)=%.4f  gap=%.4f", plainNear, plainFar, plainNear-plainFar)

	assert.Greater(t, prefixedNear, prefixedFar,
		"with correct prefixes, nomic must rank the near-synonym above the unrelated sentence")
	assert.Greater(t, prefixedNear-prefixedFar, 0.1,
		"with correct prefixes, the semantic gap should be clear — a marginal/failed gap implies a broken nomic-bert extraction path")
}

// TestSystem_Local_Embed_LongInput proves the embed path sizes the batch to the
// input instead of the old fixed 512 (which panicked in batch.Add past 512
// tokens). An input well over 512 tokens, but under the default token limit,
// must embed successfully and produce a unit-length vector.
func TestSystem_Local_Embed_LongInput(t *testing.T) {
	path := cachedModelPath(t, nomicModelURL, nomicModelName)
	// contextLength 0 => default limit (defaultEmbedTokenLimit = 4096).
	client := &localEmbedClient{modelPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// ~80 repetitions * ~9 words => ~700 words, comfortably over 512 tokens and
	// under the 4096 default limit.
	long := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 80)

	emb, err := client.Embed(ctx, long)
	require.NoError(t, err, "input over 512 tokens must not panic or fail")
	require.NotEmpty(t, emb)
	assert.InDelta(t, 1.0, l2Norm(emb), 1e-6, "embedding should be L2-normalized")
}

// TestSystem_Local_Embed_ExceedsLimit verifies that an input beyond the
// configured token limit returns a clear, actionable error rather than
// truncating silently or panicking.
func TestSystem_Local_Embed_ExceedsLimit(t *testing.T) {
	path := cachedModelPath(t, nomicModelURL, nomicModelName)
	// Tiny declared context length so a modest input exceeds it.
	client := &localEmbedClient{modelPath: path, contextLength: 64}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	long := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 40) // ~350 words >> 64 tokens

	_, err := client.Embed(ctx, long)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit is 64", "error should report the configured limit")
}

// overlongPrompt builds a prompt guaranteed to tokenize past the fixed 512
// batch used by the chat/prompt/stream paths.
func overlongPrompt() string {
	return strings.Repeat("The quick brown fox jumps over the lazy dog. ", 200)
}

// TestSystem_Local_Chat_LongPromptRecovers proves a prompt longer than the
// fixed batch returns an error instead of panicking out of the caller.
func TestSystem_Local_Chat_LongPromptRecovers(t *testing.T) {
	path := tinyModelPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("Chat", func(t *testing.T) {
		_, err := (&localChatClient{modelPath: path}).Chat(ctx, []modelrepo.Message{userMsg(overlongPrompt())})
		require.Error(t, err, "overlong prompt must return an error, not panic")
		assert.Contains(t, err.Error(), "panicked")
	})

	t.Run("Prompt", func(t *testing.T) {
		_, err := (&localPromptClient{modelPath: path}).Prompt(ctx, "", 0.5, overlongPrompt())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "panicked")
	})
}

// TestSystem_Local_Stream_LongPromptRecovers proves the spawned stream goroutine
// recovers a batch panic into an error parcel (rather than crashing the process)
// and still closes the channel.
func TestSystem_Local_Stream_LongPromptRecovers(t *testing.T) {
	path := tinyModelPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ch, err := (&localStreamClient{modelPath: path}).Stream(ctx, []modelrepo.Message{userMsg(overlongPrompt())})
	require.NoError(t, err)

	var gotErr error
	for p := range ch { // must terminate: channel has to close even on panic
		if p.Error != nil {
			gotErr = p.Error
		}
	}
	require.Error(t, gotErr, "stream must surface a panic as an error parcel")
	assert.Contains(t, gotErr.Error(), "panicked")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
