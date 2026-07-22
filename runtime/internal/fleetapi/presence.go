package fleetapi

import (
	"context"
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/presence"
)

// PresenceReader reads the OBSERVED-presence section of the fleet board: the
// editor-spawned contenox instances (Zed's `contenox acp`, VS Code's
// vscode-agent) and serve itself that self-register into the shared presence
// store (runtime/presence).
//
// It is deliberately a DISTINCT, read-only capability from fleetservice.Service
// (the kernel's config+runtime join). A presence entry is OBSERVED, not managed:
// the kernel spawned none of these processes and owns no lifecycle over them, so
// there is no Stop/Cancel to offer — the board must not render a verb the runtime
// cannot honor. Keeping presence its own narrow interface is exactly what lets
// the board gain this section WITHOUT every fleetservice.Service implementation
// (and its test doubles) growing a method it has no runtime for.
type PresenceReader interface {
	List(ctx context.Context) ([]presence.Entry, error)
}

// AddPresenceRoutes registers the presence section of the fleet board on mux.
// It is ADDITIVE and independent of AddRoutes: GET /fleet/presence sits beside
// the existing GET /fleet (which is unchanged and still returns
// []agentinstance.FleetEntry), so a board consumer fetches the managed and the
// observed halves separately. The literal /fleet/presence path is strictly more
// specific than GET /fleet/{instanceID}, so the two coexist on one mux without
// conflict.
//
// The response is []presence.Entry — each entry flattens a presence.Record and
// adds `external: true` (never kernel-managed) and `stale: bool` (its liveness
// TTL lapsed) alongside the raw `lastSeen`. See runtime/presence for the shape.
func AddPresenceRoutes(mux *http.ServeMux, reader PresenceReader) {
	h := &presenceHandler{reader: reader}
	mux.HandleFunc("GET /fleet/presence", h.list)
}

type presenceHandler struct {
	reader PresenceReader
}

// list returns the observed-presence entries: external agents (and serve
// itself) that self-registered into the shared presence store.
func (h *presenceHandler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := h.reader.List(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entries) // @response []presence.Entry
}
