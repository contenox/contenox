package openapidocs

//go:generate go run github.com/contenox/runtime/tools/openapi-gen

import _ "embed"

//go:embed openapi.json
var specJSON []byte

//go:embed rapidoc.html
var rapidocHTML []byte
