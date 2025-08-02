package tasksrecipes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
	"github.com/contenox/runtime/taskengine"
)

const (
	OpenAIChatChainID   = "openai_chat_chain"
	StandardChatChainID = "chat_chain"
)

func InitializeDefaultChains(ctx context.Context, db libdb.DBManager) error {
	// Create chains with proper IDs
	chains := []*taskengine.ChainDefinition{
		BuildOpenAIChatChain(),
		BuildChatChain(),
		BuildCodeReviewChain(),
	}
	tx, comm, end, err := db.WithTransaction(ctx)
	defer end()
	if err != nil {
		return err
	}
	// Store chains
	for _, chain := range chains {
		var value any
		err := store.New(tx).GetKV(ctx, chain.ID, &value)
		if err != nil && !errors.Is(err, libdb.ErrNotFound) {
			return fmt.Errorf("failed to retrieve chain %s: %v", chain.ID, err)
		}
		if errors.Is(err, libdb.ErrNotFound) {
			if err := SetChainDefinition(ctx, tx, chain); err != nil {
				log.Printf("failed to initialize chain %s: %v", chain.ID, err)
			}
		}
	}
	return comm(ctx)
}

func BuildOpenAIChatChain() *taskengine.ChainDefinition {
	return &taskengine.ChainDefinition{
		ID:          "openai_chat_chain",
		Description: "OpenAI Style chat processing pipeline with hooks",
		Tasks: []taskengine.ChainTask{
			{
				ID:          "convert_openai_to_history",
				Description: "Convert OpenAI request to internal history",
				Type:        taskengine.Hook,
				Hook: &taskengine.HookCall{
					Type: "convert_openai_to_history",
					Args: map[string]string{},
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: "default", Goto: "execute_model_on_messages"},
					},
				},
			},
			{
				ID:            "execute_model_on_messages",
				Description:   "Run inference using selected LLM",
				Type:          taskengine.ModelExecution,
				ExecuteConfig: &taskengine.LLMExecutionConfig{},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: "default", Goto: "convert_history_to_openai"},
					},
				},
			},
			{
				ID:          "convert_history_to_openai",
				Description: "Convert chat history to OpenAI response",
				Type:        taskengine.Hook,
				Hook: &taskengine.HookCall{
					Type: "convert_history_to_openai",
					Args: map[string]string{},
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: "default", Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}
}

type BuildChatChainReq struct {
	PreferredModelNames []string
	Provider            string
}

func BuildChatChain() *taskengine.ChainDefinition {
	return &taskengine.ChainDefinition{
		ID:          "chat_chain",
		Description: "Standard chat processing pipeline with hooks",
		Debug:       true,
		Tasks: []taskengine.ChainTask{
			{
				ID:          "mux_input",
				Description: "Check for commands like /echo",
				Type:        taskengine.ParseTransition,
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "moderate"},
						{
							Operator: "equals",
							When:     "echo",
							Goto:     "echo_message",
						},
						{
							Operator: "equals",
							When:     "help",
							Goto:     "print_help_message",
						},
						{
							Operator: "equals",
							When:     "search",
							Goto:     "search_knowledge",
						},
					},
				},
			},
			{
				ID:             "moderate",
				Description:    "Moderate the input",
				Type:           taskengine.ParseNumber,
				PromptTemplate: "Classify the input as safe (0) or unsafe (10) respond with an numeric value between 0 for safe and 10 for unsafe. Input: {{.input}}",
				InputVar:       "input",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{
							Operator: taskengine.OpGreaterThan,
							When:     "4",
							Goto:     "reject_request",
						},
						{
							Operator: "default",
							Goto:     "do_we_need_context",
						},
					},
					OnFailure: "request_failed",
				},
			},
			{
				ID:             "do_we_need_context",
				Description:    "Add context to the conversation",
				Type:           taskengine.RawString,
				PromptTemplate: "Rate how likely it is that the answer requires access to this internal information respond with an numeric value between (0) not likely and (10) highly likely. Input {{.input}}",
				InputVar:       "input",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{
							Operator: taskengine.OpDefault,
							Goto:     "swap_to_input",
						},
						{
							Operator: taskengine.OpGreaterThan,
							When:     "4",
							Goto:     "search_knowledge",
						},
					},
				},
			},
			{
				ID:       "swap_to_input",
				Type:     taskengine.Noop,
				InputVar: "input",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{
							Operator: taskengine.OpDefault, Goto: "execute_model_on_messages",
							AlertOnMatch: "Test Alert",
						},
					},
				},
			},
			{
				ID:          "echo_message",
				Description: "Echo the message",
				Type:        taskengine.Hook,
				Hook: &taskengine.HookCall{
					Type: "echo",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
			{
				ID:          "search_knowledge",
				Description: "Search knowledge base",
				Type:        taskengine.Hook,
				Hook: &taskengine.HookCall{
					Type: "search_knowledge",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "append_search_results"},
					},
				},
			},
			{
				ID:             "append_search_results",
				Type:           taskengine.Noop,
				PromptTemplate: "here are the found search results for the requested document recap them for the user: {{.search_knowledge}}",
				Compose: &taskengine.ComposeTask{
					WithVar:  "input",
					Strategy: "append_string_to_chat_history",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{
							Operator: taskengine.OpDefault,
							Goto:     "execute_model_on_messages",
						},
					},
				},
			},
			{
				ID:          "print_help_message",
				Description: "Display help message",
				Type:        taskengine.Hook,
				Hook: &taskengine.HookCall{
					Type: "print",
					Args: map[string]string{
						"message": "Available commands:\n/echo <text>\n/help\n/search <query>",
					},
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},

			{
				ID:             "reject_request",
				Description:    "Reject the request",
				Type:           taskengine.RawString,
				PromptTemplate: "respond to the user that request was rejected because the input was flagged: {{.input}}",
				InputVar:       "input",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "raise_error"},
					},
				},
			},
			{
				ID:             "request_failed",
				Description:    "Reject the request",
				Type:           taskengine.RawString,
				PromptTemplate: "respond to the user that classification of the request failed for context the exact input: {{.input}}",
				InputVar:       "input",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "raise_error"},
					},
				},
			},
			{
				ID:             "raise_error",
				Description:    "Raise an error",
				Type:           taskengine.RaiseError,
				PromptTemplate: "Error processing: {{.input}}",
				InputVar:       "input",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
			{
				ID:          "execute_model_on_messages",
				Description: "Run inference using selected LLM",
				Type:        taskengine.ModelExecution,
				SystemInstruction: "You're a helpful assistant in the contenox system. " +
					"Respond helpfully and mention available commands (/help, /echo, /search) when appropriate. " +
					"Keep conversation friendly.",
				ExecuteConfig: &taskengine.LLMExecutionConfig{},
				// InputVar: "append_user_message",
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}
}

func BuildCodeReviewChain() *taskengine.ChainDefinition {
	return &taskengine.ChainDefinition{
		ID:          "code_review_chain",
		Description: "Specialized chain for GitHub code reviews",
		Debug:       true,
		Tasks: []taskengine.ChainTask{
			{
				ID:                "execute_code_review",
				Description:       "Run inference using selected LLM for code review",
				SystemInstruction: "You are an expert code reviewer. Analyze the provided code changes and provide structured feedback with actionable suggestions.",
				Type:              taskengine.ModelExecution,
				ExecuteConfig: &taskengine.LLMExecutionConfig{
					Models:    []string{"gemini-2.5-flash"},
					Providers: []string{"gemini"},
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}
}

func BuildAppendInstruction(subjectID string) *taskengine.ChainDefinition {
	return &taskengine.ChainDefinition{
		Tasks: []taskengine.ChainTask{
			{
				ID:          "append_system_message",
				Description: "Append instruction message to chat history",
				Type:        taskengine.Hook,
				Hook: &taskengine.HookCall{
					Type: "append_system_message",
					Args: map[string]string{
						"subject_id": subjectID,
					},
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: "default", Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}
}

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
