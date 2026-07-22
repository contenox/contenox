// Package openapigen generates runtime/internal/openapidocs/openapi.json from
// the HTTP route registrations and the inline annotations in the route
// packages. It is the library core behind the tools/openapi-gen command (and
// the `go generate` directive in runtime/internal/openapidocs/embed.go);
// Generate returns the exact bytes the command writes to disk, so a test can
// byte-compare a fresh generation against the embedded file.
//
// Conventions consumed:
//
//	mux.HandleFunc("GET /backends/{id}", h.get)   // route -> handler method
//	mux.HandleFunc("GET /files", wh.wrap((*handler).list)) // wrapped mount (see below)
//	... // @request  pkg.RequestType                (inside the handler body)
//	... // @request  none <reason>                  (explicitly bodyless non-GET)
//	... // @request  binary <description>           (raw application/octet-stream body)
//	... // @response pkg.ResponseType               (JSON 200)
//	... // @response none <reason>                  (204, no content)
//	... // @response binary <description>           (200 application/octet-stream)
//	... // @response sse <description>              (200 text/event-stream)
//	... // @response redirect <description>         (302, no content, no 200)
//	... // @param    name type description...       (escape-hatch parameter)
//	Field T `json:"x" openapi_include_type:"pkg.Type"`  // document a named type
//
// Annotations also bind when the registered handler is a function literal
// passed directly to HandleFunc — comments inside the closure body document
// that route, and helper calls in the closure body feed its parameters.
//
// Parameters are derived primarily from the apiframework helper calls in the
// handler's own body (calls inside nested function literals are not seen):
//
//   - apiframework.GetQueryParam(r, "name", "default", "description") emits a
//     string query parameter carrying the description, with a schema default
//     when the default literal is non-empty.
//   - apiframework.LimitParam(r, n) emits the integer `limit` query parameter,
//     apiframework.CursorParam(r) the string `cursor` one, and
//     apiframework.ListParams(r, n) both — each with its canonical description.
//   - apiframework.GetPathParam(r, "name", "description") attaches the
//     description to the path parameter the route template already declares; a
//     name that appears in no route template bound to that handler is a strict
//     error.
//
// The helper is matched by function name under any package qualifier (or bare
// when dot-imported). Only string literals are consumed: a non-literal
// parameter NAME is a strict error at the call site, while a non-literal
// default or description is consumed as absent (counted in Stats). The
// `@param name type description...` annotation (type one of string, integer,
// number, boolean) remains the escape hatch for parameters the scan cannot
// see. Helper-derived and @param-derived parameters merge first-wins by name,
// helpers first; the emitted order is deterministic — path parameters in
// template order, then query parameters sorted by name, all `required: false`
// except path parameters.
//
// An operation's summary is the first sentence of the handler's GoDoc when it
// has one, else "METHOD /path".
//
// Annotations bind to the handler a route registers, keyed by
// ReceiverType.Method (receiver-less functions by bare name), so two handler
// types in one package may share method names without cross-contaminating each
// other's routes. To resolve `h.get` to its receiver type, the registering
// function's simple local bindings (`h := &fooHandler{...}`, `h := fooHandler{}`,
// `var h fooHandler`) and its own receiver are tracked. When a handler
// expression cannot be resolved to a receiver type, the bare name is used only
// if exactly one annotated function in the package carries it; more than one is
// an ambiguous-binding error.
//
// A registration of the form `mux.HandleFunc(spec, wh.wrap((*handler).list))`
// — a single-argument wrapper call whose argument names a function or method —
// binds the route to the NAMED function (here handler.list) for annotations,
// operationId, and summary. The wrapper's whole body, including the
// http.HandlerFunc closure it returns, additionally feeds parameter
// derivation, because that closure is the code that actually runs on the
// route (localfileapi's workspace mount reads the `root` query parameter
// there).
//
// A route-registration function whose doc comment contains a line
//
//	// openapi:exclude <one-line reason>
//
// is skipped entirely: no HandleFunc call inside its body is documented. The
// spec declares `servers: [{"url": "/api"}]`, so this is for registrars that
// serve mounts OUTSIDE the /api prefix (the root-mux /v1/* and Ollama-native
// compat aliases) and for alternate mounts that re-register an already
// documented surface (localfileapi's single-root ProjectRoot fallback).
//
// Generation is strict — problems are collected in full and reported together
// as an error, never silently degraded: a scanned directory that fails to
// parse, a route-scan glob matching no directories, a type reference that does
// not resolve to a known struct or named alias, the same METHOD+path
// registered twice in the scanned set, ambiguous annotation bindings, a
// non-literal parameter name in a consumed helper call, a malformed or
// invalid-typed @param, and a GetPathParam naming no bound route's template
// parameter all fail the run.
package openapigen

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/contenox/runtime/apiframework"
)

// OutPath is the repo-root-relative path of the generated spec — where the
// tools/openapi-gen command writes it and openapidocs embeds it from.
const OutPath = "runtime/internal/openapidocs/openapi.json"

// routeScanGlobs hold the HTTP route registrations + their request/response types.
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
	"runtime/fleetservice",
	"runtime/missionchanges",
	"runtime/missionservice",
	"runtime/operatorinbox",
	"runtime/presence",
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
	"runtime/toolsproviderservice",
}

// Stats summarizes a successful generation.
type Stats struct {
	Paths   int
	Schemas int
	// NonLiteralParamArgs counts default/description arguments of consumed
	// apiframework helper calls that were not string literals and were
	// therefore consumed as absent (the parameter still emits, undescribed).
	NonLiteralParamArgs int
}

// Generate scans the route packages under root (a repo checkout containing
// go.mod), builds the OpenAPI document, and returns the exact file bytes
// (trailing newline included) plus counts. Generation is deterministic: the
// same tree yields byte-identical output. Any strict-mode problem (see the
// package doc) is collected — all of them, not just the first — and returned
// as a single error.
func Generate(root string) ([]byte, Stats, error) {
	g := &generator{
		root:    root,
		fset:    token.NewFileSet(),
		structs: map[string]*structInfo{},
		aliases: map[string]aliasInfo{},
		funcs:   map[string]map[string]*funcInfo{},
		schemas: map[string]any{},
		pending: map[string]bool{},
	}
	g.scan()
	doc := g.build(readVersion(root))
	if len(g.problems) > 0 {
		return nil, Stats{}, fmt.Errorf("openapi generation failed with %d problem(s):\n  - %s",
			len(g.problems), strings.Join(g.problems, "\n  - "))
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, Stats{}, err
	}
	out = append(out, '\n')
	paths, _ := doc["paths"].(map[string]any)
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	return out, Stats{Paths: len(paths), Schemas: len(schemas), NonLiteralParamArgs: g.nonLiteralArgs}, nil
}

type structInfo struct {
	pkg string
	st  *ast.StructType
}

type aliasInfo struct {
	pkg        string
	underlying ast.Expr
}

// annoParam is one typed `@param name type description...` escape-hatch entry.
type annoParam struct {
	name string
	typ  string // string | integer | number | boolean (validated at collection)
	desc string
}

// Annotation kinds. The zero value ("") means "not annotated" — the coverage
// gate turns that into a strict error on every documented operation.
const (
	kindJSON     = "json"     // @request/@response pkg.Type — JSON body
	kindNone     = "none"     // @request/@response none <reason> — no body/content
	kindBinary   = "binary"   // @response binary <description> — application/octet-stream
	kindSSE      = "sse"      // @response sse <description> — text/event-stream
	kindRedirect = "redirect" // @response redirect <description> — 302, no content
)

type handlerAnno struct {
	reqKind  string    // "", kindJSON, kindNone, or kindBinary
	request  string    // "pkg.Type" when reqKind is kindJSON
	reqText  string    // reason/description for the non-JSON request kinds
	respKind string    // "", kindJSON, kindNone, kindBinary, kindSSE, or kindRedirect
	response string    // "pkg.Type" when respKind is kindJSON
	respText string    // reason/description for the non-JSON response kinds
	reqPos   token.Pos // position of the @request comment, for error reports
	respPos  token.Pos
	// docSummary is the first sentence of the function's GoDoc; when non-empty
	// becomes the operation summary.
	docSummary string
	params     []annoParam // @param entries, first-wins by name, in comment order
}

// derivedParam is one parameter derived from an apiframework helper call
// inside a function body.
type derivedParam struct {
	name string
	typ  string // "string" or "integer"
	desc string
	def  string // schema default; "" means none
	// path marks a GetPathParam call: it attaches desc to the route template's
	// path parameter instead of declaring a query parameter.
	path bool
	// inFuncLit marks a call inside a nested function literal; such calls are
	// consumed only when the function is used as a handler-returning wrapper
	// (its returned closure IS the handler).
	inFuncLit bool
	pos       token.Pos
}

// deferredProblem is a strict problem (or countable dropped argument) found
// while scanning a function's helper calls. It is reported only when a
// documented route actually consumes the function, so a never-routed helper
// in a route package cannot fail the run.
type deferredProblem struct {
	pos       token.Pos
	msg       string
	inFuncLit bool
}

// funcInfo carries everything the generator knows about one function in a
// route package: its annotations and GoDoc summary, and the parameters
// derived from apiframework helper calls in its body.
type funcInfo struct {
	anno       handlerAnno
	pos        token.Pos // declaration (or closure) position, for gate reports
	derived    []derivedParam
	problems   []deferredProblem // non-literal parameter names
	nonLiteral []deferredProblem // non-literal default/description args (counted, not fatal)
}

// documentable reports whether fi carries any information a route could bind —
// the criterion for the bare-name fallback candidate set.
func (fi *funcInfo) documentable() bool {
	return fi.anno.reqKind != "" || fi.anno.respKind != "" || fi.anno.docSummary != "" ||
		len(fi.anno.params) > 0 || len(fi.derived) > 0
}

type route struct {
	method string
	path   string
	pkg    string
	// handlerBare is the handler's bare name ("get" for `h.get`), "" if the
	// handler expression has no name (inline closure, unrecognized call). It
	// feeds the operationId.
	handlerBare string
	// handlerKey is the resolved annotation key ("fooHandler.get", or the bare
	// name for a package-level function handler); "" when the receiver could
	// not be resolved, in which case bare-name fallback rules apply.
	handlerKey string
	// wrapperKey names the function whose single-argument CALL produced the
	// registered handler (`wh.wrap((*handler).list)` -> "workspaceHandler.wrap").
	// The wrapper's whole body — including the closure it returns, which is the
	// code that actually serves the route — additionally feeds parameter
	// derivation. "" for direct registrations.
	wrapperKey string
	// closure holds the info collected from a function literal registered
	// directly on the route (`mux.HandleFunc(spec, func(w, r) {...})`): the
	// annotations and helper calls inside the closure body ARE the handler's.
	closure *funcInfo
	pos     token.Pos
}

type generator struct {
	root    string
	fset    *token.FileSet
	structs map[string]*structInfo // "pkg.Type" -> struct
	aliases map[string]aliasInfo   // "pkg.Type" -> underlying type of a non-struct named type
	// per package: function key ("Recv.name" or bare "name") -> collected info
	funcs          map[string]map[string]*funcInfo
	routes         []route
	schemas        map[string]any // component schemas, key sanitized
	pending        map[string]bool
	problems       []string
	nonLiteralArgs int
}

var basicTypes = map[string]string{
	"string": "string", "bool": "boolean",
	"int": "integer", "int8": "integer", "int16": "integer", "int32": "integer", "int64": "integer",
	"uint": "integer", "uint8": "integer", "uint16": "integer", "uint32": "integer", "uint64": "integer",
	"float32": "number", "float64": "number", "byte": "integer", "rune": "integer",
}

// paramTypes are the OpenAPI scalar types @param accepts.
var paramTypes = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true}

func (g *generator) problemf(pos token.Pos, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if pos.IsValid() {
		msg = g.rel(g.fset.Position(pos).String()) + ": " + msg
	}
	g.problems = append(g.problems, msg)
}

// rel strips the repo root from a path (or position string) for readable reports.
func (g *generator) rel(s string) string {
	return strings.TrimPrefix(s, g.root+string(filepath.Separator))
}

func (g *generator) scan() {
	var routeDirs []string
	for _, glob := range routeScanGlobs {
		matches, _ := filepath.Glob(filepath.Join(g.root, glob))
		var dirs []string
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.IsDir() {
				dirs = append(dirs, m)
			}
		}
		if len(dirs) == 0 {
			g.problemf(token.NoPos, "route scan glob %q matched no directories under %s", glob, g.root)
		}
		routeDirs = append(routeDirs, dirs...)
	}
	sort.Strings(routeDirs)

	// Parse everything to collect struct definitions globally, and collect
	// routes + annotations from route packages.
	allDirs := append([]string{}, routeDirs...)
	for _, d := range typeScanDirs {
		allDirs = append(allDirs, filepath.Join(g.root, d))
	}
	routeSet := map[string]bool{}
	for _, d := range routeDirs {
		routeSet[d] = true
	}
	for _, dir := range allDirs {
		g.parseDir(dir, routeSet[dir])
	}
}

func (g *generator) parseDir(dir string, isRoute bool) {
	pkgs, err := parser.ParseDir(g.fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		g.problemf(token.NoPos, "%s: %v", g.rel(dir), err)
		return
	}
	// Map iteration order is random; walk packages and files sorted so struct
	// collection, problem reports, and output stay deterministic.
	pkgNames := sortedKeys(pkgs)
	for _, pkgName := range pkgNames {
		fileNames := sortedKeys(pkgs[pkgName].Files)
		for _, fileName := range fileNames {
			file := pkgs[pkgName].Files[fileName]
			g.collectStructs(pkgName, file)
			if isRoute {
				g.collectFuncs(pkgName, file)
				g.collectRoutes(pkgName, file)
			}
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	reRequestLine  = regexp.MustCompile(`@request\s+(\S+)\s*(.*)$`)
	reResponseLine = regexp.MustCompile(`@response\s+(\S+)\s*(.*)$`)
	reParamLine    = regexp.MustCompile(`@param\s+(\S+)(?:\s+(\S+))?\s*(.*)$`)
)

// responseKinds maps the non-type first word of a @response line to its kind;
// any other word is a type reference (kindJSON).
var responseKinds = map[string]string{
	"none": kindNone, "binary": kindBinary, "sse": kindSSE, "redirect": kindRedirect,
}

// collectFuncs records, for every function declaration in a route package,
// its GoDoc summary, body annotations (@request/@response/@param), and the
// parameters derived from apiframework helper calls in its body.
func (g *generator) collectFuncs(pkg string, file *ast.File) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		fi := &funcInfo{pos: fd.Pos()}
		if fd.Doc != nil {
			// First sentence, not first line: GoDoc wraps lines mid-sentence.
			fi.anno.docSummary = strings.TrimSpace(doc.Synopsis(fd.Doc.Text()))
		}
		g.collectAnnotations(file, fd.Body.Pos(), fd.Body.End(), fd.Name.Name, fi)
		g.scanHelperCalls(fd.Body, fi)
		if g.funcs[pkg] == nil {
			g.funcs[pkg] = map[string]*funcInfo{}
		}
		if _, exists := g.funcs[pkg][annoKey(fd)]; !exists {
			g.funcs[pkg][annoKey(fd)] = fi
		}
	}
}

// collectAnnotations parses every @request/@response/@param comment line
// between start and end (a function or closure body) into fi. The first
// @request and the first @response line win; the non-JSON kinds (none,
// binary, sse, redirect) require their trailing reason/description text.
func (g *generator) collectAnnotations(file *ast.File, start, end token.Pos, owner string, fi *funcInfo) {
	seenParam := map[string]bool{}
	for _, cg := range file.Comments {
		if cg.Pos() < start || cg.End() > end {
			continue
		}
		for _, line := range strings.Split(cg.Text(), "\n") {
			if strings.Contains(line, "@request") && fi.anno.reqKind == "" {
				if m := reRequestLine.FindStringSubmatch(line); m != nil {
					word, rest := m[1], strings.TrimSpace(m[2])
					fi.anno.reqPos = cg.Pos()
					switch word {
					case "none", "binary":
						fi.anno.reqKind = map[string]string{"none": kindNone, "binary": kindBinary}[word]
						fi.anno.reqText = rest
						if rest == "" {
							g.problemf(cg.Pos(), "@request %s on %s requires a %s", word, owner,
								map[bool]string{true: "reason", false: "description"}[word == "none"])
						}
					default:
						fi.anno.reqKind = kindJSON
						fi.anno.request = word
					}
				}
			}
			if strings.Contains(line, "@response") && fi.anno.respKind == "" {
				if m := reResponseLine.FindStringSubmatch(line); m != nil {
					word, rest := m[1], strings.TrimSpace(m[2])
					fi.anno.respPos = cg.Pos()
					if kind, ok := responseKinds[word]; ok {
						fi.anno.respKind = kind
						fi.anno.respText = rest
						if rest == "" {
							g.problemf(cg.Pos(), "@response %s on %s requires a %s", word, owner,
								map[bool]string{true: "reason", false: "description"}[kind == kindNone])
						}
					} else {
						fi.anno.respKind = kindJSON
						fi.anno.response = word
					}
				}
			}
			if !strings.Contains(line, "@param") {
				continue
			}
			m := reParamLine.FindStringSubmatch(line)
			if m == nil || m[2] == "" {
				g.problemf(cg.Pos(), "malformed @param on %s: want `@param name type [description...]`", owner)
				continue
			}
			name, typ, desc := m[1], m[2], strings.TrimSpace(m[3])
			if !paramTypes[typ] {
				g.problemf(cg.Pos(), "@param %s on %s: type %q must be one of string, integer, number, boolean", name, owner, typ)
				continue
			}
			if !seenParam[name] {
				seenParam[name] = true
				fi.anno.params = append(fi.anno.params, annoParam{name: name, typ: typ, desc: desc})
			}
		}
	}
}

// paramHelperNames are the apiframework helpers parameter derivation consumes,
// matched by bare function name under any selector qualifier so import
// aliasing (or dot-importing) cannot hide a call.
var paramHelperNames = map[string]bool{
	"GetQueryParam": true, "LimitParam": true, "CursorParam": true,
	"ListParams": true, "GetPathParam": true,
}

// scanHelperCalls walks a function (or closure) body for apiframework
// parameter-helper calls. Calls inside nested function literals are recorded
// with inFuncLit set, so they only count when the function is consumed as a
// handler-returning wrapper — for a plain handler, only its direct body is
// the truth source.
func (g *generator) scanHelperCalls(body *ast.BlockStmt, fi *funcInfo) {
	var walk func(n ast.Node, inLit bool)
	walk = func(n ast.Node, inLit bool) {
		ast.Inspect(n, func(m ast.Node) bool {
			if fl, ok := m.(*ast.FuncLit); ok {
				walk(fl.Body, true)
				return false
			}
			if call, ok := m.(*ast.CallExpr); ok {
				g.recordHelperCall(call, inLit, fi)
			}
			return true
		})
	}
	walk(body, false)
}

func calleeName(fun ast.Expr) string {
	switch v := fun.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.Ident:
		return v.Name
	}
	return ""
}

// stringLit unquotes a string-literal expression; ok is false for anything
// that is not a string literal.
func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return strings.Trim(bl.Value, "`\""), true
	}
	return s, true
}

func (g *generator) recordHelperCall(call *ast.CallExpr, inLit bool, fi *funcInfo) {
	name := calleeName(call.Fun)
	if !paramHelperNames[name] {
		return
	}
	// optString consumes an optional (default/description) argument: a
	// non-literal is not an error, but it is counted — the parameter emits
	// without it.
	optString := func(e ast.Expr) string {
		s, ok := stringLit(e)
		if !ok {
			fi.nonLiteral = append(fi.nonLiteral, deferredProblem{pos: e.Pos(), inFuncLit: inLit})
			return ""
		}
		return s
	}
	switch name {
	case "GetQueryParam":
		if len(call.Args) != 4 {
			return
		}
		pname, ok := stringLit(call.Args[1])
		if !ok {
			fi.problems = append(fi.problems, deferredProblem{pos: call.Args[1].Pos(),
				msg: "GetQueryParam: parameter name must be a string literal", inFuncLit: inLit})
			return
		}
		fi.derived = append(fi.derived, derivedParam{
			name: pname, typ: "string",
			def:       optString(call.Args[2]),
			desc:      optString(call.Args[3]),
			inFuncLit: inLit, pos: call.Pos(),
		})
	case "LimitParam":
		if len(call.Args) != 2 {
			return
		}
		fi.derived = append(fi.derived, derivedParam{
			name: "limit", typ: "integer", desc: apiframework.LimitParamDescription,
			inFuncLit: inLit, pos: call.Pos(),
		})
	case "CursorParam":
		if len(call.Args) != 1 {
			return
		}
		fi.derived = append(fi.derived, derivedParam{
			name: "cursor", typ: "string", desc: apiframework.CursorParamDescription,
			inFuncLit: inLit, pos: call.Pos(),
		})
	case "ListParams":
		if len(call.Args) != 2 {
			return
		}
		fi.derived = append(fi.derived,
			derivedParam{name: "cursor", typ: "string", desc: apiframework.CursorParamDescription,
				inFuncLit: inLit, pos: call.Pos()},
			derivedParam{name: "limit", typ: "integer", desc: apiframework.LimitParamDescription,
				inFuncLit: inLit, pos: call.Pos()},
		)
	case "GetPathParam":
		if len(call.Args) != 3 {
			return
		}
		pname, ok := stringLit(call.Args[1])
		if !ok {
			fi.problems = append(fi.problems, deferredProblem{pos: call.Args[1].Pos(),
				msg: "GetPathParam: parameter name must be a string literal", inFuncLit: inLit})
			return
		}
		fi.derived = append(fi.derived, derivedParam{
			name: pname, typ: "string", desc: optString(call.Args[2]),
			path: true, inFuncLit: inLit, pos: call.Pos(),
		})
	}
}

// annoKey keys a function's collected info: methods as "ReceiverType.name" so
// same-named methods on different handler types cannot collide, plain
// functions by bare name.
func annoKey(fd *ast.FuncDecl) string {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		if t := typeExprName(fd.Recv.List[0].Type); t != "" {
			return t + "." + fd.Name.Name
		}
	}
	return fd.Name.Name
}

// excludeDirective reports whether fd's doc comment carries an
// `// openapi:exclude <reason>` line, plus the reason and the line's position.
func excludeDirective(fd *ast.FuncDecl) (excluded bool, reason string, pos token.Pos) {
	if fd.Doc == nil {
		return false, "", token.NoPos
	}
	for _, c := range fd.Doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), " "))
		if rest, ok := strings.CutPrefix(line, "openapi:exclude"); ok {
			return true, strings.TrimSpace(rest), c.Pos()
		}
	}
	return false, "", token.NoPos
}

func (g *generator) collectRoutes(pkg string, file *ast.File) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		if excluded, reason, pos := excludeDirective(fd); excluded {
			if reason == "" {
				g.problemf(pos, "openapi:exclude on %s requires a one-line reason", fd.Name.Name)
			}
			continue
		}
		bindings := localBindings(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
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
			bare, key, wrapper := resolveHandlerExpr(call.Args[1], bindings)
			var closure *funcInfo
			if fl, ok := call.Args[1].(*ast.FuncLit); ok {
				// Closure handler: the annotations and helper calls inside the
				// literal's body are this route's truth source.
				closure = &funcInfo{pos: fl.Pos()}
				g.collectAnnotations(file, fl.Body.Pos(), fl.Body.End(),
					fmt.Sprintf("closure handler in %s", fd.Name.Name), closure)
				g.scanHelperCalls(fl.Body, closure)
			}
			g.routes = append(g.routes, route{
				method:      strings.ToUpper(parts[0]),
				path:        parts[1],
				pkg:         pkg,
				handlerBare: bare,
				handlerKey:  key,
				wrapperKey:  wrapper,
				closure:     closure,
				pos:         call.Pos(),
			})
			return true
		})
	}
}

// localBindings maps simple local variable names inside fd to the handler type
// they hold: the function's own receiver, `x := &Type{...}` / `x := Type{...}` /
// `x = &Type{...}` assignments, and `var x Type` / `var x = &Type{...}`
// declarations. A name rebound to a different type becomes unresolvable rather
// than guessing.
func localBindings(fd *ast.FuncDecl) map[string]string {
	bindings := map[string]string{}
	bind := func(name, typ string) {
		if name == "" || name == "_" || typ == "" {
			return
		}
		if prev, ok := bindings[name]; ok && prev != typ {
			bindings[name] = "" // conflicting rebind: refuse to resolve
			return
		}
		bindings[name] = typ
	}
	if fd.Recv != nil && len(fd.Recv.List) > 0 && len(fd.Recv.List[0].Names) > 0 {
		bind(fd.Recv.List[0].Names[0].Name, typeExprName(fd.Recv.List[0].Type))
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			if len(v.Lhs) != len(v.Rhs) {
				return true
			}
			for i, lhs := range v.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				bind(id.Name, valueTypeName(v.Rhs[i]))
			}
		case *ast.ValueSpec:
			typ := ""
			if v.Type != nil {
				typ = typeExprName(v.Type)
			}
			for i, name := range v.Names {
				t := typ
				if t == "" && i < len(v.Values) {
					t = valueTypeName(v.Values[i])
				}
				bind(name.Name, t)
			}
		}
		return true
	})
	return bindings
}

// valueTypeName names the handler type of a simple RHS value: `&Type{...}`,
// `Type{...}`. Anything else (a call, a field read, ...) is unresolvable.
func valueTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return valueTypeName(v.X)
		}
	case *ast.CompositeLit:
		return typeExprName(v.Type)
	}
	return ""
}

// typeExprName names a type expression's base identifier: `Type`, `*Type`.
// Cross-package types (`pkg.Type`) return "" — handlers are always registered
// in their own package.
func typeExprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return typeExprName(v.X)
	}
	return ""
}

// resolveHandlerExpr names the handler a route registers. For `x.method` the
// receiver variable is looked up in bindings to produce a receiver-qualified
// annotation key; a bare function identifier is its own key. A single-argument
// wrapper call whose argument is a selector — `wh.wrap((*handler).list)` —
// resolves to the named inner function, and additionally names the wrapper
// (receiver-qualified when its receiver resolves) so the wrapper's body feeds
// parameter derivation. An unnameable expression (closure, unrecognized call)
// yields ("", "", "").
func resolveHandlerExpr(e ast.Expr, bindings map[string]string) (bare, key, wrapper string) {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		bare = v.Sel.Name
		if x, ok := v.X.(*ast.Ident); ok {
			if t := bindings[x.Name]; t != "" {
				key = t + "." + bare
			}
		}
	case *ast.Ident:
		bare = v.Name
		key = bare
	case *ast.CallExpr:
		if len(v.Args) != 1 {
			return "", "", ""
		}
		sel, ok := v.Args[0].(*ast.SelectorExpr)
		if !ok {
			return "", "", ""
		}
		bare = sel.Sel.Name
		switch x := sel.X.(type) {
		case *ast.ParenExpr: // method expression: (*Type).name
			if st, ok := x.X.(*ast.StarExpr); ok {
				if id, ok := st.X.(*ast.Ident); ok {
					key = id.Name + "." + bare
				}
			}
		case *ast.Ident: // bound method value: v.name
			if t := bindings[x.Name]; t != "" {
				key = t + "." + bare
			}
		}
		switch f := v.Fun.(type) {
		case *ast.SelectorExpr:
			if id, ok := f.X.(*ast.Ident); ok {
				if t := bindings[id.Name]; t != "" {
					wrapper = t + "." + f.Sel.Name
				}
			}
		case *ast.Ident:
			wrapper = f.Name
		}
	}
	return bare, key, wrapper
}

// funcFor resolves the funcInfo rt's handler binds. A closure registered
// directly on the route is authoritative, then a receiver-qualified key. An
// unresolved receiver falls back to the bare name only when exactly one
// documentable function in the package carries it; several candidates are an
// ambiguous binding and a strict error.
func (g *generator) funcFor(rt route) *funcInfo {
	if rt.closure != nil {
		return rt.closure
	}
	m := g.funcs[rt.pkg]
	if m == nil {
		return nil
	}
	if rt.handlerKey != "" {
		return m[rt.handlerKey]
	}
	if rt.handlerBare == "" {
		return nil
	}
	var candidates []string
	for key, fi := range m {
		if !fi.documentable() {
			continue
		}
		if key == rt.handlerBare || strings.HasSuffix(key, "."+rt.handlerBare) {
			candidates = append(candidates, key)
		}
	}
	sort.Strings(candidates)
	switch len(candidates) {
	case 0:
		return nil
	case 1:
		return m[candidates[0]]
	default:
		g.problemf(rt.pos, "ambiguous annotation binding for handler %q on %s %s: cannot resolve the receiver and %d annotated functions match: %s",
			rt.handlerBare, rt.method, rt.path, len(candidates), strings.Join(candidates, ", "))
		return nil
	}
}

// checkDuplicateRoutes reports every METHOD+path registered more than once in
// the scanned set, with each registration site.
func (g *generator) checkDuplicateRoutes() {
	byKey := map[string][]route{}
	for _, rt := range g.routes {
		k := rt.method + " " + rt.path
		byKey[k] = append(byKey[k], rt)
	}
	for _, k := range sortedKeys(byKey) {
		rts := byKey[k]
		if len(rts) < 2 {
			continue
		}
		sites := make([]string, len(rts))
		for i, rt := range rts {
			sites[i] = g.rel(g.fset.Position(rt.pos).String())
		}
		sort.Strings(sites)
		g.problemf(token.NoPos, "duplicate route registration %q: registered at %s", k, strings.Join(sites, " and "))
	}
}

// consumeState tracks how routes consumed one funcInfo: whether any route used
// it as a wrapper (so its func-literal calls count), and the union of path
// parameter names across every route template bound to it — the set a
// GetPathParam name must hit.
type consumeState struct {
	asWrapper bool
	pathNames map[string]bool
}

var pathParamRe = regexp.MustCompile(`\{(\w+)\}`)

func (g *generator) build(ver string) map[string]any {
	paths := map[string]any{}

	g.checkDuplicateRoutes()

	sort.Slice(g.routes, func(i, j int) bool {
		if g.routes[i].path != g.routes[j].path {
			return g.routes[i].path < g.routes[j].path
		}
		return g.routes[i].method < g.routes[j].method
	})

	opIDs := g.assignOperationIDs()

	consumed := map[*funcInfo]*consumeState{}
	var consumedOrder []*funcInfo
	consume := func(fi *funcInfo, asWrapper bool, pathNames map[string]bool) {
		st := consumed[fi]
		if st == nil {
			st = &consumeState{pathNames: map[string]bool{}}
			consumed[fi] = st
			consumedOrder = append(consumedOrder, fi)
		}
		st.asWrapper = st.asWrapper || asWrapper
		maps.Copy(st.pathNames, pathNames)
	}

	for _, rt := range g.routes {
		fi := g.funcFor(rt)
		var wfi *funcInfo
		if rt.wrapperKey != "" {
			wfi = g.funcs[rt.pkg][rt.wrapperKey]
		}

		var pathOrder []string
		pathNames := map[string]bool{}
		for _, m := range pathParamRe.FindAllStringSubmatch(rt.path, -1) {
			if !pathNames[m[1]] {
				pathNames[m[1]] = true
				pathOrder = append(pathOrder, m[1])
			}
		}
		if fi != nil {
			consume(fi, false, pathNames)
		}
		if wfi != nil && wfi != fi {
			consume(wfi, true, pathNames)
		}

		op := map[string]any{
			"operationId": opIDs[rt.method+" "+rt.path],
			"tags":        []string{tagFor(rt.pkg)},
			"summary":     summaryFor(rt, fi),
		}
		if params := g.paramsFor(rt, fi, wfi, pathOrder, pathNames); len(params) > 0 {
			op["parameters"] = params
		}

		// Coverage gate: every operation must state its response truth, and
		// every non-GET/non-DELETE operation its request truth. The `none`
		// forms are the only exemption mechanism — reasons live in-source.
		gatePos := rt.pos
		if fi != nil {
			gatePos = fi.pos
		}
		if fi == nil || fi.anno.respKind == "" {
			g.problemf(gatePos, "missing @response annotation for %s %s (handler %s)",
				rt.method, rt.path, handlerDesc(rt))
		}
		if rt.method != "GET" && rt.method != "DELETE" && (fi == nil || fi.anno.reqKind == "") {
			g.problemf(gatePos, "missing @request annotation for %s %s (handler %s; use `@request none <reason>` if genuinely bodyless)",
				rt.method, rt.path, handlerDesc(rt))
		}

		if fi != nil && rt.method != "GET" {
			switch fi.anno.reqKind {
			case kindJSON:
				rc := refCtx{pos: fi.anno.reqPos, what: fmt.Sprintf("@request for %s %s", rt.method, rt.path)}
				op["requestBody"] = map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{"schema": g.schemaForRef(fi.anno.request, rc)},
					},
				}
			case kindBinary:
				op["requestBody"] = map[string]any{
					"description": fi.anno.reqText,
					"content": map[string]any{
						"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
					},
				}
			}
		}

		responses := map[string]any{
			"default": map[string]any{
				"description": "Error",
				"content":     map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/APIError"}}},
			},
		}
		respKind, respText := "", ""
		if fi != nil {
			respKind, respText = fi.anno.respKind, fi.anno.respText
		}
		switch respKind {
		case kindNone:
			responses["204"] = map[string]any{"description": respText}
		case kindBinary:
			responses["200"] = map[string]any{
				"description": respText,
				"content": map[string]any{
					"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
				},
			}
		case kindSSE:
			responses["200"] = map[string]any{
				"description": respText,
				"content": map[string]any{
					"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}},
				},
			}
		case kindRedirect:
			responses["302"] = map[string]any{"description": respText}
		default: // kindJSON, or unannotated (already a strict error above)
			respSchema := map[string]any{}
			if fi != nil && fi.anno.response != "" {
				rc := refCtx{pos: fi.anno.respPos, what: fmt.Sprintf("@response for %s %s", rt.method, rt.path)}
				respSchema = g.schemaForRef(fi.anno.response, rc)
			}
			responses["200"] = map[string]any{
				"description": "OK",
				"content":     map[string]any{"application/json": map[string]any{"schema": respSchema}},
			}
		}
		op["responses"] = responses

		pi, _ := paths[rt.path].(map[string]any)
		if pi == nil {
			pi = map[string]any{}
			paths[rt.path] = pi
		}
		pi[strings.ToLower(rt.method)] = op
	}

	// Deferred strict checks for every consumed function: non-literal
	// parameter names, dropped non-literal optional args, and GetPathParam
	// names that no bound route template declares. Deferring to consumption
	// keeps a never-routed helper from failing the run, while nothing a
	// documented route relies on can degrade silently.
	for _, fi := range consumedOrder {
		st := consumed[fi]
		active := func(inLit bool) bool { return !inLit || st.asWrapper }
		for _, p := range fi.problems {
			if active(p.inFuncLit) {
				g.problemf(p.pos, "%s", p.msg)
			}
		}
		for _, nl := range fi.nonLiteral {
			if active(nl.inFuncLit) {
				g.nonLiteralArgs++
			}
		}
		for _, dp := range fi.derived {
			if dp.path && active(dp.inFuncLit) && !st.pathNames[dp.name] {
				g.problemf(dp.pos, "GetPathParam %q: no route bound to this handler has {%s} in its path template", dp.name, dp.name)
			}
		}
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

// paramsFor assembles one operation's parameter list: path parameters in
// template order (descriptions attached from GetPathParam calls and matching
// @param entries), then query parameters sorted by name. Sources merge
// first-wins by parameter name in fixed precedence: the handler's own direct
// helper calls (in source order), then the wrapper's calls, then @param
// entries — so the code-derived truth beats the escape hatch on a collision.
func (g *generator) paramsFor(rt route, fi, wfi *funcInfo, pathOrder []string, pathNames map[string]bool) []any {
	pathDesc := map[string]string{}
	pathType := map[string]string{}
	pathSeen := map[string]bool{}
	type queryParam struct{ typ, desc, def string }
	querySeen := map[string]queryParam{}
	var queryNames []string

	addDerived := func(dp derivedParam) {
		if dp.path {
			// Attaches only where this route's template has the name; a name in
			// no bound template at all is reported by the deferred check.
			if pathNames[dp.name] && !pathSeen[dp.name] {
				pathSeen[dp.name] = true
				pathDesc[dp.name] = dp.desc
			}
			return
		}
		if _, ok := querySeen[dp.name]; ok {
			return
		}
		querySeen[dp.name] = queryParam{typ: dp.typ, desc: dp.desc, def: dp.def}
		queryNames = append(queryNames, dp.name)
	}
	if fi != nil {
		for _, dp := range fi.derived {
			if !dp.inFuncLit {
				addDerived(dp)
			}
		}
	}
	if wfi != nil && wfi != fi {
		// The wrapper's WHOLE body counts, func literals included: the closure
		// it returns is the code registered on the route.
		for _, dp := range wfi.derived {
			addDerived(dp)
		}
	}
	if fi != nil {
		for _, ap := range fi.anno.params {
			if pathNames[ap.name] {
				if !pathSeen[ap.name] {
					pathSeen[ap.name] = true
					pathDesc[ap.name] = ap.desc
					pathType[ap.name] = ap.typ
				}
				continue
			}
			if _, ok := querySeen[ap.name]; ok {
				continue
			}
			querySeen[ap.name] = queryParam{typ: ap.typ, desc: ap.desc}
			queryNames = append(queryNames, ap.name)
		}
	}

	var params []any
	for _, name := range pathOrder {
		p := map[string]any{"name": name, "in": "path", "required": true}
		if d := pathDesc[name]; d != "" {
			p["description"] = d
		}
		typ := pathType[name]
		if typ == "" {
			typ = "string"
		}
		p["schema"] = map[string]any{"type": typ}
		params = append(params, p)
	}
	sort.Strings(queryNames)
	for _, name := range queryNames {
		q := querySeen[name]
		p := map[string]any{"name": name, "in": "query", "required": false}
		if q.desc != "" {
			p["description"] = q.desc
		}
		schema := map[string]any{"type": q.typ}
		if q.def != "" {
			schema["default"] = q.def
		}
		p["schema"] = schema
		params = append(params, p)
	}
	return params
}

// refCtx says where a type reference came from, so an unresolved one is
// reported with a file:line and the annotation or field responsible.
type refCtx struct {
	pos  token.Pos
	what string
}

// schemaForRef resolves a "pkg.Type" annotation reference to a schema (a $ref
// when the type is a known struct, an inline schema for a named alias).
// Supports "[]pkg.Type", "[]*pkg.Type", and "*pkg.Type" prefixes used in route
// response annotations. An unresolvable reference is a strict error.
func (g *generator) schemaForRef(ref string, rc refCtx) map[string]any {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "[]") {
		inner := strings.TrimPrefix(ref, "[]")
		inner = strings.TrimPrefix(inner, "*")
		return map[string]any{"type": "array", "items": g.schemaForRef(inner, rc)}
	}
	if strings.HasPrefix(ref, "*") {
		return g.schemaForRef(ref[1:], rc)
	}
	dot := strings.LastIndex(ref, ".")
	if dot < 0 {
		// A Go basic type ("string", ...) is a legitimate reference: handlers
		// respond with bare strings, and openapi_include_type:"string" documents
		// a named scalar as its wire type. (The pre-strict generator silently
		// rendered these as {"type":"object"} — wrong.)
		if t, ok := basicTypes[ref]; ok {
			return map[string]any{"type": t}
		}
		if ref == "any" {
			return map[string]any{}
		}
		g.problemf(rc.pos, "type reference %q is neither pkg.Type nor a Go basic type (%s)", ref, rc.what)
		return map[string]any{"type": "object"}
	}
	pkg, name := ref[:dot], ref[dot+1:]
	// allow refs like "internalchatapi.chatResponse" where pkg may contain dots
	if i := strings.LastIndex(pkg, "."); i >= 0 {
		pkg = pkg[i+1:]
	}
	return g.refForNamed(pkg, name, rc)
}

func (g *generator) refForNamed(pkg, name string, rc refCtx) map[string]any {
	key := pkg + "." + name
	si := g.structs[key]
	if si == nil {
		if a, ok := g.aliases[key]; ok {
			return g.schemaForExpr(a.underlying, a.pkg, rc)
		}
		g.problemf(rc.pos, "cannot resolve type %q to a known struct or named alias (%s); is its package in the generator's scan list?", key, rc.what)
		return map[string]any{"type": "object"}
	}
	compKey := sanitize(key)
	if !g.pending[compKey] {
		if _, done := g.schemas[compKey]; !done {
			g.pending[compKey] = true
			g.schemas[compKey] = g.structSchema(si, key)
		}
	}
	return map[string]any{"$ref": "#/components/schemas/" + compKey}
}

func (g *generator) structSchema(si *structInfo, key string) map[string]any {
	props := map[string]any{}
	for _, field := range si.st.Fields.List {
		jsonName, incType, skip := parseTag(field.Tag)
		if skip {
			continue
		}
		if len(field.Names) == 0 {
			// embedded field: merge the embedded struct's properties best-effort
			if emb := g.embeddedSchema(field.Type, si.pkg, key); emb != nil {
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
			rc := refCtx{pos: field.Pos(), what: fmt.Sprintf("field %q of %s", pname, key)}
			if incType != "" {
				s := g.schemaForRef(incType, rc)
				// openapi_include_type names the ELEMENT type; if the field is a
				// slice and the tag did not already spell the wrapper, honor the
				// array-ness from the Go field so the documented shape matches the
				// live array body (else a generated client types it as a scalar).
				trimmed := strings.TrimSpace(incType)
				if !strings.HasPrefix(trimmed, "[]") && isSliceType(field.Type) {
					s = map[string]any{"type": "array", "items": s}
				}
				props[pname] = s
			} else {
				props[pname] = g.schemaForExpr(field.Type, si.pkg, rc)
			}
		}
	}
	return map[string]any{"type": "object", "properties": props}
}

// isSliceType reports whether expr is a Go slice (unwrapping a leading pointer),
// but not a fixed-size array.
func isSliceType(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.ArrayType:
		return v.Len == nil
	case *ast.StarExpr:
		return isSliceType(v.X)
	}
	return false
}

func (g *generator) embeddedSchema(expr ast.Expr, pkg, key string) map[string]any {
	switch v := expr.(type) {
	case *ast.Ident:
		if si := g.structs[pkg+"."+v.Name]; si != nil {
			return g.structSchema(si, pkg+"."+v.Name)
		}
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			if si := g.structs[x.Name+"."+v.Sel.Name]; si != nil {
				return g.structSchema(si, x.Name+"."+v.Sel.Name)
			}
		}
	}
	return nil
}

func (g *generator) schemaForExpr(expr ast.Expr, pkg string, rc refCtx) map[string]any {
	switch v := expr.(type) {
	case *ast.Ident:
		if t, ok := basicTypes[v.Name]; ok {
			return map[string]any{"type": t}
		}
		if v.Name == "any" {
			return map[string]any{}
		}
		return g.refForNamed(pkg, v.Name, rc)
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
			return g.refForNamed(x.Name, v.Sel.Name, rc)
		}
		return map[string]any{"type": "object"}
	case *ast.StarExpr:
		return g.schemaForExpr(v.X, pkg, rc)
	case *ast.ArrayType:
		return map[string]any{"type": "array", "items": g.schemaForExpr(v.Elt, pkg, rc)}
	case *ast.MapType:
		return map[string]any{"type": "object", "additionalProperties": g.schemaForExpr(v.Value, pkg, rc)}
	case *ast.InterfaceType:
		return map[string]any{}
	case *ast.StructType:
		return g.structSchema(&structInfo{pkg: pkg, st: v}, "inline struct ("+rc.what+")")
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

// handlerDesc names a route's handler for coverage-gate error messages.
func handlerDesc(rt route) string {
	switch {
	case rt.closure != nil:
		return "closure"
	case rt.handlerKey != "":
		return rt.handlerKey
	case rt.handlerBare != "":
		return rt.handlerBare
	}
	return "unresolved"
}

func operationID(rt route) string {
	if rt.handlerBare != "" {
		return tagFor(rt.pkg) + "_" + rt.handlerBare
	}
	return pathOperationID(rt)
}

// pathOperationID is the route-derived operationId form (method + path slug),
// unique per (method, path). It is the fallback for inline routes and for
// disambiguating a handler that serves several routes (see assignOperationIDs).
func pathOperationID(rt route) string {
	slug := strings.NewReplacer("/", "_", "{", "", "}", "").Replace(strings.Trim(rt.path, "/"))
	return strings.ToLower(rt.method) + "_" + slug
}

// assignOperationIDs gives every route a globally unique operationId, keyed by
// "METHOD path". operationID is handler-derived, so a handler bound to several
// routes (e.g. one compat handler on five paths, or a list handler on both the
// collection and a sub-collection) yields colliding ids — and a generated
// client emits ONE symbol per operationId, so duplicates make the TypeScript and
// Go clients fail to compile. Every member of a colliding id falls back to the
// route-derived form; any residual collision (a templated vs literal path that
// slugs alike) gets a deterministic numeric suffix in sorted route order.
func (g *generator) assignOperationIDs() map[string]string {
	key := func(rt route) string { return rt.method + " " + rt.path }
	base := map[string]string{}
	count := map[string]int{}
	for _, rt := range g.routes {
		id := operationID(rt)
		base[key(rt)] = id
		count[id]++
	}
	out := map[string]string{}
	used := map[string]bool{}
	for _, rt := range g.routes {
		id := base[key(rt)]
		if count[id] > 1 {
			id = pathOperationID(rt)
		}
		final := id
		for n := 2; used[final]; n++ {
			final = fmt.Sprintf("%s_%d", id, n)
		}
		used[final] = true
		out[key(rt)] = final
	}
	return out
}

// summaryFor prefers the handler's GoDoc first sentence; a handler without GoDoc
// (or an unbound route) keeps the mechanical "METHOD /path" form.
func summaryFor(rt route, fi *funcInfo) string {
	if fi != nil && fi.anno.docSummary != "" {
		return fi.anno.docSummary
	}
	return rt.method + " " + rt.path
}

func tagFor(pkg string) string {
	return strings.TrimSuffix(pkg, "api")
}

func sanitize(s string) string {
	return strings.NewReplacer(".", "_", "/", "_").Replace(s)
}

func readVersion(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "runtime/version/version.txt"))
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(b))
}
