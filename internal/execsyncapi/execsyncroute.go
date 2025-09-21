package execsyncapi

import (
	"net/http"

	"github.com/contenox/runtime/executor"
	"github.com/contenox/runtime/internal/apiframework"
)

// Add this struct near the other handler structs
type executorHandler struct {
	service executor.ExecutorSyncTrigger
}

// Add this function to register the executor routes
func AddExecutorRoutes(mux *http.ServeMux, service executor.ExecutorSyncTrigger) {
	e := &executorHandler{service: service}
	mux.HandleFunc("POST /executor/sync", e.triggerSync)
}

// Implement the handler method
func (e *executorHandler) triggerSync(w http.ResponseWriter, r *http.Request) {
	e.service.TriggerSync()
	apiframework.Encode(w, r, http.StatusOK, "sync triggered") // @response string
}
