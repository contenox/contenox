package ollama

// The extensions this client can use against a peer that implements them. Each
// one is an addition to the Ollama wire that the upstream contract has not
// merged, so a peer that does not advertise it would ignore the field, drop the
// payload and still answer.
const (
	// ExtensionAudios is audio input carried on a message's `audios` field.
	ExtensionAudios = "audios"
	// ExtensionSession is a conversation named on a request's `contenox_session`
	// field, which lets a peer fronting several backends keep one
	// conversation's turns on one of them and its prefix cache warm.
	ExtensionSession = "session"
)

// ProductContenoxGateway is the product a contenox gateway names itself on its
// own endpoint.
const ProductContenoxGateway = "contenox-gateway"

// ContenoxPath is where a contenox gateway describes itself. Vanilla Ollama does
// not serve it, so a 404 here is what leaves every extension off.
const ContenoxPath = "/api/contenox"

const contenoxRoute = "/contenox"

// ContenoxResponse is the handshake: what the endpoint is, and which additions to
// the Ollama wire it understands.
type ContenoxResponse struct {
	Product string `json:"product"`
	// Version is the product's own version, from the same plumbing the CLI
	// reports; Build is the revision and time it came from.
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
	// OllamaVersion is the Ollama API generation the endpoint mirrors, reported
	// so a caller does not have to read it off the vanilla route.
	OllamaVersion string `json:"ollama_version,omitempty"`
	// Extensions names the wire additions this endpoint understands, so a caller
	// checks for the one it wants rather than inferring it from the product.
	Extensions []string `json:"extensions,omitempty"`
}

// IsContenoxGateway reports whether the endpoint answered as a contenox gateway.
func (r ContenoxResponse) IsContenoxGateway() bool {
	return r.Product == ProductContenoxGateway
}

// Supports reports whether the endpoint declared one of the wire extensions.
func (r ContenoxResponse) Supports(extension string) bool {
	for _, declared := range r.Extensions {
		if declared == extension {
			return true
		}
	}
	return false
}
