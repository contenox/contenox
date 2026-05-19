package providerservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/runtime/internal/runtimestate"
	"github.com/contenox/contenox/runtime/runtimetypes"
)

type Encryptor interface {
	Status(ctx context.Context) (bool, error)
	EncryptString(plain string) (string, error)
	DecryptString(value string) (string, error)
	IsEncrypted(value string) bool
}

const (
	ProviderTypeOllama          = "ollama"
	ProviderTypeOpenAI          = "openai"
	ProviderTypeGemini          = "gemini"
	ProviderTypeVertexGoogle    = "vertex-google"
	ProviderTypeVertexAnthropic = "vertex-anthropic"
	ProviderTypeVertexMeta      = "vertex-meta"
	ProviderTypeVertexMistral   = "vertex-mistralai"
)

type Service interface {
	SetProviderConfig(ctx context.Context, providerType string, upsert bool, config *runtimestate.ProviderConfig) error
	GetProviderConfig(ctx context.Context, providerType string) (*runtimestate.ProviderConfig, error)
	DeleteProviderConfig(ctx context.Context, providerType string) error
	ListProviderConfigs(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimestate.ProviderConfig, error)
}

type service struct {
	dbInstance libdb.DBManager
	encryptor  Encryptor
}

func New(dbInstance libdb.DBManager, enc Encryptor) Service {
	return &service{dbInstance: dbInstance, encryptor: enc}
}

func (s *service) SetProviderConfig(ctx context.Context, providerType string, replace bool, config *runtimestate.ProviderConfig) error {
	// Input validation
	validTypes := map[string]bool{
		ProviderTypeOllama: true, ProviderTypeOpenAI: true, ProviderTypeGemini: true,
		ProviderTypeVertexGoogle: true, ProviderTypeVertexAnthropic: true,
		ProviderTypeVertexMeta: true, ProviderTypeVertexMistral: true,
	}
	if !validTypes[providerType] {
		return fmt.Errorf("invalid provider type: %s", providerType)
	}
	if config == nil {
		return fmt.Errorf("missing config")
	}
	if config.APIKey == "" {
		return fmt.Errorf("missing credentials")
	}

	tx, com, r, err := s.dbInstance.WithTransaction(ctx)
	if err != nil {
		return err
	}
	defer r()

	storeInstance := runtimetypes.New(tx)
	count, err := storeInstance.EstimateKVCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to estimate KV count: %w", err)
	}
	err = storeInstance.EnforceMaxRowCount(ctx, count)
	if err != nil {
		return err
	}
	key := runtimestate.ProviderKeyPrefix + providerType

	// Check existence if not replacing
	if !replace {
		var existing json.RawMessage
		if err := storeInstance.GetKV(ctx, key, &existing); err == nil {
			return fmt.Errorf("provider config already exists")
		} else if !errors.Is(err, libdb.ErrNotFound) {
			return fmt.Errorf("failed to check existing config: %w", err)
		}
	}

	config.Type = providerType
	persistedKey, err := s.maybeEncrypt(ctx, config.APIKey)
	if err != nil {
		return err
	}
	persistedConfig := runtimestate.ProviderConfig{APIKey: persistedKey, Type: config.Type}
	data, err := json.Marshal(persistedConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := storeInstance.SetKV(ctx, key, data); err != nil {
		return fmt.Errorf("failed to store config: %w", err)
	}

	backendURL := ""
	switch providerType {
	case ProviderTypeOllama:
		backendURL = "https://ollama.com/api"
	case ProviderTypeOpenAI:
		backendURL = "https://api.openai.com/v1"
	case ProviderTypeGemini:
		backendURL = "https://generativelanguage.googleapis.com"
	}
	if backendURL == "" {
		return fmt.Errorf("provider %q is not yet wired for one-click configuration (missing backend URL)", providerType)
	}

	backend := &runtimetypes.Backend{
		ID:      providerType,
		Name:    providerType,
		BaseURL: backendURL,
		Type:    providerType,
	}

	if _, err := storeInstance.GetBackend(ctx, providerType); errors.Is(err, libdb.ErrNotFound) {
		if err := storeInstance.CreateBackend(ctx, backend); err != nil {
			return fmt.Errorf("failed to create backend: %w", err)
		}
	} else if err == nil && replace {
		// Update existing backend if replacing
		if err := storeInstance.UpdateBackend(ctx, backend); err != nil {
			return fmt.Errorf("failed to update backend: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check backend existence: %w", err)
	}

	return com(ctx)
}

func (s *service) GetProviderConfig(ctx context.Context, providerType string) (*runtimestate.ProviderConfig, error) {
	tx := s.dbInstance.WithoutTransaction()
	var config runtimestate.ProviderConfig
	key := runtimestate.ProviderKeyPrefix + providerType
	storeInstance := runtimetypes.New(tx)
	err := storeInstance.GetKV(ctx, key, &config)
	if err != nil {
		return nil, err
	}
	plain, err := s.maybeDecrypt(ctx, config.APIKey)
	if err != nil {
		return nil, err
	}
	config.APIKey = plain
	return &config, nil
}

func (s *service) DeleteProviderConfig(ctx context.Context, providerType string) error {
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)

	key := runtimestate.ProviderKeyPrefix + providerType
	return storeInstance.DeleteKV(ctx, key)
}

func (s *service) ListProviderConfigs(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimestate.ProviderConfig, error) {
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)

	kvs, err := storeInstance.ListKVPrefix(ctx, runtimestate.ProviderKeyPrefix, createdAtCursor, limit)
	if err != nil {
		return nil, err
	}

	var configs []*runtimestate.ProviderConfig
	for _, kv := range kvs {
		var config runtimestate.ProviderConfig
		if err := json.Unmarshal(kv.Value, &config); err != nil {
			continue
		}
		plain, err := s.maybeDecrypt(ctx, config.APIKey)
		if err != nil {
			return nil, err
		}
		config.APIKey = plain
		configs = append(configs, &config)
	}
	return configs, nil
}

func (s *service) maybeEncrypt(ctx context.Context, plain string) (string, error) {
	if s.encryptor == nil {
		return plain, nil
	}
	initialized, err := s.encryptor.Status(ctx)
	if err != nil {
		return "", err
	}
	if !initialized {
		return plain, nil
	}
	enc, err := s.encryptor.EncryptString(plain)
	if err != nil {
		return "", err
	}
	return enc, nil
}

func (s *service) maybeDecrypt(ctx context.Context, value string) (string, error) {
	if s.encryptor == nil {
		return value, nil
	}
	if !s.encryptor.IsEncrypted(value) {
		return value, nil
	}
	return s.encryptor.DecryptString(value)
}
