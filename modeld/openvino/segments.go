package openvino

import "github.com/contenox/runtime/modeld/contextasm"

// Kept as local aliases while OpenVINO call sites finish migrating to the
// backend-neutral contextasm package.
type SegmentKind = contextasm.SegmentKind
type Segment = contextasm.Segment

const (
	KindSystem    = contextasm.KindSystem
	KindTools     = contextasm.KindTools
	KindRepoRules = contextasm.KindRepoRules
	KindRepoMap   = contextasm.KindRepoMap
	KindPinned    = contextasm.KindPinned
	KindDiff      = contextasm.KindDiff
	KindTerminal  = contextasm.KindTerminal
	KindUserTurn  = contextasm.KindUserTurn
)

func AssembleContext(segs []Segment) (prompt string, stablePrefixHash string) {
	return contextasm.AssembleContext(segs)
}
