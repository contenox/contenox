package hookproviderservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/contenox/runtime/internal/apiframework"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtimetypes"
)

var (
	ErrInvalidHook = errors.New("invalid remote hook data")
)

type Service interface {
	Create(ctx context.Context, hook *runtimetypes.RemoteHook) error
	Get(ctx context.Context, id string) (*runtimetypes.RemoteHook, error)
	GetByName(ctx context.Context, name string) (*runtimetypes.RemoteHook, error)
	Update(ctx context.Context, hook *runtimetypes.RemoteHook) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.RemoteHook, error)
}

type service struct {
	dbInstance libdb.DBManager
}

func New(dbInstance libdb.DBManager) Service {
	return &service{
		dbInstance: dbInstance,
	}
}

func (s *service) Create(ctx context.Context, hook *runtimetypes.RemoteHook) error {
	if err := validate(hook); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)
	count, err := storeInstance.EstimateRemoteHookCount(ctx)
	if err != nil {
		return err
	}
	err = storeInstance.EnforceMaxRowCount(ctx, count)
	if err != nil {
		return err
	}
	return storeInstance.CreateRemoteHook(ctx, hook)
}

func (s *service) Get(ctx context.Context, id string) (*runtimetypes.RemoteHook, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).GetRemoteHook(ctx, id)
}

func (s *service) GetByName(ctx context.Context, name string) (*runtimetypes.RemoteHook, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).GetRemoteHookByName(ctx, name)
}

func (s *service) Update(ctx context.Context, hook *runtimetypes.RemoteHook) error {
	if err := validate(hook); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).UpdateRemoteHook(ctx, hook)
}

func (s *service) Delete(ctx context.Context, id string) error {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).DeleteRemoteHook(ctx, id)
}

func (s *service) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.RemoteHook, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListRemoteHooks(ctx, createdAtCursor, limit)
}

func validate(hook *runtimetypes.RemoteHook) error {
	switch {
	case hook.Name == "":
		return fmt.Errorf("%w %w: name is required", ErrInvalidHook, apiframework.ErrUnprocessableEntity)
	case hook.EndpointURL == "":
		return fmt.Errorf("%w %w: endpoint URL is required", ErrInvalidHook, apiframework.ErrUnprocessableEntity)
	case hook.Method == "":
		return fmt.Errorf("%w %w: HTTP method is required", ErrInvalidHook, apiframework.ErrUnprocessableEntity)
	case hook.TimeoutMs <= 0:
		return fmt.Errorf("%w %w: timeout must be positive", ErrInvalidHook, apiframework.ErrUnprocessableEntity)
	}

	// Validate headers if provided
	for key, value := range hook.Headers {
		if key == "" {
			return fmt.Errorf("%w %w: header name cannot be empty", ErrInvalidHook, apiframework.ErrUnprocessableEntity)
		}
		if value == "" {
			return fmt.Errorf("%w %w: header value for %s cannot be empty", ErrInvalidHook, apiframework.ErrUnprocessableEntity, key)
		}
	}

	return nil
}
