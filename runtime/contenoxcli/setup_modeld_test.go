package contenoxcli

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnit_LocalModeldSourceBuildStepsKeepModelChoices(t *testing.T) {
	oldVersion := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = oldVersion })

	var llama bytes.Buffer
	printLocalModeldSourceBuildSteps(&llama, "llama")
	got := llama.String()
	for _, want := range []string{
		"source-built modeld daemon",
		"git clone --branch v9.9.9",
		"CONTENOX_MODELD_BACKEND=llama make run-modeld",
		"VRAM     Model",
		"qwen3-4b",
		"qwen3-8b",
		"Optional VS Code autocomplete model",
		"default-autocomplete-provider llama",
		"default-autocomplete-model qwen3-coder-30b-a3b",
		"contenox model registry-list",
		"docs/modeld-source-build.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("llama modeld setup text missing %q:\n%s", want, got)
		}
	}

	var openvino bytes.Buffer
	printLocalModeldSourceBuildSteps(&openvino, "openvino")
	got = openvino.String()
	for _, want := range []string{
		"make deps-modeld",
		"CONTENOX_MODELD_BACKEND=openvino make run-modeld",
		"qwen2.5-coder-0.5b-ov",
		"gemma4-e4b-ov",
		"default-autocomplete-provider openvino",
		"default-autocomplete-model qwen2.5-coder-1.5b-ov",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("openvino modeld setup text missing %q:\n%s", want, got)
		}
	}
}
