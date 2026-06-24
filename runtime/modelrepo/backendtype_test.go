package modelrepo

import "testing"

func TestUnit_IsLocalBackendType(t *testing.T) {
	local := []string{"llama", "openvino", "local", "modeld", "LLAMA", " OpenVINO "}
	for _, v := range local {
		if !IsLocalBackendType(v) {
			t.Errorf("IsLocalBackendType(%q) = false, want true", v)
		}
	}
	remote := []string{"ollama", "openai", "gemini", "mistral", "bedrock", "vllm", "", "anthropic"}
	for _, v := range remote {
		if IsLocalBackendType(v) {
			t.Errorf("IsLocalBackendType(%q) = true, want false", v)
		}
	}
}
