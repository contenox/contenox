package backendapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/stateservice"
)

func AddStateRoutes(mux *http.ServeMux, stateService stateservice.Service) {
	s := &statemux{stateService: stateService}

	mux.HandleFunc("GET /state", s.list)
}

type statemux struct {
	stateService stateservice.Service
}

func (s *statemux) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	internalModels, err := s.stateService.Get(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, sanitizeRuntimeStates(internalModels)) // @response []statetype.BackendRuntimeState
}
