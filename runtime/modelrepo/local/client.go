package local

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/ollama/ollama/llama"
	"github.com/ollama/ollama/ml"
)

const (
	defaultNumCtx    = 4096
	defaultBatch     = 512
	defaultMaxTokens = 2048

	// defaultEmbedTokenLimit bounds a single embedding input when the model's
	// context length isn't declared. Embedding needs the whole input in one
	// ubatch, so this also bounds that allocation.
	defaultEmbedTokenLimit = 4096
	// maxEmbedTokenLimit is a hard ceiling so a model declared with an enormous
	// context can't trigger an absurd single-ubatch allocation.
	maxEmbedTokenLimit = 32768
)

// localChatClient implements modelrepo.LLMChatClient using llama.cpp in-process.
type localChatClient struct {
	modelPath       string
	maxOutputTokens int
}

func (c *localChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}

	prompt := buildPrompt(messages)
	text, err := generate(ctx, c.modelPath, prompt, cfg, c.maxOutputTokens)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	return modelrepo.ChatResult{
		Message: modelrepo.Message{Role: "assistant", Content: text},
	}, nil
}

// localPromptClient implements modelrepo.LLMPromptExecClient.
type localPromptClient struct {
	modelPath       string
	maxOutputTokens int
}

func (c *localPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	messages := []modelrepo.Message{
		{Role: "system", Content: systemInstruction},
		{Role: "user", Content: prompt},
	}
	temp := float64(temperature)
	cfg := &modelrepo.ChatConfig{Temperature: &temp}
	return generate(ctx, c.modelPath, buildPrompt(messages), cfg, c.maxOutputTokens)
}

// buildPrompt converts messages to a simple chat-ML format.
// Models with a bundled chat template will re-tokenize correctly;
// for models without one this provides a reasonable fallback.
func buildPrompt(messages []modelrepo.Message) string {
	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "system":
			fmt.Fprintf(&b, "<|system|>\n%s\n", m.Content)
		case "user":
			fmt.Fprintf(&b, "<|user|>\n%s\n", m.Content)
		case "assistant":
			fmt.Fprintf(&b, "<|assistant|>\n%s\n", m.Content)
		default:
			fmt.Fprintf(&b, "%s\n", m.Content)
		}
	}
	b.WriteString("<|assistant|>\n")
	return b.String()
}

func generate(ctx context.Context, modelPath, prompt string, cfg *modelrepo.ChatConfig, maxOutputTokens int) (text string, err error) {
	// The prompt is decoded in a single fixed-size batch (defaultBatch); a
	// prompt longer than that overflows batch.Add, which panics with an
	// out-of-range index (a recoverable Go bounds panic — allocSize matches the
	// C buffer, so it trips before touching invalid memory). Convert it into a
	// returned error rather than letting it unwind the caller's goroutine.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("local generate panicked (prompt may exceed the %d-token batch): %v", defaultBatch, r)
		}
	}()

	lm, err := acquireModel(modelPath)
	if err != nil {
		return "", err
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	ctxParams := llama.NewContextParams(
		defaultNumCtx,
		defaultBatch,
		1,
		runtime.NumCPU(),
		ml.FlashAttentionDisabled,
		"",
	)
	llamaCtx, err := llama.NewContextWithModel(lm.model, ctxParams)
	if err != nil {
		return "", fmt.Errorf("create context: %w", err)
	}

	tokens, err := lm.model.Tokenize(prompt, true, true)
	if err != nil {
		return "", fmt.Errorf("tokenize: %w", err)
	}

	batch, err := llama.NewBatch(defaultBatch, 1, 0)
	if err != nil {
		return "", fmt.Errorf("create batch: %w", err)
	}
	defer batch.Free()

	for i, tok := range tokens {
		batch.Add(tok, nil, i, i == len(tokens)-1, 0)
	}
	if err := llamaCtx.Decode(batch); err != nil {
		return "", fmt.Errorf("decode prompt: %w", err)
	}

	samplerParams := llama.SamplingParams{
		TopK: 40,
		TopP: 0.9,
		MinP: 0.05,
		Temp: 0.8,
	}
	if cfg != nil && cfg.Temperature != nil {
		samplerParams.Temp = float32(*cfg.Temperature)
	}

	sampler, err := llama.NewSamplingContext(lm.model, samplerParams)
	if err != nil {
		return "", fmt.Errorf("create sampler: %w", err)
	}

	maxTokens := defaultMaxTokens
	if cfg != nil && cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
		maxTokens = *cfg.MaxTokens
	}
	maxTokens, _ = modelrepo.ClampMaxOutputTokens(maxTokens, maxOutputTokens)

	var out strings.Builder
	for pos := len(tokens); pos < len(tokens)+maxTokens; pos++ {
		select {
		case <-ctx.Done():
			return out.String(), ctx.Err()
		default:
		}

		id := sampler.Sample(llamaCtx, -1)
		sampler.Accept(id, true)

		if lm.model.TokenIsEog(id) {
			break
		}

		out.WriteString(lm.model.TokenToPiece(id))

		batch.Clear()
		batch.Add(id, nil, pos, true, 0)
		if err := llamaCtx.Decode(batch); err != nil {
			return out.String(), fmt.Errorf("decode token: %w", err)
		}
	}

	return strings.TrimSpace(out.String()), nil
}

// localStreamClient implements modelrepo.LLMStreamClient using llama.cpp in-process.
type localStreamClient struct {
	modelPath       string
	maxOutputTokens int
}

func (c *localStreamClient) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}

	prompt := buildPrompt(messages)
	ch := make(chan *modelrepo.StreamParcel, 16)

	go func() {
		defer close(ch)
		// An unrecovered panic here (e.g. a prompt longer than defaultBatch
		// overflowing batch.Add) would crash the whole process, since this runs
		// in a spawned goroutine. Recover and surface it as an error parcel. This
		// defer is registered after defer close(ch), so it runs first (LIFO): the
		// parcel is sent before the channel closes. The buffered channel (cap 16)
		// guarantees the send never blocks.
		defer func() {
			if r := recover(); r != nil {
				ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("local stream panicked (prompt may exceed the %d-token batch): %v", defaultBatch, r)}
			}
		}()

		lm, err := acquireModel(c.modelPath)
		if err != nil {
			ch <- &modelrepo.StreamParcel{Error: err}
			return
		}
		lm.mu.Lock()
		defer lm.mu.Unlock()

		ctxParams := llama.NewContextParams(defaultNumCtx, defaultBatch, 1, runtime.NumCPU(), ml.FlashAttentionDisabled, "")
		llamaCtx, err := llama.NewContextWithModel(lm.model, ctxParams)
		if err != nil {
			ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("create context: %w", err)}
			return
		}

		tokens, err := lm.model.Tokenize(prompt, true, true)
		if err != nil {
			ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("tokenize: %w", err)}
			return
		}

		batch, err := llama.NewBatch(defaultBatch, 1, 0)
		if err != nil {
			ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("create batch: %w", err)}
			return
		}
		defer batch.Free()

		for i, tok := range tokens {
			batch.Add(tok, nil, i, i == len(tokens)-1, 0)
		}
		if err := llamaCtx.Decode(batch); err != nil {
			ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("decode prompt: %w", err)}
			return
		}

		samplerParams := llama.SamplingParams{TopK: 40, TopP: 0.9, MinP: 0.05, Temp: 0.8}
		if cfg.Temperature != nil {
			samplerParams.Temp = float32(*cfg.Temperature)
		}
		sampler, err := llama.NewSamplingContext(lm.model, samplerParams)
		if err != nil {
			ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("create sampler: %w", err)}
			return
		}

		maxTokens := defaultMaxTokens
		if cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
			maxTokens = *cfg.MaxTokens
		}
		maxTokens, _ = modelrepo.ClampMaxOutputTokens(maxTokens, c.maxOutputTokens)

		for pos := len(tokens); pos < len(tokens)+maxTokens; pos++ {
			select {
			case <-ctx.Done():
				ch <- &modelrepo.StreamParcel{Error: ctx.Err()}
				return
			default:
			}
			id := sampler.Sample(llamaCtx, -1)
			sampler.Accept(id, true)
			if lm.model.TokenIsEog(id) {
				break
			}
			ch <- &modelrepo.StreamParcel{Data: lm.model.TokenToPiece(id)}
			batch.Clear()
			batch.Add(id, nil, pos, true, 0)
			if err := llamaCtx.Decode(batch); err != nil {
				ch <- &modelrepo.StreamParcel{Error: fmt.Errorf("decode token: %w", err)}
				return
			}
		}
	}()
	return ch, nil
}

// localEmbedClient implements modelrepo.LLMEmbedClient using llama.cpp in-process.
type localEmbedClient struct {
	modelPath string
	// contextLength is the model's declared context length (CapabilityConfig);
	// it caps the size of a single embedding input. 0 means "unknown" and falls
	// back to defaultEmbedTokenLimit.
	contextLength int
}

// embedTokenLimit returns the maximum number of tokens a single Embed call will
// accept, derived from the declared context length, with a sane default when
// unknown and a hard upper clamp so an over-large declared context can't trigger
// an absurd single-ubatch allocation.
func (c *localEmbedClient) embedTokenLimit() int {
	limit := c.contextLength
	if limit <= 0 {
		limit = defaultEmbedTokenLimit
	}
	if limit > maxEmbedTokenLimit {
		limit = maxEmbedTokenLimit
	}
	return limit
}

func (c *localEmbedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	lm, err := acquireModel(c.modelPath)
	if err != nil {
		return nil, err
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Tokenize before creating the context so both the context and the batch can
	// be sized to the actual input. Tokenization only needs the model's vocab,
	// not a context.
	tokens, err := lm.model.Tokenize(prompt, true, true)
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}

	// Embedding models (BERT, nomic-bert) are non-causal: the entire input must
	// be processed in a single ubatch, so the batch must hold every token. The
	// limit is purely an error boundary — we never clamp the batch below the
	// token count and proceed, which would silently produce partially-pooled
	// (wrong) embeddings. Exceeding the previous fixed 512 batch used to panic
	// (index out of range in batch.Add); now it sizes to fit or errors clearly.
	limit := c.embedTokenLimit()
	if len(tokens) > limit {
		return nil, fmt.Errorf("embedding input is %d tokens but the limit is %d; raise the model's context length or chunk the input", len(tokens), limit)
	}
	batchSize := max(len(tokens), 1)

	ctxParams := llama.NewContextParams(batchSize, batchSize, 1, runtime.NumCPU(), ml.FlashAttentionDisabled, "")
	llamaCtx, err := llama.NewContextWithModel(lm.model, ctxParams)
	if err != nil {
		return nil, fmt.Errorf("create context: %w", err)
	}

	batch, err := llama.NewBatch(batchSize, 1, 0)
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	defer batch.Free()

	for i, tok := range tokens {
		batch.Add(tok, nil, i, true, 0)
	}
	if err := llamaCtx.Decode(batch); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	emb, err := extractEmbedding(llamaCtx, len(tokens))
	if err != nil {
		return nil, err
	}
	// Ollama normalizes embeddings server-side (server/routes.go normalize());
	// its runner returns them raw. The local provider must do the same so a
	// model produces the same unit vectors regardless of backend, and so
	// L2-distance ANN indexes (which downstream code is tuned against) rank
	// by direction rather than magnitude.
	return l2Normalize(emb)
}

// extractEmbedding returns the sequence embedding for the just-decoded batch.
//
// When the model declares a pooling type (MEAN/CLS/LAST — e.g. nomic, BERT),
// llama.cpp pools internally and GetEmbeddingsSeq(0) returns the pooled vector.
// When pooling_type is NONE, GetEmbeddingsSeq returns nil and only per-token
// hidden states are exposed; we mean-pool them ourselves (the standard
// sentence-transformers recipe) rather than picking a single token, which
// would only be correct for LAST-token pooling.
func extractEmbedding(llamaCtx *llama.Context, numTokens int) ([]float64, error) {
	if pooled := llamaCtx.GetEmbeddingsSeq(0); pooled != nil {
		out := make([]float64, len(pooled))
		for i, v := range pooled {
			out[i] = float64(v)
		}
		return out, nil
	}

	var sum []float64
	n := 0
	for i := range numTokens {
		tok := llamaCtx.GetEmbeddingsIth(i)
		if tok == nil {
			continue
		}
		if sum == nil {
			sum = make([]float64, len(tok))
		}
		for j, v := range tok {
			sum[j] += float64(v)
		}
		n++
	}
	if n == 0 {
		return nil, fmt.Errorf("no embeddings returned (model may not support embedding extraction)")
	}
	for j := range sum {
		sum[j] /= float64(n)
	}
	return sum, nil
}

// l2Normalize scales vec to unit length in place, mirroring the normalization
// Ollama applies to embeddings (server/routes.go normalize): a 1e-12 floor
// guards the zero vector, and NaN/Inf are rejected rather than propagated.
func l2Normalize(vec []float64) ([]float64, error) {
	var sum float64
	for _, v := range vec {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("embedding contains NaN or Inf values")
		}
		sum += v * v
	}
	norm := 1.0 / math.Max(math.Sqrt(sum), 1e-12)
	for i := range vec {
		vec[i] *= norm
	}
	return vec, nil
}
