package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var pkgs map[string]*ast.Package

func main() {
	fset := token.NewFileSet()
	pkgs = make(map[string]*ast.Package)

	// Recursively parse all .go files in the project
	err := parseProject(fset, "/home/naro/src/github.com/contenox/runtime/", pkgs)
	if err != nil {
		log.Fatal("Failed to parse project:", err)
	}

	swagger := &openapi3.T{
		OpenAPI: "3.0.0",
	}
	swagger.Info = &openapi3.Info{
		Title:   "LLM Backend Management API",
		Version: "1.0",
	}
	swagger.Paths = openapi3.NewPaths()

	processRouteFiles(fset, pkgs, swagger)
	addSchemasToSpec(swagger)

	data, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		log.Fatal("Failed to marshal spec:", err)
	}

	os.MkdirAll("docs", 0755)
	if err := os.WriteFile("docs/openapi.json", data, 0644); err != nil {
		log.Fatal("Failed to write spec:", err)
	}

	fmt.Println("✅ OpenAPI spec generated at docs/openapi.json")
}

func parseProject(fset *token.FileSet, rootDir string, pkgs map[string]*ast.Package) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories that are not relevant to the project, e.g., hidden dirs, test dirs
		if info.IsDir() {
			dirName := info.Name()
			if strings.HasPrefix(dirName, ".") || dirName == "tools" || dirName == "vendor" || dirName == "apitests" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only parse Go files that are not test files
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			log.Printf("Found Go file: %s", path)

			// Parse the file and check for errors
			fileAST, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				log.Printf("Error parsing file %s: %v", path, err)
				// Returning nil here will continue the walk, but log the error
				return nil
			}

			// Add the parsed file to the correct package in the map
			pkgName := fileAST.Name.Name
			if pkgs[pkgName] == nil {
				pkgs[pkgName] = &ast.Package{
					Name:  pkgName,
					Files: make(map[string]*ast.File),
				}
			}
			pkgs[pkgName].Files[path] = fileAST
		}
		return nil
	})
}

func processRouteFiles(fset *token.FileSet, pkgs map[string]*ast.Package, swagger *openapi3.T) {
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			// Corrected line to get the file path
			filePath := fset.File(file.Pos()).Name()
			log.Printf("Processing file: %s", filePath)

			ast.Inspect(file, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					if strings.HasPrefix(fn.Name.Name, "Add") && strings.HasSuffix(fn.Name.Name, "Routes") {
						extractRoutesFromFunction(fset, file, fn, swagger)
					}
				}
				return true
			})
		}
	}
}
func extractRoutesFromFunction(fset *token.FileSet, file *ast.File, fn *ast.FuncDecl, swagger *openapi3.T) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "HandleFunc" {
				extractRoute(fset, file, call, swagger)
			}
		}
		return true
	})
}

func extractRoute(fset *token.FileSet, file *ast.File, call *ast.CallExpr, swagger *openapi3.T) {
	var path, method string
	if len(call.Args) > 0 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			parts := strings.Split(lit.Value[1:len(lit.Value)-1], " ")
			if len(parts) == 2 {
				method = parts[0]
				path = parts[1]
			} else {
				path = parts[0]
				method = "GET"
			}
		}
	}

	var handler *ast.FuncDecl
	if len(call.Args) > 1 {
		if funcLit, ok := call.Args[1].(*ast.FuncLit); ok {
			handler = &ast.FuncDecl{
				Name: ast.NewIdent("handler"),
				Type: funcLit.Type,
				Body: funcLit.Body,
			}
		} else if sel, ok := call.Args[1].(*ast.SelectorExpr); ok {
			ast.Inspect(file, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == sel.Sel.Name {
					handler = fn
					return false
				}
				return true
			})
		}
	}

	if handler == nil {
		return
	}

	if swagger.Paths.Find(path) == nil {
		swagger.Paths.Set(path, &openapi3.PathItem{})
	}
	pathItem := swagger.Paths.Find(path)

	op := openapi3.NewOperation()
	op.Summary = strings.TrimPrefix(handler.Name.Name, "handle")

	if reqType := extractRequestType(handler); reqType != "" {
		schemaRef := &openapi3.SchemaRef{
			Ref: fmt.Sprintf("#/components/schemas/%s", reqType),
		}

		content := openapi3.Content{}
		content["application/json"] = &openapi3.MediaType{
			Schema: schemaRef,
		}

		op.RequestBody = &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Content: content,
			},
		}
	}

	statusCodes := extractStatusCodes(handler)
	for status, respType := range statusCodes {
		var schemaRef *openapi3.SchemaRef
		if respType == "string" {
			schemaRef = &openapi3.SchemaRef{
				Value: openapi3.NewSchema(),
			}
			schemaRef.Value.Type = &openapi3.Types{openapi3.TypeString}
		} else {
			schemaRef = &openapi3.SchemaRef{
				Ref: fmt.Sprintf("#/components/schemas/%s", respType),
			}
		}

		content := openapi3.Content{}
		content["application/json"] = &openapi3.MediaType{
			Schema: schemaRef,
		}

		response := openapi3.NewResponse()
		description := httpStatusToDescription(status)
		response.Description = &description
		response.Content = content

		op.AddResponse(status, response)
	}

	switch strings.ToUpper(method) {
	case "GET":
		pathItem.Get = op
	case "POST":
		pathItem.Post = op
	case "PUT":
		pathItem.Put = op
	case "DELETE":
		pathItem.Delete = op
	}
}

func extractRequestType(handler *ast.FuncDecl) string {
	var reqType string
	ast.Inspect(handler.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if gen, ok := call.Fun.(*ast.IndexExpr); ok {
				if sel, ok := gen.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Decode" {
					if id, ok := gen.Index.(*ast.Ident); ok {
						reqType = id.Name
						return false
					}
				}
			}
		}
		return true
	})
	return reqType
}

func extractStatusCodes(handler *ast.FuncDecl) map[int]string {
	statusCodes := make(map[int]string)

	ast.Inspect(handler.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Encode" {
				if len(call.Args) >= 2 {
					if id, ok := call.Args[1].(*ast.Ident); ok {
						status := httpStatusToCode(id.Name)
						respType := "object"
						if len(call.Args) >= 4 && status != 204 {
							if sel, ok := call.Args[3].(*ast.SelectorExpr); ok {
								respType = sel.Sel.Name
							}
						}
						statusCodes[status] = respType
					}
				}
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
				if len(call.Args) >= 3 {
					if sel, ok := call.Args[2].(*ast.SelectorExpr); ok {
						status := httpStatusFromOperation(sel.Sel.Name)
						statusCodes[status] = "ErrorResponse"
					}
				}
			}
		}
		return true
	})
	return statusCodes
}

func httpStatusToCode(name string) int {
	switch name {
	case "StatusOK":
		return 200
	case "StatusCreated":
		return 201
	case "StatusNoContent":
		return 204
	case "StatusBadRequest":
		return 400
	case "StatusUnauthorized":
		return 401
	case "StatusForbidden":
		return 403
	case "StatusNotFound":
		return 404
	case "StatusConflict":
		return 409
	case "StatusUnprocessableEntity":
		return 422
	default:
		return 500
	}
}

func httpStatusToDescription(code int) string {
	switch code {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 204:
		return "No Content"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 409:
		return "Conflict"
	case 422:
		return "Unprocessable Entity"
	default:
		return "Internal Server Error"
	}
}

func httpStatusFromOperation(opName string) int {
	switch opName {
	case "CreateOperation":
		return 422
	case "GetOperation", "ListOperation":
		return 404
	case "UpdateOperation":
		return 400
	case "DeleteOperation":
		return 404
	case "AuthorizeOperation":
		return 403
	default:
		return 500
	}
}

func addSchemasToSpec(swagger *openapi3.T) {
	if swagger.Components == nil {
		swagger.Components = &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		}
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if structType, ok := typeSpec.Type.(*ast.StructType); ok {
						addStructSchema(swagger, typeSpec.Name.Name, structType)
					}
				}
				return true
			})
		}
	}
}

func addStructSchema(swagger *openapi3.T, name string, structType *ast.StructType) {
	if swagger.Components == nil {
		swagger.Components = &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		}
	}

	schema := openapi3.NewSchema()
	schema.Type = &openapi3.Types{openapi3.TypeObject}
	schema.Properties = make(openapi3.Schemas)

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue
		}

		fieldName := field.Names[0].Name
		jsonTag := ""
		if field.Tag != nil {
			tag := strings.Trim(field.Tag.Value, "`")
			if tagParts := strings.Split(tag, " "); len(tagParts) > 0 {
				if jsonParts := strings.Split(tagParts[0], ":"); len(jsonParts) > 1 {
					jsonTag = strings.Trim(jsonParts[1], `"`)
					if commaPos := strings.Index(jsonTag, ","); commaPos != -1 {
						jsonTag = jsonTag[:commaPos]
					}
				}
			}
		}

		if fieldName[0] < 'A' || fieldName[0] > 'Z' || jsonTag == "-" {
			continue
		}

		fieldSchema := openapi3.NewSchema()
		switch fieldType := field.Type.(type) {
		case *ast.Ident:
			fieldSchema.Type = goTypeToSwaggerType(fieldType.Name)
		case *ast.SelectorExpr:
			fieldSchema.Type = goTypeToSwaggerType(fieldType.Sel.Name)
		case *ast.ArrayType:
			fieldSchema.Type = &openapi3.Types{openapi3.TypeArray}
			if elemType, ok := fieldType.Elt.(*ast.Ident); ok {
				fieldSchema.Items = &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: goTypeToSwaggerType(elemType.Name),
					},
				}
			}
		case *ast.StarExpr:
			if elemType, ok := fieldType.X.(*ast.Ident); ok {
				fieldSchema.Type = goTypeToSwaggerType(elemType.Name)
			}
		}

		if field.Tag != nil {
			tag := strings.Trim(field.Tag.Value, "`")
			if strings.Contains(tag, `example:"`) {
				if exampleStart := strings.Index(tag, `example:"`); exampleStart != -1 {
					exampleStart += len(`example:"`)
					exampleEnd := strings.Index(tag[exampleStart:], `"`)
					if exampleEnd != -1 {
						example := tag[exampleStart : exampleStart+exampleEnd]
						fieldSchema.Example = example
					}
				}
			}
		}

		schema.Properties[jsonTag] = &openapi3.SchemaRef{Value: fieldSchema}
	}

	swagger.Components.Schemas[name] = &openapi3.SchemaRef{Value: schema}
}

func goTypeToSwaggerType(goType string) *openapi3.Types {
	switch goType {
	case "string":
		return &openapi3.Types{openapi3.TypeString}
	case "int", "int32", "int64":
		return &openapi3.Types{openapi3.TypeInteger}
	case "float32", "float64":
		return &openapi3.Types{openapi3.TypeNumber}
	case "bool":
		return &openapi3.Types{openapi3.TypeBoolean}
	case "time.Time":
		return &openapi3.Types{openapi3.TypeString}
	default:
		return &openapi3.Types{openapi3.TypeObject}
	}
}
