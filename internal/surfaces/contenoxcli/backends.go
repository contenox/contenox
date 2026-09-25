package contenoxcli

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
)

func setBackendCredentialKV(ctx context.Context, store runtimetypes.Store, providerType, backendID, apiKey string) error {
	pc := runtimestate.ProviderConfig{APIKey: apiKey, Type: providerType}
	data, err := json.Marshal(pc)
	if err != nil {
		return nil // Non-fatal for backward compat.
	}
	if err := store.SetKV(ctx, runtimestate.BackendCredentialKey(providerType, backendID), json.RawMessage(data)); err != nil {
		return err
	}
	return setProviderConfigKV(ctx, store, providerType, apiKey)
}

func setProviderConfigKV(ctx context.Context, store runtimetypes.Store, providerType, apiKey string) error {
	key := runtimestate.ProviderKeyPrefix + strings.ToLower(providerType)
	pc := runtimestate.ProviderConfig{APIKey: apiKey, Type: providerType}
	data, err := json.Marshal(pc)
	if err != nil {
		return nil // Non-fatal for backward compat.
	}
	return store.SetKV(ctx, key, json.RawMessage(data))
}
