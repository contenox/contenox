package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/llmresolver"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/trouble"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_ModelAllowed(t *testing.T) {
	require.True(t, modelAllowed([]string{"*"}, "anything"))
	require.True(t, modelAllowed([]string{"a", "b"}, "a"))
	require.False(t, modelAllowed([]string{"a"}, "b"))
	require.False(t, modelAllowed(nil, "a"))
	require.False(t, modelAllowed([]string{"a"}, ""))
}

func TestUnit_ModelDetailsFromRuntime(t *testing.T) {
	d := modelDetailsFromRuntime(runtimestate.ModelDetails{
		Format:            "gguf",
		Family:            "qwen2",
		ParameterSize:     "7B",
		QuantizationLevel: "Q4_K_M",
	})
	require.Equal(t, "gguf", d.Format)
	require.Equal(t, "qwen2", d.Family)
	require.Equal(t, "7B", d.ParameterSize)
	require.Equal(t, "Q4_K_M", d.QuantizationLevel)
}

func TestUnit_ParcelToChatResponse_TerminalCarriesUsage(t *testing.T) {
	p := parcelToChatResponse("m", &modelrepo.StreamParcel{
		Terminal: &modelrepo.StreamTerminal{
			FinishReason: "stop",
			Usage:        &modelrepo.TokenUsage{PromptTokens: 7, CompletionTokens: 3},
		},
	})
	require.True(t, p.Done)
	require.Equal(t, "stop", p.DoneReason)
	require.Equal(t, 7, p.PromptEvalCount)
	require.Equal(t, 3, p.EvalCount)
}

func TestUnit_HandleProviderTurnError_SanitizesSensitiveUpstreamErrors(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "runtime.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	rec := trouble.NewRecorder(db, libtracker.NoopTracker{}, func() time.Time { return time.Now().UTC() })
	svc := &service{trouble: rec}

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedMsg    string
		unexpectedSub  string
	}{
		{
			name: "access denied leaks no master key or internal provider message",
			err: fmt.Errorf("%w: OpenAI API returned non-200 status: 401, Type: invalid_api_key, Key: sk-secret-master-key-12345",
				modelrepo.ErrModelAccessDenied),
			expectedStatus: http.StatusServiceUnavailable,
			expectedMsg:    "upstream provider authentication or access failed; please contact your administrator",
			unexpectedSub:  "sk-secret-master-key-12345",
		},
		{
			name: "rate limit / quota exhaustion sanitized",
			err: fmt.Errorf("%w: OpenAI API returned non-200 status: 429, Type: insufficient_quota, Message: You exceeded your current quota",
				modelrepo.ErrRateLimited),
			expectedStatus: http.StatusTooManyRequests,
			expectedMsg:    "upstream provider rate limit or capacity limit reached; please try again later",
			unexpectedSub:  "insufficient_quota",
		},
		{
			name: "context length exceeded is user-actionable bad request",
			err: fmt.Errorf("%w: prompt is too long: 45000 > 32768",
				modelrepo.ErrContextLengthExceeded),
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "request exceeds the model's context window",
			unexpectedSub:  "45000",
		},
		{
			name: "model not found on backend sanitized",
			err: fmt.Errorf("%w: backend vllm-internal-node-4 does not serve gpt-4o",
				modelrepo.ErrModelNotFoundOnBackend),
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "upstream provider does not serve the requested model",
			unexpectedSub:  "vllm-internal-node-4",
		},
		{
			name:           "no model in runtime state is a not-found, not an upstream failure",
			err:            fmt.Errorf("embed: client resolution failed: %w", llmresolver.ErrNoAvailableModels),
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "embed: client resolution failed: no models found in runtime state",
		},
		{
			name: "a model that exists but cannot serve the request is a not-found",
			err: fmt.Errorf("embed: client resolution failed: failed to apply resolver: %w",
				fmt.Errorf("%w: no available model supports image input (vision)", llmresolver.ErrNoSatisfactoryModel)),
			expectedStatus: http.StatusNotFound,
			expectedMsg: "embed: client resolution failed: failed to apply resolver: no model matched the requirements: " +
				"no available model supports image input (vision)",
		},
		{
			name:           "generic upstream failure sanitized",
			err:            errors.New("raw upstream TLS handshake failed with internal-proxy.corp:8443"),
			expectedStatus: http.StatusServiceUnavailable,
			expectedMsg:    "upstream provider request failed; please try again later",
			unexpectedSub:  "internal-proxy.corp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recWriter := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/chat", nil)

			svc.handleProviderTurnError(recWriter, req, "chat", tt.err)

			assert.Equal(t, tt.expectedStatus, recWriter.Code)

			var body struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal(recWriter.Body.Bytes(), &body),
				"the answer has to be the shape an Ollama client reads")
			assert.Equal(t, tt.expectedMsg, body.Error)
			if tt.unexpectedSub != "" {
				assert.NotContains(t, recWriter.Body.String(), tt.unexpectedSub)
			}
		})
	}
}

func TestUnit_SanitizeTurnError_PreservesSentinelsWithoutLeaking(t *testing.T) {
	rawErr := fmt.Errorf("%w: Anthropic returned 401 with api_key=sk-ant-admin-key", modelrepo.ErrModelAccessDenied)
	sanitized := sanitizeTurnError(rawErr)

	require.Error(t, sanitized)
	require.True(t, errors.Is(sanitized, modelrepo.ErrModelAccessDenied))
	assert.NotContains(t, sanitized.Error(), "sk-ant-admin-key")
	assert.Equal(t, "backend denied access to the requested model: upstream provider access or authentication failed", sanitized.Error())

	rateLimitErr := fmt.Errorf("%w: 429 quota exhausted on billing org-999", modelrepo.ErrRateLimited)
	sanitizedRate := sanitizeTurnError(rateLimitErr)
	require.True(t, errors.Is(sanitizedRate, modelrepo.ErrRateLimited))
	assert.NotContains(t, sanitizedRate.Error(), "org-999")
}

func TestUnit_RecordTurnFailure_ClassifiesTrouble(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "runtime.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	rec := trouble.NewRecorder(db, libtracker.NoopTracker{}, func() time.Time { return time.Now().UTC() })
	svc := &service{trouble: rec}

	accessDeniedErr := fmt.Errorf("%w: invalid key", modelrepo.ErrModelAccessDenied)
	svc.recordTurnFailure(ctx, "chat", accessDeniedErr)

	events, err := runtimetypes.NewEventStore(db).ListEventsSince(ctx, trouble.PlatformScope, 0, runtimetypes.MaxEventListLimit)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	var payload struct {
		Surface   string `json:"surface"`
		Operation string `json:"operation"`
	}
	require.NoError(t, json.Unmarshal(events[0].Data, &payload))
	assert.Equal(t, "gateway", payload.Surface)
	assert.Equal(t, "upstream_access_denied", payload.Operation)
}
