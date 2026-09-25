package modelstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnit_ModelNamesCannotEscapeStore(t *testing.T) {
	root := t.TempDir()
	storeDir := filepath.Join(root, "models")
	outside := filepath.Join(root, "outside", "model.gguf")
	writeFile(t, outside, []byte("keep"))
	admin := NewAdmin(storeDir)
	for _, name := range []string{"..", ".", "../outside", "sub/../../outside", `..\outside`, "/outside", `C:\outside`} {
		t.Run(name, func(t *testing.T) {
			if _, err := Resolve(storeDir, name, "llama", ""); err == nil {
				t.Fatal("resolved unsafe name")
			}
			if err := admin.RemoveModel(context.Background(), name); err == nil {
				t.Fatal("removed unsafe name")
			}
			if _, err := admin.ReceiveModel(context.Background(), PushManifest{Name: name, Type: "llama", Format: PushFormatFile}, strings.NewReader("replace")); err == nil {
				t.Fatal("received unsafe name")
			}
			if err := EnsureModelAvailable(context.Background(), nil, name, AutoPullOptions{DataRoot: root}); err == nil {
				t.Fatal("pulled unsafe name")
			}
		})
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
}
