// Package llamacppshim owns the direct llama.cpp C API boundary for modeld.
//
// It is intentionally small: product code talks to modeld/llama.Session, and
// llamasession will use this package for the native substrate once the Ollama
// binding is removed from the hot path.
package llamacppshim
