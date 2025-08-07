package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// main is the entry point for the static HTML generator.
// It reads the openapi.json file, embeds its content into an HTML template,
// and writes the result to index.html.
func main() {
	// 1. Read the OpenAPI JSON file.
	openAPISpec, err := os.ReadFile("./docs/openapi.json")
	if err != nil {
		fmt.Printf("Error reading openapi.json: %v\n", err)
		os.Exit(1)
	}

	// 2. Validate the JSON content. This is a good practice.
	var jsonData map[string]interface{}
	if err := json.Unmarshal(openAPISpec, &jsonData); err != nil {
		fmt.Printf("Error unmarshalling openapi.json: %v\n", err)
		os.Exit(1)
	}

	// 3. Define the HTML template. This is a self-contained document
	// that includes the Swagger UI library and embeds the OpenAPI spec.
	htmlTemplate := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>API Documentation</title>
    <!-- We are using local Swagger UI files to avoid internet dependency.
         In a real-world scenario, you would bundle these files locally.
         For this example, we'll embed the spec directly. -->
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.17.14/swagger-ui.min.css" />
    <style>
        body {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
            font-family: 'Inter', sans-serif;
        }
    </style>
</head>
<body>

    <!-- The container for Swagger UI to render the docs -->
    <div id="swagger-ui"></div>

    <!-- Swagger UI JavaScript -->
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.17.14/swagger-ui-bundle.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.17.14/swagger-ui-standalone-preset.min.js"></script>

    <script>
        // Use the window.onload event to ensure the script runs after the page is fully loaded
        window.onload = function() {
            // Check if the required scripts are loaded
            if (typeof SwaggerUIBundle !== 'undefined') {
                // We embed the JSON content directly into the JavaScript to make it self-contained.
                const spec = %s;
                const ui = SwaggerUIBundle({
                    // Point Swagger UI to the embedded JSON object
                    spec: spec,
                    dom_id: '#swagger-ui',
                    deepLinking: true,
                    presets: [
                        SwaggerUIBundle.presets.apis,
                        SwaggerUIStandalonePreset
                    ],
                    plugins: [
                        SwaggerUIBundle.plugins.DownloadUrl
                    ],
                    layout: "StandaloneLayout"
                });
            } else {
                console.error("Swagger UI scripts failed to load.");
            }
        };
    </script>

</body>
</html>`, string(openAPISpec))

	// 4. Write the generated HTML to a new file.
	outputFile := "./docs/index.html"
	if err := os.WriteFile(outputFile, []byte(htmlTemplate), 0644); err != nil {
		fmt.Printf("Error writing index.html: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s\n", outputFile)
}
