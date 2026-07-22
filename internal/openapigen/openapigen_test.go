package openapigen

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixtureTree lays out a minimal repo shape Generate can scan: every
// type-scan directory exists (empty is fine, missing is a strict error), both
// route-scan globs match, and a synthetic go.mod/version.txt anchor the root.
// files maps repo-root-relative paths to Go source.
func writeFixtureTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range typeScanDirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Both route-scan globs must match at least one directory.
	for _, d := range []string{"runtime/internal/testapi", "runtime/serverapi", "runtime/version"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime/version/version.txt"), []byte("0.0.0-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, src := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func mustGenerate(t *testing.T, root string) (map[string]any, Stats, []byte) {
	t.Helper()
	out, stats, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	return doc, stats, out
}

func docPaths(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("doc has no paths object")
	}
	return paths
}

// respRef digs out the 200-response schema for method on path.
func respSchema(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()
	op, ok := docPaths(t, doc)[path].(map[string]any)[method].(map[string]any)
	if !ok {
		t.Fatalf("no %s %s in doc", method, path)
	}
	schema := op["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	return schema
}

func TestUnit_ExcludeAnnotationHonoredAndDeterministic(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type okResponse struct {
	Value string ` + "`json:\"value\"`" + `
}

type thingHandler struct{}

func (h *thingHandler) get(w http.ResponseWriter, r *http.Request) {
	// @response testapi.okResponse
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.get)
}

// AddRootRoutes serves a mount outside the documented prefix.
//
// openapi:exclude test fixture: mounted on the root mux, not under /api
func AddRootRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /root-things", h.get)
	mux.HandleFunc("POST /root-things", h.get)
}
`,
	})
	doc, stats, out := mustGenerate(t, root)
	paths := docPaths(t, doc)
	if _, ok := paths["/things"]; !ok {
		t.Errorf("expected /things in paths, got %v", paths)
	}
	if _, ok := paths["/root-things"]; ok {
		t.Errorf("openapi:exclude not honored: /root-things documented")
	}
	if stats.Paths != 1 {
		t.Errorf("Stats.Paths = %d, want 1", stats.Paths)
	}
	if got := respSchema(t, doc, "/things", "get")["$ref"]; got != "#/components/schemas/testapi_okResponse" {
		t.Errorf("response schema = %v, want $ref to testapi_okResponse", got)
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		t.Error("output must end with a trailing newline")
	}
	// Determinism: a second run over the same tree is byte-identical.
	out2, _, err := Generate(root)
	if err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if !bytes.Equal(out, out2) {
		t.Error("Generate is not deterministic: two runs differ")
	}
}

func TestUnit_ExcludeWithoutReasonFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

// openapi:exclude
func AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /things", func(w http.ResponseWriter, r *http.Request) {})
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for openapi:exclude without a reason")
	}
	if !strings.Contains(err.Error(), "requires a one-line reason") {
		t.Errorf("error should mention the missing reason, got: %v", err)
	}
}

func TestUnit_DuplicateRouteFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

func AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /things", func(w http.ResponseWriter, r *http.Request) {})
}

func AddMoreRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /things", func(w http.ResponseWriter, r *http.Request) {})
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for duplicate METHOD+path registration")
	}
	msg := err.Error()
	if !strings.Contains(msg, `duplicate route registration "GET /things"`) {
		t.Errorf("error should name the duplicate route, got: %v", err)
	}
	// Both registration sites must be reported with file:line.
	if strings.Count(msg, "routes.go:") < 2 {
		t.Errorf("error should carry both registration sites, got: %v", err)
	}
}

func TestUnit_UnresolvableResponseFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) get(w http.ResponseWriter, r *http.Request) {
	// @response testapi.DoesNotExist
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.get)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for unresolvable @response type")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"testapi.DoesNotExist"`) {
		t.Errorf("error should name the unresolved type, got: %v", err)
	}
	if !strings.Contains(msg, "routes.go:") {
		t.Errorf("error should carry the annotation's file:line, got: %v", err)
	}
	if !strings.Contains(msg, "GET /things") {
		t.Errorf("error should name the route, got: %v", err)
	}
}

func TestUnit_ReceiverQualifiedAnnotationBinding(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type aResponse struct {
	A string ` + "`json:\"a\"`" + `
}

type bResponse struct {
	B string ` + "`json:\"b\"`" + `
}

type aHandler struct{}

func (h *aHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response []testapi.aResponse
}

type bHandler struct{}

func (h *bHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response []testapi.bResponse
}

func AddRoutes(mux *http.ServeMux) {
	a := &aHandler{}
	b := &bHandler{}
	mux.HandleFunc("GET /as", a.list)
	mux.HandleFunc("GET /bs", b.list)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	aItems := respSchema(t, doc, "/as", "get")["items"].(map[string]any)
	bItems := respSchema(t, doc, "/bs", "get")["items"].(map[string]any)
	if got := aItems["$ref"]; got != "#/components/schemas/testapi_aResponse" {
		t.Errorf("GET /as items = %v, want testapi_aResponse (same-name method on another receiver bled through)", got)
	}
	if got := bItems["$ref"]; got != "#/components/schemas/testapi_bResponse" {
		t.Errorf("GET /bs items = %v, want testapi_bResponse (same-name method on another receiver bled through)", got)
	}
}

func TestUnit_AmbiguousBareNameBindingFails(t *testing.T) {
	// The handler variable comes from a constructor call, so its receiver type
	// cannot be resolved; with TWO annotated `list` methods in the package the
	// bare-name fallback is ambiguous and must fail rather than guess.
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type aResponse struct {
	A string ` + "`json:\"a\"`" + `
}

type bResponse struct {
	B string ` + "`json:\"b\"`" + `
}

type aHandler struct{}

func (h *aHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response []testapi.aResponse
}

type bHandler struct{}

func (h *bHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response []testapi.bResponse
}

func newHandler() *aHandler { return &aHandler{} }

func AddRoutes(mux *http.ServeMux) {
	h := newHandler()
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for ambiguous annotation binding")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ambiguous annotation binding") {
		t.Errorf("error should say the binding is ambiguous, got: %v", err)
	}
	if !strings.Contains(msg, "aHandler.list") || !strings.Contains(msg, "bHandler.list") {
		t.Errorf("error should list the candidate annotated functions, got: %v", err)
	}
}

func TestUnit_UnambiguousBareNameFallbackStillBinds(t *testing.T) {
	// Same constructor-call shape, but only ONE annotated `list` exists in the
	// package: the bare-name fallback binds it (this is how a registrar that
	// builds its handler through a helper keeps its annotations).
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type aResponse struct {
	A string ` + "`json:\"a\"`" + `
}

type aHandler struct{}

func (h *aHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response []testapi.aResponse
}

func newHandler() *aHandler { return &aHandler{} }

func AddRoutes(mux *http.ServeMux) {
	h := newHandler()
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	items := respSchema(t, doc, "/things", "get")["items"].(map[string]any)
	if got := items["$ref"]; got != "#/components/schemas/testapi_aResponse" {
		t.Errorf("bare-name fallback did not bind: items = %v", got)
	}
}

// opAt digs out one operation object.
func opAt(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()
	op, ok := docPaths(t, doc)[path].(map[string]any)[method].(map[string]any)
	if !ok {
		t.Fatalf("no %s %s in doc", method, path)
	}
	return op
}

// opParams returns the operation's parameters keyed by name, plus their order.
func opParams(t *testing.T, doc map[string]any, path, method string) (map[string]map[string]any, []string) {
	t.Helper()
	raw, _ := opAt(t, doc, path, method)["parameters"].([]any)
	byName := map[string]map[string]any{}
	var order []string
	for _, p := range raw {
		pm := p.(map[string]any)
		name := pm["name"].(string)
		byName[name] = pm
		order = append(order, name)
	}
	return byName, order
}

func TestUnit_GetQueryParamDerivation(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response string
	path := apiframework.GetQueryParam(r, "path", ".", "Directory path relative to the root.")
	filter := apiframework.GetQueryParam(r, "filter", "", "View filter to apply.")
	_, _ = path, filter
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	params, order := opParams(t, doc, "/things", "get")
	p := params["path"]
	if p == nil {
		t.Fatalf("no `path` query parameter derived; got %v", order)
	}
	if p["in"] != "query" || p["required"] != false {
		t.Errorf("path param must be in:query required:false, got %v", p)
	}
	if p["description"] != "Directory path relative to the root." {
		t.Errorf("path description = %v", p["description"])
	}
	schema := p["schema"].(map[string]any)
	if schema["type"] != "string" || schema["default"] != "." {
		t.Errorf("path schema = %v, want string with default %q", schema, ".")
	}
	f := params["filter"]
	if f == nil {
		t.Fatal("no `filter` query parameter derived")
	}
	if _, hasDefault := f["schema"].(map[string]any)["default"]; hasDefault {
		t.Errorf("empty default literal must not emit schema.default, got %v", f["schema"])
	}
	// Deterministic order: query params sorted by name.
	if len(order) != 2 || order[0] != "filter" || order[1] != "path" {
		t.Errorf("parameter order = %v, want [filter path]", order)
	}
}

func TestUnit_ListParamsExpansion(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response string
	cursor, limit, err := apiframework.ListParams(r, 100)
	_, _, _ = cursor, limit, err
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	params, order := opParams(t, doc, "/things", "get")
	if len(order) != 2 {
		t.Fatalf("want cursor+limit, got %v", order)
	}
	c := params["cursor"]
	if c == nil || c["description"] != "An optional RFC3339Nano timestamp to fetch the next page of results." {
		t.Errorf("cursor param wrong: %v", c)
	}
	if c["schema"].(map[string]any)["type"] != "string" {
		t.Errorf("cursor must be string, got %v", c["schema"])
	}
	l := params["limit"]
	if l == nil || l["description"] != "The maximum number of items to return per page." {
		t.Errorf("limit param wrong: %v", l)
	}
	if l["schema"].(map[string]any)["type"] != "integer" {
		t.Errorf("limit must be integer, got %v", l["schema"])
	}
}

func TestUnit_GetPathParamDescriptionAttach(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type thingHandler struct{}

func (h *thingHandler) get(w http.ResponseWriter, r *http.Request) {
	// @response string
	id := apiframework.GetPathParam(r, "id", "The unique thing ID.")
	_ = id
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things/{id}", h.get)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	params, _ := opParams(t, doc, "/things/{id}", "get")
	p := params["id"]
	if p == nil {
		t.Fatal("no id path parameter")
	}
	if p["in"] != "path" || p["required"] != true {
		t.Errorf("id must stay in:path required:true, got %v", p)
	}
	if p["description"] != "The unique thing ID." {
		t.Errorf("GetPathParam description not attached: %v", p["description"])
	}
}

func TestUnit_GetPathParamUnknownNameFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type thingHandler struct{}

func (h *thingHandler) get(w http.ResponseWriter, r *http.Request) {
	id := apiframework.GetPathParam(r, "nope", "Not in the template.")
	_ = id
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things/{id}", h.get)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for GetPathParam name missing from every bound route template")
	}
	if !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(err.Error(), "routes.go:") {
		t.Errorf("error should name the parameter and carry the call site, got: %v", err)
	}
}

func TestUnit_TypedParamAnnotation(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response string
	// @param count integer Maximum number of things to return.
	// @param verbose boolean
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	params, _ := opParams(t, doc, "/things", "get")
	c := params["count"]
	if c == nil {
		t.Fatal("no count param from @param")
	}
	if c["schema"].(map[string]any)["type"] != "integer" {
		t.Errorf("count must be integer, got %v", c["schema"])
	}
	if c["description"] != "Maximum number of things to return." {
		t.Errorf("count description = %v", c["description"])
	}
	v := params["verbose"]
	if v == nil || v["schema"].(map[string]any)["type"] != "boolean" {
		t.Errorf("verbose must be boolean, got %v", v)
	}
	if _, hasDesc := v["description"]; hasDesc {
		t.Errorf("description-less @param must omit description, got %v", v)
	}
}

func TestUnit_ParamAnnotationInvalidTypeFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	// @param count widget Not a real type.
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for invalid @param type")
	}
	if !strings.Contains(err.Error(), "must be one of string, integer, number, boolean") {
		t.Errorf("error should name the allowed types, got: %v", err)
	}
}

func TestUnit_HelperWinsOverParamAnnotation(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response string
	// @param q string From the annotation.
	q := apiframework.GetQueryParam(r, "q", "", "From the helper call.")
	_ = q
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	params, order := opParams(t, doc, "/things", "get")
	if len(order) != 1 {
		t.Fatalf("q must dedupe to one param, got %v", order)
	}
	if got := params["q"]["description"]; got != "From the helper call." {
		t.Errorf("helper-derived must win the dedupe, got description %v", got)
	}
}

func TestUnit_DocSummaryUsed(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

// list returns the registered things, spread over
// two wrapped GoDoc lines.
func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	// @response string
}

func (h *thingHandler) create(w http.ResponseWriter, r *http.Request) {
	// @request none test fixture: bodyless create
	// @response string
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
	mux.HandleFunc("POST /things", h.create)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	if got := opAt(t, doc, "/things", "get")["summary"]; got != "list returns the registered things, spread over two wrapped GoDoc lines." {
		t.Errorf("GoDoc summary not used (or line-wrapping mishandled): %v", got)
	}
	if got := opAt(t, doc, "/things", "post")["summary"]; got != "POST /things" {
		t.Errorf("doc-less handler must keep METHOD /path summary, got %v", got)
	}
}

func TestUnit_NonLiteralParamNameFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

const paramName = "q"

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {
	q := apiframework.GetQueryParam(r, paramName, "", "Dynamic name.")
	_ = q
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for non-literal parameter name")
	}
	if !strings.Contains(err.Error(), "must be a string literal") {
		t.Errorf("error should say the name must be a literal, got: %v", err)
	}
}

func TestUnit_WrapperSeeThrough(t *testing.T) {
	// The workspace-mount shape: the registered handler is wh.wrap((*handler).list).
	// The route must bind the inner method (operationId, annotations, summary,
	// its direct helper calls) AND pick up query params read inside the closure
	// the wrapper returns.
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type okResponse struct {
	Value string ` + "`json:\"value\"`" + `
}

type handler struct{ root string }

// list returns the entries under path.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	path := apiframework.GetQueryParam(r, "path", ".", "Directory path.")
	_ = path
	// @response []testapi.okResponse
}

type wrapHandler struct{}

func (wh *wrapHandler) wrap(fn func(*handler, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root := apiframework.GetQueryParam(r, "root", "", "Workspace root to serve.")
		fn(&handler{root: root}, w, r)
	}
}

func AddRoutes(mux *http.ServeMux) {
	wh := &wrapHandler{}
	mux.HandleFunc("GET /things", wh.wrap((*handler).list))
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	op := opAt(t, doc, "/things", "get")
	if op["operationId"] != "test_list" {
		t.Errorf("operationId = %v, want test_list (inner method must name the operation)", op["operationId"])
	}
	if op["summary"] != "list returns the entries under path." {
		t.Errorf("summary = %v, want the inner method's GoDoc", op["summary"])
	}
	items := respSchema(t, doc, "/things", "get")["items"].(map[string]any)
	if items["$ref"] != "#/components/schemas/testapi_okResponse" {
		t.Errorf("inner method's @response must bind, got %v", items)
	}
	params, order := opParams(t, doc, "/things", "get")
	if params["path"] == nil || params["root"] == nil {
		t.Fatalf("want path (inner) and root (wrapper closure) params, got %v", order)
	}
	if params["root"]["description"] != "Workspace root to serve." {
		t.Errorf("wrapper-closure param description = %v", params["root"]["description"])
	}
}

// opResponses digs out the responses object for method on path.
func opResponses(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()
	resp, ok := opAt(t, doc, path, method)["responses"].(map[string]any)
	if !ok {
		t.Fatalf("no responses on %s %s", method, path)
	}
	return resp
}

func TestUnit_ResponseGrammarNonJSONForms(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) remove(w http.ResponseWriter, r *http.Request) {
	// @response none thing removed; handler writes 204 with no body
}

func (h *thingHandler) download(w http.ResponseWriter, r *http.Request) {
	// @response binary The file's raw bytes.
}

func (h *thingHandler) stream(w http.ResponseWriter, r *http.Request) {
	// @response sse One event per thing change.
}

func (h *thingHandler) callback(w http.ResponseWriter, r *http.Request) {
	// @response redirect 302 back to the UI.
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("DELETE /things/{id}", h.remove)
	mux.HandleFunc("GET /things/{id}/download", h.download)
	mux.HandleFunc("GET /thing-events", h.stream)
	mux.HandleFunc("GET /things/callback", h.callback)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)

	del := opResponses(t, doc, "/things/{id}", "delete")
	if _, has200 := del["200"]; has200 {
		t.Errorf("@response none must not emit a 200, got %v", del)
	}
	noContent, ok := del["204"].(map[string]any)
	if !ok {
		t.Fatalf("@response none must emit a 204, got %v", del)
	}
	if noContent["description"] != "thing removed; handler writes 204 with no body" {
		t.Errorf("204 description = %v", noContent["description"])
	}
	if _, hasContent := noContent["content"]; hasContent {
		t.Errorf("204 must carry no content object, got %v", noContent)
	}

	bin := opResponses(t, doc, "/things/{id}/download", "get")["200"].(map[string]any)
	if bin["description"] != "The file's raw bytes." {
		t.Errorf("binary description = %v", bin["description"])
	}
	schema := bin["content"].(map[string]any)["application/octet-stream"].(map[string]any)["schema"].(map[string]any)
	if schema["type"] != "string" || schema["format"] != "binary" {
		t.Errorf("binary schema = %v", schema)
	}

	sse := opResponses(t, doc, "/thing-events", "get")["200"].(map[string]any)
	sseSchema := sse["content"].(map[string]any)["text/event-stream"].(map[string]any)["schema"].(map[string]any)
	if sseSchema["type"] != "string" {
		t.Errorf("sse schema = %v", sseSchema)
	}

	redir := opResponses(t, doc, "/things/callback", "get")
	if _, has200 := redir["200"]; has200 {
		t.Errorf("@response redirect must drop the 200, got %v", redir)
	}
	found, ok := redir["302"].(map[string]any)
	if !ok {
		t.Fatalf("@response redirect must emit a 302, got %v", redir)
	}
	if found["description"] != "302 back to the UI." {
		t.Errorf("302 description = %v", found["description"])
	}
	if _, hasContent := found["content"]; hasContent {
		t.Errorf("302 must carry no content object, got %v", found)
	}
	// The error envelope stays on every form.
	for _, path := range []string{"/things/{id}", "/things/callback"} {
		method := map[string]string{"/things/{id}": "delete", "/things/callback": "get"}[path]
		if _, hasDefault := opResponses(t, doc, path, method)["default"]; !hasDefault {
			t.Errorf("default error response missing on %s %s", method, path)
		}
	}
}

func TestUnit_RequestNoneAndBinaryForms(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) refresh(w http.ResponseWriter, r *http.Request) {
	// @request none re-check trigger; no body is read
	// @response string
}

func (h *thingHandler) push(w http.ResponseWriter, r *http.Request) {
	// @request binary Raw artifact bytes, streamed.
	// @response string
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("POST /things/refresh", h.refresh)
	mux.HandleFunc("POST /things/push", h.push)
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	if _, has := opAt(t, doc, "/things/refresh", "post")["requestBody"]; has {
		t.Errorf("@request none must not emit a requestBody")
	}
	rb, ok := opAt(t, doc, "/things/push", "post")["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("@request binary must emit a requestBody")
	}
	if rb["description"] != "Raw artifact bytes, streamed." {
		t.Errorf("binary requestBody description = %v", rb["description"])
	}
	schema := rb["content"].(map[string]any)["application/octet-stream"].(map[string]any)["schema"].(map[string]any)
	if schema["type"] != "string" || schema["format"] != "binary" {
		t.Errorf("binary requestBody schema = %v", schema)
	}
}

func TestUnit_NonJSONFormsRequireTextFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) remove(w http.ResponseWriter, r *http.Request) {
	// @request none
	// @response none
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("POST /things/remove", h.remove)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected error for reason-less none forms")
	}
	msg := err.Error()
	if !strings.Contains(msg, "@request none on remove requires a reason") {
		t.Errorf("error should demand the @request reason, got: %v", err)
	}
	if !strings.Contains(msg, "@response none on remove requires a reason") {
		t.Errorf("error should demand the @response reason, got: %v", err)
	}
}

func TestUnit_ClosureHandlerAnnotationsBind(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import (
	"net/http"

	"example.com/apiframework"
)

type okResponse struct {
	Value string ` + "`json:\"value\"`" + `
}

func AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /things", func(w http.ResponseWriter, r *http.Request) {
		// @response testapi.okResponse
		q := apiframework.GetQueryParam(r, "q", "", "Filter query.")
		_ = q
	})
}
`,
	})
	doc, _, _ := mustGenerate(t, root)
	if got := respSchema(t, doc, "/things", "get")["$ref"]; got != "#/components/schemas/testapi_okResponse" {
		t.Errorf("closure @response did not bind: %v", got)
	}
	params, _ := opParams(t, doc, "/things", "get")
	if params["q"] == nil || params["q"]["description"] != "Filter query." {
		t.Errorf("closure helper call did not derive the q param: %v", params)
	}
	if got := opAt(t, doc, "/things", "get")["operationId"]; got != "get_things" {
		t.Errorf("closure operationId = %v, want get_things", got)
	}
}

func TestUnit_CoverageGateMissingResponseFails(t *testing.T) {
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) list(w http.ResponseWriter, r *http.Request) {}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("GET /things", h.list)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected coverage-gate error for missing @response")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing @response annotation for GET /things") {
		t.Errorf("error should name the route, got: %v", err)
	}
	if !strings.Contains(msg, "thingHandler.list") || !strings.Contains(msg, "routes.go:") {
		t.Errorf("error should name the handler and its position, got: %v", err)
	}
}

func TestUnit_CoverageGateMissingRequestFails(t *testing.T) {
	// POST without @request fails; GET and DELETE without @request pass.
	root := writeFixtureTree(t, map[string]string{
		"runtime/internal/testapi/routes.go": `package testapi

import "net/http"

type thingHandler struct{}

func (h *thingHandler) create(w http.ResponseWriter, r *http.Request) {
	// @response string
}

func (h *thingHandler) remove(w http.ResponseWriter, r *http.Request) {
	// @response string
}

func AddRoutes(mux *http.ServeMux) {
	h := &thingHandler{}
	mux.HandleFunc("POST /things", h.create)
	mux.HandleFunc("DELETE /things", h.remove)
}
`,
	})
	_, _, err := Generate(root)
	if err == nil {
		t.Fatal("expected coverage-gate error for missing @request on POST")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing @request annotation for POST /things") {
		t.Errorf("error should name the POST route, got: %v", err)
	}
	if strings.Contains(msg, "DELETE /things") {
		t.Errorf("DELETE must be exempt from the request gate, got: %v", err)
	}
}
