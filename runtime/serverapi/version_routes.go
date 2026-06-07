package serverapi

import (
	"net/http"

	"github.com/contenox/runtime/apiframework"
)

// AddVersionRoutes registers GET /version.
func AddVersionRoutes(mux *http.ServeMux, version, nodeInstanceID, tenancy string) {
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		_ = apiframework.Encode(w, r, http.StatusOK, apiframework.AboutServer{
			Version:        version,
			NodeInstanceID: nodeInstanceID,
			Tenancy:        tenancy,
		})
	})
}
