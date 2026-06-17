//go:build !openvino_genai && !llamanode

package main

import (
	"context"
	"errors"

	"github.com/contenox/runtime/runtime/transport"
)

// errNoBackend is returned by OpenSession when this modeld build has no local
// inference backend compiled in. The daemon still owns the lease and answers
// health probes — detection reports it running — it just cannot open sessions.
var errNoBackend = errors.New("modeld: no local inference backend compiled in this build (build with -tags 'openvino openvino_genai' or -tags llamanode)")

type unavailableBackend struct{}

func (unavailableBackend) OpenSession(context.Context, transport.OpenSessionRequest) (transport.Session, error) {
	return nil, errNoBackend
}

// selectBackend returns the transport.Service this build serves. The default
// build has no CGO backend.
func selectBackend() (transport.Service, string) { return unavailableBackend{}, "none" }
