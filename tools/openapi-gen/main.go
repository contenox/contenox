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
	"reflect"
	"strconv"
	"strings"

	"github.com/contenox/runtime/apiframework"
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
			Title:   "contenox/runtime – LLM Backend Management API",
			Version: apiframework.GetVersion(),
		},
		Paths: openapi3.NewPaths(),
	}

	swagger.Security = *openapi3.NewSecurityRequirements().
		With(openapi3.SecurityRequirement{"X-API-Key": []string{}})

	processRouteFiles(fset, pkgs, swagger)
	addSchemasToSpec(swagger)
	swagger.Components.Schemas["array_string"] = openapi3.NewSchemaRef("", openapi3.NewArraySchema().WithItems(openapi3.NewStringSchema()))
	cleanUnusedSchemas(swagger)
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

			// Add parameters ONLY when first creating the path
			for _, paramName := range extractPathParams(path) {
				param := openapi3.NewPathParameter(paramName)
				param.Required = true
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

	if reqType := extractRequestType(handler, file); reqType != "" {
		reqType = strings.Replace(reqType, ".", "_", -1)
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

	statusCodes := extractStatusCodes(handler, file)
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
	if isSSEHandler(handler) {
		// Add SSE response definition
		response := openapi3.NewResponse()
		desc := "Server-Sent Events stream"
		response.Description = &desc
		mediaType := openapi3.NewMediaType()
		mediaType.Schema = openapi3.NewStringSchema().NewRef()
		content := openapi3.Content{"text/event-stream": mediaType}
		response.Content = content
		op.AddResponse(200, response)
	} else if len(statusCodes) == 0 { // Existing response handling
		// Add default response if no others found
		response := openapi3.NewResponse()
		desc := "OK"
		response.Description = &desc
		op.AddResponse(200, response)
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

func extractRequestType(handler *ast.FuncDecl, file *ast.File) string {
	var reqType string
	ast.Inspect(handler.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if gen, ok := call.Fun.(*ast.IndexExpr); ok {
				if sel, ok := gen.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Decode" {
					// Look for comment annotation after the Decode call
					if comment := findFollowingComment(call, file); comment != "" {
						if after, ok0 := strings.CutPrefix(comment, "@request "); ok0 {
							typeStr := after

							// Check if it's a slice type
							isArray := false
							if strings.HasPrefix(typeStr, "[]") {
								isArray = true
								typeStr = strings.TrimPrefix(typeStr, "[]")
							}

							// Convert package.Type to package_Type
							typeStr = strings.ReplaceAll(typeStr, ".", "_")

							// For slice types, prefix with "array_"
							if isArray {
								typeStr = "array_" + typeStr
							}

							reqType = typeStr
							return false
						}
					}

					// Fallback to existing method if no annotation
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

func extractStatusCodes(handler *ast.FuncDecl, file *ast.File) map[int]string {
	statusCodes := make(map[int]string)
	ast.Inspect(handler.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Encode" {
				if len(call.Args) < 4 {
					return true
				}

				// Get status code
				status := 0
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

				// Look for comment annotation after the Encode call
				if comment := findFollowingComment(call, file); comment != "" {
					if after, ok0 := strings.CutPrefix(comment, "@response "); ok0 {
						typeStr := after

						// Check if it's a slice type
						isArray := false
						if strings.HasPrefix(typeStr, "[]") {
							isArray = true
							typeStr = strings.TrimPrefix(typeStr, "[]")
						}

						// Convert package.Type to package_Type
						typeStr = strings.ReplaceAll(typeStr, ".", "_")

						// For slice types, prefix with "array_"
						if isArray {
							typeStr = "array_" + typeStr
						}

						statusCodes[status] = typeStr
						return true
					}
				}

				// Fallback to existing method if no annotation
				respType := "object"
				if len(call.Args) >= 4 {
					switch arg := call.Args[3].(type) {
					case *ast.Ident:
						respType = arg.Name
					case *ast.SelectorExpr:
						respType = arg.Sel.Name
					case *ast.CompositeLit:
						if id, ok := arg.Type.(*ast.Ident); ok {
							respType = id.Name
						}
					}
				}
				statusCodes[status] = respType
			}
		}
		return true
	})
	return statusCodes
}

// findFollowingComment looks for a comment immediately following a node
func findFollowingComment(node ast.Node, file *ast.File) string {
	pos := node.End()

	// Check if there's a comment group at this position
	for _, group := range file.Comments {
		for _, comment := range group.List {
			// Check if the comment starts right after our node
			// We allow for a space or newline between the node and comment
			if comment.Slash == pos+1 || comment.Slash == pos+2 || comment.Slash == pos+3 {
				text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
				if strings.HasPrefix(text, "@") {
					return text
				}
			}
		}
	}

	return ""
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
			pkgName := file.Name.Name // This is the package name (e.g., "backendapi")

			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if structType, ok := typeSpec.Type.(*ast.StructType); ok {
						// Get struct documentation
						doc := extractComments(typeSpec.Doc)
						// Pass package name to addStructSchema
						addStructSchema(swagger, pkgName, typeSpec.Name.Name, structType, doc)
					}
				}
				return true
			})
		}
	}
}

func addStructSchema(swagger *openapi3.T, pkgName string, typeName string, structType *ast.StructType, description string) {
	// Create a package-prefixed name: "backendapi.RespBackend"
	fullName := pkgName + "_" + typeName

	if swagger.Components == nil {
		swagger.Components = &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		}
	}

	// Skip if already added
	if _, exists := swagger.Components.Schemas[fullName]; exists {
		return
	}

	schema := openapi3.NewSchema()
	schema.Type = &openapi3.Types{openapi3.TypeObject}
	schema.Properties = make(openapi3.Schemas)
	schema.Description = description

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
							// Recursively add embedded struct with package context
							addStructSchema(swagger, pkgName, ident.Name, st, extractComments(spec.Doc))
						}
					}
				}
			}
			continue
		}

		// Process field tags to get JSON name
		jsonTag := ""
		isOmitempty := false
		if field.Tag != nil {
			tag := strings.Trim(field.Tag.Value, "`")
			if jsonTagVal, hasJson := reflect.StructTag(tag).Lookup("json"); hasJson {
				parts := strings.Split(jsonTagVal, ",")
				jsonTag = parts[0]
				if len(parts) > 1 && parts[1] == "omitempty" {
					isOmitempty = true
				}
			}
		}
		if !isOmitempty {
			schema.Required = append(schema.Required, jsonTag)
		}

		// If no json tag, use field name
		if jsonTag == "" {
			jsonTag = fieldName
		}

		// Add field to schema
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
			fieldSchema.Format = goTypeToSwaggerFormat(fieldType.Name)
		case *ast.SelectorExpr:
			fieldSchema.Type = goTypeToSwaggerType(fieldType.Sel.Name)
			fieldSchema.Format = goTypeToSwaggerFormat(fieldType.Sel.Name)
		case *ast.ArrayType:
			fieldSchema.Type = &openapi3.Types{openapi3.TypeArray}
			var elemTypeName string
			var isCustomType bool

			// First, try to determine element type
			if elemType, ok := fieldType.Elt.(*ast.Ident); ok {
				fullTypeName := pkgName + "_" + elemType.Name
				if _, exists := swagger.Components.Schemas[fullTypeName]; exists {
					fieldSchema.Items = &openapi3.SchemaRef{
						Ref: fmt.Sprintf("#/components/schemas/%s", fullTypeName),
					}
					isCustomType = true
				} else {
					elemTypeName = elemType.Name
				}
			}

			// Process oapiinclude tag to override or specify custom type
			if field.Tag != nil {
				tag := strings.Trim(field.Tag.Value, "`")
				if strings.Contains(tag, "oapiinclude:") {
					if includeStart := strings.Index(tag, `oapiinclude:"`); includeStart != -1 {
						includeStart += len(`oapiinclude:"`)
						//panic(tag)
						includeEnd := strings.Index(tag[includeStart:], `"`)
						if includeEnd != -1 {
							includedType := tag[includeStart : includeStart+includeEnd]
							includedType = strings.ReplaceAll(includedType, ".", "_")

							//	if _, exists := swagger.Components.Schemas[includedType]; exists {
							fieldSchema.Items = &openapi3.SchemaRef{
								Ref: fmt.Sprintf("#/components/schemas/%s", includedType),
							}
							isCustomType = true
							//		} else {
							//	log.Printf("WARNING: oapiinclude schema not found: %s", includedType)
							//	}
						}
					}
				}
			}

			// Fallback to basic type if no custom type was set
			if !isCustomType {
				if elemTypeName != "" {
					fieldSchema.Items = &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type:   goTypeToSwaggerType(elemTypeName),
							Format: goTypeToSwaggerFormat(elemTypeName),
						},
					}
				} else {
					// Final fallback to generic object
					fieldSchema.Items = &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeObject},
						},
					}
				}
			}
		case *ast.StarExpr:
			if elemType, ok := fieldType.X.(*ast.Ident); ok {
				fieldSchema.Type = goTypeToSwaggerType(elemType.Name)
				fieldSchema.Format = goTypeToSwaggerFormat(elemType.Name)
			}
		case *ast.MapType:
			has := true
			fieldSchema.Type = &openapi3.Types{openapi3.TypeObject}
			fieldSchema.AdditionalProperties = openapi3.AdditionalProperties{
				Has: &has,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:   &openapi3.Types{openapi3.TypeString},
						Format: goTypeToSwaggerFormat("string"),
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
			if strings.Contains(tag, `omitempty`) {
				//schema.Required = append(schema.Required, jsonTag)
			} else if strings.Contains(tag, `required:"true"`) {
				schema.Required = append(schema.Required, jsonTag)
			}
			if strings.Contains(tag, "oapiinclude:") {
				if exampleStart := strings.Index(tag, `oapiinclude:"`); exampleStart != -1 {
					exampleStart += len(`oapiinclude:"`)
					exampleEnd := strings.Index(tag[exampleStart:], `"`)
					if exampleEnd != -1 {
						example := tag[exampleStart : exampleStart+exampleEnd]
						example = strings.ReplaceAll(example, ".", "_")
						if _, exists := swagger.Components.Schemas[example]; exists {
							fieldSchema.Items = &openapi3.SchemaRef{
								Ref: fmt.Sprintf("#/components/schemas/%s", example),
							}
						}
					}
				}
			}
		}

		schema.Properties[jsonTag] = &openapi3.SchemaRef{Value: fieldSchema}
	}

	swagger.Components.Schemas[fullName] = &openapi3.SchemaRef{Value: schema}
	arrayName := "array_" + fullName
	if _, exists := swagger.Components.Schemas[arrayName]; !exists {
		arraySchema := openapi3.NewSchema()
		arraySchema.Type = &openapi3.Types{openapi3.TypeArray}
		arraySchema.Items = &openapi3.SchemaRef{
			Ref: fmt.Sprintf("#/components/schemas/%s", fullName),
		}

		swagger.Components.Schemas[arrayName] = openapi3.NewSchemaRef("", arraySchema)
	}
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
	case "Time":
		return &openapi3.Types{openapi3.TypeString}
	default:
		return &openapi3.Types{openapi3.TypeObject}
	}
}

func goTypeToSwaggerFormat(goType string) string {
	switch goType {
	case "Time":
		return "date-time"
	default:
		return ""
	}
}

func resolveVariableType(handler *ast.FuncDecl, varName string) string {
	// First, check for direct variable assignments
	var foundType string
	ast.Inspect(handler.Body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for i, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == varName {
					if i < len(assign.Rhs) {
						// Case 1: Direct struct initialization: resp := RespBackend{}
						if compLit, ok := assign.Rhs[i].(*ast.CompositeLit); ok {
							if structType, ok := compLit.Type.(*ast.Ident); ok {
								foundType = structType.Name
								return false
							}

							// Case 2: Slice initialization: resp := []RespBackendList{}
							if arrType, ok := compLit.Type.(*ast.ArrayType); ok {
								if elemType, ok := arrType.Elt.(*ast.Ident); ok {
									foundType = elemType.Name
									return false
								}
							}
						}

						// Case 3: Function call result: resp := b.getBackend(...)
						if call, ok := assign.Rhs[i].(*ast.CallExpr); ok {
							if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
								// We need to look up the actual return type
								// This is a simplified approach - in reality you'd need type checking
								// But for your convention, we'll assume the function returns the struct type
								foundType = sel.Sel.Name
								return false
							}
						}
					}
				}
			}
		}

		// Case 4: Short variable declaration (:=)
		if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			for i, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == varName {
					if i < len(assign.Rhs) {
						if compLit, ok := assign.Rhs[i].(*ast.CompositeLit); ok {
							if structType, ok := compLit.Type.(*ast.Ident); ok {
								foundType = structType.Name
								return false
							}

							if arrType, ok := compLit.Type.(*ast.ArrayType); ok {
								if elemType, ok := arrType.Elt.(*ast.Ident); ok {
									foundType = elemType.Name
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

	if foundType != "" {
		return foundType
	}

	// Fallback: If we can't determine the type, return "object"
	// This is better than returning a wrong type name
	return "object"
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

func cleanUnusedSchemas(swagger *openapi3.T) {
	if swagger.Components == nil || swagger.Components.Schemas == nil {
		return
	}

	// Step 1: Find all directly referenced schemas in operations
	directlyUsed := make(map[string]bool)
	for _, pathItem := range swagger.Paths.Map() {
		for _, op := range []*openapi3.Operation{
			pathItem.Get, pathItem.Put, pathItem.Post,
			pathItem.Delete, pathItem.Options, pathItem.Head,
			pathItem.Patch, pathItem.Trace,
		} {
			if op == nil {
				continue
			}
			collectDirectRefs(op, directlyUsed)
		}
	}

	// Step 2: Do BFS to find all referenced schemas
	usedSchemas := make(map[string]bool)
	queue := make([]string, 0, len(directlyUsed))

	// Initialize queue with directly used schemas
	for name := range directlyUsed {
		usedSchemas[name] = true
		queue = append(queue, name)
	}

	// Process queue
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		// Get schema from components
		schemaRef, exists := swagger.Components.Schemas[name]
		if !exists {
			continue
		}

		// Find references in this schema
		refs := findRefsInSchema(schemaRef)
		for _, ref := range refs {
			// Only process if we haven't seen this schema before
			if !usedSchemas[ref] {
				usedSchemas[ref] = true
				queue = append(queue, ref)
			}
		}
	}

	// Step 3: Remove unused schemas
	cleaned := openapi3.Schemas{}
	for name, schema := range swagger.Components.Schemas {
		if usedSchemas[name] {
			cleaned[name] = schema
		}
	}
	swagger.Components.Schemas = cleaned
}

func collectDirectRefs(op *openapi3.Operation, refs map[string]bool) {
	// Request body
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for _, mediaType := range op.RequestBody.Value.Content {
			if mediaType.Schema != nil && mediaType.Schema.Ref != "" {
				if name := getSchemaNameFromRef(mediaType.Schema.Ref); name != "" {
					refs[name] = true
				}
			}
		}
	}

	// Responses
	for _, response := range op.Responses.Map() {
		if response.Value == nil {
			continue
		}
		for _, mediaType := range response.Value.Content {
			if mediaType.Schema != nil && mediaType.Schema.Ref != "" {
				if name := getSchemaNameFromRef(mediaType.Schema.Ref); name != "" {
					refs[name] = true
				}
			}
		}
	}
}

func getSchemaNameFromRef(ref string) string {
	if after, ok := strings.CutPrefix(ref, "#/components/schemas/"); ok {
		return after
	}
	return ""
}

func findRefsInSchema(schemaRef *openapi3.SchemaRef) []string {
	refs := []string{}

	// Handle direct reference
	if schemaRef.Ref != "" {
		if name := getSchemaNameFromRef(schemaRef.Ref); name != "" {
			refs = append(refs, name)
		}
		return refs
	}

	if schemaRef.Value == nil {
		return refs
	}

	// Check items
	if schemaRef.Value.Items != nil {
		refs = append(refs, findRefsInSchema(schemaRef.Value.Items)...)
	}

	// Check properties
	for _, propRef := range schemaRef.Value.Properties {
		refs = append(refs, findRefsInSchema(propRef)...)
	}

	// Check additional properties
	if schemaRef.Value.AdditionalProperties.Schema != nil {
		refs = append(refs, findRefsInSchema(schemaRef.Value.AdditionalProperties.Schema)...)
	}

	// Check allOf, anyOf, oneOf
	for _, subRef := range schemaRef.Value.AllOf {
		refs = append(refs, findRefsInSchema(subRef)...)
	}
	for _, subRef := range schemaRef.Value.AnyOf {
		refs = append(refs, findRefsInSchema(subRef)...)
	}
	for _, subRef := range schemaRef.Value.OneOf {
		refs = append(refs, findRefsInSchema(subRef)...)
	}

	return refs
}

func referencesUsedSchema(ref *openapi3.SchemaRef, used map[string]bool) bool {
	if ref == nil {
		return false
	}

	// Check direct reference
	if ref.Ref != "" {
		name := strings.TrimPrefix(ref.Ref, "#/components/schemas/")
		return used[name]
	}

	if ref.Value == nil {
		return false
	}

	// Check nested references
	if referencesUsedSchema(ref.Value.Items, used) {
		return true
	}

	for _, prop := range ref.Value.Properties {
		if referencesUsedSchema(prop, used) {
			return true
		}
	}

	if ref.Value.AdditionalProperties.Schema != nil {
		return referencesUsedSchema(ref.Value.AdditionalProperties.Schema, used)
	}

	return false
}

// Recursively collect all $ref references from a schema reference
func collectRefsFromSchemaRef(schemaRef *openapi3.SchemaRef, usedSchemas map[string]bool) {
	if schemaRef == nil {
		return
	}

	// Handle $ref directly
	if schemaRef.Ref != "" {
		// Extract schema name from "#/components/schemas/SchemaName"
		if after, ok := strings.CutPrefix(schemaRef.Ref, "#/components/schemas/"); ok {
			schemaName := after
			usedSchemas[schemaName] = true
		}
		return
	}

	// Process the schema if it's not a reference
	if schemaRef.Value != nil {
		collectRefsFromSchema(schemaRef.Value, usedSchemas)
	}
}

// Recursively collect all references from a schema
func collectRefsFromSchema(schema *openapi3.Schema, usedSchemas map[string]bool) {
	if schema == nil {
		return
	}

	// Check properties
	for _, propRef := range schema.Properties {
		collectRefsFromSchemaRef(propRef, usedSchemas)
	}

	// Check items (for arrays)
	if schema.Items != nil {
		collectRefsFromSchemaRef(schema.Items, usedSchemas)
	}

	// Check additionalProperties
	if schema.AdditionalProperties.Schema != nil {
		collectRefsFromSchemaRef(schema.AdditionalProperties.Schema, usedSchemas)
	}

	// Check anyOf, allOf, oneOf
	for _, subSchemaRef := range schema.AnyOf {
		collectRefsFromSchemaRef(subSchemaRef, usedSchemas)
	}
	for _, subSchemaRef := range schema.AllOf {
		collectRefsFromSchemaRef(subSchemaRef, usedSchemas)
	}
	for _, subSchemaRef := range schema.OneOf {
		collectRefsFromSchemaRef(subSchemaRef, usedSchemas)
	}
}

func isSSEHandler(handler *ast.FuncDecl) bool {
	if handler.Doc == nil {
		return false
	}
	for _, comment := range handler.Doc.List {
		if strings.Contains(comment.Text, "Server-Sent Events") ||
			strings.Contains(comment.Text, "streams status updates") {
			return true
		}
	}
	return false
}

func extractPathParams(path string) []string {
	seen := make(map[string]bool)
	var params []string
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.Trim(part, "{}")
			if !seen[name] {
				seen[name] = true
				params = append(params, name)
			}
		}
	}
	return params
}
