// Command openapi-gen regenerates runtime/internal/openapidocs/openapi.json
// from the HTTP route registrations and inline annotations in the route
// packages. It is a thin wrapper around internal/openapigen, which documents
// the annotation conventions and holds all generation logic; keeping the core
// there lets tests byte-compare a fresh generation against the embedded file.
//
// Run from anywhere inside the repo: `go run ./tools/openapi-gen`
// (or `go generate ./...`). Generation is strict: any problem (parse failure,
// unresolved type reference, duplicate route, ambiguous annotation binding,
// an operation missing its @request/@response annotation) is reported with
// its location and the command exits non-zero.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/internal/openapigen"
)

func main() {
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	out, stats, err := openapigen.Generate(root)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(root, openapigen.OutPath), out, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s: %d paths, %d schemas\n", openapigen.OutPath, stats.Paths, stats.Schemas)
	if stats.NonLiteralParamArgs > 0 {
		fmt.Printf("note: %d helper-call default/description argument(s) were not string literals and emitted as absent\n", stats.NonLiteralParamArgs)
	}
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
