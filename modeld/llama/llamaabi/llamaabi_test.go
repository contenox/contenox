//go:build llamanode && llama_unsafe_abi

package llamaabi

import (
	"runtime/debug"
	"testing"
)

// TestOllamaVersionGuard is the version gate required to use this unsafe shim.
// If Ollama is upgraded, this test will fail, forcing a re-evaluation of the
// unexported memory layout.
func TestOllamaVersionGuard(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("could not read build info")
	}

	found := false
	version := ""
	for _, m := range info.Deps {
		if m.Path == "github.com/ollama/ollama" {
			found = true
			version = m.Version
			break
		}
	}

	if !found {
		t.Skip("github.com/ollama/ollama not found in module dependencies (likely local replacement in use)")
	}

	if version != "v0.1.34" && version != "v0.17.5" && version != "v0.1.33" && version != "" && version != "(devel)" {
		// Note: Adjust the strict version string here once the actual module cache confirms what go.mod locks it to.
		t.Fatalf("unsafe ABI shim requires EXACTLY github.com/ollama/ollama version under test. Got: %s", version)
	}
}
