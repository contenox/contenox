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
	ToolCall      = transport.ToolCall
	ContextReport = transport.ContextReport
)

var (
	// ErrSessionUnavailable means modeld could not serve a local openvino session.
	ErrSessionUnavailable = errors.New("openvino: session backend unavailable")
	// ErrUnsupportedFeature marks deliberately unsupported product surfaces.
	ErrUnsupportedFeature = errors.New("openvino: unsupported feature")
)

type SessionFactory func(ref modeldconn.ModelRef, cfg Config) (Session, error)

var sessionFactory SessionFactory

// SetSessionFactory registers a test/session backend. Production runtime paths
// leave this nil and talk to modeld through modeldconn.
func SetSessionFactory(f SessionFactory) { sessionFactory = f }

// SessionAvailable reports whether the modeld daemon serves the openvino backend.
// It uses modeldconn.ServeableBackend (not the strict Backend) so a brief lease
// gap during a daemon restart does not momentarily drop openvino models from
// capability advertisement / the model picker. A daemon running in a different
// mode (e.g. llama) advertises no openvino capability.
func SessionAvailable() bool {
	return sessionFactory != nil || modeldconn.ServeableBackend() == "openvino"
}

// newSession opens an openvino session on the modeld daemon over the transport.
// ref.Path is the OpenVINO IR directory; modeld makes it resident after checking
// the model type matches the served backend.
func newSession(ref modeldconn.ModelRef, cfg Config) (Session, error) {
	if sessionFactory != nil {
		return sessionFactory(ref, cfg)
	}
	s, err := modeldconn.OpenSession(context.Background(), ref, cfg)
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
	return errors.Is(err, transport.ErrSessionClosed) ||
		errors.Is(err, transport.ErrSessionFatal) ||
		errors.Is(err, transport.ErrStaleFence) ||
		errors.Is(err, transport.ErrSlotGenerationStale) ||
		errors.Is(err, transport.ErrModelNotActive)
}
