package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type subscriptionTransport func(*http.Request) (*http.Response, error)

func (f subscriptionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSystem_Beam_ChatGPTSubscriptionToolApproval(t *testing.T) {
	old := modelrepo.SharedHTTPClient
	t.Cleanup(func() { modelrepo.SharedHTTPClient = old })
	var turns atomic.Int32
	modelrepo.SharedHTTPClient = &http.Client{Transport: subscriptionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "chatgpt.com" {
			return nil, fmt.Errorf("unexpected host in subscription test")
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			return nil, fmt.Errorf("missing subscription authorization")
		}
		body := `{"models":[{"slug":"subscription-test","visibility":"list","context_window":128000}]}`
		if strings.HasSuffix(r.URL.Path, "/responses") {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			var wire struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			}
			if err := json.Unmarshal(raw, &wire); err != nil {
				return nil, err
			}
			switch turns.Add(1) {
			case 1:
				body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"general\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n"
			case 2:
				name := ""
				for _, tool := range wire.Tools {
					if strings.HasSuffix(tool.Name, "git_diff") {
						name = tool.Name
						break
					}
				}
				if name == "" {
					return nil, fmt.Errorf("git_diff missing from subscription tool schema")
				}
				body = fmt.Sprintf("data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"call_id\":\"subscription-call\",\"name\":%q}}\n\ndata: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":1,\"delta\":\"{}\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"reasoning\",\"id\":\"rs_1\",\"encrypted_content\":\"opaque-continuation\",\"summary\":[]}]}}\n\n", name)
			case 3:
				if !strings.Contains(string(raw), "opaque-continuation") || !strings.Contains(string(raw), "function_call_output") {
					return nil, fmt.Errorf("tool result or continuation lost in native loop")
				}
				body = fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n", scriptedFinalAnswer)
			default:
				return nil, fmt.Errorf("unexpected additional subscription turn")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	rt := newBeamRuntimeWithProvider(t, commitMessageDialog, askEverything, modelauth.ProviderType, "subscription-test", func(db libdb.DBManager) {
		ctx := context.Background()
		store := runtimetypes.New(db.WithoutTransaction())
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{ID: "subscription", Name: "chatgpt", Type: modelauth.ProviderType, BaseURL: modelauth.BaseURL}))
		raw, err := json.Marshal(map[string]any{"generation": "fixture", "account": "fixture-account", "token": &oauth2.Token{AccessToken: "test-token", RefreshToken: "unused", Expiry: time.Now().Add(time.Hour)}})
		require.NoError(t, err)
		require.NoError(t, store.SetKV(ctx, "model-oauth:subscription", raw))
	})
	beam := rt.openBeam()
	beam.submit("can you suggest a commit message?")
	ask := rt.waitForPendingAsk()
	require.Equal(t, "git_diff", ask.ToolName)
	beam.waitFor("approval required", "subscription tool must use native approval routing")
	beam.allow()
	beam.waitFor(scriptedFinalAnswer, "native loop must resume with the subscription tool result")
	rt.requireGatedToolRan()
	rt.requireApprovedAndClosed(ask.ID)
	require.EqualValues(t, 3, turns.Load())
}
