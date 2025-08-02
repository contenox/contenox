package chainservice

import (
	"context"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/taskengine"
	"github.com/contenox/runtime/tasksrecipes"
)

type Service interface {
	Set(ctx context.Context, chain *taskengine.ChainDefinition) error
	Get(ctx context.Context, id string) (*taskengine.ChainDefinition, error)
	Update(ctx context.Context, chain *taskengine.ChainDefinition) error
	List(ctx context.Context) ([]*taskengine.ChainDefinition, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	dbInstance libdb.DBManager
}

func New(db libdb.DBManager) Service {
	return &service{
		dbInstance: db,
	}
}

func (s *service) Set(ctx context.Context, chain *taskengine.ChainDefinition) error {
	if err := validateChain(chain); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()

	return tasksrecipes.SetChainDefinition(ctx, tx, chain)
}

func (s *service) Get(ctx context.Context, id string) (*taskengine.ChainDefinition, error) {
	tx := s.dbInstance.WithoutTransaction()

	return tasksrecipes.GetChainDefinition(ctx, tx, id)
}

func (s *service) Update(ctx context.Context, chain *taskengine.ChainDefinition) error {
	if err := validateChain(chain); err != nil {
		return err
	}
	tx := s.dbInstance.WithoutTransaction()

	return tasksrecipes.UpdateChainDefinition(ctx, tx, chain)
}

func (s *service) List(ctx context.Context) ([]*taskengine.ChainDefinition, error) {
	tx := s.dbInstance.WithoutTransaction()
	return tasksrecipes.ListChainDefinitions(ctx, tx)
}

func (s *service) Delete(ctx context.Context, id string) error {
	tx := s.dbInstance.WithoutTransaction()

	return tasksrecipes.DeleteChainDefinition(ctx, tx, id)
}

func validateChain(chain *taskengine.ChainDefinition) error {
	if chain.ID == "" {
		return fmt.Errorf("chain ID is required")
	}
	if len(chain.Tasks) == 0 {
		return fmt.Errorf("chain must have at least one task")
	}
	return nil
}
