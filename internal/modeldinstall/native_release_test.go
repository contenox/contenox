package modeldinstall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSystem_EnsureInstalled_NativeRelease(t *testing.T) {
	root := os.Getenv("MODELD_TEST_RELEASE_DIR")
	if root == "" {
		t.Skip("set MODELD_TEST_RELEASE_DIR to a packaged native release store")
	}
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	opts := Options{BaseURL: server.URL, DataRoot: t.TempDir()}
	result, err := EnsureInstalled(context.Background(), "llama", opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyInstalled {
		t.Fatal("fresh install reused an unexpected worker")
	}
	if _, err := ProbeBinary(context.Background(), result.LauncherPath); err != nil {
		t.Fatal(err)
	}
	again, err := EnsureInstalled(context.Background(), "llama", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyInstalled || again.LauncherPath != result.LauncherPath {
		t.Fatalf("installed worker was not reused: %+v", again)
	}
}
