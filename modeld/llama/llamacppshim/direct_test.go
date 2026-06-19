//go:build llamacpp_direct

package llamacppshim

import (
	"os"
	"strings"
	"testing"
)

func TestDirectShimBackendInfo(t *testing.T) {
	info := SystemInfo()
	if strings.TrimSpace(info) == "" {
		t.Fatal("SystemInfo returned empty string")
	}
	devices := Devices()
	if len(devices) == 0 {
		t.Fatal("expected at least one ggml backend device")
	}
	t.Logf("system_info=%s", info)
	for _, dev := range devices {
		t.Logf("device[%d] type=%s name=%q desc=%q free=%d total=%d",
			dev.Index, dev.Type, dev.Name, dev.Description, dev.MemoryFree, dev.MemoryTotal)
	}
}

func TestDirectShimLoadTinyModel(t *testing.T) {
	path := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if path == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to test direct model load/tokenize")
	}
	model, err := LoadModel(path, ModelConfig{UseMmap: true})
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()

	if got := model.ContextTrain(); got <= 0 {
		t.Fatalf("ContextTrain = %d, want > 0", got)
	}
	toks, err := model.Tokenize("hello from contenox", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) == 0 {
		t.Fatal("Tokenize returned no tokens")
	}
	t.Logf("model=%s n_ctx_train=%d tokens=%v", model.Description(), model.ContextTrain(), toks)
}
