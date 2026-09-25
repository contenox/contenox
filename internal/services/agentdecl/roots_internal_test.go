package agentdecl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// rootOf opens dir as a compilation root for the in-package tests, which reach
// the unexported half of the overlay merge directly.
func rootOf(t *testing.T, dir string) Root {
	t.Helper()
	root, err := LocalRoot(dir)
	require.NoError(t, err)
	return root
}
