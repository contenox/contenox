package serverapi

import (
	"net/http"

	"github.com/contenox/runtime/apiframework"
)

type HealthResponse struct {
	Status string `json:"status"`
}

// AddHealthRoutes registers GET /health for liveness checks. serverapi.New
// mounts it on the api mux (as well as serve's root mux), so /api/health is a
// real, documented endpoint.
func AddHealthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		_ = apiframework.Encode(w, r, http.StatusOK, HealthResponse{Status: "ok"}) // @response serverapi.HealthResponse
	})
}
