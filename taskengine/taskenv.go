package taskengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"dario.cat/mergo"
	"github.com/contenox/runtime/internal/apiframework"
	"github.com/contenox/runtime/libtracker"
)

// DataType represents the type of data passed between tasks.
// All types support JSON/YAML marshaling and unmarshaling.
type DataType int

const (
	DataTypeAny                DataType = iota // Any type (use sparingly, loses type safety)
	DataTypeString                             // String data
	DataTypeBool                               // Boolean value
	DataTypeInt                                // Integer number
	DataTypeFloat                              // Floating-point number
	DataTypeVector                             // Embedding vector ([]float64)
	DataTypeSearchResults                      // Search results array
	DataTypeJSON                               // Generic JSON data
	DataTypeChatHistory                        // Chat conversation history
	DataTypeOpenAIChat                         // OpenAI chat request format
	DataTypeOpenAIChatResponse                 // OpenAI chat response format
)

// String returns the string representation of the data type.
func (d *DataType) String() string {
	switch *d {
	case DataTypeAny:
		return "any"
	case DataTypeString:
		return "string"
	case DataTypeBool:
		return "bool"
	case DataTypeInt:
		return "int"
	case DataTypeFloat:
		return "float"
	case DataTypeVector:
		return "vector"
	case DataTypeSearchResults:
		return "search_results"
	case DataTypeJSON:
		return "json"
	case DataTypeChatHistory:
		return "chat_history"
	case DataTypeOpenAIChat:
		return "openai_chat"
	case DataTypeOpenAIChatResponse:
		return "openai_chat_response"
	default:
		return "unknown"
	}
}

// DataTypeFromString converts a string to DataType, returns error for unknown types.
func DataTypeFromString(s string) (DataType, error) {
	switch strings.ToLower(s) {
	case "any":
		return DataTypeAny, nil
	case "string":
		return DataTypeString, nil
	case "bool":
		return DataTypeBool, nil
	case "int":
		return DataTypeInt, nil
	case "float":
		return DataTypeFloat, nil
	case "vector":
		return DataTypeVector, nil
	case "search_results":
		return DataTypeSearchResults, nil
	case "json":
		return DataTypeJSON, nil
	case "chat_history":
		return DataTypeChatHistory, nil
	case "openai_chat":
		return DataTypeOpenAIChat, nil
	case "openai_chat_response":
		return DataTypeOpenAIChatResponse, nil
	default:
		return DataTypeAny, fmt.Errorf("unknown data type: %s", s)
	}
}

// EnvExecutor executes complete task chains with input and environment management.
type EnvExecutor interface {
	// ExecEnv executes a task chain with the given input and data type.
	// Returns final output, output type, execution history, and error.
	ExecEnv(ctx context.Context, chain *TaskChainDefinition, input any, dataType DataType) (any, DataType, []CapturedStateUnit, error)
}

// ErrUnsupportedTaskType indicates unrecognized task type
var ErrUnsupportedTaskType = errors.New("executor does not support the task type")

// HookRepo defines interface for external system integrations and side effects.
type HookRepo interface {
	// Exec executes a hook with the given input and arguments.
	// Returns output, output type, transition value, and error.
	Exec(ctx context.Context, startingTime time.Time, input any, dataType DataType, transition string, args *HookCall) (any, DataType, string, error)
	// HookRegistry provides hook discovery functionality.
	HookRegistry
}

type HookRegistry interface {
	Supports(ctx context.Context) ([]string, error)
}

// SimpleEnv is the default implementation of EnvExecutor.
// this is the default EnvExecutor implementation
// It executes tasks in order, using retry and timeout policies, and tracks execution
// progress using an ActivityTracker.
type SimpleEnv struct {
	exec      TaskExecutor
	tracker   libtracker.ActivityTracker
	inspector Inspector
}

// NewEnv creates a new SimpleEnv with the given tracker and task executor.
func NewEnv(
	_ context.Context,
	tracker libtracker.ActivityTracker,
	exec TaskExecutor,
	inspector Inspector,
) (EnvExecutor, error) {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &SimpleEnv{
		exec:      exec,
		tracker:   tracker,
		inspector: inspector,
	}, nil
}

// ExecEnv executes the given chain with the provided input.
//
// It manages the full lifecycle of task execution: rendering prompts, calling the
// TaskExecutor, handling timeouts, retries, transitions, and collecting final output.
func (exe SimpleEnv) ExecEnv(ctx context.Context, chain *TaskChainDefinition, input any, dataType DataType) (any, DataType, []CapturedStateUnit, error) {
	stack := exe.inspector.Start(ctx)

	vars := map[string]any{
		"input": input,
	}
	varTypes := map[string]DataType{"input": dataType}
	startingTime := time.Now().UTC()
	var err error

	if err := validateChain(chain.Tasks); err != nil {
		return nil, DataTypeAny, stack.GetExecutionHistory(), err
	}

	currentTask, err := findTaskByID(chain.Tasks, chain.Tasks[0].ID)
	if err != nil {
		return nil, DataTypeAny, stack.GetExecutionHistory(), err
	}

	var finalOutput any
	var transitionEval string
	var output any = input
	var outputType DataType = dataType
	var taskErr error

	for {
		if ctx.Err() != nil {
			return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: context canceled", currentTask.ID)
		}

		// Determine task input
		taskInput := output
		taskInputType := outputType
		if currentTask.InputVar != "" {
			var ok bool
			taskInput, ok = vars[currentTask.InputVar]
			if !ok {
				return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: input variable %q not found", currentTask.ID, currentTask.InputVar)
			}
			taskInputType, ok = varTypes[currentTask.InputVar]
			if !ok {
				return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: input variable %q missing type info", currentTask.ID, currentTask.InputVar)
			}
		}

		// Render prompt template if exists
		if currentTask.PromptTemplate != "" {
			rendered, err := renderTemplate(currentTask.PromptTemplate, vars)
			if err != nil {
				return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: template error: %v", currentTask.ID, err)
			}
			taskInput = rendered
			taskInputType = DataTypeString
		}
		maxRetries := max(currentTask.RetryOnFailure, 0)

	retryLoop:
		for retry := 0; retry <= maxRetries; retry++ {
			// Note: Return on breakpoint for now
			if stack.HasBreakpoint(currentTask.ID) {
				return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: breakpoint set", currentTask.ID)
			}

			// Track task attempt start
			taskCtx := context.Background()
			taskCtx = libtracker.CopyTrackingValues(ctx, taskCtx)
			var cancel context.CancelFunc
			if currentTask.Timeout != "" {
				timeout, err := time.ParseDuration(currentTask.Timeout)
				if err != nil {
					return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: invalid timeout: %v", currentTask.ID, err)
				}
				taskCtx, cancel = context.WithTimeout(ctx, timeout)
			}
			reportErrAttempt, reportChangeAttempt, endAttempt := exe.tracker.Start(
				taskCtx,
				"task_attempt",
				currentTask.ID,
				"retry", retry,
				"task_type", currentTask.Handler,
			)

			startTime := time.Now().UTC()

			output, outputType, transitionEval, taskErr = exe.exec.TaskExec(taskCtx, startingTime, int(chain.TokenLimit), currentTask, taskInput, taskInputType)
			if taskErr != nil {
				taskErr = fmt.Errorf("task %s: %w", currentTask.ID, taskErr)
				reportErrAttempt(taskErr)
			}
			endAttempt()
			if cancel != nil {
				cancel()
			}
			duration := time.Since(startTime)
			errState := ErrorResponse{
				ErrorInternal: taskErr,
			}
			if taskErr != nil {
				errState.Error = taskErr.Error()
			}
			// Record execution step
			step := CapturedStateUnit{
				TaskID:      currentTask.ID,
				TaskHandler: currentTask.Handler.String(),
				InputType:   taskInputType,
				OutputType:  outputType,
				Transition:  transitionEval,
				Duration:    duration,
				Error:       errState,
			}
			if chain.Debug {
				step.Input = fmt.Sprintf("%v", taskInput)
				step.Output = fmt.Sprintf("%v", output)
			}
			stack.RecordStep(step)

			if taskErr != nil {
				reportErrAttempt(taskErr)
				continue retryLoop
			}

			// Handle compose statement
			if currentTask.Compose != nil {
				compose := currentTask.Compose

				// Fetch right value
				rightVal, ok := vars[compose.WithVar]
				if !ok {
					return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("compose right_var %q not found", compose.WithVar)
				}

				// Determine strategy
				strategy := compose.Strategy
				if strategy == "" {
					// Automatic strategy selection based on types
					if dataType == DataTypeChatHistory && varTypes[compose.WithVar] == DataTypeChatHistory {
						strategy = "merge_chat_histories"
					} else {
						strategy = "override"
					}
				}

				// Clone current output to avoid mutation during merge
				merged := output

				// Merge based on strategy
				switch strategy {
				case "override":
					baseMap, isBaseMap := output.(map[string]any)
					overridesMap, isOverridesMap := rightVal.(map[string]any)
					if !isBaseMap || !isOverridesMap {
						return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("invalid types for override")
					}
					if err := mergo.Merge(&baseMap, overridesMap, mergo.WithOverride); err != nil {
						return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("merge failed (override): %w", err)
					}
					merged = baseMap
				case "append_string_to_chat_history":
					leftCH, leftIsCH := output.(string)
					rightCH, rightIsCH := rightVal.(ChatHistory)
					if !leftIsCH || !rightIsCH {
						leftCH, leftIsCH = rightVal.(string)
						rightCH, rightIsCH = output.(ChatHistory)
					}
					if !leftIsCH || !rightIsCH {
						return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("compose strategy 'append_string_to_chat_history' requires both left and right values to be either string or ChatHistory")
					}
					merged = ChatHistory{
						Messages: append([]Message{
							{
								Content:   leftCH,
								Role:      "system",
								Timestamp: time.Now().UTC(),
							},
						}, rightCH.Messages...),
						Model:        rightCH.Model,
						OutputTokens: 0,
						InputTokens:  0, // should be recalculated.
					}
				case "merge_chat_histories":
					var leftReq OpenAIChatRequest
					var rightReq OpenAIChatRequest
					inputTokenCountLeft := 0
					outputTokenCountLeft := 0
					inputTokenCountRight := 0
					outputTokenCountRight := 0
					leftWasHistory := false
					leftModel := ""
					rightModel := ""
					if outputType == DataTypeChatHistory {
						leftWasHistory = true
						chatHistory, ok := output.(ChatHistory)
						if !ok {
							err := fmt.Errorf("unexpected output type")
							return nil, DataTypeAny, stack.GetExecutionHistory(), err
						}
						inputTokenCountLeft = chatHistory.InputTokens
						outputTokenCountLeft = chatHistory.OutputTokens
						leftModel = chatHistory.Model
						leftReq, _, _ = ConvertChatHistoryToOpenAIRequest(chatHistory)
					}
					rightType := varTypes[compose.WithVar]
					rightWasHistory := false
					if rightType == DataTypeChatHistory {
						rightWasHistory = true
						chatHistory, ok := rightVal.(ChatHistory)
						if !ok {
							err := fmt.Errorf("unexpected compose.WithVar type")
							return nil, DataTypeAny, stack.GetExecutionHistory(), err
						}
						rightModel = rightReq.Model
						inputTokenCountRight = chatHistory.InputTokens
						outputTokenCountRight = chatHistory.OutputTokens
						rightReq, _, _ = ConvertChatHistoryToOpenAIRequest(chatHistory)
					}

					mergedReq := leftReq
					if err := mergo.Merge(&mergedReq, rightReq, mergo.WithOverride); err != nil {
						return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("failed to merge request parameters: %w", err)
					}
					mergedReq.Messages = append(rightReq.Messages, leftReq.Messages...)
					if rightModel != leftModel {
						mergedReq.Model = ""
					}
					merged = mergedReq

					outputType = DataTypeOpenAIChat
					if leftWasHistory && rightWasHistory {
						history, _, _ := ConvertOpenAIToChatHistory(mergedReq)
						history.InputTokens = inputTokenCountLeft + inputTokenCountRight
						history.OutputTokens = outputTokenCountLeft + outputTokenCountRight
						merged = history
						outputType = DataTypeChatHistory
					}
				default:
					return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("unsupported compose strategy: %q", strategy)
				}

				// Update task output to composed value
				output = merged
				var composedVarType DataType
				if dataType == varTypes[compose.WithVar] {
					composedVarType = dataType
				} else {
					// Types differ (e.g., ChatHistory + String), result type is ambiguous.
					composedVarType = DataTypeAny
				}
				// Store in new variable
				outputVarName := currentTask.ID + "_composed"
				vars[outputVarName] = output
				varTypes[outputVarName] = composedVarType
				outputType = composedVarType
			}

			// Report successful attempt
			reportChangeAttempt(currentTask.ID, output)
			break retryLoop
		}

		if taskErr != nil {
			if currentTask.Transition.OnFailure != "" {
				previousTaskID := currentTask.ID
				currentTask, err = findTaskByID(chain.Tasks, currentTask.Transition.OnFailure)
				if err != nil {
					return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("error transition target not found: %v", err)
				}
				// Track error-based transition
				_, reportChangeErrTransition, endErrTransition := exe.tracker.Start(
					ctx,
					"next_task",
					previousTaskID,
					"next_task", currentTask.ID,
					"reason", "error",
				)
				defer endErrTransition()
				reportChangeErrTransition(currentTask.ID, taskErr)
				continue
			}
			return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s failed after %d retries: %v", currentTask.ID, maxRetries, taskErr)
		}

		// Update execution variables
		vars["previous_output"] = output
		vars[currentTask.ID] = output
		varTypes["previous_output"] = outputType
		varTypes[currentTask.ID] = outputType

		// Handle print statement
		if currentTask.Print != "" {
			printMsg, err := renderTemplate(currentTask.Print, vars)
			if err != nil {
				return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: print template error: %v", currentTask.ID, err)
			}
			fmt.Println(printMsg)
		}

		// Evaluate transitions
		nextTaskID, err := exe.evaluateTransitions(ctx, currentTask.ID, currentTask.Transition, transitionEval)
		if err != nil {
			return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("task %s: transition error: %v", currentTask.ID, err)
		}

		if nextTaskID == "" || nextTaskID == TermEnd {
			finalOutput = output
			// Track final output
			_, reportChangeFinal, endFinal := exe.tracker.Start(
				ctx,
				"chain_complete",
				"chain")
			defer endFinal()
			reportChangeFinal("chain", finalOutput)
			break
		}

		// Track normal transition to next task
		_, reportChangeTransition, endTransition := exe.tracker.Start(
			ctx,
			"next_task",
			currentTask.ID,
			"next_task", nextTaskID,
		)
		defer endTransition()
		reportChangeTransition(nextTaskID, transitionEval)

		// Find next task
		currentTask, err = findTaskByID(chain.Tasks, nextTaskID)
		if err != nil {
			return nil, DataTypeAny, stack.GetExecutionHistory(), fmt.Errorf("next task %s not found: %v", nextTaskID, err)
		}
	}

	return finalOutput, outputType, stack.GetExecutionHistory(), nil
}

func renderTemplate(tmplStr string, vars map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (exe SimpleEnv) evaluateTransitions(ctx context.Context, taskID string, transition TaskTransition, eval string) (string, error) {
	// First check explicit matches
	for _, ct := range transition.Branches {
		if ct.Operator == OpDefault {
			continue
		}

		match, err := compare(ct.Operator, eval, ct.When)
		if err != nil {
			return "", err
		}
		if match {
			return ct.Goto, nil
		}
	}

	// Then check for default
	for _, ct := range transition.Branches {
		if ct.Operator == "default" {
			return ct.Goto, nil
		}
	}

	return "", fmt.Errorf("no matching transition found for eval: %s", eval)
}

// parseNumber attempts to parse a string as either an integer or float.
func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, " ", "")

	// Handle empty string
	if s == "" {
		return 0, fmt.Errorf("cannot parse number from empty string")
	}

	// If it looks like a float or int, parse it
	if num, err := strconv.ParseFloat(s, 64); err == nil {
		return num, nil
	}

	// Try to extract first number from the string (in case of garbage prefix/suffix)
	re := regexp.MustCompile(`[-+]?\d*\.?\d+`)
	match := re.FindString(s)
	if match == "" {
		return 0, fmt.Errorf("no valid number found in %q", s)
	}

	num, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse number from %q (extracted %q): %w", s, match, err)
	}
	return num, nil
}

// compare applies a logical operator to a model response and a target value.
//
// Supported operators include equality, string containment, numeric comparisons,
// and range checks using "parse_range".
func compare(operator OperatorTerm, response, when string) (bool, error) {
	switch operator {
	case OpEquals:
		return response == when, nil
	case OpContains:
		return strings.Contains(response, when), nil
	case OpStartsWith:
		return strings.HasPrefix(response, when), nil
	case OpEndsWith:
		return strings.HasSuffix(response, when), nil
	case OpGreaterThan, OpGt:
		resNum, err := parseNumber(response)
		if err != nil {
			return false, err
		}
		targetNum, err := parseNumber(when)
		if err != nil {
			return false, err
		}
		return resNum > targetNum, nil
	case OpLessThan, OpLt:
		resNum, err := parseNumber(response)
		if err != nil {
			return false, err
		}
		targetNum, err := parseNumber(when)
		if err != nil {
			return false, err
		}
		return resNum < targetNum, nil
	case OpInRange:
		parts := strings.Split(when, "-")
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid inrange format: %s (expected 'min-max')", when)
		}

		lower, err := parseNumber(strings.TrimSpace(parts[0]))
		if err != nil {
			return false, fmt.Errorf("invalid lower bound in range %q: %w", when, err)
		}

		upper, err := parseNumber(strings.TrimSpace(parts[1]))
		if err != nil {
			return false, fmt.Errorf("invalid upper bound in range %q: %w", when, err)
		}

		if lower > upper {
			return false, fmt.Errorf("invalid range: lower bound %f > upper bound %f", lower, upper)
		}

		resNum, err := parseNumber(response)
		if err != nil {
			return false, fmt.Errorf("failed to parse response as number: %q: %w", response, err)
		}

		return resNum >= lower && resNum <= upper, nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// findTaskByID returns the task with the given ID from the task list.
func findTaskByID(tasks []TaskDefinition, id string) (*TaskDefinition, error) {
	for _, task := range tasks {
		if task.ID == id {
			return &task, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", id)
}

func validateChain(tasks []TaskDefinition) error {
	if len(tasks) == 0 {
		return fmt.Errorf("chain has no tasks %w", apiframework.ErrBadRequest)
	}
	for _, ct := range tasks {
		if ct.ID == "" || ct.ID == TermEnd {
			if ct.ID == "" {
				return fmt.Errorf("task ID cannot be empty %w", apiframework.ErrBadRequest)
			}
			if ct.ID == TermEnd {
				return fmt.Errorf("task ID cannot be '%s' %w", TermEnd, apiframework.ErrBadRequest)
			}
		}
	}
	return nil
}
