package toolsapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/toolsproviderservice"
)

func AddRemoteToolsRoutes(mux *http.ServeMux, service toolsproviderservice.Service) {
	s := &remoteToolsService{service: service}

	mux.HandleFunc("POST /tools/remote", s.create)
	mux.HandleFunc("GET /tools/remote", s.list)
	mux.HandleFunc("GET /tools/remote/{id}", s.get)
	mux.HandleFunc("GET /tools/remote/by-name/{name}", s.getByName)
	mux.HandleFunc("PUT /tools/remote/{id}", s.update)
	mux.HandleFunc("DELETE /tools/remote/{id}", s.delete)

	mux.HandleFunc("GET /tools/local", s.listLocal)
	mux.HandleFunc("GET /tools/schemas", s.getSchemas)
}

type remoteToolsService struct {
	service toolsproviderservice.Service
}

// listLocal returns every locally available tool group (builtin, MCP-backed,
// and remote) with the tools each currently exposes.
func (s *remoteToolsService) listLocal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	localTools, err := s.service.ListLocalTools(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, localTools) // @response []toolsproviderservice.LocalTools
}

// getSchemas returns the OpenAPI schema document for every supported tool,
// keyed by tool name.
func (s *remoteToolsService) getSchemas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	schemas, err := s.service.GetSchemasForSupportedTools(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, schemas) // @response any
}

// create registers a new remote tools provider and returns it with its
// assigned ID.
func (s *remoteToolsService) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tool, err := apiframework.Decode[runtimetypes.RemoteTools](r) // @request runtimetypes.RemoteTools
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if err := s.service.Create(ctx, &tool); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusCreated, tool) // @response runtimetypes.RemoteTools
}

// list returns the registered remote tools providers, paginated by cursor.
func (s *remoteToolsService) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cursor, limit, err := apiframework.ListParams(r, 100)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	tools, err := s.service.List(ctx, cursor, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, tools) // @response []*runtimetypes.RemoteTools
}

// get returns one remote tools provider by ID.
func (s *remoteToolsService) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the remote tool.")

	tool, err := s.service.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, tool) // @response runtimetypes.RemoteTools
}

// update replaces a remote tools provider's configuration by ID and returns
// the stored result.
func (s *remoteToolsService) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the remote tool.")

	tool, err := apiframework.Decode[runtimetypes.RemoteTools](r) // @request runtimetypes.RemoteTools
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}

	tool.ID = id
	if err := s.service.Update(ctx, &tool); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, tool) // @response runtimetypes.RemoteTools
}

// delete removes a remote tools provider by ID.
func (s *remoteToolsService) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the remote tool.")

	if err := s.service.Delete(ctx, id); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, "deleted") // @response string
}

// getByName returns one remote tools provider by its unique name.
func (s *remoteToolsService) getByName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := apiframework.GetPathParam(r, "name", "The unique name for the remote tool.")
	tool, err := s.service.GetByName(ctx, name)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, tool) // @response runtimetypes.RemoteTools
}
