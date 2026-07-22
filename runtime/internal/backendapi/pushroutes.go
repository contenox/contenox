package backendapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

// pushModelResponse mirrors transport.PushResult for the wire, plus the name
// pushed under (the request only carries the raw file bytes, so the name
// that was actually stored is worth echoing back).
type pushModelResponse struct {
	Name           string `json:"name" example:"my-qwen"`
	AlreadyPresent bool   `json:"alreadyPresent,omitempty" example:"false"`
	BytesWritten   int64  `json:"bytesWritten,omitempty" example:"4294967296"`
}

// pushModel streams a GGUF file straight from the request body to the models
// store of a specific modeld backend (local or remote) — the HTTP twin of
// `contenox model push`. The body is the raw file bytes (no multipart, no
// framework Decode, which buffers the whole body in memory); this keeps a
// multi-gigabyte model upload streaming end to end, from browser to modeld.
func (b *backendManager) pushModel(w http.ResponseWriter, r *http.Request) {
	// @request binary Raw model file bytes (GGUF or OpenVINO artifact), streamed as the request body.
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the backend.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.ExecuteOperation)
		return
	}
	name := strings.TrimSpace(apiframework.GetQueryParam(r, "name", "", "The model name to store the pushed artifact under."))
	if name == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("%w: query parameter %q is required", apiframework.ErrMissingParameter, "name"), apiframework.ExecuteOperation)
		return
	}
	// name becomes a directory/file component under modeld's models root
	// (modelstore.ReceiveModel joins it in unsanitized); reject anything that
	// could escape that root before it ever reaches the wire.
	if name != filepath.Base(name) || name == "." || name == ".." {
		_ = apiframework.Error(w, r, fmt.Errorf("%w: %q is not a valid model name (no path separators)", apiframework.ErrInvalidParameterValue, name), apiframework.ExecuteOperation)
		return
	}
	if r.ContentLength == 0 {
		_ = apiframework.Error(w, r, apiframework.ErrFileEmpty, apiframework.ExecuteOperation)
		return
	}

	backend, err := b.service.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	typ := modelrepo.CanonicalBackendType(backend.Type)
	if typ != "modeld" && typ != "llama" && typ != "openvino" {
		_ = apiframework.Error(w, r, fmt.Errorf("%w: backend %q is type %q, not modeld (or llama/openvino)", apiframework.ErrUnprocessableEntity, backend.Name, backend.Type), apiframework.ExecuteOperation)
		return
	}

	addr := backend.BaseURL
	if addr == "" || addr == modeldconn.LocalSentinel {
		resolved, rerr := modeldconn.LocalEndpointAddr(ctx)
		if rerr != nil {
			_ = apiframework.Error(w, r, fmt.Errorf("resolve local modeld: %w", rerr), apiframework.ExecuteOperation)
			return
		}
		addr = resolved
	}

	// Endpoint is the same self-healing, per-backend-ID connection cache the
	// hot chat-serving path's targeted providers use (see modelrepo/llama and
	// modelrepo/openvino provider.go) — it health-probes and redials across a
	// modeld restart instead of latching onto a stale connection.
	ec, err := modeldconn.Endpoint(ctx, backend.ID, addr)
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("connect to modeld backend %s: %w", backend.Name, err), apiframework.ExecuteOperation)
		return
	}

	manifestType := "llama"
	if ec.Backend != "" {
		manifestType = ec.Backend
	}
	manifest := transport.PushManifest{
		Name:       name,
		Type:       manifestType,
		Format:     transport.PushFormatFile,
		TotalBytes: r.ContentLength,
	}

	res, err := ec.PushModel(ctx, manifest, r.Body)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ExecuteOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, pushModelResponse{ // @response backendapi.pushModelResponse
		Name:           name,
		AlreadyPresent: res.AlreadyPresent,
		BytesWritten:   res.BytesWritten,
	})
}
