package tasksrecipes

import (
	"context"
	"encoding/json"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
	"github.com/contenox/runtime/taskengine"
)

const (
	OpenAIChatChainID   = "openai_chat_chain"
	StandardChatChainID = "chat_chain"
)

const ChainKeyPrefix = "chain:"

func SetChainDefinition(ctx context.Context, tx libdb.Exec, chain *taskengine.ChainDefinition) error {
	s := store.New(tx)
	key := ChainKeyPrefix + chain.ID
	data, err := json.Marshal(chain)
	if err != nil {
		return err
	}
	return s.SetKV(ctx, key, data)
}

func UpdateChainDefinition(ctx context.Context, tx libdb.Exec, chain *taskengine.ChainDefinition) error {
	s := store.New(tx)
	key := ChainKeyPrefix + chain.ID
	data, err := json.Marshal(chain)
	if err != nil {
		return err
	}
	return s.UpdateKV(ctx, key, data)
}

func GetChainDefinition(ctx context.Context, tx libdb.Exec, id string) (*taskengine.ChainDefinition, error) {
	s := store.New(tx)
	key := ChainKeyPrefix + id
	var chain taskengine.ChainDefinition
	if err := s.GetKV(ctx, key, &chain); err != nil {
		return nil, err
	}
	return &chain, nil
}

func ListChainDefinitions(ctx context.Context, tx libdb.Exec) ([]*taskengine.ChainDefinition, error) {
	s := store.New(tx)
	kvs, err := s.ListKVPrefix(ctx, ChainKeyPrefix)
	if err != nil {
		return nil, err
	}

	chains := make([]*taskengine.ChainDefinition, 0, len(kvs))
	for _, kv := range kvs {
		var chain taskengine.ChainDefinition
		if err := json.Unmarshal(kv.Value, &chain); err != nil {
			return nil, err
		}
		chains = append(chains, &chain)
	}
	return chains, nil
}

func DeleteChainDefinition(ctx context.Context, tx libdb.Exec, id string) error {
	s := store.New(tx)
	key := ChainKeyPrefix + id
	return s.DeleteKV(ctx, key)
}
