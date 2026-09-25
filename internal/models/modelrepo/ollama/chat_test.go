package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnit_Chat_SendsToolCallIDs asserts the pairing ids reach the wire. A tool
// result is matched to its call by string equality upstream, and an
// OpenAI-compatible provider refuses the whole turn when either side is missing
// its id — while Ollama itself neither sends nor needs one, so nothing local
// notices when the conversion drops them.
func TestUnit_Chat_SendsToolCallIDs(t *testing.T) {
	const callID = "call_00_vYiHOCMbz8E11RpCtp1o7524"
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"done"},"done":true,"done_reason":"stop"}`))
	}))
	defer srv.Close()

	provider := NewOllamaProvider("m", []string{srv.URL}, srv.Client(), modelrepo.CapabilityConfig{CanChat: true, CanStream: true}, "", nil)
	conn, err := provider.GetChatConnection(context.Background(), srv.URL)
	require.NoError(t, err)

	_, err = conn.Chat(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "what is this codebase about"},
		{Role: "assistant", ToolCalls: []modelrepo.ToolCall{{
			ID: callID, Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "local_fs.list_dir", Arguments: `{"path":"."}`},
		}}},
		{Role: "tool", Content: "README.md", ToolCallID: callID},
	})
	require.NoError(t, err)

	var sent struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &sent))
	require.Len(t, sent.Messages, 3)

	calls, ok := sent.Messages[1]["tool_calls"].([]any)
	require.True(t, ok, "the assistant turn must carry its tool calls: %s", body)
	require.Len(t, calls, 1)
	assert.Equal(t, callID, calls[0].(map[string]any)["id"],
		"the call's id must survive the conversion, or its result cannot name it")

	assert.Equal(t, callID, sent.Messages[2]["tool_call_id"],
		"the tool result must name the call it answers")
}

func TestUnit_Chat_SendsAudioOnTheAudiosField(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"transcribed"},"done":true,"done_reason":"stop"}`))
	}))
	defer srv.Close()

	provider := NewOllamaProvider("m", []string{srv.URL}, srv.Client(), modelrepo.CapabilityConfig{
		CanChat: true, CanStream: true, CanAudio: true, AudioExtension: true,
	}, "", nil)
	conn, err := provider.GetChatConnection(context.Background(), srv.URL)
	require.NoError(t, err)

	audio := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")
	_, err = conn.Chat(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "transcribe this", Audio: []modelrepo.AudioPart{{Data: audio, MimeType: modelrepo.WAVMimeType}}},
	})
	require.NoError(t, err)

	var sent struct {
		Messages []struct {
			Images []string `json:"images"`
			Audios []string `json:"audios"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &sent))
	require.Len(t, sent.Messages, 1)
	require.Empty(t, sent.Messages[0].Images, "audio is not an image")
	require.Len(t, sent.Messages[0].Audios, 1)
	require.Equal(t, base64.StdEncoding.EncodeToString(audio), sent.Messages[0].Audios[0],
		"the raw WAV bytes are what crosses, base64 in JSON")
}

func TestUnit_Chat_RefusesAudioWithoutTheExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request may reach a peer that does not speak the extension")
	}))
	defer srv.Close()

	provider := NewOllamaProvider("m", []string{srv.URL}, srv.Client(), modelrepo.CapabilityConfig{
		CanChat: true, CanStream: true, CanAudio: true, AudioExtension: false,
	}, "", nil)
	conn, err := provider.GetChatConnection(context.Background(), srv.URL)
	require.NoError(t, err)

	_, err = conn.Chat(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "transcribe this", Audio: []modelrepo.AudioPart{{
			Data: []byte("RIFF\x24\x00\x00\x00WAVEfmt "), MimeType: modelrepo.WAVMimeType,
		}}},
	})
	require.ErrorIs(t, err, modelrepo.ErrAudioNotSupported)
}

func TestUnit_Contenox_ReadsTheHandshake(t *testing.T) {
	for name, tc := range map[string]struct {
		status   int
		body     string
		supports bool
	}{
		"vanilla ollama": {status: http.StatusNotFound, body: `404 page not found`},
		"contenox gateway": {
			status:   http.StatusOK,
			body:     `{"product":"contenox-gateway","version":"v1.0.0","ollama_version":"0.5.1","extensions":["audios"]}`,
			supports: true,
		},
		"a gateway older than the extension": {
			status: http.StatusOK,
			body:   `{"product":"contenox-gateway","version":"v0.9.0","extensions":[]}`,
		},
		"another product": {
			status: http.StatusOK,
			body:   `{"product":"something-else","extensions":["audios"]}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, ContenoxPath, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client, err := newOllamaHTTPClient(srv.URL, "", srv.Client())
			require.NoError(t, err)

			endpoint, err := client.Contenox(context.Background())
			if tc.status != http.StatusOK {
				require.Error(t, err, "a peer without the route is vanilla, and the error is that answer")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.supports, endpoint.IsContenoxGateway() && endpoint.Supports(ExtensionAudios))
		})
	}
}
