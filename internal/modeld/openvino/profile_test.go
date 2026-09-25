package openvino

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnit_ParserProtocolsFromModelProfile(t *testing.T) {
	dir := t.TempDir()
	profile := `{"tool_calls":{"protocol":"openvino:llama3_json_tool_parser"},"reasoning":{"protocol":"openvino:reasoning_parser"}}`
	path := filepath.Join(dir, "contenox-openvino.json")
	if err := os.WriteFile(path, []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := parserProtocolsFromProfile(dir)
	want := []string{"openvino:llama3_json_tool_parser", "openvino:reasoning_parser"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("protocols = %v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := parserProtocolsFromProfile(dir); err == nil {
		t.Fatal("invalid profile silently accepted")
	}
}

func TestUnit_DeviceEnvironmentOverridesModelProfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contenox-openvino.json"), []byte(`{"device":"CPU"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENOX_OPENVINO_DEVICE", "GPU.1")
	if got := genAIConfigFromProfile(dir, "GPU.1").Device; got != "GPU.1" {
		t.Fatalf("device = %s", got)
	}
	t.Setenv("CONTENOX_OPENVINO_DEVICE", "")
	if got := genAIConfigFromProfile(dir, "AUTO").Device; got != "CPU" {
		t.Fatalf("profile fallback = %s", got)
	}
}
