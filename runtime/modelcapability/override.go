package modelcapability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

const KeyPrefix = "model-capability:"

type Override struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	CanThink *bool  `json:"canThink,omitempty"`
}

type storedOverride struct {
	CanThink *bool `json:"canThink"`
}

type Service struct {
	store runtimetypes.Store
}

func New(store runtimetypes.Store) Service {
	return Service{store: store}
}

func Key(provider, model string) (string, string, string, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.TrimSpace(model)
	if p == "" {
		return "", "", "", fmt.Errorf("provider is required")
	}
	if m == "" {
		return "", "", "", fmt.Errorf("model is required")
	}
	return KeyPrefix + p + ":" + m, p, m, nil
}

func (s Service) SetThink(ctx context.Context, provider, model string, canThink bool) (*Override, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store is required")
	}
	key, p, m, err := Key(provider, model)
	if err != nil {
		return nil, err
	}
	v := storedOverride{CanThink: &canThink}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetKV(ctx, key, json.RawMessage(data)); err != nil {
		return nil, err
	}
	return &Override{Provider: p, Model: m, CanThink: &canThink}, nil
}

func (s Service) Get(ctx context.Context, provider, model string) (*Override, bool, error) {
	if s.store == nil {
		return nil, false, fmt.Errorf("store is required")
	}
	key, p, m, err := Key(provider, model)
	if err != nil {
		return nil, false, err
	}
	var v storedOverride
	if err := s.store.GetKV(ctx, key, &v); err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &Override{Provider: p, Model: m, CanThink: v.CanThink}, true, nil
}

func (s Service) Unset(ctx context.Context, provider, model string) (bool, error) {
	if s.store == nil {
		return false, fmt.Errorf("store is required")
	}
	key, _, _, err := Key(provider, model)
	if err != nil {
		return false, err
	}
	if err := s.store.DeleteKV(ctx, key); err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
