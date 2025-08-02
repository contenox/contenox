package modelservice

import (
	"context"
	"errors"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
)

var ErrInvalidModel = errors.New("invalid model data")

type service struct {
	dbInstance              libdb.DBManager
	immutableEmbedModelName string
}

type Service interface {
	Append(ctx context.Context, model *store.Model) error
	List(ctx context.Context) ([]*store.Model, error)
	Delete(ctx context.Context, modelName string) error
}

func New(db libdb.DBManager, embedModel string) Service {
	return &service{
		dbInstance:              db,
		immutableEmbedModelName: embedModel,
	}
}

func (s *service) Append(ctx context.Context, model *store.Model) error {
	if err := validate(model); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).AppendModel(ctx, model)
}

func (s *service) List(ctx context.Context) ([]*store.Model, error) {
	tx := s.dbInstance.WithoutTransaction()
	return store.New(tx).ListModels(ctx)
}

func (s *service) Delete(ctx context.Context, modelName string) error {
	tx := s.dbInstance.WithoutTransaction()
	if modelName == s.immutableEmbedModelName {
		return fmt.Errorf("immutable model cannot be deleted")
	}
	return store.New(tx).DeleteModel(ctx, modelName)
}

func validate(model *store.Model) error {
	if model.Model == "" {
		return fmt.Errorf("%w: model name is required", ErrInvalidModel)
	}
	return nil
}

func (s *service) GetServiceName() string {
	return "modelservice"
}
