package shellenvapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/runtime/shellenvservice"
)

// shellEnvResponse is the GET/PUT body: the global environment variables contenox
// injects into the shells it spawns, as a name→value object.
type shellEnvResponse struct {
	Vars map[string]string `json:"vars"`
}

// shellEnvRequest is the PUT body: the FULL replacement set of variables (PUT
// replaces, it does not merge). An empty object clears the injected variables.
type shellEnvRequest struct {
	Vars map[string]string `json:"vars"`
}

// AddShellEnvRoutes registers the global shell-env read/write surface: the
// operator-defined variables injected into every shell contenox spawns. Both
// verbs are authenticated exactly like the other mutating routes; the values are
// plaintext config, not secrets.
func AddShellEnvRoutes(mux *http.ServeMux, svc shellenvservice.Service, auth middleware.AuthZReader) {
	h := &handler{svc: svc, auth: auth}
	mux.HandleFunc("GET /shell-env", h.get)
	mux.HandleFunc("PUT /shell-env", h.put)
}

type handler struct {
	svc  shellenvservice.Service
	auth middleware.AuthZReader
}

func (h *handler) authorize(r *http.Request) error {
	if h.auth == nil {
		return nil
	}
	_, err := h.auth.GetIdentity(r.Context())
	return err
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	vars, err := h.svc.Get(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, shellEnvResponse{Vars: vars}) // @response shellenvapi.shellEnvResponse
}

func (h *handler) put(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	body, err := apiframework.Decode[shellEnvRequest](r) // @request shellenvapi.shellEnvRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	if err := h.svc.Set(r.Context(), body.Vars); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	vars, err := h.svc.Get(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, shellEnvResponse{Vars: vars}) // @response shellenvapi.shellEnvResponse
}
