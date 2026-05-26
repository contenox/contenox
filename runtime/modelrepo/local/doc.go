// Package local implements the modelrepo.Provider contract for in-process
// inference using llama.cpp via github.com/ollama/ollama/llama (CGo). The
// package registers its catalog at init time; depend on it via blank import
// where the catalog must be discoverable from runtimestate.
//
// Importing this package requires CGO_ENABLED=1 and the toolchain
// prerequisites documented in CONTRIBUTING.md.
package local
