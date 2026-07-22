package libacp

// NegotiateProtocolVersion returns the protocol version two peers will speak,
// given the version the other peer requested or offered (theirs) and the
// highest version this peer implements (ours — normally ProtocolVersion). It
// accepts theirs when this peer can speak it (1 <= theirs <= ours) and
// otherwise falls back to ours, matching acpsvc's spec-correct agent-side
// negotiation (runtime/acpsvc/initialize.go).
//
// This deliberately does NOT require exact equality between the requested and
// returned version. A client that hard-fails unless the agent echoes back the
// literal version it sent is stricter than the spec and will break interop
// with any future peer that legitimately answers a different version it can
// still speak. Pinning min-of-both here gives any future libacp client
// consumer the resilient semantics as a reusable primitive so a refactor
// cannot drift toward that fragility.
func NegotiateProtocolVersion(theirs, ours int) int {
	if theirs >= 1 && theirs <= ours {
		return theirs
	}
	return ours
}
