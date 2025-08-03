package hooksapi

import (
	"net/http"
	"net/url"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/hookproviderservice"
	"github.com/contenox/runtime/store"
)

func AddRemoteHookRoutes(mux *http.ServeMux, service hookproviderservice.Service) {
	s := &remoteHookService{service: service}

	mux.HandleFunc("POST /hooks/remote", s.create)
	mux.HandleFunc("GET /hooks/remote", s.list)
	mux.HandleFunc("GET /hooks/remote/{id}", s.get)
	mux.HandleFunc("PUT /hooks/remote/{id}", s.update)
	mux.HandleFunc("DELETE /hooks/remote/{id}", s.delete)
}

type remoteHookService struct {
	service hookproviderservice.Service
}

func (s *remoteHookService) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hook, err := serverops.Decode[store.RemoteHook](r)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	if err := s.service.Create(ctx, &hook); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, hook)
}

func (s *remoteHookService) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hooks, err := s.service.List(ctx)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, hooks)
}

func (s *remoteHookService) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))

	hook, err := s.service.Get(ctx, id)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, hook)
}

func (s *remoteHookService) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))

	hook, err := serverops.Decode[store.RemoteHook](r)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	hook.ID = id
	if err := s.service.Update(ctx, &hook); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, hook)
}

func (s *remoteHookService) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))

	if err := s.service.Delete(ctx, id); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}
