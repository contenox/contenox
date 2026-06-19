//go:build !llamanode || !llamacpp_direct

package llamasession

import (
	"errors"

	"github.com/contenox/runtime/modeld/llama"
)

// Available reports whether the llama.cpp backend is compiled into this build.
const Available = false

// New reports that the llama.cpp backend is not compiled in. Rebuild with
// `-tags "llamanode llamacpp_direct"` (and CGO + the direct llama.cpp
// toolchain) to enable it.
func New(_ string, _ llama.Config) (llama.Session, error) {
	return nil, errors.New("llamasession: built without the 'llamanode llamacpp_direct' tags")
}
