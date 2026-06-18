package backendservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

var ErrInvalidBackend = errors.New("invalid backend data")

type Service interface {
	Create(ctx context.Context, backend *runtimetypes.Backend) error
	Get(ctx context.Context, id string) (*runtimetypes.Backend, error)
	Update(ctx context.Context, backend *runtimetypes.Backend) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Backend, error)
}

type service struct {
	dbInstance libdb.DBManager
}

func New(db libdb.DBManager) Service {
	return &service{dbInstance: db}
}

func (s *service) Create(ctx context.Context, backend *runtimetypes.Backend) error {
	tx := s.dbInstance.WithoutTransaction()
	if err := validate(backend); err != nil {
		return err
	}
	storeInstance := runtimetypes.New(tx)
	count, err := storeInstance.EstimateBackendCount(ctx)
	if err != nil {
		return err
	}
	if err := storeInstance.EnforceMaxRowCount(ctx, count); err != nil {
		return fmt.Errorf("too many rows in the system (current %d): %w", count, err)
	}

	return storeInstance.CreateBackend(ctx, backend)
}

func (s *service) Get(ctx context.Context, id string) (*runtimetypes.Backend, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).GetBackend(ctx, id)
}

func (s *service) Update(ctx context.Context, backend *runtimetypes.Backend) error {
	if err := validate(backend); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).UpdateBackend(ctx, backend)
}

func (s *service) Delete(ctx context.Context, id string) error {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).DeleteBackend(ctx, id)
}

func (s *service) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Backend, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListBackends(ctx, createdAtCursor, limit)
}

func validate(backend *runtimetypes.Backend) error {
	if backend.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidBackend)
	}
	if backend.BaseURL == "" {
		return fmt.Errorf("%w: baseURL is required", ErrInvalidBackend)
	}
	switch modelrepo.CanonicalBackendType(backend.Type) {
	case "ollama", "vllm", "openai", "anthropic", "mistral", "bedrock", "gemini", "llama", "openvino", "vertex-google":
	default:
		return fmt.Errorf("%w: Type must be ollama, vllm, openai, anthropic, mistral, bedrock, gemini, llama (local alias accepted), openvino, or vertex-google", ErrInvalidBackend)
	}

	return nil
}
