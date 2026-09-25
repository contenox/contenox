package acpsvc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/contenox/contenox/internal/services/localfileservice"
	"github.com/contenox/contenox/libacp"
)

const (
	extMethodAssetActions = "_contenox/asset/actions"
	extMethodAssetRun     = "_contenox/asset/run"
)

// assetAction mirrors the SaaS toolbar contract: the machine advertises what
// may act on a preview kind and the client renders the list, inventing
// nothing. Actions are host-level (they read the workspace file the client is
// previewing), so they live behind the same host-workspace guard as the
// _contenox/fs family.
type assetAction struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Kinds    []string `json:"kinds"`
	ReadOnly bool     `json:"readOnly"`
	Hint     string   `json:"hint,omitempty"`
}

// imageMetadata is the first real action: read-only, zero-dependency, and it
// proves the pipe by parsing the raw bytes of the asset the client previews.
var imageMetadata = assetAction{
	ID:       "image.metadata",
	Label:    "Metadata",
	Kinds:    []string{"image"},
	ReadOnly: true,
	Hint:     "Reports the image's width, height and format from its header bytes",
}

// imageDuplicate is the first transform action: readOnly false because it
// WRITES a new asset, and its result carries an `artifact` the client preview
// switches to. Real DCC transforms (flip, convert, LOD) replace the copy
// behind the same contract and gate behind the same policy machinery.
var imageDuplicate = assetAction{
	ID:       "image.duplicate",
	Label:    "Duplicate",
	Kinds:    []string{"image"},
	ReadOnly: false,
	Hint:     "Writes a copy of the image beside it and opens the copy",
}

// handleAssetActions lists the actions the machine can run on a previewed
// asset. Nil host files answer MethodNotFound — the editor profile owns its
// filesystem client-side, exactly the _contenox/fs posture.
func (t *Transport) handleAssetActions(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	if _, ok := t.hostFiles(); !ok {
		return nil, libacp.MethodNotFound(extMethodAssetActions + " is not enabled on this server")
	}
	out, _ := json.Marshal([]assetAction{imageMetadata, imageDuplicate})
	return out, nil
}

// handleAssetRun executes one advertised action on the asset at path, reading
// the raw bytes from the workspace file service. Read-only today; a transform
// lands here with the same shape and its own policy gate.
func (t *Transport) handleAssetRun(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Action string `json:"action"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("asset/run: " + err.Error())
	}
	if strings.TrimSpace(p.Action) == "" {
		return nil, libacp.InvalidParams("asset/run: action is required")
	}
	if strings.TrimSpace(p.Path) == "" {
		return nil, libacp.InvalidParams("asset/run: path is required")
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodAssetRun + " is not enabled on this server")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("asset/run: " + err.Error())
	}
	data, entry, err := svc.Read(ctx, rel)
	if err != nil {
		return nil, fsServiceErr("asset/run", err)
	}
	if entry.IsDirectory {
		return nil, libacp.InvalidParams("asset/run: " + p.Path + " is a directory")
	}

	switch p.Action {
	case imageMetadata.ID:
		notice := pngMetadataNotice(data)
		if notice == "" {
			return nil, libacp.InvalidParams("image.metadata: only PNG metadata is supported")
		}
		out, _ := json.Marshal(map[string]any{"ok": true, "notice": notice})
		return out, nil
	case imageDuplicate.ID:
		newRel, err := duplicateAssetRel(svc, rel)
		if err != nil {
			return nil, libacp.InternalError("image.duplicate: " + err.Error())
		}
		if _, err := svc.Write(ctx, newRel, data, false); err != nil {
			return nil, libacp.InternalError("image.duplicate: " + err.Error())
		}
		out, _ := json.Marshal(map[string]any{
			"ok":     true,
			"notice": "Created " + filepath.Base(newRel),
			"artifact": map[string]any{
				"path": newRel,
				"kind": "image",
			},
		})
		return out, nil
	default:
		return nil, libacp.InvalidParams("asset/run: unknown action " + p.Action)
	}
}

// duplicateAssetRel returns the next free "<name>-<n><ext>" sibling of rel.
func duplicateAssetRel(svc localFileRooterRead, rel string) (string, error) {
	dir := filepath.Dir(rel)
	ext := filepath.Ext(rel)
	stem := strings.TrimSuffix(filepath.Base(rel), ext)
	for n := 1; ; n++ {
		name := fmt.Sprintf("%s-%d%s", stem, n, ext)
		candidate := name
		if dir != "." {
			candidate = filepath.Join(dir, name)
		}
		if _, _, err := svc.Read(context.Background(), candidate); err != nil {
			return candidate, nil
		}
	}
}

type localFileRooterRead = localfileservice.Service

// pngMetadataNotice reports "W × H PNG" from a PNG header, or "" when data is
// not a PNG.
func pngMetadataNotice(data []byte) string {
	if len(data) < 24 {
		return ""
	}
	sig := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	for i, b := range sig {
		if data[i] != b {
			return ""
		}
	}
	width := binary.BigEndian.Uint32(data[16:20])
	height := binary.BigEndian.Uint32(data[20:24])
	return fmt.Sprintf("%d × %d PNG", width, height)
}
