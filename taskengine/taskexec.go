package taskengine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/contenox/activitytracker"
	libmodelprovider "github.com/contenox/modelprovider"
	"github.com/contenox/modelprovider/llmresolver"
	"github.com/contenox/runtime/llmrepo"
)

// TaskExecutor defines the interface for executing a individual tasks.
type TaskExecutor interface {
	// TaskExec runs a single task and returns output
	// It consumes a prompt and resolver policy, and returns structured output
	// TODO: THIS IS NOT TRUE: alongside the raw LLM response.
	TaskExec(ctx context.Context, startingTime time.Time, resolver llmresolver.Policy, ctxLength int, currentTask *ChainTask, input any, dataType DataType) (any, DataType, string, error)
}

// SimpleExec is a basic implementation of TaskExecutor.
// It supports prompt-to-string, number, score, range, boolean condition evaluation,
// and delegation to registered hooks.
type SimpleExec struct {
	repo         llmrepo.ModelRepo
	embedRepo    llmrepo.ModelRepo
	hookProvider HookRepo
	tracker      activitytracker.ActivityTracker
}

// NewExec creates a new SimpleExec instance
func NewExec(
	_ context.Context,
	repo llmrepo.ModelRepo,
	embedRepo llmrepo.ModelRepo,
	hookProvider HookRepo,
	tracker activitytracker.ActivityTracker,
) (TaskExecutor, error) {
	if hookProvider == nil {
		return nil, fmt.Errorf("hook provider is nil")
	}
	if repo == nil {
		return nil, fmt.Errorf("repo executor is nil")
	}
	return &SimpleExec{
		hookProvider: hookProvider,
		repo:         repo,
		embedRepo:    embedRepo,
		tracker:      tracker,
	}, nil
}

// Prompt resolves a model client using the resolver policy and sends the prompt
// to be executed. Returns the trimmed response string or an error.
func (exe *SimpleExec) Prompt(ctx context.Context, resolver llmresolver.Policy, llmCall LLMExecutionConfig, prompt string) (string, error) {
	reportErr, reportChange, end := exe.tracker.Start(ctx, "SimpleExec", "prompt_model",
		"model_name", llmCall.Model,
		"model_names", llmCall.Models,
		"provider_types", llmCall.Providers,
		"provider_type", llmCall.Provider,
	)
	defer end()

	if prompt == "" {
		err := fmt.Errorf("unprocessable empty prompt")
		reportErr(err)
		return "", err
	}
	providerNames := []string{}
	if llmCall.Provider != "" {
		providerNames = append(providerNames, llmCall.Provider)
	}
	if llmCall.Providers != nil {
		providerNames = append(providerNames, llmCall.Providers...)
	}
	modelNames := []string{}
	if llmCall.Model != "" {
		modelNames = append(modelNames, llmCall.Model)
	}
	if llmCall.Models != nil {
		modelNames = append(modelNames, llmCall.Models...)
	}

	runtimeStateResolution := exe.repo.GetRuntime(ctx)
	if len(modelNames) == 0 && len(providerNames) == 0 {
		provider, err := exe.repo.GetDefaultSystemProvider(ctx)
		if err != nil {
			err = fmt.Errorf("failed to get providers: %w", err)
			reportErr(err)
			return "", err
		}
		modelNames = append(modelNames, provider.ModelName())
		providerNames = append(providerNames, provider.GetType())
		runtimeStateResolution = func(ctx context.Context, backendTypes ...string) ([]libmodelprovider.Provider, error) {
			ids := []string{provider.GetID()}
			reportChange("available_providers", ids)
			return []libmodelprovider.Provider{provider}, nil // Use direct providers list
		}
	}
	// Resolve client using available providers
	client, err := llmresolver.PromptExecute(ctx,
		llmresolver.PromptRequest{
			ProviderTypes: providerNames,
			ModelNames:    modelNames,
			Tracker:       exe.tracker,
		},
		runtimeStateResolution,
		resolver,
	)
	if err != nil {
		err = fmt.Errorf("client resolution failed: %w", err)
		reportErr(err)
		return "", err
	}

	response, err := client.Prompt(ctx, prompt)
	if err != nil {
		err = fmt.Errorf("prompt execution failed: %w", err)
		reportErr(err)
		return "", err
	}

	return strings.TrimSpace(response), nil
}

// Prompt resolves a model client using the resolver policy and sends the prompt
// to be executed. Returns the trimmed response string or an error.
func (exe *SimpleExec) Embed(ctx context.Context, resolver llmresolver.Policy, llmCall LLMExecutionConfig, prompt string) ([]float64, error) {
	reportErr, reportChange, end := exe.tracker.Start(ctx, "SimpleExec", "Embed",
		"model_name", llmCall.Model,
		"model_names", llmCall.Models,
		"provider_types", llmCall.Providers,
		"provider_type", llmCall.Provider,
	)
	defer end()

	if prompt == "" {
		err := fmt.Errorf("unprocessable empty prompt")
		reportErr(err)
		return nil, err
	}
	providerNames := []string{}
	if llmCall.Provider != "" {
		providerNames = append(providerNames, llmCall.Provider)
	}
	if llmCall.Providers != nil {
		providerNames = append(providerNames, llmCall.Providers...)
	}
	modelNames := []string{}
	if llmCall.Model != "" {
		modelNames = append(modelNames, llmCall.Model)
	}
	if llmCall.Models != nil {
		modelNames = append(modelNames, llmCall.Models...)
	}

	runtimeStateResolution := exe.embedRepo.GetRuntime(ctx)
	if len(modelNames) == 0 && len(providerNames) == 0 {
		provider, err := exe.embedRepo.GetDefaultSystemProvider(ctx)
		if err != nil {
			err = fmt.Errorf("failed to get providers: %w", err)
			reportErr(err)
			return nil, err
		}
		modelNames = append(modelNames, provider.ModelName())
		providerNames = append(providerNames, provider.GetType())
		runtimeStateResolution = func(ctx context.Context, backendTypes ...string) ([]libmodelprovider.Provider, error) {
			ids := []string{provider.GetID()}
			reportChange("available_providers", ids)
			return []libmodelprovider.Provider{provider}, nil // Use direct providers list
		}
	}
	if len(providerNames) > 1 {
		return nil, fmt.Errorf("multiple providers specified")
	}
	if len(providerNames) == 0 {
		return nil, fmt.Errorf("no provider specified")
	}
	if len(modelNames) == 0 {
		return nil, fmt.Errorf("no model specified")
	}
	if len(modelNames) > 1 {
		return nil, fmt.Errorf("multiple models specified")
	}
	// Resolve client using available providers
	client, err := llmresolver.Embed(ctx,
		llmresolver.EmbedRequest{
			ProviderType: providerNames[0],
			ModelName:    modelNames[0],
			// Tracker:      exe.tracker,
		},
		runtimeStateResolution,
		resolver,
	)
	if err != nil {
		err = fmt.Errorf("client resolution failed: %w", err)
		reportErr(err)
		return nil, err
	}

	response, err := client.Embed(ctx, prompt)
	if err != nil {
		err = fmt.Errorf("prompt execution failed: %w", err)
		reportErr(err)
		return nil, err
	}

	return response, nil
}

// rang executes the prompt and attempts to parse the response as a range string (e.g. "6-8").
// If the response is a single number, it returns a degenerate range like "6-6".
func (exe *SimpleExec) rang(ctx context.Context, resolver llmresolver.Policy, llmCall LLMExecutionConfig, prompt string) (string, error) {
	response, err := exe.Prompt(ctx, resolver, llmCall, prompt)
	if err != nil {
		return "", err
	}

	return parseRangeString(response, prompt, response)
}

// parseRangeString parses and validates a string as either a range ("6-8") or a single number.
// Returns the normalized range string (e.g., "6-8" or "6-6") or an error.
// parseRangeString parses and validates a string as either a range ("6-8") or a single number.
// Returns the normalized range string (e.g., "6-8" or "6-6") or an error.
func parseRangeString(input, prompt, response string) (string, error) {
	clean := strings.TrimSpace(input)
	clean = strings.ReplaceAll(clean, " ", "")
	clean = strings.ReplaceAll(clean, "\"", "")
	clean = strings.ReplaceAll(clean, "'", "")

	if strings.Contains(clean, "-") {
		parts := strings.Split(clean, "-")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid range format: %q", input)
		}
		_, err1 := parseNumber(parts[0])
		_, err2 := parseNumber(parts[1])
		if err1 != nil {
			return "", fmt.Errorf("invalid number format: prompt %s answer %s invalid part %q %w", prompt, response, parts[0], err1)
		}
		if err2 != nil {
			return "", fmt.Errorf("invalid number format: prompt %s answer %s invalid part %q %w", prompt, response, parts[1], err2)
		}
		return parts[0] + "-" + parts[1], nil // return normalized (already clean)
	}

	// Try as single number
	if _, err := parseNumber(clean); err != nil {
		return "", fmt.Errorf("invalid number format: prompt %s answer %s %w", prompt, response, err)
	}

	return clean + "-" + clean, nil
}

// number executes the prompt and parses the response as an integer.
func (exe *SimpleExec) number(ctx context.Context, resolver llmresolver.Policy, llmCall LLMExecutionConfig, prompt string) (int, error) {
	response, err := exe.Prompt(ctx, resolver, llmCall, prompt)
	if err != nil {
		return 0, err
	}

	num, err := parseNumber(response)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: prompt %s answer %s %w", prompt, response, err)
	}

	// Check if the float is actually a whole number
	if num != float64(int(num)) {
		return 0, fmt.Errorf("parsed number is not an integer: %g", num)
	}

	return int(num), nil
}

// score executes the prompt and parses the response as a floating-point score.
func (exe *SimpleExec) score(ctx context.Context, resolver llmresolver.Policy, llmCall LLMExecutionConfig, prompt string) (float64, error) {
	response, err := exe.Prompt(ctx, resolver, llmCall, prompt)
	if err != nil {
		return 0, err
	}
	f, err := parseNumber(response)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: prompt %s answer %s %w", prompt, response, err)
	}
	return f, nil
}

// TaskExec dispatches task execution based on the task type.
func (exe *SimpleExec) TaskExec(taskCtx context.Context, startingTime time.Time, resolver llmresolver.Policy, ctxLength int, currentTask *ChainTask, input any, dataType DataType) (any, DataType, string, error) {
	var transitionEval string
	var taskErr error
	var output any = input
	var outputType DataType = dataType
	if currentTask.Handler == HandleNoop {
		return output, outputType, "noop", nil
	}
	// Unified prompt extraction function
	getPrompt := func() (string, error) {
		switch outputType {
		case DataTypeString:
			prompt, ok := input.(string)
			if !ok {
				return "", fmt.Errorf("SEVERBUG: input is not a string")
			}
			return prompt, nil
		case DataTypeInt:
			return fmt.Sprintf("%d", input), nil
		case DataTypeFloat:
			return fmt.Sprintf("%f", input), nil
		case DataTypeBool:
			return fmt.Sprintf("%t", input), nil
		case DataTypeChatHistory:
			history, ok := input.(ChatHistory)
			if !ok {
				return "", fmt.Errorf("SEVERBUG: input is not a chat history")
			}
			if len(history.Messages) == 0 {
				return "", fmt.Errorf("SEVERBUG: chat history is empty")
			}
			return history.Messages[len(history.Messages)-1].Content, nil
		default:
			return "", fmt.Errorf("getPrompt unsupported input type for task %v: %v", currentTask.Handler.String(), outputType.String())
		}
	}
	if len(currentTask.Handler) == 0 {
		return output, dataType, transitionEval, fmt.Errorf("%w: task-type is empty", ErrUnsupportedTaskType)
	}
	switch currentTask.Handler {
	case HandleRawString, HandleConditionKey, HandleParseNumber, HandleParseScore, HandleParseRange, HandleParseTransition, HandleEmbedding, HandleRaiseError:
		prompt, err := getPrompt()
		if err != nil {
			return nil, DataTypeAny, "", err
		}

		if currentTask.SystemInstruction != "" {
			prompt = fmt.Sprintf("%s\n%s", currentTask.SystemInstruction, prompt)
		}

		if currentTask.ExecuteConfig == nil {
			currentTask.ExecuteConfig = &LLMExecutionConfig{}
		}

		switch currentTask.Handler {
		case HandleRawString:
			transitionEval, taskErr = exe.Prompt(taskCtx, resolver, *currentTask.ExecuteConfig, prompt)
			output = transitionEval
			outputType = DataTypeString
		case HandleConditionKey:
			var hit bool
			hit, taskErr = exe.condition(taskCtx, resolver, *currentTask.ExecuteConfig, currentTask.ValidConditions, prompt)
			output = hit
			outputType = DataTypeBool
			transitionEval = strconv.FormatBool(hit)
		case HandleParseNumber:
			var number int
			number, taskErr = exe.number(taskCtx, resolver, *currentTask.ExecuteConfig, prompt)
			output = number
			outputType = DataTypeInt
			transitionEval = strconv.FormatInt(int64(number), 10)
		case HandleParseScore:
			var score float64
			score, taskErr = exe.score(taskCtx, resolver, *currentTask.ExecuteConfig, prompt)
			output = score
			outputType = DataTypeFloat
			transitionEval = strconv.FormatFloat(score, 'f', 2, 64)
		case HandleParseRange:
			transitionEval, taskErr = exe.rang(taskCtx, resolver, *currentTask.ExecuteConfig, prompt)
			outputType = DataTypeString
			output = transitionEval
		case HandleParseTransition:
			transitionEval, taskErr = exe.parseTransition(prompt)
			// output = output // pass as is to the next task
			// outputType = outputType
		case HandleEmbedding:
			message, err := getPrompt()
			if err != nil {
				return nil, DataTypeAny, "", fmt.Errorf("failed to get prompt: %w", err)
			}
			output, taskErr = exe.Embed(taskCtx, resolver, *currentTask.ExecuteConfig, message)
			outputType = DataTypeVector
			transitionEval = "ok"
		case HandleRaiseError:
			message, err := getPrompt()
			if err != nil {
				return nil, DataTypeAny, "", fmt.Errorf("failed to get prompt: %w", err)
			}
			return nil, DataTypeAny, "", errors.New(message)
		}
	case HandleModelExecution:
		if currentTask.ExecuteConfig == nil {
			return nil, DataTypeAny, "", fmt.Errorf("missing llm_execution config")
		}
		if dataType != DataTypeChatHistory {
			return nil, DataTypeAny, "", fmt.Errorf("llm_execution requires chat history input was %s", dataType.String())
		}
		chatHistory, ok := input.(ChatHistory)
		if !ok {
			return nil, DataTypeAny, "", fmt.Errorf("llm_execution requires chat history input was claimed as %s but actually %T", dataType.String(), input)
		}
		if currentTask.SystemInstruction != "" {
			// Check if the exact system instruction already exists in the messages
			alreadyPresent := false
			for _, msg := range chatHistory.Messages {
				if msg.Role == "system" && msg.Content == currentTask.SystemInstruction {
					alreadyPresent = true
					break
				}
			}

			// Only add if not already present
			if !alreadyPresent {
				messages := []Message{
					{
						Role:      "system",
						Content:   currentTask.SystemInstruction,
						Timestamp: time.Now().UTC(),
					},
				}
				chatHistory.Messages = append(messages, chatHistory.Messages...)
			}
		}
		output, outputType, transitionEval, taskErr = exe.executeLLM(
			taskCtx,
			chatHistory,
			ctxLength,
			resolver,
			currentTask.ExecuteConfig,
		)

	case HandleHook:
		if currentTask.Hook == nil {
			taskErr = fmt.Errorf("hook task missing hook definition")
		} else {
			if currentTask.Hook.Args == nil {
				currentTask.Hook.Args = make(map[string]string)
			}
			output, outputType, transitionEval, taskErr = exe.hookengine(
				taskCtx,
				startingTime,
				output,
				outputType,
				transitionEval,
				currentTask.Hook,
			)
		}

	default:
		taskErr = fmt.Errorf("unknown task type: %w -- %s", ErrUnsupportedTaskType, currentTask.Handler.String())
	}

	return output, outputType, transitionEval, taskErr
}

func (exe *SimpleExec) parseTransition(inputStr string) (string, error) {
	if inputStr == "" {
		return "", nil
	}
	trimmedInput := strings.TrimSpace(inputStr)
	if !strings.HasPrefix(trimmedInput, "/") {
		return "pass", nil
	}

	// Parse command
	parts := strings.SplitN(trimmedInput, " ", 2)
	command := strings.TrimPrefix(parts[0], "/")
	if command == "" {
		return "", fmt.Errorf("empty command")
	}

	return command, nil
}

func (exe *SimpleExec) executeLLM(ctx context.Context, input ChatHistory, ctxLength int, resolverPolicy llmresolver.Policy, llmCall *LLMExecutionConfig) (any, DataType, string, error) {
	reportErr, reportChange, end := exe.tracker.Start(ctx, "SimpleExec", "prompt_model",
		"model_name", llmCall.Model,
		"model_names", llmCall.Models,
		"provider_types", llmCall.Providers,
		"provider_type", llmCall.Provider)
	defer end()
	providerNames := []string{}
	if llmCall.Provider != "" {
		providerNames = append(providerNames, llmCall.Provider)
	}
	if llmCall.Providers != nil {
		providerNames = append(providerNames, llmCall.Providers...)
	}
	runtimeStateResolution := exe.repo.GetRuntime(ctx)
	if len(llmCall.Models) == 0 && len(providerNames) == 0 {
		provider, err := exe.repo.GetDefaultSystemProvider(ctx)
		if err != nil {
			err = fmt.Errorf("failed to get providers: %w", err)
			reportErr(err)
			return nil, DataTypeAny, "", err
		}
		llmCall.Models = append(llmCall.Models, provider.ModelName())
		providerNames = append(providerNames, provider.GetType())
		runtimeStateResolution = func(ctx context.Context, backendTypes ...string) ([]libmodelprovider.Provider, error) {
			ids := []string{provider.GetID()}
			reportChange("available_providers", ids)
			return []libmodelprovider.Provider{provider}, nil // Use direct providers list
		}
	}
	tokenizer, err := exe.repo.GetTokenizer(ctx, llmCall.Model)
	if err != nil {
		reportErr(fmt.Errorf("tokenizer failed: %w", err))
		return nil, DataTypeAny, "", fmt.Errorf("tokenizer failed: %w", err)
	}
	if input.InputTokens <= 0 {
		for _, m := range input.Messages {
			InputCount, err := tokenizer.CountTokens(ctx, m.Content)
			if err != nil {
				reportErr(fmt.Errorf("token count failed: %w", err))
				return nil, DataTypeAny, "", fmt.Errorf("token count failed: %w", err)
			}
			input.InputTokens += InputCount
		}
	}
	if ctxLength > 0 && input.InputTokens > ctxLength {
		reportErr(fmt.Errorf("input token count %d exceeds context length %d", input.InputTokens, ctxLength))
		err := fmt.Errorf("input token count %d exceeds context length %d", input.InputTokens, ctxLength)
		return nil, DataTypeAny, "", err
	}
	modelNames := []string{}
	if llmCall.Model != "" {
		modelNames = append(modelNames, llmCall.Model)
	}
	if llmCall.Models != nil {
		modelNames = append(modelNames, llmCall.Models...)
	}

	// Resolve client using available providers
	client, _, err := llmresolver.Chat(ctx,
		llmresolver.Request{
			ProviderTypes: providerNames,
			ModelNames:    modelNames,
			ContextLength: input.InputTokens,
			Tracker:       exe.tracker,
		},
		runtimeStateResolution,
		resolverPolicy,
	)
	if err != nil {
		err = fmt.Errorf("client resolution failed: %w", err)
		reportErr(err)
		return nil, DataTypeAny, "", err
	}
	messagesC := []libmodelprovider.Message{}
	for _, m := range input.Messages {
		messagesC = append(messagesC, libmodelprovider.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	resp, err := client.Chat(ctx, messagesC)
	if err != nil {
		return nil, DataTypeAny, "", fmt.Errorf("chat failed: %w", err)
	}
	input.Messages = append(input.Messages, Message{
		Role:      resp.Role,
		Content:   resp.Content,
		Timestamp: time.Now().UTC(),
	})

	outputTokensCount, err := tokenizer.CountTokens(ctx, resp.Content)
	if err != nil {
		err = fmt.Errorf("tokenizer failed: %w", err)
		reportErr(err)
		return nil, DataTypeAny, "", err
	}
	input.OutputTokens = outputTokensCount

	return input, DataTypeChatHistory, "executed", nil
}

func (exe *SimpleExec) hookengine(ctx context.Context, startingTime time.Time, input any, dataType DataType, transition string, hook *HookCall) (any, DataType, string, error) {
	status, res, dataType, transition, err := exe.hookProvider.Exec(ctx, startingTime, input, dataType, transition, hook)
	if status != StatusSuccess {
		if err != nil {
			return res, dataType, transition, fmt.Errorf("hook execution failed bad status: %v %w", status, err)
		}
		return res, dataType, transition, fmt.Errorf("hook execution failed bad status: %v", status)
	}
	return res, dataType, transition, err
}

// condition executes a prompt and evaluates its result against a provided condition mapping.
// It returns true/false based on the resolved condition value or fallback heuristics.
func (exe *SimpleExec) condition(ctx context.Context, resolver llmresolver.Policy, llmCall LLMExecutionConfig, validConditions map[string]bool, prompt string) (bool, error) {
	response, err := exe.Prompt(ctx, resolver, llmCall, prompt)
	if err != nil {
		return false, err
	}
	found := false
	for k := range validConditions {
		if k == response {
			found = true
		}
	}
	if !found {
		return false, fmt.Errorf("failed to parse into valid condition output was %s", response)
	}
	for key, val := range validConditions {
		if strings.EqualFold(response, key) {
			if val {
				return strings.EqualFold(strings.TrimSpace(response), key), nil
			}
			return !strings.EqualFold(strings.TrimSpace(response), key), nil
		}
	}

	return strings.EqualFold(strings.TrimSpace(response), "yes"), nil
}
