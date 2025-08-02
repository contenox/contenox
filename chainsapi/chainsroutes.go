package chainsapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/chainservice"
	"github.com/contenox/runtime/taskengine"
)

func AddChainRoutes(mux *http.ServeMux, chainService chainservice.Service) {
	s := &service{service: chainService}

	mux.HandleFunc("POST /chains", s.set)
	mux.HandleFunc("GET /chains/{id}", s.get)
	mux.HandleFunc("PUT /chains/{id}", s.update)
	mux.HandleFunc("GET /chains", s.list)
	mux.HandleFunc("DELETE /chains/{id}", s.delete)
}

type service struct {
	service chainservice.Service
}

func (s *service) set(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var chain taskengine.ChainDefinition
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	if err := s.service.Set(ctx, &chain); err != nil {
		serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	serverops.Encode(w, r, http.StatusCreated, chain)
}

func (s *service) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("chain ID is required"), serverops.GetOperation)
		return
	}

	chain, err := s.service.Get(ctx, id)
	if err != nil {
		serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	serverops.Encode(w, r, http.StatusOK, chain)
}

func (s *service) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	chains, err := s.service.List(ctx)
	if err != nil {
		serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	serverops.Encode(w, r, http.StatusOK, chains)
}

func (s *service) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("chain ID is required"), serverops.DeleteOperation)
		return
	}

	if err := s.service.Delete(ctx, id); err != nil {
		serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	serverops.Encode(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *service) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("chain ID is required"), serverops.UpdateOperation)
		return
	}

	var chain taskengine.ChainDefinition
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	// Ensure we are updating the correct chain
	chain.ID = id

	if err := s.service.Update(ctx, &chain); err != nil {
		serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	serverops.Encode(w, r, http.StatusOK, chain)
}
