// Package transport defines the protocol-agnostic surface that modeld exposes
// over a wire transport. Concrete transports (the grpc subpackage today; an
// HTTP transport later) adapt this Service to their protocol so the daemon's
// API is described in exactly one place.
package transport

import (
	"context"

	"github.com/contenox/runtime/modeld"
)

// Service is the modeld API as seen by a transport. It is intentionally a thin
// projection of *modeld.Daemon so transports never reach into daemon state
// directly.
type Service interface {
	RegisterBackend(ctx context.Context, backendID string, spec modeld.BackendSpec) error
	RemoveBackend(ctx context.Context, backendID string) error
	ListBackends(ctx context.Context) ([]string, error)
	ListModels(ctx context.Context, backendID string) ([]modeld.ObservedModel, error)
}

// FromDaemon adapts a *modeld.Daemon to the Service interface.
func FromDaemon(d *modeld.Daemon) Service {
	return daemonService{d: d}
}

type daemonService struct {
	d *modeld.Daemon
}

func (s daemonService) RegisterBackend(ctx context.Context, backendID string, spec modeld.BackendSpec) error {
	return s.d.RegisterBackend(backendID, spec)
}

func (s daemonService) RemoveBackend(ctx context.Context, backendID string) error {
	s.d.RemoveBackend(backendID)
	return nil
}

func (s daemonService) ListBackends(ctx context.Context) ([]string, error) {
	return s.d.ListBackends(), nil
}

func (s daemonService) ListModels(ctx context.Context, backendID string) ([]modeld.ObservedModel, error) {
	return s.d.ListModels(ctx, backendID)
}

var _ Service = daemonService{}
