package taskengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"dario.cat/mergo"
	"github.com/contenox/activitytracker"
	"github.com/contenox/modelprovider/llmresolver"
	"github.com/contenox/runtime/apiframework"
)

const (
	StatusSuccess             = 1
	StatusUnknownHookProvider = 2
	StatusError               = 3
)

// DataType represents the type of data that can be passed between tasks
type DataType int

// Constants representing hook execution status
const (
	DataTypeAny DataType = iota
	DataTypeString
	DataTypeBool
	DataTypeInt
	DataTypeFloat
	DataTypeSearchResults
	DataTypeJSON
	DataTypeChatHistory
	DataTypeOpenAIChat
	DataTypeOpenAIChatResponse
)

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

// EnvExecutor defines an environment for executing ChainDefinitions
type EnvExecutor interface {
	// ExecEnv executes a chain with input and returns final output
	ExecEnv(ctx context.Context, chain *ChainDefinition, input any, dataType DataType) (any, []CapturedStateUnit, error)
}

// ErrUnsupportedTaskType indicates unrecognized task type
var ErrUnsupportedTaskType = errors.New("executor does not support the task type")

// HookRepo defines an interface for external system integrations
// and to conduct side effects on internal state.
type HookRepo interface {
	// Exec runs a hook with input and returns results
	Exec(ctx context.Context, startingTime time.Time, input any, dataType DataType, transition string, args *HookCall) (int, any, DataType, string, error)
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
	exec           TaskExecutor
	tracker        activitytracker.ActivityTracker
	inspector      Inspector
	alertCollector AlertSink
}

// NewEnv creates a new SimpleEnv with the given tracker and task executor.
func NewEnv(
	_ context.Context,
	tracker activitytracker.ActivityTracker,
	alertCollector AlertSink,
	exec TaskExecutor,
	inspector Inspector,
) (EnvExecutor, error) {
	return &SimpleEnv{
		exec:           exec,
		tracker:        tracker,
		inspector:      inspector,
		alertCollector: alertCollector,
	}, nil
}

// ExecEnv executes the given chain with the provided input.
//
// It manages the full lifecycle of task execution: rendering prompts, calling the
// TaskExecutor, handling timeouts, retries, transitions, and collecting final output.
func (exe SimpleEnv) ExecEnv(ctx context.Context, chain *ChainDefinition, input any, dataType DataType) (any, []CapturedStateUnit, error) {
	stack := exe.inspector.Start(ctx)

	vars := map[string]any{
		"input": input,
	}
	varTypes := map[string]DataType{"input": dataType}
	startingTime := time.Now().UTC()
	resolver := llmresolver.Randomly
	var err error

	if len(chain.RoutingStrategy) > 0 {
		resolver, err = llmresolver.PolicyFromString(chain.RoutingStrategy)
		if err != nil {
			return nil, stack.GetExecutionHistory(), err
		}
	}

	if err := validateChain(chain.Tasks); err != nil {
		return nil, stack.GetExecutionHistory(), err
	}

	currentTask, err := findTaskByID(chain.Tasks, chain.Tasks[0].ID)
	if err != nil {
		return nil, stack.GetExecutionHistory(), err
	}

	var finalOutput any
	var transitionEval string
	var output any = input
	var outputType DataType = dataType
	var taskErr error

	for {
		// Determine task input
		taskInput := output
		taskInputType := outputType
		if currentTask.InputVar != "" {
			var ok bool
			taskInput, ok = vars[currentTask.InputVar]
			if !ok {
				return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: input variable %q not found", currentTask.ID, currentTask.InputVar)
			}
			taskInputType, ok = varTypes[currentTask.InputVar]
			if !ok {
				return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: input variable %q missing type info", currentTask.ID, currentTask.InputVar)
			}
		}

		// Render prompt template if exists
		if currentTask.PromptTemplate != "" {
			rendered, err := renderTemplate(currentTask.PromptTemplate, vars)
			if err != nil {
				return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: template error: %v", currentTask.ID, err)
			}
			taskInput = rendered
			taskInputType = DataTypeString
		}
		maxRetries := max(currentTask.RetryOnFailure, 0)

	retryLoop:
		for retry := 0; retry <= maxRetries; retry++ {
			// Note: Return on breakpoint for now
			if stack.HasBreakpoint(currentTask.ID) {
				return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: breakpoint set", currentTask.ID)
			}

			// Track task attempt start
			taskCtx := ctx
			var cancel context.CancelFunc
			if currentTask.Timeout != "" {
				timeout, err := time.ParseDuration(currentTask.Timeout)
				if err != nil {
					return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: invalid timeout: %v", currentTask.ID, err)
				}
				taskCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			reportErrAttempt, reportChangeAttempt, endAttempt := exe.tracker.Start(
				taskCtx,
				"task_attempt",
				currentTask.ID,
				"retry", retry,
				"task_type", currentTask.Type,
			)
			defer endAttempt()

			startTime := time.Now().UTC()

			output, outputType, transitionEval, taskErr = exe.exec.TaskExec(taskCtx, startingTime, resolver, int(chain.TokenLimit), currentTask, taskInput, taskInputType)
			if taskErr != nil {
				taskErr = fmt.Errorf("task %s: %w", currentTask.ID, taskErr)
				reportErrAttempt(taskErr)
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
				TaskID:     currentTask.ID,
				TaskType:   currentTask.Type.String(),
				InputType:  taskInputType,
				OutputType: outputType,
				Transition: transitionEval,
				Duration:   duration,
				Error:      errState,
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
					return nil, stack.GetExecutionHistory(), fmt.Errorf("compose right_var %q not found", compose.WithVar)
				}

				// Determine strategy
				strategy := compose.Strategy
				if strategy == "" {
					// Automatic strategy selection based on types
					if dataType == DataTypeChatHistory && varTypes[compose.WithVar] == DataTypeChatHistory {
						strategy = "chathistory_append"
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
						return nil, stack.GetExecutionHistory(), fmt.Errorf("invalid types for override")
					}
					if err := mergo.Merge(&baseMap, overridesMap, mergo.WithOverride); err != nil {
						return nil, stack.GetExecutionHistory(), fmt.Errorf("merge failed (override): %w", err)
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
						return nil, stack.GetExecutionHistory(), fmt.Errorf("compose strategy 'append_string_to_chat_history' requires both left and right values to be either string or ChatHistory")
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
					leftCH, leftIsCH := output.(ChatHistory)
					rightCH, rightIsCH := rightVal.(ChatHistory)

					if !leftIsCH || !rightIsCH {
						rightType, ok := varTypes[compose.WithVar]
						if !ok {
							return nil, stack.GetExecutionHistory(), fmt.Errorf("compose strategy 'chathistory_append' requires both right value to exist")
						}

						return nil, stack.GetExecutionHistory(), fmt.Errorf("compose strategy 'chathistory_append' requires both left (type: %s) and right (type: %s) values to be ChatHistory", dataType.String(), rightType.String())
					}

					leftCH.Messages = append(rightCH.Messages, leftCH.Messages...)

					// Sum the token counts
					leftCH.InputTokens += rightCH.InputTokens
					leftCH.OutputTokens += rightCH.OutputTokens

					// Clear the Model field if the models are different
					if leftCH.Model != rightCH.Model {
						leftCH.Model = ""
					}
					// If models are the same, leftCH.Model remains unchanged.
					merged = leftCH
				default:
					return nil, stack.GetExecutionHistory(), fmt.Errorf("unsupported compose strategy: %q", strategy)
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
				if currentTask.Transition.OnFailureAlert != "" {
					exe.alertCollector.SendAlert(ctx, currentTask.Transition.OnFailureAlert, "task_id", currentTask.ID, "error", err.Error())
				}
				previousTaskID := currentTask.ID
				currentTask, err = findTaskByID(chain.Tasks, currentTask.Transition.OnFailure)
				if err != nil {
					return nil, stack.GetExecutionHistory(), fmt.Errorf("error transition target not found: %v", err)
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
			return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s failed after %d retries: %v", currentTask.ID, maxRetries, taskErr)
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
				return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: print template error: %v", currentTask.ID, err)
			}
			fmt.Println(printMsg)
		}

		// Evaluate transitions
		nextTaskID, err := exe.evaluateTransitions(ctx, currentTask.ID, currentTask.Transition, transitionEval)
		if err != nil {
			return nil, stack.GetExecutionHistory(), fmt.Errorf("task %s: transition error: %v", currentTask.ID, err)
		}

		if nextTaskID == "" || nextTaskID == TermEnd {
			finalOutput = output
			// Track final output
			_, reportChangeFinal, endFinal := exe.tracker.Start(
				ctx,
				"chain_complete",
				"chain",
				"final_output", finalOutput,
			)
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
			return nil, stack.GetExecutionHistory(), fmt.Errorf("next task %s not found: %v", nextTaskID, err)
		}
	}

	return finalOutput, stack.GetExecutionHistory(), nil
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
			if ct.AlertOnMatch != "" {
				exe.alertCollector.SendAlert(ctx, ct.AlertOnMatch, "task_id", taskID, "eval", eval)
			}
			return ct.Goto, nil
		}
	}

	// Then check for default
	for _, ct := range transition.Branches {
		if ct.Operator == "default" {
			if ct.AlertOnMatch != "" {
				exe.alertCollector.SendAlert(ctx, ct.AlertOnMatch, "task_id", taskID, "eval", eval)
			}
			return ct.Goto, nil
		}
	}

	return "", fmt.Errorf("no matching transition found")
}

// parseNumber attempts to parse a string as either an integer or float.
func parseNumber(s string) (float64, error) {
	// Try int first
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return float64(i), nil
	}
	// Fallback to float
	return strconv.ParseFloat(s, 64)
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
			return false, fmt.Errorf("invalid between range format: %s", when)
		}
		lower, err := parseNumber(strings.TrimSpace(parts[0]))
		if err != nil {
			return false, fmt.Errorf("invalid lower bound: %v", err)
		}
		upper, err := parseNumber(strings.TrimSpace(parts[1]))
		if err != nil {
			return false, fmt.Errorf("invalid upper bound: %v", err)
		}
		resNum, err := parseNumber(response)
		if err != nil {
			return false, err
		}
		return resNum >= lower && resNum <= upper, nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// findTaskByID returns the task with the given ID from the task list.
func findTaskByID(tasks []ChainTask, id string) (*ChainTask, error) {
	for _, task := range tasks {
		if task.ID == id {
			return &task, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", id)
}

func validateChain(tasks []ChainTask) error {
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
