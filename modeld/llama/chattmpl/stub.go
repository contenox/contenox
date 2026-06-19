//go:build !llamanode && !llamacpp_direct

package chattmpl

import "errors"

// Available reports that the minja renderer is not compiled into this build.
const Available = false

// Render is unavailable without the llamanode or llamacpp_direct build tag.
func Render(_, _, _, _, _ string, _ bool) (string, error) {
	return "", errors.New("chattmpl: built without -tags llamanode or llamacpp_direct")
}
