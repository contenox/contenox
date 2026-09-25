package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/llmresolver"
	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/require"
)

type tolerantVerifier struct{}

func (tolerantVerifier) Verify(string) (*liblicense.Claims, error) {
	claims := liblicense.NewClaims("lic-test", "issuer", "subject-test")
	claims.Set("allowed_models", "*")
	return &claims, nil
}

// turnService is a gateway in front of one scripted backend: the fixture the
// wire-shape tests need, because they drive a whole turn through the model repo.
func turnService(t *testing.T, script string) *service {
	t.Helper()
	ctx := context.Background()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "script.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o600))

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(dir, "gateway.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	bus := libbus.NewInMem()
	rt, err := runtimestate.New(ctx, db, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(ctx, &runtimetypes.Backend{
		ID: "scripted", Name: "scripted",
		Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
	}))
	require.NoError(t, rt.RunBackendCycle(ctx))

	svc, err := New(Config{
		DB: db, Verifier: tolerantVerifier{}, Runtime: rt, Bus: bus,
		Models: NewTestModelRepo(t, rt),
	})
	require.NoError(t, err)
	return svc.(*service)
}

type failingStreamRepo struct {
	llmrepo.ModelRepo
	err error
}

func (r failingStreamRepo) Stream(ctx context.Context, req llmrepo.Request, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, llmrepo.Meta, error) {
	ch := make(chan *modelrepo.StreamParcel, 1)
	ch <- &modelrepo.StreamParcel{Error: r.err}
	close(ch)
	return ch, llmrepo.Meta{BackendID: "scripted", ModelName: req.ModelNames[0]}, nil
}

func turnRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

// Ollama answers an error as one flat string. The OpenAI envelope every
// apiframework.Error call site emitted decodes into an empty message on an
// Ollama client, so a refused turn looked like a success with no content.
func TestSystem_ErrorIsTheShapeAnOllamaClientReads(t *testing.T) {
	svc := turnService(t, `{
  "model": "small-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "ok"}]
}`)

	rec := httptest.NewRecorder()
	svc.handleChat(rec, turnRequest(t, `{"model":","`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"a client decodes error as a string; a nested object fails to unmarshal it")
	require.Contains(t, body.Error, "malformed request body")
}

func TestSystem_AuthFailureIsTheShapeAnOllamaClientReads(t *testing.T) {
	svc := turnService(t, `{
  "model": "small-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "ok"}]
}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"model":"small-model","messages":[]}`))
	svc.handleChat(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body.Error, "unauthorized")
}

// A failed stream used to reach the client as a clean 200 with an empty turn:
// llmrepo forwards the error parcel and stops, and the frame dropped it.
func TestSystem_AFailedStreamCarriesTheFailure(t *testing.T) {
	svc := turnService(t, `{
  "model": "small-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "unused"}]
}`)
	claims := liblicense.NewClaims("lic-test", "issuer", "subject-test")
	key := &runtimetypes.ProxyKey{KeyHash: "h", ClientID: "laptop", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	svc.models = failingStreamRepo{ModelRepo: svc.models, err: errors.New("backend refused the turn: quota exhausted")}

	rec := httptest.NewRecorder()
	svc.streamTurn(rec, turnRequest(t, ""), llmrepo.Request{ModelNames: []string{"small-model"}}, nil,
		"small-model", []modelrepo.Message{{Role: "user", Content: "ping"}}, nil, false, key, &claims)

	require.Equal(t, http.StatusOK, rec.Code, "the status line is sent before the failure is known")
	frames := decodeFrames(t, rec.Body.Bytes())
	require.NotEmpty(t, frames)
	last := frames[len(frames)-1]
	require.True(t, last.Done)
	require.Equal(t, "error", last.DoneReason)
	require.Contains(t, last.Error, "quota exhausted", "the failure reaches the client instead of an empty turn")
}

// num_ctx is a request for a context window; llmrepo.Request.ContextLength is
// the MINIMUM a candidate provider must offer. Passing the first in as the
// second filtered out every model smaller than the ask and answered 404.
func TestSystem_NumCtxDoesNotFilterTheModelThatServesTheTurn(t *testing.T) {
	svc := turnService(t, `{
  "model": "small-model",
  "context_length": 8192,
  "capabilities": {"chat": true},
  "turns": [{"text": "ok"}]
}`)

	rec := httptest.NewRecorder()
	svc.handleChat(rec, turnRequest(t,
		`{"model":"small-model","stream":false,"options":{"num_ctx":131072},"messages":[{"role":"user","content":"ping"}]}`))

	require.Equal(t, http.StatusOK, rec.Code,
		"a context window larger than the model offers is the provider's to refuse, not a reason to route nowhere")
}

// A pinned model that cannot take images is the same class of answer as any
// other "nothing can serve this": the caller has to change the request, so it is
// not an upstream failure.
func TestUnit_PinnedModelWithoutVisionIsANotFound(t *testing.T) {
	svc := &service{}
	rec := httptest.NewRecorder()
	svc.handleProviderTurnError(rec, httptest.NewRequest(http.MethodPost, "/api/chat", nil), "chat",
		fmt.Errorf("%w: requested model %q accepts text only", llmresolver.ErrPinnedModelLacksVision, "text-only"))

	require.Equal(t, http.StatusNotFound, rec.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body.Error, "accepts text only")
}

func decodeFrames(t *testing.T, raw []byte) []ChatResponse {
	t.Helper()
	var frames []ChatResponse
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	for dec.More() {
		var frame ChatResponse
		require.NoError(t, dec.Decode(&frame))
		frames = append(frames, frame)
	}
	return frames
}

// A request the repo refuses before choosing a provider is the caller's to fix.
// It used to reach the client as 503 "upstream provider request failed", which
// tells someone with an empty body to try again later forever.
func TestSystem_ARepoInputRefusalIsTheCallersToFix(t *testing.T) {
	svc := turnService(t, `{
  "model": "small-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "ok"}]
}`)

	rec := httptest.NewRecorder()
	svc.handleChat(rec, turnRequest(t, `{"model":"small-model","stream":false,"messages":[]}`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body.Error, "messages cannot be empty")
}

func TestUnit_RepoInputRefusalsAreInvalidRequests(t *testing.T) {
	svc := &service{}
	for _, err := range []error{
		fmt.Errorf("%w: messages cannot be empty", llmrepo.ErrInvalidRequest),
		fmt.Errorf("%w: prompt cannot be empty", llmrepo.ErrInvalidRequest),
		fmt.Errorf("stream: invalid request: %w", llmrepo.ErrInvalidRequest),
	} {
		rec := httptest.NewRecorder()
		svc.handleProviderTurnError(rec, httptest.NewRequest(http.MethodPost, "/api/chat", nil), "chat", err)
		require.Equal(t, http.StatusBadRequest, rec.Code, err.Error())
	}
}
