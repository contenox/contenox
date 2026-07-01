//go:build !llamacpp_direct

package llamacppshim

import "errors"

// Available reports whether the direct llama.cpp shim was compiled in.
const Available = false

// ModelKVProfile contains model metadata needed to budget KV cache.
type ModelKVProfile struct {
	ContextLength              int
	BlockCount                 int
	HeadCountKV                int
	HeadCount                  int
	KeyLength                  int
	EmbeddingLength            int
	SlidingWindow              int
	SlidingWindowPattern       []bool
	SlidingWindowPatternStride int
}

// InspectModelKVProfile is available only in direct llama.cpp builds.
func InspectModelKVProfile(string) (ModelKVProfile, error) {
	return ModelKVProfile{}, errors.New("llamacppshim: direct llama.cpp backend is not compiled in")
}
