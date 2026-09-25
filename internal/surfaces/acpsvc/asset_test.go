package acpsvc

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	libacp "github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

// onePixelPNG is a 1×1 transparent PNG, the same fixture the SaaS fake serves.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func writePngFixture(t *testing.T, svc localFileRooter) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(onePixelPNG)
	require.NoError(t, err)
	path := filepath.Join(svc.Root(), "pixel.png")
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	return path
}

type localFileRooter interface{ Root() string }

func TestAssetActionsAndRunOverTheWire(t *testing.T) {
	svc := newLocalFileSvc(t)
	writePngFixture(t, svc)
	_, client, _ := startExtWire(t, Deps{Files: svc})

	raw, err := extCall(t, client, extMethodAssetActions, nil)
	require.NoError(t, err)
	var actions []assetAction
	require.NoError(t, json.Unmarshal(raw, &actions))
	require.Len(t, actions, 2)
	require.Equal(t, "image.metadata", actions[0].ID)
	require.Equal(t, "Metadata", actions[0].Label)
	require.Equal(t, []string{"image"}, actions[0].Kinds)
	require.True(t, actions[0].ReadOnly)
	require.Equal(t, "image.duplicate", actions[1].ID)
	require.False(t, actions[1].ReadOnly)

	raw, err = extCall(t, client, extMethodAssetRun, map[string]any{
		"action": "image.metadata", "path": "pixel.png",
	})
	require.NoError(t, err)
	var result struct {
		Ok     bool   `json:"ok"`
		Notice string `json:"notice"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.True(t, result.Ok)
	require.Equal(t, "1 × 1 PNG", result.Notice)
}

func TestAssetRunRefusesUnknownActionAndNonPng(t *testing.T) {
	svc := newLocalFileSvc(t)
	writePngFixture(t, svc)
	_, client, _ := startExtWire(t, Deps{Files: svc})

	_, err := extCall(t, client, extMethodAssetRun, map[string]any{
		"action": "image.nonexistent", "path": "pixel.png",
	})
	extWireError(t, err, libacp.ErrInvalidParams)

	other := filepath.Join(svc.Root(), "notes.txt")
	require.NoError(t, os.WriteFile(other, []byte("plain text"), 0o644))
	_, err = extCall(t, client, extMethodAssetRun, map[string]any{
		"action": "image.metadata", "path": "notes.txt",
	})
	extWireError(t, err, libacp.ErrInvalidParams)
}

func TestAssetActionsRequireTheHostWorkspace(t *testing.T) {
	_, client, _ := startExtWire(t, Deps{})
	_, err := extCall(t, client, extMethodAssetActions, nil)
	extWireError(t, err, libacp.ErrMethodNotFound)
	_, err = extCall(t, client, extMethodAssetRun, map[string]any{"action": "image.metadata", "path": "x.png"})
	extWireError(t, err, libacp.ErrMethodNotFound)
}

func TestAssetDuplicateWritesArtifactAndKeepsBytes(t *testing.T) {
	svc := newLocalFileSvc(t)
	writePngFixture(t, svc)
	_, client, _ := startExtWire(t, Deps{Files: svc})

	raw, err := extCall(t, client, extMethodAssetRun, map[string]any{
		"action": "image.duplicate", "path": "pixel.png",
	})
	require.NoError(t, err)
	var result struct {
		Ok       bool   `json:"ok"`
		Notice   string `json:"notice"`
		Artifact struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.True(t, result.Ok)
	require.Equal(t, "pixel-1.png", result.Artifact.Path)
	require.Equal(t, "image", result.Artifact.Kind)

	// The copy exists beside the original with identical bytes.
	orig, err := os.ReadFile(filepath.Join(svc.Root(), "pixel.png"))
	require.NoError(t, err)
	copy, err := os.ReadFile(filepath.Join(svc.Root(), "pixel-1.png"))
	require.NoError(t, err)
	require.Equal(t, orig, copy)
}
