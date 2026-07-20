// Command openapi-gen generates runtime/internal/openapidocs/openapi.json from
// the HTTP route registrations and the inline annotations in the route
// packages.
//
// Conventions consumed:
//
//	mux.HandleFunc("GET /backends/{id}", h.get)   // route -> handler method
//	... // @request  pkg.RequestType                (inside the handler body)
//	... // @response pkg.ResponseType
//	... // @param    name type                      (path param if {name} in route, else query)
//	Field T `json:"x" openapi_include_type:"pkg.Type"`  // document a named type
//
// Request/response/param types are resolved to JSON Schema by walking the
// struct definitions found in the scanned packages; types that cannot be
// resolved degrade to a generic object schema rather than failing.
//
// Run from the repo root: `go run ./tools/openapi-gen` (or `go generate ./...`).
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// routeScanDirs hold the HTTP route registrations + their request/response types.
var routeScanGlobs = []string{
	"runtime/internal/*api",
	"runtime/serverapi",
}

// typeScanDirs are additionally parsed only to resolve cross-package types
// referenced by responses / openapi_include_type tags.
var typeScanDirs = []string{
	"apiframework",
	"runtime/taskengine",
	"runtime/taskengine/llmretry",
	"runtime/agentinstance",
	"runtime/agentservice",
	"runtime/missionservice",
	"runtime/runtimetypes",
	"runtime/stateservice",
	"runtime/internal/setupcheck",
	"runtime/localfileservice",
	"runtime/modelregistry",
	"runtime/modelregistryservice",
	"runtime/backendservice",
	"runtime/hitlservice",
	"runtime/mcpserverservice",
	"runtime/providerservice",
	"runtime/statetype",
	"runtime/terminalstore",
	"runtime/terminalservice",
}

const outPath = "runtime/internal/openapidocs/openapi.json"

type structInfo struct {
	pkg string
	st  *ast.StructType
}

type aliasInfo struct {
	pkg        string
	underlying ast.Expr
}

type handlerAnno struct {
	request    string // "pkg.Type" or ""
	response   string
	params     map[string]string // name -> description (from @param)
	paramOrd   []string
	docSummary string
}

type route struct {
	method  string
	path    string
	pkg     string
	handler string // handler method name, "" if inline
}

type generator struct {
	fset    *token.FileSet
	structs map[string]*structInfo // "pkg.Type" -> struct
	aliases map[string]aliasInfo   // "pkg.Type" -> underlying type of a non-struct named type
	// per package: handler func name -> annotations
	annos   map[string]map[string]*handlerAnno // pkg -> funcName -> anno
	routes  []route
	schemas map[string]any // component schemas, key sanitized
	pending map[string]bool
}

var basicTypes = map[string]string{
	"string": "string", "bool": "boolean",
	"int": "integer", "int8": "integer", "int16": "integer", "int32": "integer", "int64": "integer",
	"uint": "integer", "uint8": "integer", "uint16": "integer", "uint32": "integer", "uint64": "integer",
	"float32": "number", "float64": "number", "byte": "integer", "rune": "integer",
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	g := &generator{
		fset:    token.NewFileSet(),
		structs: map[string]*structInfo{},
		aliases: map[string]aliasInfo{},
		annos:   map[string]map[string]*handlerAnno{},
		schemas: map[string]any{},
		pending: map[string]bool{},
	}

	var routeDirs []string
	for _, glob := range routeScanGlobs {
		matches, _ := filepath.Glob(filepath.Join(root, glob))
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.IsDir() {
				routeDirs = append(routeDirs, m)
			}
		}
	}
	sort.Strings(routeDirs)

	// Pass 1: parse everything to collect struct definitions globally, and
	// collect routes + annotations from route packages.
	allDirs := append([]string{}, routeDirs...)
	for _, d := range typeScanDirs {
		allDirs = append(allDirs, filepath.Join(root, d))
	}
	routeSet := map[string]bool{}
	for _, d := range routeDirs {
		routeSet[d] = true
	}
	for _, dir := range allDirs {
		g.parseDir(dir, routeSet[dir])
	}

	doc := g.build(version(root))
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fail(err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(filepath.Join(root, outPath), out, 0o644); err != nil {
		fail(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	fmt.Printf("wrote %s: %d paths, %d schemas\n", outPath, len(paths), len(schemas))
}

func (g *generator) parseDir(dir string, isRoute bool) {
	pkgs, err := parser.ParseDir(g.fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return
	}
	for pkgName, pkg := range pkgs {
		for _, file := range pkg.Files {
			g.collectStructs(pkgName, file)
			if isRoute {
				g.collectAnnos(pkgName, file)
				g.collectRoutes(pkgName, file)
			}
		}
	}
}

func (g *generator) collectStructs(pkg string, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		key := pkg + "." + ts.Name.Name
		if st, ok := ts.Type.(*ast.StructType); ok {
			if _, exists := g.structs[key]; !exists {
				g.structs[key] = &structInfo{pkg: pkg, st: st}
			}
		} else if _, exists := g.aliases[key]; !exists {
			// Named non-struct type (e.g. `type StopReason string`): record its
			// underlying type so it resolves to a scalar schema, not object.
			g.aliases[key] = aliasInfo{pkg: pkg, underlying: ts.Type}
		}
		return true
	})
}

var (
	reRequest  = regexp.MustCompile(`@request\s+([\w./]+)`)
	reResponse = regexp.MustCompile(`@response\s+([\w./\[\]*]+)`)
	reParam    = regexp.MustCompile(`@param\s+(\w+)\s+\w+`)
)

func (g *generator) collectAnnos(pkg string, file *ast.File) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		anno := &handlerAnno{params: map[string]string{}}
		if fd.Doc != nil {
			anno.docSummary = strings.TrimSpace(strings.SplitN(fd.Doc.Text(), "\n", 2)[0])
		}
		bodyStart, bodyEnd := fd.Body.Pos(), fd.Body.End()
		for _, cg := range file.Comments {
			if cg.Pos() < bodyStart || cg.End() > bodyEnd {
				continue
			}
			text := cg.Text()
			if m := reRequest.FindStringSubmatch(text); m != nil {
				anno.request = m[1]
			}
			if m := reResponse.FindStringSubmatch(text); m != nil && anno.response == "" {
				anno.response = m[1]
			}
			for _, pm := range reParam.FindAllStringSubmatch(text, -1) {
				if _, seen := anno.params[pm[1]]; !seen {
					anno.params[pm[1]] = ""
					anno.paramOrd = append(anno.paramOrd, pm[1])
				}
			}
		}
		if anno.request != "" || anno.response != "" || len(anno.params) > 0 || anno.docSummary != "" {
			if g.annos[pkg] == nil {
				g.annos[pkg] = map[string]*handlerAnno{}
			}
			g.annos[pkg][fd.Name.Name] = anno
		}
	}
}

func (g *generator) collectRoutes(pkg string, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" || len(call.Args) < 2 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		spec := strings.Trim(lit.Value, "`\"")
		parts := strings.Fields(spec)
		if len(parts) != 2 {
			return true
		}
		g.routes = append(g.routes, route{
			method:  strings.ToUpper(parts[0]),
			path:    parts[1],
			pkg:     pkg,
			handler: handlerName(call.Args[1]),
		})
		return true
	})
}

func handlerName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.Ident:
		return v.Name
	}
	return ""
}

func (g *generator) build(ver string) map[string]any {
	pathParam := regexp.MustCompile(`\{(\w+)\}`)
	paths := map[string]any{}

	sort.Slice(g.routes, func(i, j int) bool {
		if g.routes[i].path != g.routes[j].path {
			return g.routes[i].path < g.routes[j].path
		}
		return g.routes[i].method < g.routes[j].method
	})

	for _, rt := range g.routes {
		op := map[string]any{
			"operationId": operationID(rt),
			"tags":        []string{tagFor(rt.pkg)},
			"summary":     summaryFor(rt),
		}
		var anno *handlerAnno
		if m := g.annos[rt.pkg]; m != nil {
			anno = m[rt.handler]
		}

		// Parameters: path params from the template, plus annotated query params.
		var params []any
		pathNames := map[string]bool{}
		for _, m := range pathParam.FindAllStringSubmatch(rt.path, -1) {
			pathNames[m[1]] = true
			params = append(params, map[string]any{
				"name": m[1], "in": "path", "required": true,
				"schema": map[string]any{"type": "string"},
			})
		}
		if anno != nil {
			for _, pn := range anno.paramOrd {
				if pathNames[pn] {
					continue
				}
				params = append(params, map[string]any{
					"name": pn, "in": "query", "required": false,
					"schema": map[string]any{"type": "string"},
				})
			}
		}
		if len(params) > 0 {
			op["parameters"] = params
		}

		if anno != nil && anno.request != "" && rt.method != "GET" {
			op["requestBody"] = map[string]any{
				"content": map[string]any{
					"application/json": map[string]any{"schema": g.schemaForRef(anno.request)},
				},
			}
		}

		respSchema := map[string]any{}
		if anno != nil && anno.response != "" {
			respSchema = g.schemaForRef(anno.response)
		}
		op["responses"] = map[string]any{
			"200": map[string]any{
				"description": "OK",
				"content":     map[string]any{"application/json": map[string]any{"schema": respSchema}},
			},
			"default": map[string]any{
				"description": "Error",
				"content":     map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/APIError"}}},
			},
		}

		pi, _ := paths[rt.path].(map[string]any)
		if pi == nil {
			pi = map[string]any{}
			paths[rt.path] = pi
		}
		pi[strings.ToLower(rt.method)] = op
	}

	// Hard-code the apiframework error envelope used by every route.
	g.schemas["APIError"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"error": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
					"type":    map[string]any{"type": "string"},
					"code":    map[string]any{"type": "string"},
				},
			},
		},
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Contenox Runtime API",
			"version":     ver,
			"description": "HTTP API served by `contenox serve` under /api. Generated from route annotations by tools/openapi-gen.",
		},
		"servers": []any{map[string]any{"url": "/api"}},
		"paths":   paths,
		"components": map[string]any{
			"schemas": g.schemas,
		},
	}
}

// schemaForRef resolves a "pkg.Type" annotation reference to a schema (a $ref
// when the type is a known struct, else a generic object). Supports "[]pkg.Type",
// "[]*pkg.Type", and "*pkg.Type" prefixes used in route response annotations.
func (g *generator) schemaForRef(ref string) map[string]any {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "[]") {
		inner := strings.TrimPrefix(ref, "[]")
		inner = strings.TrimPrefix(inner, "*")
		return map[string]any{"type": "array", "items": g.schemaForRef(inner)}
	}
	if strings.HasPrefix(ref, "*") {
		return g.schemaForRef(ref[1:])
	}
	dot := strings.LastIndex(ref, ".")
	if dot < 0 {
		return map[string]any{"type": "object"}
	}
	pkg, name := ref[:dot], ref[dot+1:]
	// allow refs like "internalchatapi.chatResponse" where pkg may contain dots
	if i := strings.LastIndex(pkg, "."); i >= 0 {
		pkg = pkg[i+1:]
	}
	return g.refForNamed(pkg, name)
}

func (g *generator) refForNamed(pkg, name string) map[string]any {
	key := pkg + "." + name
	si := g.structs[key]
	if si == nil {
		if a, ok := g.aliases[key]; ok {
			return g.schemaForExpr(a.underlying, a.pkg)
		}
		return map[string]any{"type": "object", "description": "Go type " + key}
	}
	compKey := sanitize(key)
	if !g.pending[compKey] {
		if _, done := g.schemas[compKey]; !done {
			g.pending[compKey] = true
			g.schemas[compKey] = g.structSchema(si)
		}
	}
	return map[string]any{"$ref": "#/components/schemas/" + compKey}
}

func (g *generator) structSchema(si *structInfo) map[string]any {
	props := map[string]any{}
	for _, field := range si.st.Fields.List {
		jsonName, incType, skip := parseTag(field.Tag)
		if skip {
			continue
		}
		if len(field.Names) == 0 {
			// embedded field: merge the embedded struct's properties best-effort
			if emb := g.embeddedSchema(field.Type, si.pkg); emb != nil {
				if ep, ok := emb["properties"].(map[string]any); ok {
					maps.Copy(props, ep)
				}
			}
			continue
		}
		for _, nm := range field.Names {
			if !nm.IsExported() {
				continue
			}
			pname := jsonName
			if pname == "" {
				pname = nm.Name
			}
			if incType != "" {
				props[pname] = g.schemaForRef(incType)
			} else {
				props[pname] = g.schemaForExpr(field.Type, si.pkg)
			}
		}
	}
	return map[string]any{"type": "object", "properties": props}
}

func (g *generator) embeddedSchema(expr ast.Expr, pkg string) map[string]any {
	switch v := expr.(type) {
	case *ast.Ident:
		if si := g.structs[pkg+"."+v.Name]; si != nil {
			return g.structSchema(si)
		}
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			if si := g.structs[x.Name+"."+v.Sel.Name]; si != nil {
				return g.structSchema(si)
			}
		}
	}
	return nil
}

func (g *generator) schemaForExpr(expr ast.Expr, pkg string) map[string]any {
	switch v := expr.(type) {
	case *ast.Ident:
		if t, ok := basicTypes[v.Name]; ok {
			return map[string]any{"type": t}
		}
		if v.Name == "any" {
			return map[string]any{}
		}
		return g.refForNamed(pkg, v.Name)
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			full := x.Name + "." + v.Sel.Name
			switch full {
			case "time.Time":
				return map[string]any{"type": "string", "format": "date-time"}
			case "time.Duration":
				return map[string]any{"type": "integer", "description": "nanoseconds"}
			case "json.RawMessage":
				return map[string]any{}
			}
			return g.refForNamed(x.Name, v.Sel.Name)
		}
		return map[string]any{"type": "object"}
	case *ast.StarExpr:
		return g.schemaForExpr(v.X, pkg)
	case *ast.ArrayType:
		return map[string]any{"type": "array", "items": g.schemaForExpr(v.Elt, pkg)}
	case *ast.MapType:
		return map[string]any{"type": "object", "additionalProperties": g.schemaForExpr(v.Value, pkg)}
	case *ast.InterfaceType:
		return map[string]any{}
	case *ast.StructType:
		return g.structSchema(&structInfo{pkg: pkg, st: v})
	}
	return map[string]any{"type": "object"}
}

func parseTag(tag *ast.BasicLit) (jsonName, includeType string, skip bool) {
	if tag == nil {
		return "", "", false
	}
	st := reflect.StructTag(strings.Trim(tag.Value, "`"))
	if j, ok := st.Lookup("json"); ok {
		name, _, _ := strings.Cut(j, ",")
		if name == "-" {
			return "", "", true
		}
		jsonName = name
	}
	includeType = st.Get("openapi_include_type")
	return jsonName, includeType, false
}

func operationID(rt route) string {
	if rt.handler != "" {
		return tagFor(rt.pkg) + "_" + rt.handler
	}
	slug := strings.NewReplacer("/", "_", "{", "", "}", "").Replace(strings.Trim(rt.path, "/"))
	return strings.ToLower(rt.method) + "_" + slug
}

func summaryFor(rt route) string {
	return rt.method + " " + rt.path
}

func tagFor(pkg string) string {
	return strings.TrimSuffix(pkg, "api")
}

func sanitize(s string) string {
	return strings.NewReplacer(".", "_", "/", "_").Replace(s)
}

func version(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "runtime/version/version.txt"))
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(b))
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from working directory")
		}
		dir = parent
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "openapi-gen:", err)
	os.Exit(1)
}
