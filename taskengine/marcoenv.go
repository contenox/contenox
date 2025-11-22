package taskengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MacroEnv is a transparent decorator around EnvExecutor that expands
// special macros like {{hookservice:list}} in task templates before execution.
type MacroEnv struct {
	inner        EnvExecutor
	hookProvider HookRepo
}

// NewMacroEnv wraps an existing EnvExecutor with macro expansion.
func NewMacroEnv(inner EnvExecutor, hookProvider HookRepo) (EnvExecutor, error) {
	if inner == nil {
		return nil, fmt.Errorf("NewMacroEnv: inner EnvExecutor is nil")
	}
	return &MacroEnv{
		inner:        inner,
		hookProvider: hookProvider,
	}, nil
}

func (m *MacroEnv) ExecEnv(
	ctx context.Context,
	chain *TaskChainDefinition,
	input any,
	dataType DataType,
) (any, DataType, []CapturedStateUnit, error) {
	if chain == nil {
		return nil, DataTypeAny, nil, fmt.Errorf("chain is nil")
	}

	// Shallow copy the chain, deep copy tasks so we don't mutate the original.
	clone := *chain
	clone.Tasks = make([]TaskDefinition, len(chain.Tasks))
	copy(clone.Tasks, chain.Tasks)

	// Expand macros in all relevant string fields of each task.
	for i := range clone.Tasks {
		t := &clone.Tasks[i]

		var err error
		if t.PromptTemplate != "" {
			t.PromptTemplate, err = m.expandSpecialTemplates(ctx, t.PromptTemplate)
			if err != nil {
				return nil, DataTypeAny, nil, fmt.Errorf("task %s: prompt_template macro error: %w", t.ID, err)
			}
		}
		if t.Print != "" {
			t.Print, err = m.expandSpecialTemplates(ctx, t.Print)
			if err != nil {
				return nil, DataTypeAny, nil, fmt.Errorf("task %s: print macro error: %w", t.ID, err)
			}
		}
		if t.OutputTemplate != "" {
			t.OutputTemplate, err = m.expandSpecialTemplates(ctx, t.OutputTemplate)
			if err != nil {
				return nil, DataTypeAny, nil, fmt.Errorf("task %s: output_template macro error: %w", t.ID, err)
			}
		}
		if t.SystemInstruction != "" {
			t.SystemInstruction, err = m.expandSpecialTemplates(ctx, t.SystemInstruction)
			if err != nil {
				return nil, DataTypeAny, nil, fmt.Errorf("task %s: system_instruction macro error: %w", t.ID, err)
			}
		}
	}

	// Delegate to the real EnvExecutor with the rewritten chain.
	return m.inner.ExecEnv(ctx, &clone, input, dataType)
}

// Precompile once at package level if you like:
var macroRe = regexp.MustCompile(`\{\{hookservice:([a-zA-Z0-9_]+)(?:\s+([^}]+))?\}\}`)

// expandSpecialTemplates finds and replaces our custom macros.
// Right now it supports:
//
//	{{hookservice:list}}              -> JSON with hooks + tools
//	{{hookservice:hooks}}             -> JSON array of hook names
//	{{hookservice:tools <hook_name>}} -> JSON array of tool names for that hook
func (m *MacroEnv) expandSpecialTemplates(ctx context.Context, in string) (string, error) {
	if m.hookProvider == nil {
		// If there's no hookProvider, just leave macros as-is.
		return in, nil
	}

	matches := macroRe.FindAllStringSubmatchIndex(in, -1)
	if len(matches) == 0 {
		return in, nil
	}

	var buf bytes.Buffer
	last := 0

	for _, loc := range matches {
		start, end := loc[0], loc[1]
		cmdStart, cmdEnd := loc[2], loc[3]
		argStart, argEnd := loc[4], loc[5]

		// Write text before the macro
		buf.WriteString(in[last:start])

		cmd := in[cmdStart:cmdEnd]
		var arg string
		if argStart != -1 && argEnd != -1 {
			arg = strings.TrimSpace(in[argStart:argEnd])
		}

		var replacement string
		var err error

		switch cmd {
		case "list":
			replacement, err = m.renderHooksAndToolsJSON(ctx)
		case "hooks":
			replacement, err = m.renderHookNamesJSON(ctx)
		case "tools":
			if arg == "" {
				err = fmt.Errorf("hookservice:tools requires a hook name argument")
			} else {
				replacement, err = m.renderToolsForHookJSON(ctx, arg)
			}
		default:
			// unknown command: leave as-is
			replacement = in[start:end]
		}

		if err != nil {
			return "", err
		}

		buf.WriteString(replacement)
		last = end
	}

	// Tail
	buf.WriteString(in[last:])
	return buf.String(), nil
}

func (m *MacroEnv) renderHookNamesJSON(ctx context.Context) (string, error) {
	names, err := m.hookProvider.Supports(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list hooks: %w", err)
	}
	b, err := json.Marshal(names)
	if err != nil {
		return "", fmt.Errorf("failed to marshal hook names: %w", err)
	}
	return string(b), nil
}

func (m *MacroEnv) renderHooksAndToolsJSON(ctx context.Context) (string, error) {
	names, err := m.hookProvider.Supports(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list hooks: %w", err)
	}

	result := make(map[string][]string, len(names))
	for _, name := range names {
		tools, err := m.hookProvider.GetToolsForHookByName(ctx, name)
		if err != nil {
			// Skip broken hooks; you can also choose to fail hard here.
			continue
		}
		fnNames := make([]string, 0, len(tools))
		for _, t := range tools {
			fnNames = append(fnNames, t.Function.Name)
		}
		result[name] = fnNames
	}

	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal hooks+tools: %w", err)
	}
	return string(b), nil
}

func (m *MacroEnv) renderToolsForHookJSON(ctx context.Context, hookName string) (string, error) {
	tools, err := m.hookProvider.GetToolsForHookByName(ctx, hookName)
	if err != nil {
		return "", fmt.Errorf("failed to get tools for hook %s: %w", hookName, err)
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Function.Name)
	}
	b, err := json.Marshal(names)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tools for hook %s: %w", hookName, err)
	}
	return string(b), nil
}
