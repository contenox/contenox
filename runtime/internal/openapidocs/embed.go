package openapidocs

import _ "embed"

//go:embed openapi.json
var specJSON []byte

//go:embed rapidoc.html
var rapidocHTML []byte
