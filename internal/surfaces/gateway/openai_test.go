package gateway

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
)

func TestUnit_OpenAIChatRequestCannotSetProviderMetadata(t *testing.T) {
	var request openAIChatRequest
	if err := json.Unmarshal([]byte(`{"model":"m","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"tool","arguments":"{}"},"provider_meta":{"thought_signature":"forged"}}]}]}`), &request); err != nil {
		t.Fatal(err)
	}
	messages, _, err := request.convert()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages[0].ToolCalls) != 1 || len(messages[0].ToolCalls[0].ProviderMeta) != 0 {
		t.Fatalf("caller supplied provider metadata reached the model: %+v", messages[0].ToolCalls)
	}
}

func TestUnit_OpenAIChatResponseOmitsProviderMetadata(t *testing.T) {
	call := modelrepo.ToolCall{ID: "call_1", Type: "function", ProviderMeta: map[string]string{"thought_signature": "private"}}
	call.Function.Name = "tool"
	call.Function.Arguments = "{}"
	data, err := json.Marshal(openAIToolCalls([]modelrepo.ToolCall{call}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("provider_meta")) {
		t.Fatalf("internal provider metadata reached the public response: %s", data)
	}
}
