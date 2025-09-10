package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/contenox/runtime/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

type OpenAPIToolProtocol struct{}

func (p *OpenAPIToolProtocol) FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (*openapi3.T, error) {
	specURL := endpointURL + "/openapi.json"

	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	loader := openapi3.NewLoader()
	loader.Context = ctx
	loader.IsExternalRefsAllowed = true

	if httpClient != nil {
		loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
			if err != nil {
				return nil, err
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("failed to fetch OpenAPI spec: %s (status %d)", url.String(), resp.StatusCode)
			}

			return io.ReadAll(resp.Body)
		}
	}

	schema, err := loader.LoadFromURI(u)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI schema: %w", err)
	}

	return schema, nil
}

type ArgLocation int

const (
	ArgLocationQuery ArgLocation = iota
	ArgLocationHeader
	ArgLocationPath
	ArgLocationBody
)

type ParamArg struct {
	Name  string
	Value string
	In    ArgLocation
}

// operationDetails holds the schema information needed to execute a tool.
type operationDetails struct {
	Path      string
	Method    string
	Operation *openapi3.Operation
	PathItem  *openapi3.PathItem
}

// findOperationDetails fetches the OpenAPI schema and finds the specific
// operation that corresponds to the given tool name.
func (p *OpenAPIToolProtocol) findOperationDetails(
	ctx context.Context,
	endpointURL string,
	httpClient *http.Client,
	toolName string,
) (*operationDetails, error) {
	schema, err := p.FetchSchema(ctx, endpointURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("could not fetch schema for execution: %w", err)
	}

	for path, pathItem := range schema.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			name := p.extractToolName(path, method, operation)
			if name == toolName {
				return &operationDetails{
					Path:      path,
					Method:    method,
					Operation: operation,
					PathItem:  pathItem,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("tool '%s' not found in the OpenAPI schema", toolName)
}

// ExecuteTool performs a tool call by making a corresponding HTTP request.
func (p *OpenAPIToolProtocol) ExecuteTool(
	ctx context.Context,
	endpointURL string,
	httpClient *http.Client,
	injectParams map[string]ParamArg,
	toolCall taskengine.ToolCall,
) (interface{}, error) {
	details, err := p.findOperationDetails(ctx, endpointURL, httpClient, toolCall.Function.Name)
	if err != nil {
		return nil, err
	}

	// Consolidate arguments, with injectParams taking priority.
	finalArgs := make(map[string]interface{})
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &finalArgs); err != nil {
		return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
	}
	for _, p := range injectParams {
		finalArgs[p.Name] = p.Value
	}

	finalURL := endpointURL + details.Path
	queryParams := url.Values{}
	headers := http.Header{}

	// Process parameters that are defined in the OpenAPI spec.
	allParams := append(openapi3.Parameters{}, details.PathItem.Parameters...)
	allParams = append(allParams, details.Operation.Parameters...)
	for _, paramRef := range allParams {
		param := paramRef.Value
		if argVal, ok := finalArgs[param.Name]; ok {
			valStr := fmt.Sprintf("%v", argVal)
			if param.In == "path" {
				finalURL = strings.Replace(finalURL, "{"+param.Name+"}", valStr, 1)
			} else if param.In == "query" {
				queryParams.Add(param.Name, valStr)
			}
			delete(finalArgs, param.Name)
		}
	}

	// Apply all injectParams based on their specified location, regardless of the spec.
	for _, injectedParam := range injectParams {
		valStr := fmt.Sprintf("%v", finalArgs[injectedParam.Name])
		switch injectedParam.In {
		case ArgLocationPath:
			finalURL = strings.Replace(finalURL, "{"+injectedParam.Name+"}", valStr, 1)
			delete(finalArgs, injectedParam.Name)
		case ArgLocationQuery:
			queryParams.Set(injectedParam.Name, valStr)
			delete(finalArgs, injectedParam.Name)
		case ArgLocationHeader:
			headers.Set(injectedParam.Name, valStr)
			delete(finalArgs, injectedParam.Name)
		}
	}

	if len(queryParams) > 0 {
		finalURL += "?" + queryParams.Encode()
	}

	// Any remaining arguments are treated as the request body.
	var reqBody io.Reader
	if details.Operation.RequestBody != nil && slices.Contains([]string{"POST", "PUT", "PATCH"}, strings.ToUpper(details.Method)) {
		if len(finalArgs) > 0 {
			bodyBytes, err := json.Marshal(finalArgs)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			reqBody = bytes.NewBuffer(bodyBytes)
		}
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(details.Method), finalURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header = headers
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tool request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(responseBody) == 0 {
		return nil, nil
	}

	// Return structured JSON if possible, otherwise fall back to a raw string.
	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		var result interface{}
		if err := json.Unmarshal(responseBody, &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON response: %w", err)
		}
		return result, nil
	}
	return string(responseBody), nil
}

func (p *OpenAPIToolProtocol) FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error) {
	schema, err := p.FetchSchema(ctx, endpointURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schema for tools: %w", err)
	}
	if schema == nil {
		return nil, nil
	}

	var tools []taskengine.Tool

	for path, pathItem := range schema.Paths.Map() {
		if pathItem == nil {
			continue
		}

		operations := pathItem.Operations()
		for method, operation := range operations {
			switch strings.ToUpper(method) {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
				// supported
			default:
				continue
			}

			// --- 🎯 STEP 1: Extract tool name from spec ---
			name := p.extractToolName(path, method, operation)
			if name == "" {
				continue // skip if no valid name
			}

			// --- 📝 STEP 2: Extract description ---
			description := operation.Description
			if description == "" {
				description = operation.Summary
			} else {
				description += "/n" + operation.Summary

			}

			// --- 🧩 STEP 3: Build parameters schema ---
			parameters, err := p.buildParametersSchema(pathItem, operation, method)
			if err != nil {
				continue
			}

			tool := taskengine.Tool{
				Type: "function",
				Function: taskengine.FunctionTool{
					Name:        name,
					Description: description,
					Parameters:  parameters,
				},
			}
			tools = append(tools, tool)
		}
	}

	return tools, nil
}

// extractToolName extracts the tool name using operationId, x-tool-name, or fallback.
func (p *OpenAPIToolProtocol) extractToolName(path, method string, operation *openapi3.Operation) string {
	// 1. Try operationId (standard OpenAPI)
	if operation.OperationID != "" {
		return operation.OperationID
	}

	// 2. Try x-tool-name (custom extension)
	if toolName, ok := operation.Extensions["x-tool-name"].(string); ok && toolName != "" {
		if !isValidToolName(toolName) {
			return "" // skip invalid names
		}
		return toolName
	}

	// 3. Fallback: derive from path + method
	parts := strings.Split(strings.Trim(path, "/"), "/")
	baseName := parts[len(parts)-1]
	if baseName == "" && len(parts) > 1 {
		baseName = parts[len(parts)-2]
	}
	return fmt.Sprintf("%s_%s", baseName, strings.ToLower(method))
}

func (p *OpenAPIToolProtocol) buildParametersSchema(pathItem *openapi3.PathItem, operation *openapi3.Operation, method string) (map[string]interface{}, error) {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	// Helper to add a parameter schema to properties/required
	addParam := func(paramRef *openapi3.ParameterRef, isRequiredOverride bool) error {
		if paramRef == nil || paramRef.Value == nil {
			return nil
		}
		param := paramRef.Value

		name := param.Name
		if name == "" {
			return nil
		}

		// Only support path + query for now
		if param.In != "path" && param.In != "query" {
			return nil
		}

		schemaRef := param.Schema
		if schemaRef == nil || schemaRef.Value == nil {
			return nil
		}

		schemaJSON, err := schemaRef.Value.MarshalJSON()
		if err != nil {
			return err
		}

		var propSchema map[string]interface{}
		if err := json.Unmarshal(schemaJSON, &propSchema); err != nil {
			return err
		}

		properties[name] = propSchema

		if isRequiredOverride || param.Required {
			required = append(required, name)
		}

		return nil
	}

	// Add path parameters from PathItem (shared across operations)
	for _, paramRef := range pathItem.Parameters {
		if err := addParam(paramRef, true); err != nil { // path params always required
			return nil, err
		}
	}

	// Add operation-specific parameters
	for _, paramRef := range operation.Parameters {
		if err := addParam(paramRef, false); err != nil {
			return nil, err
		}
	}

	// Add request body (only for POST/PUT/PATCH)
	if operation.RequestBody != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		content := operation.RequestBody.Value.Content
		if jsonContent, ok := content["application/json"]; ok && jsonContent.Schema != nil {
			schema := jsonContent.Schema.Value

			// If it's an object, lift its properties into the top-level schema
			if schema.Type.Is("object") && len(schema.Properties) > 0 {
				for propName, propSchemaRef := range schema.Properties {
					if propSchemaRef != nil && propSchemaRef.Value != nil {
						schemaJSON, err := propSchemaRef.Value.MarshalJSON()
						if err != nil {
							continue
						}
						var propSchema map[string]interface{}
						if err := json.Unmarshal(schemaJSON, &propSchema); err != nil {
							continue
						}
						properties[propName] = propSchema

						if schema.Required != nil {
							if slices.Contains(schema.Required, propName) {
								required = append(required, propName)
							}
						}
					}
				}
			} else {
				// Not an object? Just embed the whole schema under a "body" property
				schemaJSON, err := schema.MarshalJSON()
				if err != nil {
					return nil, err
				}
				var bodySchema map[string]interface{}
				if err := json.Unmarshal(schemaJSON, &bodySchema); err != nil {
					return nil, err
				}
				properties["body"] = bodySchema
				if operation.RequestBody.Value.Required {
					required = append(required, "body")
				}
			}
		}
	}

	// If no properties, return nil (or empty schema)
	if len(properties) == 0 {
		return nil, nil
	}

	// Construct final schema
	finalSchema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		finalSchema["required"] = required
	}

	return finalSchema, nil
}

func isValidToolName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
