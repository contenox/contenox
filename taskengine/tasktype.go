package taskengine

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// TaskHandler defines the expected output format of a task.
// It determines how the LLM's response will be interpreted.
type TaskHandler string

const (
	// HandleConditionKey interprets the response as a condition key,
	// used to determine which transition to follow.
	HandleConditionKey TaskHandler = "condition_key"

	// HandleParseNumber expects a numeric response and parses it into an integer.
	HandleParseNumber TaskHandler = "parse_number"

	// HandleParseScore expects a floating-point score (e.g., quality rating).
	HandleParseScore TaskHandler = "parse_score"

	// HandleParseRange expects a numeric range like "5-7", or defaults to N-N for single numbers.
	HandleParseRange TaskHandler = "parse_range"

	// HandleRawString returns the raw string result from the LLM.
	HandleRawString TaskHandler = "raw_string"

	HandleEmbedding TaskHandler = "embedding"

	// HandleRaiseError raises an error with the provided message.
	HandleRaiseError TaskHandler = "raise_error"

	// HandleModelExecution will execute the system default or specified model on a chathistory.
	HandleModelExecution TaskHandler = "model_execution"

	// HandleParseTransition will attempt to parse a transition command from the input and strip the transition prefix if it exists.
	HandleParseTransition TaskHandler = "parse_transition"

	HandleNoop TaskHandler = "noop"

	// HandleHook indicates this task should execute an external action rather than calling the LLM.
	HandleHook TaskHandler = "hook"
)

func (t TaskHandler) String() string {
	return string(t)
}

func (d DataType) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d DataType) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(d.String())
}

// TriggerType defines the type of trigger that starts a chain.
type TriggerType string

const (
	// TriggerManual means the chain must be started manually via API or UI.
	TriggerManual TriggerType = "manual"

	// TriggerKeyword starts the chain if input matches a specific keyword.
	TriggerKeyword TriggerType = "keyword"

	// TriggerSemantic starts the chain based on semantic similarity (e.g., embeddings).
	TriggerSemantic TriggerType = "embedding"

	// TriggerEvent starts the chain in response to an external event or webhook.
	TriggerEvent TriggerType = "webhook"
)

func (t TriggerType) String() string {
	return string(t)
}

// Trigger defines how and when a chain should be started.
type Trigger struct {
	// Type specifies the trigger mode (manual, keyword, etc.).
	Type TriggerType `yaml:"type" json:"type"`

	// Description is a human-readable explanation of the trigger.
	Description string `yaml:"description" json:"description"`

	// Pattern is used for matching input in keyword or event triggers.
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
}

// TaskTransition defines what happens after a task completes,
// including which task to go to next and how to handle errors.
type TaskTransition struct {
	// OnFailure is the task ID to jump to in case of failure.
	OnFailure string `yaml:"on_failure" json:"on_failure"`

	// OnFailureAlert specifies the alert message to send if the task fails.
	OnFailureAlert string `yaml:"on_failure_alert" json:"on_failure_alert"`

	// Branches defines conditional branches for successful task completion.
	Branches []TransitionBranch `yaml:"branches" json:"branches"`
}

// TransitionBranch defines a single possible path in the workflow,
// selected when the task's output matches the specified condition.
type TransitionBranch struct {
	// Operator defines how to compare the task's output to When.
	Operator OperatorTerm `yaml:"operator,omitempty" json:"operator,omitempty"`

	// When specifies the condition that must be met to follow this branch.
	// Format depends on the task type:
	// - For condition_key: exact string match
	// - For parse_number: numeric comparison (using Operator)
	When string `yaml:"when" json:"when"`

	// Goto specifies the target task ID if this branch is taken.
	// Leave empty or use taskengine.TermEnd to end the chain.
	Goto string `yaml:"goto" json:"goto"`

	// AlertOnMatch specifies the alert message to send if this branch is taken.
	AlertOnMatch string `yaml:"alert_on_match" json:"alert_on_match"`
}

// OperatorTerm represents logical operators used for task transition evaluation
type OperatorTerm string

const (
	OpEquals      OperatorTerm = "equals"
	OpContains    OperatorTerm = "contains"
	OpStartsWith  OperatorTerm = "starts_with"
	OpEndsWith    OperatorTerm = "ends_with"
	OpGreaterThan OperatorTerm = ">"
	OpGt          OperatorTerm = "gt"
	OpLessThan    OperatorTerm = "<"
	OpLt          OperatorTerm = "lt"
	OpInRange     OperatorTerm = "in_range"
	OpDefault     OperatorTerm = "default"
)

func (t OperatorTerm) String() string {
	return string(t)
}

func SupportedOperators() []string {
	return []string{
		string(OpEquals),
		string(OpContains),
		string(OpStartsWith),
		string(OpEndsWith),
		string(OpGreaterThan),
		string(OpGt),
		string(OpLessThan),
		string(OpLt),
		string(OpInRange),
		string(OpDefault),
	}
}

func ToOperatorTerm(s string) (OperatorTerm, error) {
	switch s {
	case string(OpEquals):
		return OpEquals, nil
	case string(OpContains):
		return OpContains, nil
	case string(OpStartsWith):
		return OpStartsWith, nil
	case string(OpEndsWith):
		return OpEndsWith, nil
	case string(OpGreaterThan):
		return OpGreaterThan, nil
	case string(OpGt):
		return OpGt, nil
	case string(OpLessThan):
		return OpLessThan, nil
	case string(OpLt):
		return OpLt, nil
	case string(OpInRange):
		return OpInRange, nil
	case string(OpDefault):
		return OpDefault, nil
	default:
		return "", fmt.Errorf("unsupported operator: %s", s)
	}
}

type LLMExecutionConfig struct {
	Model     string   `yaml:"model" json:"model"`
	Models    []string `yaml:"models,omitempty" json:"models,omitempty"`
	Provider  string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	Providers []string `yaml:"providers,omitempty" json:"providers,omitempty"`
}

type MessageConfig struct {
	Role    string `yaml:"role" json:"role"` // user/system/assistant
	Content string `yaml:"content,omitempty" json:"content,omitempty"`
}

// HookCall represents an external integration or side-effect triggered during a task.
// Hooks allow tasks to interact with external systems (e.g., "send_email", "update_db").
type HookCall struct {
	// Name is the registered hook name to invoke (e.g., "send_email").
	Name string `yaml:"name" json:"name"`

	// Args are key-value pairs to parameterize the hook call.
	// Example: {"to": "user@example.com", "subject": "Notification"}
	Args map[string]string `yaml:"args" json:"args"`
}

// ChainTask represents a single step in a workflow.
// Each task has a handler that dictates how its prompt will be processed.
//
// Field validity by task type:
// | Field               | ConditionKey | ParseNumber | ParseScore | ParseRange | RawString | Hook  | Noop  |
// |---------------------|--------------|-------------|------------|------------|-----------|-------|-------|
// | ValidConditions     | Required     | -           | -          | -          | -         | -     | -     |
// | Hook                | -            | -           | -          | -          | -         | Req   | -     |
// | PromptTemplate      | Required     | Required    | Required   | Required   | Required  | -     | Opt   |
// | Print               | Optional     | Optional    | Optional   | Optional   | Optional  | Opt   | Opt   |
// | ExecuteConfig       | Optional     | Optional    | Optional   | Optional   | Optional  | -     | -     |
// | InputVar            | Optional     | Optional    | Optional   | Optional   | Optional  | Opt   | Opt   |
// | SystemInstruction   | Optional     | Optional    | Optional   | Optional   | Optional  | Opt   | Opt   |
// | Compose             | Optional     | Optional    | Optional   | Optional   | Optional  | Opt   | Opt   |
// | Transition          | Required     | Required    | Required   | Required   | Required  | Req   | Req   |
type ChainTask struct {
	// ID uniquely identifies the task within the chain.
	ID string `yaml:"id" json:"id"`

	// Description is a human-readable summary of what the task does.
	Description string `yaml:"description" json:"description"`

	// Handler determines how the LLM output (or hook) will be interpreted.
	Handler TaskHandler `yaml:"handler" json:"handler"`

	// SystemInstruction provides additional instructions to the LLM, if applicable system level will be used.
	SystemInstruction string `yaml:"system_instruction,omitempty" json:"system_instruction,omitempty"`

	// ValidConditions defines allowed values for ConditionKey tasks.
	// Required for ConditionKey tasks, ignored for all other types.
	// Example: {"yes": true, "no": true} for a yes/no condition.
	ValidConditions map[string]bool `yaml:"valid_conditions,omitempty" json:"valid_conditions,omitempty"`

	// ExecuteConfig defines the configuration for executing prompt or chat model tasks.
	ExecuteConfig *LLMExecutionConfig `yaml:"execute_config,omitempty" json:"execute_config,omitempty"`

	// Hook defines an external action to run.
	// Required for Hook tasks, must be nil/omitted for all other types.
	// Example: {type: "send_email", args: {"to": "user@example.com"}}
	Hook *HookCall `yaml:"hook,omitempty" json:"hook,omitempty"`

	// Print optionally formats the output for display/logging.
	// Supports template variables from previous task outputs.
	// Optional for all task types except Hook where it's rarely used.
	// Example: "The score is: {{.previous_output}}"
	Print string `yaml:"print,omitempty" json:"print,omitempty"`

	// PromptTemplate is the text prompt sent to the LLM.
	// It's Required and only applicable for the raw_string type.
	// Supports template variables from previous task outputs.
	// Example: "Rate the quality from 1-10: {{.input}}"
	PromptTemplate string `yaml:"prompt_template" json:"prompt_template"`

	// InputVar is the name of the variable to use as input for the task.
	// Example: "input" for the original input.
	// Each task stores its output in a variable named with it's task id.
	InputVar string `yaml:"input_var,omitempty" json:"input_var,omitempty"`

	// Compose merges the specified the output with the withVar side.
	// Optional. compose is applied before the input reaches the task execution,
	Compose *ComposeTask `yaml:"compose,omitempty" json:"compose,omitempty"`

	// Transition defines what to do after this task completes.
	Transition TaskTransition `yaml:"transition" json:"transition"`

	// Timeout optionally sets a timeout for task execution.
	// Format: "10s", "2m", "1h" etc.
	// Optional for all task types.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// RetryOnFailure sets how many times to retry this task on failure.
	// Applies to all task types including Hooks.
	// Default: 0 (no retries)
	RetryOnFailure int `yaml:"retry_on_failure,omitempty" json:"retry_on_failure,omitempty"`
}

// ComposeTask is a task that composes multiple variables into a single output.
// the composed output is stored in a variable named after the task ID with "_composed" suffix.
// and is also directly mutating the task's output.
// example:
//
// compose:
//
//	with_var: "chat2"
//	strategy: "override"
type ComposeTask struct {
	WithVar string `yaml:"with_var,omitempty" json:"with_var,omitempty"`
	// Strategy defines how values should be merged ("override", "merge_chat_histories", "append_string_to_chat_history").
	// Optional; defaults to "override" or "merge_chat_histories" if both output and WithVar values are ChatHistory.
	// "merge_chat_histories": If both output and WithVar values are ChatHistory,
	// appends the WithVar's Messages to the output's Messages.
	Strategy string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

// ChainWithTrigger is a convenience struct that combines triggers and chain definition.
type ChainWithTrigger struct {
	// Triggers defines when the chain should be started.
	Triggers []Trigger `yaml:"triggers,omitempty" json:"triggers,omitempty"`

	// ChainDefinition contains the actual task sequence.
	ChainDefinition
}

type ChainTerms string

const (
	TermEnd = "end"
)

// ChainDefinition describes a sequence of tasks to execute in order,
// along with branching logic, retry policies, and model preferences.
//
// ChainDefinition support dynamic routing based on LLM outputs or conditions,
// and can include hooks to perform external actions (e.g., sending emails).
//
// Example usage:
//
// Define a YAML chain:
//
//	chains/article.yaml:
//	  id: article-generator
//	  description: Generates articles based on topic and length
//	  triggers:
//	    - type: manual
//	      description: Run manually via API
//	  tasks:
//	    - id: get_length
//	      handler: parse_number
//	      prompt_template: "How many words should the article be?"
//	      transition:
//	        branches:
//	          - when: "default"
//	            goto: generate_article
//
//	    - id: generate_article
//	      handler: raw_string
//	      prompt_template: "Write a {{ .get_length }}-word article about {{ .input }}"
//	      print: "Generated article:\n{{ .previous_output }}"
//	      transition:
//	        branches:
//	          - when: "default"
//	            goto: end
//
//	    - id: end
//	      handler: raw_string
//	      prompt_template: "{{ .generate_article }}"
//
// Parse and execute it:
//
//	file, err := os.Open("chains/article.yaml")
//	if err != nil {
//	    log.Fatalf("failed to open YAML: %v", err)
//	}
//	defer file.Close()
//
//	var chainDef taskengine.ChainDefinition
//	if err := yaml.NewDecoder(file).Decode(&chainDef); err != nil {
//	    log.Fatalf("failed to parse YAML: %v", err)
//	}
//
//	exec, _ := taskengine.NewExec(ctx, modelRepo, hookProvider)
//	env, _ := taskengine.NewEnv(ctx, tracker, exec)
//	output, err := env.ExecEnv(ctx, &chainDef, userInput, taskengine.DataTypeString)
//	if err != nil {
//	    log.Fatalf("execution failed: %v", err)
//	}
//
//	fmt.Println("Final output:", output)
type ChainDefinition struct {
	// ID uniquely identifies the chain.
	ID string `yaml:"id" json:"id"`

	// Enables capturing user input and output.
	Debug bool `yaml:"debug" json:"debug"`

	// Description provides a human-readable summary of the chain's purpose.
	Description string `yaml:"description" json:"description"`

	// Tasks is the list of tasks to execute in sequence.
	Tasks []ChainTask `yaml:"tasks" json:"tasks"`

	// TokenLimit is the token limit for the context window (used during execution).
	TokenLimit int64 `yaml:"token_limit" json:"token_limit"`

	// RoutingStrategy defines how transitions should be evaluated (optional).
	RoutingStrategy string `yaml:"routing_strategy" json:"routing_strategy"`
}

type SearchResult struct {
	ID           string  `json:"id"`
	ResourceType string  `json:"type"`
	Distance     float32 `json:"distance"`
}

type ChatHistory struct {
	Messages     []Message `json:"messages"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"inputTokens"`
	OutputTokens int       `json:"outputTokens"`
}

type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type OpenAIChatRequest struct {
	Model            string                     `json:"model"`
	Messages         []OpenAIChatRequestMessage `json:"messages"`
	MaxTokens        int                        `json:"max_tokens,omitempty"`
	Temperature      float64                    `json:"temperature,omitempty"`
	TopP             float64                    `json:"top_p,omitempty"`
	Stop             []string                   `json:"stop,omitempty"`
	N                int                        `json:"n,omitempty"`
	Stream           bool                       `json:"stream,omitempty"`
	PresencePenalty  float64                    `json:"presence_penalty,omitempty"`
	FrequencyPenalty float64                    `json:"frequency_penalty,omitempty"`
	User             string                     `json:"user,omitempty"`
}

type OpenAIChatRequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatResponse struct {
	ID                string                     `json:"id"`
	Object            string                     `json:"object"`
	Created           int64                      `json:"created"`
	Model             string                     `json:"model"`
	Choices           []OpenAIChatResponseChoice `json:"choices"`
	Usage             OpenAITokenUsage           `json:"usage"`
	SystemFingerprint string                     `json:"system_fingerprint,omitempty"`
}

type OpenAIChatResponseChoice struct {
	Index        int                      `json:"index"`
	Message      OpenAIChatRequestMessage `json:"message"`
	FinishReason string                   `json:"finish_reason"`
}

type OpenAITokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
