package modeldprobe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnit_BackendPreferenceRoundTrip(t *testing.T) {
	root := t.TempDir()
	if value, err := PreferredBackend(root); err != nil || value != "" {
		t.Fatalf("missing preference=%q %v", value, err)
	}
	for _, backend := range []string{"llama", "openvino"} {
		if err := SetBackendPreference(root, backend); err != nil {
			t.Fatal(err)
		}
		if value, err := PreferredBackend(root); err != nil || value != backend {
			t.Fatalf("preference=%q %v", value, err)
		}
	}
	if err := SetBackendPreference(root, "other"); err == nil {
		t.Fatal("invalid backend accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "modeld-backend"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PreferredBackend(root); err == nil {
		t.Fatal("invalid persisted backend accepted")
	}
}
