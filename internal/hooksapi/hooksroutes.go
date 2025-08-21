package hooksapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/contenox/runtime/hookproviderservice"
	serverops "github.com/contenox/runtime/internal/apiframework"
	"github.com/contenox/runtime/runtimetypes"
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

// Creates a new remote hook configuration.
// Remote hooks allow task-chains to trigger external HTTP services during execution.
func (s *remoteHookService) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hook, err := serverops.Decode[runtimetypes.RemoteHook](r) // @request runtimetypes.RemoteHook
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}
	if err := s.service.Create(ctx, &hook); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, hook) // @response runtimetypes.RemoteHook
}

// Lists all configured remote hooks with pagination support.
func (s *remoteHookService) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination parameters from query string
	var cursor *time.Time
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		t, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			err = fmt.Errorf("%w: invalid cursor format, expected RFC3339Nano", serverops.ErrUnprocessableEntity)
			_ = serverops.Error(w, r, err, serverops.ListOperation)
			return
		}
		cursor = &t
	}

	limit := 100 // Default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		i, err := strconv.Atoi(limitStr)
		if err != nil {
			err = fmt.Errorf("%w: invalid limit format, expected integer", serverops.ErrUnprocessableEntity)
			_ = serverops.Error(w, r, err, serverops.ListOperation)
			return
		}
		limit = i
	}

	hooks, err := s.service.List(ctx, cursor, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, hooks) // @response []runtimetypes.RemoteHook
}

// Retrieves a specific remote hook configuration by ID.
func (s *remoteHookService) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))

	hook, err := s.service.Get(ctx, id)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, hook) // @response runtimetypes.RemoteHook
}

// Updates an existing remote hook configuration.
// The ID from the URL path overrides any ID in the request body.
func (s *remoteHookService) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))

	hook, err := serverops.Decode[runtimetypes.RemoteHook](r) // @request runtimetypes.RemoteHook
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	hook.ID = id
	if err := s.service.Update(ctx, &hook); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, hook) // @response runtimetypes.RemoteHook
}

// Deletes a remote hook configuration by ID.
// Returns a simple "deleted" confirmation message on success.
func (s *remoteHookService) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))

	if err := s.service.Delete(ctx, id); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "deleted") // @response string
}
