package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

var pkgs map[string]*ast.Package

func main() {
	var projectDir string
	var outputDir string

	flag.StringVar(&projectDir, "project", "", "The root directory of the Go project to parse.")
	flag.StringVar(&outputDir, "output", "docs", "The output directory for the generated OpenAPI spec.")

	flag.Parse()

	if projectDir == "" {
		fmt.Println("Error: The --project flag is required.")
		flag.Usage()
		os.Exit(1)
	}

	fset := token.NewFileSet()
	pkgs = make(map[string]*ast.Package)

	// Use the argument for the project directory
	err := parseProject(fset, projectDir, pkgs)
	if err != nil {
		log.Fatal("Failed to parse project:", err)
	}

	swagger := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:   "LLM Backend Management API",
			Version: "1.0",
		},
		Paths: openapi3.NewPaths(),
	}

	swagger.Security = *openapi3.NewSecurityRequirements().
		With(openapi3.SecurityRequirement{"X-API-Key": []string{}})

	processRouteFiles(fset, pkgs, swagger)
	addSchemasToSpec(swagger)

	swagger.Components.SecuritySchemes = openapi3.SecuritySchemes{
		"X-API-Key": &openapi3.SecuritySchemeRef{
			Value: openapi3.NewSecurityScheme().
				WithType("apiKey").
				WithName("X-API-Key").
				WithIn("header"),
		},
	}

	data, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		log.Fatal("Failed to marshal spec:", err)
	}

	// Use the argument for the output directory
	os.MkdirAll(outputDir, 0755)
	outputFilePath := filepath.Join(outputDir, "openapi.json")
	if err := os.WriteFile(outputFilePath, data, 0644); err != nil {
		log.Fatal("Failed to write spec:", err)
	}
	data, err = yaml.Marshal(swagger)
	if err != nil {
		log.Fatal("Failed to marshal spec:", err)
	}
	outputFilePath = filepath.Join(outputDir, "openapi.yaml")
	if err := os.WriteFile(outputFilePath, data, 0644); err != nil {
		log.Fatal("Failed to write spec:", err)
	}

	fmt.Printf("✅ OpenAPI spec generated at %s\n", outputFilePath)
}

func parseProject(fset *token.FileSet, rootDir string, pkgs map[string]*ast.Package) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			dirName := info.Name()
			if strings.HasPrefix(dirName, ".") || dirName == "tools" || dirName == "vendor" || dirName == "apitests" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			log.Printf("Found Go file: %s", path)

			// Parse with comments
			fileAST, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				log.Printf("Error parsing file %s: %v", path, err)
				return nil
			}

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

// Extracts comments and cleans them up
func extractComments(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}

	comments := make([]string, 0, len(doc.List))
	for _, c := range doc.List {
		text := c.Text

		// Clean up comment markers
		switch {
		case strings.HasPrefix(text, "//"):
			text = strings.TrimPrefix(text, "//")
		case strings.HasPrefix(text, "/*"):
			text = strings.TrimPrefix(text, "/*")
			text = strings.TrimSuffix(text, "*/")
		}

		text = strings.TrimSpace(text)
		if text != "" {
			comments = append(comments, text)
		}
	}
	return strings.Join(comments, "\n")
}

func processRouteFiles(fset *token.FileSet, pkgs map[string]*ast.Package, swagger *openapi3.T) {
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
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
			parts := strings.Split(strings.Trim(lit.Value, `"`), " ")
			if len(parts) == 2 {
				method = parts[0]
				path = parts[1]
			} else {
				path = parts[0]
				method = "GET"
			}
		}
	}

	// Extract parameters from path
	if strings.Contains(path, "{") {
		pathItem := swagger.Paths.Find(path)
		if pathItem == nil {
			pathItem = &openapi3.PathItem{}
			swagger.Paths.Set(path, pathItem)
		}

		parts := strings.Split(path, "/")
		for _, part := range parts {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				paramName := strings.Trim(part, "{}")
				param := openapi3.NewPathParameter(paramName)
				param.Schema = openapi3.NewStringSchema().NewRef()
				pathItem.Parameters = append(pathItem.Parameters, &openapi3.ParameterRef{
					Value: param,
				})
			}
		}
	}

	var handler *ast.FuncDecl
	var handlerDocs string
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
					handlerDocs = extractComments(fn.Doc)
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

	// Use handler docs for operation description
	if handlerDocs != "" {
		op.Description = handlerDocs
	}

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
				Content:  content,
				Required: true,
			},
		}
	}

	statusCodes := extractStatusCodes(handler)
	for status, respType := range statusCodes {
		var schemaRef *openapi3.SchemaRef

		if openapiType := toOpenAPIType(respType); openapiType != nil {
			schemaRef = &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: openapiType,
				},
			}
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
	case "PATCH":
		pathItem.Patch = op
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
			// Look for Encode calls
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Encode" {
				if len(call.Args) < 4 {
					return true
				}

				status := 0
				// Handle status argument (could be identifier, selector or literal)
				switch arg := call.Args[2].(type) {
				case *ast.Ident:
					status = httpStatusToCode(arg.Name)
				case *ast.SelectorExpr:
					status = httpStatusToCode(arg.Sel.Name)
				case *ast.BasicLit:
					if i, err := strconv.Atoi(arg.Value); err == nil {
						status = i
					}
				}

				if status == 0 {
					return true
				}

				respType := "object"
				// Handle response type argument
				switch arg := call.Args[3].(type) {
				case *ast.Ident:
					respType = arg.Name // Use identifier name directly
				case *ast.SelectorExpr:
					respType = arg.Sel.Name
				case *ast.CompositeLit:
					if id, ok := arg.Type.(*ast.Ident); ok {
						respType = id.Name
					}
				}

				statusCodes[status] = respType
			}

			// Look for Error calls
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
				if len(call.Args) >= 3 {
					status := 500
					// First try to get status from operation type
					if sel, ok := call.Args[2].(*ast.SelectorExpr); ok {
						status = httpStatusFromOperation(sel.Sel.Name)
					}
					// Then check if we have a specific status code
					if len(call.Args) >= 4 {
						if lit, ok := call.Args[3].(*ast.BasicLit); ok && lit.Kind == token.INT {
							if code, err := strconv.Atoi(lit.Value); err == nil {
								status = code
							}
						}
					}
					statusCodes[status] = "ErrorResponse"
				}
			}
		}
		return true
	})

	if _, exists := statusCodes[400]; !exists {
		statusCodes[400] = "ErrorResponse"
	}
	if _, exists := statusCodes[500]; !exists {
		statusCodes[500] = "ErrorResponse"
	}

	return statusCodes
}

func getActualTypeName(expr ast.Expr, name string) string {
	// If it's a selector expression like runtimetypes.Backend, extract the type name
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}

	// If it's an identifier, just use the name
	if _, ok := expr.(*ast.Ident); ok {
		return name
	}

	return name
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
						// Get struct documentation
						doc := extractComments(typeSpec.Doc)
						addStructSchema(swagger, typeSpec.Name.Name, structType, doc)
					}
				}
				return true
			})
		}
	}
}

func addStructSchema(swagger *openapi3.T, name string, structType *ast.StructType, description string) {
	if swagger.Components == nil {
		swagger.Components = &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		}
	}

	schema := openapi3.NewSchema()
	schema.Type = &openapi3.Types{openapi3.TypeObject}
	schema.Properties = make(openapi3.Schemas)
	schema.Description = description // Add struct-level description

	for _, field := range structType.Fields.List {
		fieldName := ""
		if len(field.Names) > 0 {
			fieldName = field.Names[0].Name
		}

		// Handle embedded structs
		if fieldName == "" {
			if ident, ok := field.Type.(*ast.Ident); ok {
				if obj := ident.Obj; obj != nil {
					if spec, ok := obj.Decl.(*ast.TypeSpec); ok {
						if st, ok := spec.Type.(*ast.StructType); ok {
							// Recursively add embedded struct with its docs
							doc := extractComments(spec.Doc)
							addStructSchema(swagger, ident.Name, st, doc)
						}
					}
				}
			}
			continue
		}

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

		// Add field documentation
		if doc := extractComments(field.Doc); doc != "" {
			fieldSchema.Description = doc
		} else if comment := extractComments(field.Comment); comment != "" {
			fieldSchema.Description = comment
		}

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
		case *ast.MapType:
			has := true
			fieldSchema.Type = &openapi3.Types{openapi3.TypeObject}
			fieldSchema.AdditionalProperties = openapi3.AdditionalProperties{
				Has: &has,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{openapi3.TypeString},
					},
				},
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

func resolveVariableType(handler *ast.FuncDecl, varName string) string {
	var resolvedType string

	ast.Inspect(handler.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == varName {
					rhs := node.Rhs[i]
					switch t := rhs.(type) {
					case *ast.CompositeLit:
						// Handle struct literals
						if ident, ok := t.Type.(*ast.Ident); ok {
							resolvedType = ident.Name
							return false
						}
					case *ast.CallExpr:
						// Handle function returns
						if sel, ok := t.Fun.(*ast.SelectorExpr); ok {
							resolvedType = sel.Sel.Name
							return false
						}
					}
				}
			}
		case *ast.DeclStmt:
			// Handle variable declarations
			if genDecl, ok := node.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
				for _, spec := range genDecl.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range valueSpec.Names {
							if name.Name == varName && valueSpec.Type != nil {
								if ident, ok := valueSpec.Type.(*ast.Ident); ok {
									resolvedType = ident.Name
									return false
								}
							}
						}
					}
				}
			}
		}
		return true
	})

	if resolvedType != "" {
		return resolvedType
	}
	return varName // Fallback to variable name
}

func toOpenAPIType(goType string) *openapi3.Types {
	switch goType {
	case "string", "time.Time", "time.Duration":
		return &openapi3.Types{openapi3.TypeString}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return &openapi3.Types{openapi3.TypeInteger}
	case "float32", "float64":
		return &openapi3.Types{openapi3.TypeNumber}
	case "bool":
		return &openapi3.Types{openapi3.TypeBoolean}
	case "interface{}", "any":
		return &openapi3.Types{openapi3.TypeObject} // Generic object type
	default:
		return nil
	}
}
