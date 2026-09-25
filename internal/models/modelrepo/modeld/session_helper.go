package modeld

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/contenox/contenox/internal/modeld/modelstore"
	"github.com/contenox/contenox/internal/models/modelregistry"
	"github.com/contenox/contenox/internal/models/modelrepo/modeldconn"
	"github.com/contenox/contenox/internal/transport"
	"github.com/contenox/contenox/libtracker"
)

func openSessionWithAutoPull(ctx context.Context, target modeldconn.ModeldTarget, modelName string, cfg transport.Config, tracker libtracker.ActivityTracker) (transport.Session, error) {
	ref, err := resolveModelRef(ctx, target, modelName)
	if err != nil {
		return nil, err
	}
	sess, err := modeldconn.OpenSessionTarget(ctx, target, ref, cfg)
	if err == nil {
		return sess, nil
	}

	if target.Endpoint == "" && isModelNotFoundError(err) {
		reg := modelregistry.New(nil)
		pullErr := modelstore.EnsureModelAvailable(ctx, reg, modelName, modelstore.AutoPullOptions{
			DataRoot:    modeldconn.DataRoot(),
			ProgressOut: os.Stderr,
			Tracker:     tracker,
		})
		if pullErr != nil {
			return nil, fmt.Errorf("model %q not found locally and auto-pull failed: %w (original error: %v)", modelName, pullErr, err)
		}
		// Retry opening session after successful auto-pull
		return modeldconn.OpenSessionTarget(ctx, target, ref, cfg)
	}

	return nil, err
}

func embedWithAutoPull(ctx context.Context, target modeldconn.ModeldTarget, modelName string, cfg transport.Config, text string, tracker libtracker.ActivityTracker) (transport.EmbedResult, error) {
	ref, err := resolveModelRef(ctx, target, modelName)
	if err != nil {
		return transport.EmbedResult{}, err
	}
	res, err := modeldconn.EmbedTarget(ctx, target, ref, cfg, text)
	if err == nil {
		return res, nil
	}

	if target.Endpoint == "" && isModelNotFoundError(err) {
		reg := modelregistry.New(nil)
		pullErr := modelstore.EnsureModelAvailable(ctx, reg, modelName, modelstore.AutoPullOptions{
			DataRoot:    modeldconn.DataRoot(),
			ProgressOut: os.Stderr,
			Tracker:     tracker,
		})
		if pullErr != nil {
			return transport.EmbedResult{}, fmt.Errorf("model %q not found locally and auto-pull failed: %w (original error: %v)", modelName, pullErr, err)
		}
		return modeldconn.EmbedTarget(ctx, target, ref, cfg, text)
	}

	return transport.EmbedResult{}, err
}

func isModelNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, transport.ErrModelNotFound) || errors.Is(err, modelstore.ErrModelNotFound) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "model not found") || strings.Contains(msg, "ModelNotFound")
}

func resolveModelRef(ctx context.Context, target modeldconn.ModeldTarget, name string) (modeldconn.ModelRef, error) {
	if desc, err := modelregistry.New(nil).Resolve(ctx, name); err == nil {
		return modeldconn.ModelRef{Name: name, Type: desc.BackendType()}, nil
	}
	endpoint := target.Endpoint
	if endpoint == "" {
		var err error
		endpoint, err = modeldconn.LocalEndpointAddr(ctx)
		if err != nil {
			return modeldconn.ModelRef{}, err
		}
	}
	client, err := modeldconn.Endpoint(ctx, target.BackendID, endpoint)
	if err != nil {
		return modeldconn.ModelRef{}, err
	}
	if client.Backend != "llama" && client.Backend != "openvino" {
		return modeldconn.ModelRef{}, fmt.Errorf("%w: daemon reports %q", transport.ErrUnsupportedModelType, client.Backend)
	}
	return modeldconn.ModelRef{Name: name, Type: client.Backend}, nil
}
