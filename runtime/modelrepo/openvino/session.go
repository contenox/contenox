package openvino

import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

// The OpenVINO backend speaks the runtime/transport session contract directly,
// so these aliases let the provider/client use it without an adapter. The CGO
// OpenVINO GenAI backend lives in modeld; the runtime reaches it only through
// runtime/transport, dialing the lease leader via modeldconn.
type (
	Config        = transport.Config
	Session       = transport.Session
	PrefixInput   = transport.PrefixInput
	SuffixInput   = transport.SuffixInput
	PrefixStatus  = transport.PrefixStatus
	SuffixStatus  = transport.SuffixStatus
	DecodeConfig  = transport.DecodeConfig
	StreamChunk   = transport.StreamChunk
	ContextReport = transport.ContextReport
)

var (
	// ErrSessionUnavailable means modeld could not serve a local openvino session.
	ErrSessionUnavailable = errors.New("openvino: session backend unavailable")
	// ErrUnsupportedFeature marks deliberately unsupported product surfaces.
	ErrUnsupportedFeature = errors.New("openvino: unsupported feature")
)

// SessionAvailable reports whether the modeld daemon holds a fresh lease to
// serve openvino sessions (the cheap offline check).
func SessionAvailable() bool { return modeldconn.Available() }

// newSession opens an openvino session on the modeld daemon over the transport.
// The model id is the OpenVINO IR directory; modeld makes it resident.
func newSession(modelDir string, cfg Config) (Session, error) {
	s, err := modeldconn.OpenSession(context.Background(), modelDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionUnavailable, err)
	}
	return s, nil
}

// NewUnsupportedFeatureError builds an unsupported-feature error.
func NewUnsupportedFeatureError(feature string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedFeature, feature)
}

// fatalSessionError reports whether the cached session must be evicted: it is
// closed, or the owner changed under us (stale fence after a lease takeover), in
// which case the next call reopens against the new leader.
func fatalSessionError(err error) bool {
	return errors.Is(err, transport.ErrSessionClosed) || errors.Is(err, transport.ErrStaleFence)
}
