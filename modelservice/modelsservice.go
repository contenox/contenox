package modelservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtimetypes"
)

var ErrInvalidModel = errors.New("invalid model data")

type service struct {
	dbInstance              libdb.DBManager
	immutableEmbedModelName string
}

type Service interface {
	Append(ctx context.Context, model *runtimetypes.Model) error
	List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Model, error)
	Delete(ctx context.Context, modelName string) error
}

func New(db libdb.DBManager, embedModel string) Service {
	return &service{
		dbInstance:              db,
		immutableEmbedModelName: embedModel,
	}
}

func (s *service) Append(ctx context.Context, model *runtimetypes.Model) error {

	if err := validate(model); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)
	count, err := storeInstance.EstimateModelCount(ctx)
	if err != nil {
		return err
	}
	err = storeInstance.EnforceMaxRowCount(ctx, count)
	if err != nil {
		return err
	}
	return storeInstance.AppendModel(ctx, model)
}

func (s *service) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Model, error) {
	tx := s.dbInstance.WithoutTransaction()
	return runtimetypes.New(tx).ListModels(ctx, createdAtCursor, limit)
}

func (s *service) Delete(ctx context.Context, modelName string) error {
	tx := s.dbInstance.WithoutTransaction()
	if modelName == s.immutableEmbedModelName {
		return apiframework.ErrImmutableModel
	}
	return runtimetypes.New(tx).DeleteModel(ctx, modelName)
}

func validate(model *runtimetypes.Model) error {
	if model.Model == "" {
		return fmt.Errorf("%w: model name is required", ErrInvalidModel)
	}
	return nil
}

func (s *service) GetServiceName() string {
	return "modelservice"
}
