// Package missionchangesapi exposes the attention layer's per-mission
// changed-files and diff endpoints over REST (runtime/missionchanges). It is the
// backend of Beam's flagship oversight arc: the changed-files list a reviewer
// opens on a mission, ordered by where the unit's attention concentrated, plus
// the {original, modified} diff Monaco renders per file, plus the scope summary
// that flags a wandering unit.
//
// The route/handler shape mirrors runtime/internal/missionapi so the mission
// surfaces stay easy to compare, and the endpoints deliberately live UNDER
// /missions/{id} — they are a read-only view onto one mission's work, never a
// second resource. Everything here is read-only: the attention layer ranks and
// flags, it never gates (see runtime/missionchanges' package doc), so this API
// has no mutation.
package missionchangesapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/missionchanges"
)

// AddRoutes registers the changed-files and diff routes on mux. Both sit under
// the mission id so they compose with the mission CRUD routes missionapi mounts.
func AddRoutes(mux *http.ServeMux, svc missionchanges.Service) {
	h := &handler{svc: svc}
	mux.HandleFunc("GET /missions/{id}/changes", h.changes)
	mux.HandleFunc("GET /missions/{id}/changes/diff", h.diff)
}

type handler struct {
	svc missionchanges.Service
}

// changes returns mission id's changed-files list (ordered by Degree-of-Interest)
// and scope summary. An unknown mission id surfaces as 404; a known mission whose
// unit left no recoverable journal returns an empty list, not an error.
func (h *handler) changes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")
	res, err := h.svc.Changes(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, res) // @response missionchanges.Changes
}

// diff returns the {original, modified} pair for one changed path in mission id,
// selected by the required `path` query parameter. A path the mission never wrote
// surfaces as 404 so the frontend can distinguish "no such changed file" from an
// empty diff.
func (h *handler) diff(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")
	path := apiframework.GetQueryParam(r, "path", "", "The changed file path to diff.")
	if path == "" {
		_ = apiframework.Error(w, r, apiframework.InvalidParameterValue("path", "the path query parameter is required"), apiframework.GetOperation)
		return
	}
	res, err := h.svc.Diff(ctx, id, path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, res) // @response missionchanges.Diff
}
