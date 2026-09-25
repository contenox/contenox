package libollama_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/contenox/contenox/libollama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationSerialization(t *testing.T) {
	d := libollama.Duration{Duration: 10 * time.Minute}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Equal(t, `"10m0s"`, string(b))

	var parsed libollama.Duration
	err = json.Unmarshal([]byte(`"15m0s"`), &parsed)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Minute, parsed.Duration)

	err = json.Unmarshal([]byte(`120`), &parsed)
	require.NoError(t, err)
	assert.Equal(t, 120*time.Second, parsed.Duration)

	dNeg := libollama.Duration{Duration: -1}
	bNeg, err := json.Marshal(dNeg)
	require.NoError(t, err)
	assert.Equal(t, `-1`, string(bNeg))
}

func TestThinkValueSerialization(t *testing.T) {
	tv := libollama.ThinkValue{Value: "high"}
	b, err := json.Marshal(&tv)
	require.NoError(t, err)
	assert.Equal(t, `"high"`, string(b))

	var parsed libollama.ThinkValue
	err = json.Unmarshal([]byte(`true`), &parsed)
	require.NoError(t, err)
	assert.Equal(t, true, parsed.Value)

	err = json.Unmarshal([]byte(`"low"`), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "low", parsed.Value)

	err = json.Unmarshal([]byte(`"invalid"`), &parsed)
	assert.Error(t, err)
}

func TestMessageRoleLowercase(t *testing.T) {
	raw := `{"role":"USER","content":"hello"}`
	var msg libollama.Message
	err := json.Unmarshal([]byte(raw), &msg)
	require.NoError(t, err)
	assert.Equal(t, "user", msg.Role)
	assert.Equal(t, "hello", msg.Content)
}

func TestChatRoundtrip(t *testing.T) {
	stream := true
	req := libollama.ChatRequest{
		Model: "deepseek-chat",
		Messages: []libollama.Message{
			{Role: "user", Content: "ping"},
		},
		Stream: &stream,
	}

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded libollama.ChatRequest
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-chat", decoded.Model)
	assert.Len(t, decoded.Messages, 1)
	assert.Equal(t, "user", decoded.Messages[0].Role)
	assert.Equal(t, "ping", decoded.Messages[0].Content)
	assert.True(t, *decoded.Stream)
}
