package poolservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/llmrepo"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/google/uuid"
)

var (
	ErrInvalidPool = errors.New("invalid pool data")
	ErrNotFound    = libdb.ErrNotFound
)

type service struct {
	dbInstance libdb.DBManager
}

func New(db libdb.DBManager) Service {
	return &service{dbInstance: db}
}

type Service interface {
	Create(ctx context.Context, pool *runtimetypes.Pool) error
	GetByID(ctx context.Context, id string) (*runtimetypes.Pool, error)
	GetByName(ctx context.Context, name string) (*runtimetypes.Pool, error)
	Update(ctx context.Context, pool *runtimetypes.Pool) error
	Delete(ctx context.Context, id string) error
	ListAll(ctx context.Context) ([]*runtimetypes.Pool, error)
	ListByPurpose(ctx context.Context, purpose string, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Pool, error)
	AssignBackend(ctx context.Context, poolID, backendID string) error
	RemoveBackend(ctx context.Context, poolID, backendID string) error
	ListBackends(ctx context.Context, poolID string) ([]*runtimetypes.Backend, error)
	ListPoolsForBackend(ctx context.Context, backendID string) ([]*runtimetypes.Pool, error)
	AssignModel(ctx context.Context, poolID, modelID string) error
	RemoveModel(ctx context.Context, poolID, modelID string) error
	ListModels(ctx context.Context, poolID string) ([]*runtimetypes.Model, error)
	ListPoolsForModel(ctx context.Context, modelID string) ([]*runtimetypes.Pool, error)
}

func (s *service) Create(ctx context.Context, pool *runtimetypes.Pool) error {
	pool.ID = uuid.New().String()
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)
	count, err := storeInstance.EstimatePoolCount(ctx)
	if err != nil {
		return err
	}
	err = storeInstance.EnforceMaxRowCount(ctx, count)
	if err != nil {
		return err
	}
	return storeInstance.CreatePool(ctx, pool)
}

func (s *service) GetByID(ctx context.Context, id string) (*runtimetypes.Pool, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).GetPool(ctx, id)
}

func (s *service) GetByName(ctx context.Context, name string) (*runtimetypes.Pool, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).GetPoolByName(ctx, name)
}

func (s *service) Update(ctx context.Context, pool *runtimetypes.Pool) error {
	if pool.ID == llmrepo.EmbedPoolID {
		return fmt.Errorf("pool %s is immutable", pool.ID)
	}
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).UpdatePool(ctx, pool)
}

func (s *service) Delete(ctx context.Context, id string) error {
	if id == llmrepo.EmbedPoolID {
		return fmt.Errorf("pool %s is immutable", id)
	}
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).DeletePool(ctx, id)
}

func (s *service) ListAll(ctx context.Context) ([]*runtimetypes.Pool, error) {
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)

	var allPools []*runtimetypes.Pool
	var lastCursor *time.Time
	limit := 100 // A reasonable page size

	for {
		page, err := storeInstance.ListPools(ctx, lastCursor, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to list pools: %w", err)
		}

		allPools = append(allPools, page...)

		if len(page) < limit {
			break // No more pages
		}

		lastCursor = &page[len(page)-1].CreatedAt
	}

	return allPools, nil
}

func (s *service) ListByPurpose(ctx context.Context, purpose string, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Pool, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListPoolsByPurpose(ctx, purpose, createdAtCursor, limit)
}

func (s *service) AssignBackend(ctx context.Context, poolID, backendID string) error {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).AssignBackendToPool(ctx, poolID, backendID)
}

func (s *service) RemoveBackend(ctx context.Context, poolID, backendID string) error {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).RemoveBackendFromPool(ctx, poolID, backendID)
}

func (s *service) ListBackends(ctx context.Context, poolID string) ([]*runtimetypes.Backend, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListBackendsForPool(ctx, poolID)
}

func (s *service) ListPoolsForBackend(ctx context.Context, backendID string) ([]*runtimetypes.Pool, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListPoolsForBackend(ctx, backendID)
}

func (s *service) AssignModel(ctx context.Context, poolID, modelID string) error {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).AssignModelToPool(ctx, poolID, modelID)
}

func (s *service) RemoveModel(ctx context.Context, poolID, modelID string) error {
	if poolID == llmrepo.EmbedPoolID {
		return apiframework.ErrImmutablePool
	}
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).RemoveModelFromPool(ctx, poolID, modelID)
}

func (s *service) ListModels(ctx context.Context, poolID string) ([]*runtimetypes.Model, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListModelsForPool(ctx, poolID)
}

func (s *service) ListPoolsForModel(ctx context.Context, modelID string) ([]*runtimetypes.Pool, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListPoolsForModel(ctx, modelID)
}
