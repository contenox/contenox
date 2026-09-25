// Package gateway is the self-hosted model gateway: an Ollama-compatible HTTP
// surface that authenticates callers with license tokens and proxy keys,
// validates the requested model against the token's claims, meters each turn
// against the key's allowance, and routes the turn through the runtime's model
// repo.
package gateway
