//go:build !llamanode

package chattmpl

import "errors"

// Available reports that the minja renderer is not compiled into this build.
const Available = false

// Render is unavailable without the llamanode build tag.
func Render(_, _, _, _, _ string, _ bool) (string, error) {
	return "", errors.New("chattmpl: built without -tags llamanode")
}
