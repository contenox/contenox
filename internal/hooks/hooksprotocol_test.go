package hooks_test

import (
	"testing"

	"github.com/contenox/runtime/internal/hooks"
	"github.com/stretchr/testify/require"
)

func TestUnit_OpenAIProtocol(t *testing.T) {
	handler := &hooks.OpenAIProtocol{}
	toolName := "openai_tool"
	args := map[string]any{"arg1": "val1"}
	bodyProps := map[string]any{"access_token": "secret-token"}
	expectedRequest := `{"name":"openai_tool","arguments":"{\"access_token\":\"secret-token\",\"arg1\":\"val1\"}"}`

	t.Run("BuildRequest", func(t *testing.T) {
		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("BuildRequest_NoBodyProps", func(t *testing.T) {
		expectedRequestNoProps := `{"name":"openai_tool","arguments":"{\"arg1\":\"val1\"}"}`
		body, err := handler.BuildRequest(toolName, args, nil)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequestNoProps, string(body))
	})

	t.Run("ParseResponse", func(t *testing.T) {
		responseBody := []byte(`{"status":"ok"}`)
		output, err := handler.ParseResponse(responseBody)
		require.NoError(t, err)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ok", outputMap["status"])
	})
}

func TestUnit_OllamaProtocol(t *testing.T) {
	handler := &hooks.OllamaProtocol{}
	toolName := "ollama_tool"
	args := map[string]any{"arg1": "val1"}
	bodyProps := map[string]any{"access_token": "secret-token"}
	expectedRequest := `{"name":"ollama_tool","arguments":{"access_token":"secret-token","arg1":"val1"}}`

	t.Run("BuildRequest", func(t *testing.T) {
		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("BuildRequest_NoBodyProps", func(t *testing.T) {
		expectedRequestNoProps := `{"name":"ollama_tool","arguments":{"arg1":"val1"}}`
		body, err := handler.BuildRequest(toolName, args, nil)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequestNoProps, string(body))
	})

	t.Run("ParseResponse_Success", func(t *testing.T) {
		responseBody := []byte(`{"message":{"content":{"status":"ok"}}}`)
		output, err := handler.ParseResponse(responseBody)
		require.NoError(t, err)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ok", outputMap["status"])
	})

	t.Run("ParseResponse_Failure", func(t *testing.T) {
		responseBody := []byte(`{"message":{"wrong_key":"value"}}`)
		_, err := handler.ParseResponse(responseBody)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing 'message.content' field")
	})
}

func TestUnit_LangServeOpenAIProtocol(t *testing.T) {
	handler := &hooks.LangServeOpenAIProtocol{}
	toolName := "langserve_openai_tool"
	args := map[string]any{"arg1": "val1"}
	bodyProps := map[string]any{"access_token": "secret-token"}
	expectedRequest := `{"name":"langserve_openai_tool","arguments":"{\"access_token\":\"secret-token\",\"arg1\":\"val1\"}"}`

	t.Run("BuildRequest", func(t *testing.T) {
		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("BuildRequest_NoBodyProps", func(t *testing.T) {
		expectedRequestNoProps := `{"name":"langserve_openai_tool","arguments":"{\"arg1\":\"val1\"}"}`
		body, err := handler.BuildRequest(toolName, args, nil)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequestNoProps, string(body))
	})

	t.Run("ParseResponse_Success", func(t *testing.T) {
		responseBody := []byte(`{"output":{"status":"ok"}, "other_field": "ignored"}`)
		output, err := handler.ParseResponse(responseBody)
		require.NoError(t, err)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ok", outputMap["status"])
	})

	t.Run("ParseResponse_Failure", func(t *testing.T) {
		responseBody := []byte(`{"wrong_key":{"status":"ok"}}`)
		_, err := handler.ParseResponse(responseBody)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing 'output' field")
	})
}

func TestUnit_LangServeDirectProtocol(t *testing.T) {
	handler := &hooks.LangServeDirectProtocol{}
	toolName := "langserve_direct_tool"
	args := map[string]any{"arg1": "val1"}
	bodyProps := map[string]any{"access_token": "secret-token"}
	expectedRequest := `{"access_token":"secret-token","arg1":"val1"}`

	t.Run("BuildRequest", func(t *testing.T) {
		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("BuildRequest_NoBodyProps", func(t *testing.T) {
		expectedRequestNoProps := `{"arg1":"val1"}`
		body, err := handler.BuildRequest(toolName, args, nil)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequestNoProps, string(body))
	})

	t.Run("ParseResponse", func(t *testing.T) {
		responseBody := []byte(`{"status":"ok"}`)
		output, err := handler.ParseResponse(responseBody)
		require.NoError(t, err)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ok", outputMap["status"])
	})
}

func TestUnit_OpenAIObjectProtocol(t *testing.T) {
	handler := &hooks.OpenAIObjectProtocol{}
	toolName := "openai_object_tool"
	args := map[string]any{"arg1": "val1"}
	bodyProps := map[string]any{"access_token": "secret-token"}
	expectedRequest := `{"name":"openai_object_tool","arguments":{"access_token":"secret-token","arg1":"val1"}}`

	t.Run("BuildRequest", func(t *testing.T) {
		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("BuildRequest_NoBodyProps", func(t *testing.T) {
		expectedRequestNoProps := `{"name":"openai_object_tool","arguments":{"arg1":"val1"}}`
		body, err := handler.BuildRequest(toolName, args, nil)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequestNoProps, string(body))
	})

	t.Run("ParseResponse", func(t *testing.T) {
		responseBody := []byte(`{"status":"ok"}`)
		output, err := handler.ParseResponse(responseBody)
		require.NoError(t, err)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ok", outputMap["status"])
	})
}

// This test suite is designed to expose the bug in OpenAIObjectProtocol.ParseResponse.
func TestUnit_OpenAIObjectProtocol_BugHunt(t *testing.T) {
	handler := &hooks.OpenAIObjectProtocol{}

	t.Run("ParseResponse with standard OpenAI response", func(t *testing.T) {
		responseBody := []byte(`{"status":"ok"}`)
		_, err := handler.ParseResponse(responseBody)
		require.NoError(t, err, "This test fails because ParseResponse incorrectly delegates to Ollama's parser instead of OpenAI's")
	})

	t.Run("ParseResponse with Ollama-style response (proves correct non-unwrapping behavior)", func(t *testing.T) {
		responseBody := []byte(`{"message":{"content":{"status":"ok"}}}`)
		output, err := handler.ParseResponse(responseBody)

		require.NoError(t, err)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok)

		messageField, ok := outputMap["message"].(map[string]any)
		require.True(t, ok, "The 'message' field should exist because the response is no longer unwrapped")

		contentField, ok := messageField["content"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ok", contentField["status"])
	})
}

// New tests for body properties merging behavior
func TestUnit_Protocols_BodyProperties_Merging(t *testing.T) {
	t.Run("OpenAIProtocol_Merge_With_Conflict", func(t *testing.T) {
		handler := &hooks.OpenAIProtocol{}
		toolName := "test_tool"
		args := map[string]any{"access_token": "arg-token", "arg1": "val1"}
		bodyProps := map[string]any{"access_token": "body-token", "env": "prod"}

		// Arguments should take precedence over body properties in case of conflict
		expectedRequest := `{"name":"test_tool","arguments":"{\"access_token\":\"arg-token\",\"arg1\":\"val1\",\"env\":\"prod\"}"}`

		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("OllamaProtocol_Merge_With_Conflict", func(t *testing.T) {
		handler := &hooks.OllamaProtocol{}
		toolName := "test_tool"
		args := map[string]any{"access_token": "arg-token", "arg1": "val1"}
		bodyProps := map[string]any{"access_token": "body-token", "env": "prod"}

		// Arguments should take precedence over body properties in case of conflict
		expectedRequest := `{"name":"test_tool","arguments":{"access_token":"arg-token","arg1":"val1","env":"prod"}}`

		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})

	t.Run("LangServeDirectProtocol_Merge_With_Conflict", func(t *testing.T) {
		handler := &hooks.LangServeDirectProtocol{}
		toolName := "test_tool"
		args := map[string]any{"access_token": "arg-token", "arg1": "val1"}
		bodyProps := map[string]any{"access_token": "body-token", "env": "prod"}

		// Arguments should take precedence over body properties in case of conflict
		expectedRequest := `{"access_token":"arg-token","arg1":"val1","env":"prod"}`

		body, err := handler.BuildRequest(toolName, args, bodyProps)
		require.NoError(t, err)
		require.JSONEq(t, expectedRequest, string(body))
	})
}
