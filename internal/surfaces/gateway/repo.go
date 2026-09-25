package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/trouble"
	"github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
)

// Repo meters native model calls through the gateway's accounting path.
// NewRepo also enforces bearer allowances; NewLocalRepo records local usage
// without authorization or spending limits.
type Repo struct {
	svc    *service
	bearer string
	local  *caller
}

var _ llmrepo.ModelRepo = (*Repo)(nil)

// NewRepo returns the gateway as a model repo for one identity. The bearer is
// the licence whose allowance every call spends.
func NewRepo(svc Service, bearer string) (*Repo, error) {
	s, ok := svc.(*service)
	if !ok {
		return nil, errors.New("gateway: the service is not a gateway")
	}
	if s == nil {
		return nil, errors.New("gateway: a service is required")
	}
	return &Repo{svc: s, bearer: bearer}, nil
}

// LocalClientID identifies harness calls made with this installation's provider credentials.
const LocalClientID = "local-harness"

// NewLocalRepo meters harness calls without licenses or allowance enforcement.
// Usage is recorded directly in the shared transactional ledger; no HTTP server
// or message-bus consumer is needed.
func NewLocalRepo(db libdbexec.DBManager, models llmrepo.ModelRepo, state *runtimestate.State, tracker libtracker.ActivityTracker) (*Repo, error) {
	svc, err := newService(Config{DB: db, Models: models, Runtime: state, Tracker: tracker,
		Trouble: trouble.NewRecorder(db, tracker, nil)})
	if err != nil {
		return nil, err
	}
	return &Repo{svc: svc, local: &caller{key: &runtimetypes.ProxyKey{
		KeyHash: LocalClientID, ClientID: LocalClientID,
	}}}, nil
}

func (r *Repo) authorize(ctx context.Context, who caller, model string) error {
	if r.local != nil {
		return nil
	}
	return r.svc.authorizeCaller(ctx, who, model)
}

func (r *Repo) caller(ctx context.Context) (caller, error) {
	if r.local != nil {
		return *r.local, nil
	}
	return r.svc.callerFromBearer(ctx, r.bearer)
}

// Chat runs one turn and charges what it reported, refusing before the call when
// the caller is out of allowance.
func (r *Repo) Chat(ctx context.Context, req llmrepo.Request, messages []modelrepo.Message, opts ...modelrepo.ChatArgument) (modelrepo.ChatResult, llmrepo.Meta, error) {
	who, err := r.caller(ctx)
	if err != nil {
		return modelrepo.ChatResult{}, llmrepo.Meta{}, err
	}
	model := namedModel(req)

	if err := r.authorize(ctx, who, model); err != nil {
		return modelrepo.ChatResult{}, llmrepo.Meta{}, err
	}

	start := time.Now()
	res, meta, err := r.svc.models.Chat(ctx, req, messages, opts...)
	r.charge(context.WithoutCancel(ctx), who, meta, model, messages, res.Usage, res.FinishReason, start)
	return res, meta, err
}

// Stream forwards native parcels and records reported usage on completion or cancellation.
// Providers that omit usage on interruption leave their final consumption unknown.
func (r *Repo) Stream(ctx context.Context, req llmrepo.Request, messages []modelrepo.Message, opts ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, llmrepo.Meta, error) {
	who, err := r.caller(ctx)
	if err != nil {
		return nil, llmrepo.Meta{}, err
	}
	model := namedModel(req)
	if err := r.authorize(ctx, who, model); err != nil {
		return nil, llmrepo.Meta{}, err
	}

	start := time.Now()
	upstream, meta, err := r.svc.models.Stream(ctx, req, messages, opts...)
	if err != nil {
		return nil, meta, err
	}

	out := make(chan *modelrepo.StreamParcel)
	go func() {
		defer close(out)
		var usage *modelrepo.TokenUsage
		finish := ""
		var counters modelrepo.TokenUsage
		defer func() { r.charge(context.WithoutCancel(ctx), who, meta, model, messages, usage, finish, start) }()
		for {
			var parcel *modelrepo.StreamParcel
			select {
			case <-ctx.Done():
				return
			case p, ok := <-upstream:
				if !ok {
					return
				}
				parcel = p
			}
			if parcel == nil {
				continue
			}
			if parcel.Usage != nil {
				counters.Merge(parcel.Usage)
				usage = &counters
			}
			if parcel.Terminal != nil {
				if parcel.Terminal.Usage != nil {
					counters.Merge(parcel.Terminal.Usage)
					usage = &counters
				}
				finish = parcel.Terminal.FinishReason
			}
			select {
			case out <- parcel:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, meta, nil
}

func (r *Repo) Embed(ctx context.Context, embedReq llmrepo.EmbedRequest, prompt string) ([]float64, llmrepo.Meta, error) {
	who, err := r.caller(ctx)
	if err != nil {
		return nil, llmrepo.Meta{}, err
	}
	model := embedReq.ModelName
	if err := r.authorize(ctx, who, model); err != nil {
		return nil, llmrepo.Meta{}, err
	}

	start := time.Now()
	vectors, meta, err := r.svc.models.Embed(ctx, embedReq, prompt)
	if err != nil {
		return nil, meta, err
	}
	if meta.ModelName != "" {
		model = meta.ModelName
	}
	promptTokens := int64(0)
	if count, err := r.svc.tokenizer.CountTokens(ctx, model, prompt); err == nil {
		promptTokens = int64(count)
	}
	r.svc.chargeTurn(context.WithoutCancel(ctx), who.key, who.claims, turnUsage{
		backend: meta.BackendID, model: model, prompt: promptTokens,
		durationMs: time.Since(start).Milliseconds(), finishReason: embedFinishReason,
	})
	return vectors, meta, nil
}

// PromptExecute is a completion rather than a turn: the provider reports its own
// usage, and the charge is the same meter a chat turn lands on.
func (r *Repo) PromptExecute(ctx context.Context, req llmrepo.Request, systemInstruction string, temperature float32, prompt string) (string, llmrepo.Meta, error) {
	who, err := r.caller(ctx)
	if err != nil {
		return "", llmrepo.Meta{}, err
	}
	model := namedModel(req)
	if err := r.authorize(ctx, who, model); err != nil {
		return "", llmrepo.Meta{}, err
	}

	start := time.Now()
	text, meta, err := r.svc.models.PromptExecute(ctx, req, systemInstruction, temperature, prompt)
	r.charge(context.WithoutCancel(ctx), who, meta, model, nil, meta.Usage, "stop", start)
	return text, meta, err
}

func (r *Repo) Tokenize(ctx context.Context, modelName string, prompt string) ([]int, error) {
	return r.svc.models.Tokenize(ctx, modelName, prompt)
}

func (r *Repo) CountTokens(ctx context.Context, modelName string, prompt string) (int, error) {
	return r.svc.models.CountTokens(ctx, modelName, prompt)
}

func (r *Repo) charge(ctx context.Context, who caller, meta llmrepo.Meta, model string, messages []modelrepo.Message, usage *modelrepo.TokenUsage, finish string, start time.Time) {
	if meta.ModelName != "" {
		model = meta.ModelName
	}
	var prompt, completion, thinking, cacheRead, cacheWrite int64
	if usage != nil {
		prompt = int64(usage.PromptTokens)
		completion = int64(usage.CompletionTokens)
		thinking = int64(usage.ThinkingTokens)
		cacheRead = int64(usage.CacheReadTokens)
		cacheWrite = int64(usage.CacheWriteTokens)
	}
	images, audioBytes := 0, 0
	if len(messages) > 0 {
		images = modelrepo.MessagesImageCount(messages)
		_, audioBytes = modelrepo.MessagesAudioBytes(messages)
	}
	r.svc.chargeTurn(context.WithoutCancel(ctx), who.key, who.claims, turnUsage{
		backend: meta.BackendID, model: model,
		prompt: prompt, completion: completion,
		thinking:  thinking,
		cacheRead: cacheRead, cacheWrite: cacheWrite,
		images: int64(images), audioBytes: int64(audioBytes),
		durationMs: time.Since(start).Milliseconds(), finishReason: finish,
	})
}

// namedModel is the model this request will resolve to: the first name it states,
// which is what the allowance is keyed on and what the caller named.
func namedModel(req llmrepo.Request) string {
	if len(req.ModelNames) > 0 {
		return req.ModelNames[0]
	}
	return ""
}
