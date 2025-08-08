package backendapi

import (
	"net/http"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/stateservice"
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
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}
	serverops.Encode(w, r, http.StatusOK, internalModels) // @response []runtimestate.LLMState
}
