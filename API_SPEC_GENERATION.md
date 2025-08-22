OpenAPI Generator ManualThis guide explains how to write and structure your Go HTTP handlers and data types so that the custom OpenAPI generator tool can correctly produce a complete and accurate API specification. By following these conventions, you ensure that your documentation is always in sync with your code.1. Documenting Endpoints (Handlers)The documentation for each API endpoint is derived directly from the GoDoc comments above its handler function.Summary and DescriptionThe generator uses a simple rule:The first line of the comment block becomes the summary.The entire comment block becomes the description.To create a clean separation, always write a concise summary on the first line, leave a blank line, and then write the more detailed description.✅ Example// Create a new backend connection.
//
// Creates a new backend connection to an LLM provider. Backends represent
// connections to LLM services (e.g., Ollama, OpenAI) that can host models.
// Note: Creating a backend will be provisioned on the next synchronization cycle.
func (b *backendManager) createBackend(w http.ResponseWriter, r *http.Request) {
    // ...
}
🤖 Generated OpenAPIpaths:
  /backends:
    post:
      summary: Create a new backend connection.
      description: |-
        Create a new backend connection.

        Creates a new backend connection to an LLM provider. Backends represent
        connections to LLM services (e.g., Ollama, OpenAI) that can host models.
        Note: Creating a backend will be provisioned on the next synchronization cycle.
2. Documenting ParametersParameters are documented in the code by using dedicated helper functions. This makes the documentation explicit and verifiable by the compiler.Path ParametersTo document a path parameter, use the apiframework.GetPathParam function instead of the standard r.PathValue().name: The name of the parameter as it appears in the route (e.g., "id").description: A clear and concise description of the parameter.✅ Example// in your handler
id := apiframework.GetPathParam(r, "id", "The unique identifier for the backend.")
🤖 Generated OpenAPIparameters:
  - name: id
    in: path
    required: true
    description: The unique identifier for the backend.
    schema:
      type: string
Query ParametersTo document a query parameter, use the apiframework.GetQueryParam function.name: The name of the query parameter (e.g., "limit").defaultValue: The default value if the parameter is not provided. An empty string "" means no default.description: A clear description of the parameter.✅ Example// in your handler
limitStr := apiframework.GetQueryParam(r, "limit", "100", "The maximum number of items to return.")
cursorStr := apiframework.GetQueryParam(r, "cursor", "", "An optional timestamp for pagination.")
🤖 Generated OpenAPIparameters:
  - name: limit
    in: query
    required: false
    description: The maximum number of items to return.
    schema:
      type: string
      default: "100"
  - name: cursor
    in: query
    required: false
    description: An optional timestamp for pagination.
    schema:
      type: string
3. Documenting Request and Response BodiesThe request and response bodies are documented using special inline comments immediately following the serverops.Decode and serverops.Encode calls.The format is // @<type> <package>.<structName> or // @<type> []<package>.<structName>.Request BodyUse // @request after a call to serverops.Decode.✅ Example// in your handler
backend, err := serverops.Decode[runtimetypes.Backend](r) // @request runtimetypes.Backend
Response BodyUse // @response after a call to serverops.Encode. This annotation is mandatory for generating the response schema.✅ ExamplesSingle Object Response:// in your handler
_ = serverops.Encode(w, r, http.StatusOK, backend) // @response runtimetypes.Backend
Slice of Objects Response:// in your handler
_ = serverops.Encode(w, r, http.StatusOK, backends) // @response []runtimetypes.Backend
Slice of Pointers to Objects Response:// in your handler
_ = serverops.Encode(w, r, http.StatusOK, models) // @response []*runtimetypes.Model
Simple Type Response (e.g., string):// in your handler
_ = serverops.Encode(w, r, http.StatusOK, "deleted") // @response string
4. Documenting Schemas (Structs)The generator automatically converts exported Go structs into OpenAPI schemas. You can add rich detail using standard Go comments and struct tags.Field Descriptions and ExamplesDescription: A standard Go comment directly above a field becomes its description.Example: The example:"..." struct tag provides an example value.JSON Name: The json:"..." tag is required to define the property name in the OpenAPI spec.✅ Exampletype BackendRuntimeState struct {
	// ID is the unique identifier for the backend.
	ID string `json:"id" example:"b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e"`

	// Error stores a description of the last encountered error, if any.
	Error string `json:"error,omitempty" example:"connection timeout"`
}
🤖 Generated OpenAPIstatetype_BackendRuntimeState:
  type: object
  properties:
    id:
      type: string
      description: ID is the unique identifier for the backend.
      example: b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e
    error:
      type: string
      description: Error stores a description of the last encountered error, if any.
      example: connection timeout
Documenting Slices of StructsWhen a struct contains a slice of another complex struct, you must provide the openapi_include_type tag to tell the generator what the items in the slice are.✅ Exampletype backendSummary struct {
    // ...
    PulledModels []statetype.ModelPullStatus `json:"pulledModels" openapi_include_type:"statetype.ModelPullStatus"`
}
🤖 Generated OpenAPIbackendapi_backendSummary:
  type: object
  properties:
    pulledModels:
      type: array
      items:
        $ref: '#/components/schemas/statetype_ModelPullStatus'
