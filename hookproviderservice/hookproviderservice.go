package hookproviderservice

import (
	"context"
	"errors"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
)

var (
	ErrInvalidHook = errors.New("invalid remote hook data")
)

type Service interface {
	Create(ctx context.Context, hook *store.RemoteHook) error
	Get(ctx context.Context, id string) (*store.RemoteHook, error)
	GetByName(ctx context.Context, name string) (*store.RemoteHook, error)
	Update(ctx context.Context, hook *store.RemoteHook) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*store.RemoteHook, error)
}

type service struct {
	dbInstance libdb.DBManager
}

func New(dbInstance libdb.DBManager) Service {
	return &service{
		dbInstance: dbInstance,
	}
}

func (s *service) Create(ctx context.Context, hook *store.RemoteHook) error {
	if err := validate(hook); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).CreateRemoteHook(ctx, hook)
}

func (s *service) Get(ctx context.Context, id string) (*store.RemoteHook, error) {
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).GetRemoteHook(ctx, id)
}

func (s *service) GetByName(ctx context.Context, name string) (*store.RemoteHook, error) {
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).GetRemoteHookByName(ctx, name)
}

func (s *service) Update(ctx context.Context, hook *store.RemoteHook) error {
	if err := validate(hook); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).UpdateRemoteHook(ctx, hook)
}

func (s *service) Delete(ctx context.Context, id string) error {
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).DeleteRemoteHook(ctx, id)
}

func (s *service) List(ctx context.Context) ([]*store.RemoteHook, error) {
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).ListRemoteHooks(ctx)
}

func validate(hook *store.RemoteHook) error {
	switch {
	case hook.Name == "":
		return fmt.Errorf("%w: name is required", ErrInvalidHook)
	case hook.EndpointURL == "":
		return fmt.Errorf("%w: endpoint URL is required", ErrInvalidHook)
	case hook.Method == "":
		return fmt.Errorf("%w: HTTP method is required", ErrInvalidHook)
	case hook.TimeoutMs <= 0:
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidHook)
	}
	return nil
}
