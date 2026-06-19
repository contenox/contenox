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
		"granite-3.2-2b",
		"qwen3-8b",
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
		"phi-4-mini-ov",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("openvino modeld setup text missing %q:\n%s", want, got)
		}
	}
}
