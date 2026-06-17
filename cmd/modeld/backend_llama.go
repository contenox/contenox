//go:build llamanode && llama_unsafe_abi

package main

import (
	"github.com/contenox/runtime/modeld/llama"
	// Blank-import the CGo llama.cpp adapter so its init registers the session
	// and embed factories on modeld/llama. Without this the daemon links the
	// pure-Go contract but never the backend, leaving OpenSession unavailable.
	_ "github.com/contenox/runtime/modeld/llama/llamasession"
	"github.com/contenox/runtime/runtime/transport"
)

// selectBackend serves the llama.cpp backend when built with -tags llamanode.
func selectBackend() (transport.Service, string) { return &llama.Service{}, "llama" }
