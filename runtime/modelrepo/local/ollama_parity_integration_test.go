//go:build integration

package local

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSystem_Local_OllamaParity embeds the same texts through the fixed local
// provider and through Ollama, using the SAME gguf weights, and compares them.
//
// This is the decisive divergence check: point the local provider at Ollama's
// own nomic blob so quantization is identical, embed each text both ways, and
// compare cosine. After v0.26.0 (normalization + pooling + batch sizing) the two
// paths should agree to ~1.0; a material gap means a residual provider bug.
//
// Requires a running Ollama with the model pulled, and the blob path:
//
//	NOMIC_GGUF=/path/to/ollama/blobs/sha256-... \
//	OLLAMA_MODEL=nomic-embed-text \
//	OLLAMA_HOST=127.0.0.1:11434 \
//	CGO_ENABLED=1 go test -tags integration -run TestSystem_Local_OllamaParity -v ./runtime/modelrepo/local/...
func TestSystem_Local_OllamaParity(t *testing.T) {
	gguf := os.Getenv("NOMIC_GGUF")
	if gguf == "" {
		t.Skip("set NOMIC_GGUF to Ollama's nomic blob path to run the parity check")
	}
	model := envOr("OLLAMA_MODEL", "nomic-embed-text")
	host := envOr("OLLAMA_HOST", "127.0.0.1:11434")

	client := &localEmbedClient{modelPath: gguf}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	texts := []string{
		"A dog is running through the park.",
		"A canine sprints across the grassy field.",
		"The quarterly stock market report was released today.",
		"search_query: how do I reset my password",
		"search_document: To reset your password, open settings and choose Security.",
	}

	var minParity = 1.0
	for _, txt := range texts {
		local, err := client.Embed(ctx, txt)
		require.NoError(t, err)
		remote, err := ollamaEmbed(ctx, host, model, txt)
		require.NoError(t, err)
		require.Equal(t, len(local), len(remote), "dim mismatch — wrong blob?")

		parity := cosine(local, remote)
		t.Logf("parity cosine(local, ollama)=%.5f  for %q", parity, truncate(txt, 48))
		if parity < minParity {
			minParity = parity
		}
	}

	t.Logf("worst-case parity cosine = %.5f", minParity)
	// Same weights + same math should agree to ~1.0; quantized reduction order
	// can jiggle the last digits, so allow a small tolerance.
	assert.Greater(t, minParity, 0.999,
		"fixed local provider should match Ollama on identical weights; a gap means a residual provider divergence")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ollamaEmbed calls Ollama's /api/embed (which L2-normalizes server-side).
func ollamaEmbed(ctx context.Context, host, model, input string) ([]float64, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+host+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Embeddings[0], nil
}
