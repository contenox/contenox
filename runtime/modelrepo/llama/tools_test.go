package llama

import (
	"encoding/json"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func TestUnit_LlamaSerializeToolDefs_NormalizesDescriptionOnlySchema(t *testing.T) {
	body := serializedToolParameters(t, modelrepo.Tool{
		Type: "function",
		Function: &modelrepo.FunctionTool{
			Name: "web_post",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"body": map[string]any{"description": "Request body."},
				},
			},
		},
	})

	props := body["properties"].(map[string]any)
	bodyProp := props["body"].(map[string]any)
	if bodyProp["type"] != "value" {
		t.Fatalf("description-only schema type = %#v, want value", bodyProp["type"])
	}
	if bodyProp["description"] != "Request body." {
		t.Fatalf("description was not preserved: %#v", bodyProp["description"])
	}
}

func TestUnit_LlamaSerializeToolDefs_AlwaysEmitsDescriptionAndParameters(t *testing.T) {
	raw, err := serializeToolDefs([]modelrepo.Tool{{
		Type:     "function",
		Function: &modelrepo.FunctionTool{Name: "noop"},
	}})
	if err != nil {
		t.Fatalf("serializeToolDefs: %v", err)
	}

	var tools []map[string]any
	if err := json.Unmarshal([]byte(raw), &tools); err != nil {
		t.Fatalf("unmarshal serialized tools: %v", err)
	}
	fn := tools[0]["function"].(map[string]any)
	if _, ok := fn["description"]; !ok {
		t.Fatal("description key must be emitted even when empty")
	}
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %T, want object", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Fatalf("default parameters type = %#v, want object", params["type"])
	}
}

func TestUnit_LlamaSerializeToolDefs_NormalizesBooleanSchemas(t *testing.T) {
	params := serializedToolParameters(t, modelrepo.Tool{
		Type: "function",
		Function: &modelrepo.FunctionTool{
			Name: "accept_any",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payload": true,
				},
			},
		},
	})

	props := params["properties"].(map[string]any)
	payload := props["payload"].(map[string]any)
	if payload["type"] != "value" {
		t.Fatalf("boolean true schema type = %#v, want value", payload["type"])
	}
}

func serializedToolParameters(t *testing.T, tool modelrepo.Tool) map[string]any {
	t.Helper()
	raw, err := serializeToolDefs([]modelrepo.Tool{tool})
	if err != nil {
		t.Fatalf("serializeToolDefs: %v", err)
	}
	var tools []map[string]any
	if err := json.Unmarshal([]byte(raw), &tools); err != nil {
		t.Fatalf("unmarshal serialized tools: %v", err)
	}
	fn := tools[0]["function"].(map[string]any)
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %T, want object", fn["parameters"])
	}
	return params
}
