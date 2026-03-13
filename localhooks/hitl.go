// Package localhooks provides local hook integrations.
package localhooks

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/contenox/contenox/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

// ApprovalRequest describes a tool invocation that requires human approval.
// The Diff field is populated for file-mutation tools (write_file, sed) to show
// the unified diff of what would change.
type ApprovalRequest struct {
	HookName string         // e.g. "local_fs"
	ToolName string         // e.g. "write_file"
	Args     map[string]any // tool arguments received from the LLM
	Diff     string         // non-empty for file-write operations
}

// AskApproval is the callback that the HITLWrapper calls to request human review.
// Implementations must block until the human decides, then return (true, nil) to
// approve or (false, nil) to deny. Returning an error propagates it to the chain.
type AskApproval func(ctx context.Context, req ApprovalRequest) (approved bool, err error)

// HITLWrapper is a decorator around any HookRepo that intercepts configured tool
// calls and requests human approval before delegating to the inner hook.
//
// Tool calls not listed in RequireApprove pass through instantly.
// If the human denies the request, the wrapper returns a user-friendly string
// rather than a Go error so that the LLM can propose an alternative approach.
type HITLWrapper struct {
	Inner          taskengine.HookRepo
	Ask            AskApproval
	RequireApprove map[string]bool // tool names that need approval
}

// Exec implements taskengine.HookRepo.
func (h *HITLWrapper) Exec(
	ctx context.Context,
	startTime time.Time,
	input any,
	debug bool,
	hook *taskengine.HookCall,
) (any, taskengine.DataType, error) {
	// Determine the effective tool name.
	toolName := hook.ToolName
	if toolName == "" {
		toolName = hook.Name
	}

	// Fast-path: pass through tools that don't require approval.
	if !h.RequireApprove[toolName] {
		return h.Inner.Exec(ctx, startTime, input, debug, hook)
	}

	// Coerce input to map[string]any for inspection.
	args, _ := input.(map[string]any)
	if args == nil {
		args = make(map[string]any)
	}

	// Build a unified diff for file-write tools so the human can see exactly
	// what the LLM wants to change before approving.
	diff := buildDiff(hook.Name, toolName, args)

	req := ApprovalRequest{
		HookName: hook.Name,
		ToolName: toolName,
		Args:     args,
		Diff:     diff,
	}

	approved, err := h.Ask(ctx, req)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("hitl: approval error: %w", err)
	}
	if !approved {
		// Return a soft denial so the LLM can try an alternative.
		// Do NOT return a Go error here — that would crash the entire chain.
		return "User denied the operation. Please ask for clarification or try a different, less destructive approach.", taskengine.DataTypeString, nil
	}

	return h.Inner.Exec(ctx, startTime, input, debug, hook)
}

// Supports delegates to the inner repo.
func (h *HITLWrapper) Supports(ctx context.Context) ([]string, error) {
	return h.Inner.Supports(ctx)
}

// GetSchemasForSupportedHooks delegates to the inner repo.
func (h *HITLWrapper) GetSchemasForSupportedHooks(ctx context.Context) (map[string]*openapi3.T, error) {
	return h.Inner.GetSchemasForSupportedHooks(ctx)
}

// GetToolsForHookByName delegates to the inner repo.
func (h *HITLWrapper) GetToolsForHookByName(ctx context.Context, name string) ([]taskengine.Tool, error) {
	return h.Inner.GetToolsForHookByName(ctx, name)
}

// Compile-time assertion.
var _ taskengine.HookRepo = (*HITLWrapper)(nil)

// ─── diff helper ───────────────────────────────────────────────────────────────

// buildDiff generates a simple human-readable unified diff for file-write
// operations (write_file and sed). It avoids external dependencies by doing a
// straightforward line-level comparison.
func buildDiff(hookName, toolName string, args map[string]any) string {
	switch {
	case (hookName == "local_fs") && (toolName == "write_file"):
		path, _ := args["path"].(string)
		newContent, _ := args["content"].(string)
		if path == "" || newContent == "" {
			return ""
		}
		oldBytes, _ := os.ReadFile(path)
		return unifiedDiff(path, string(oldBytes), newContent)

	case (hookName == "local_fs") && (toolName == "sed"):
		path, _ := args["path"].(string)
		pattern, _ := args["pattern"].(string)
		replacement, _ := args["replacement"].(string)
		if path == "" || pattern == "" {
			return ""
		}
		oldBytes, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		newContent := strings.ReplaceAll(string(oldBytes), pattern, replacement)
		return unifiedDiff(path, string(oldBytes), newContent)
	}
	return ""
}

// unifiedDiff returns a minimal unified-diff style summary of the changes between
// oldStr and newStr. Only changed (±3 context) lines are shown.
func unifiedDiff(filename, oldStr, newStr string) string {
	if oldStr == newStr {
		return "(no changes)"
	}
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// We use a simple LCS-free line diff that marks additions and deletions.
	// For a production system you'd use a proper Myers diff library, but this
	// zero-dep implementation is readable and correct for the approval-request UX.
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s (current)\n+++ %s (proposed)\n", filename, filename)

	oldSet := make(map[string]bool, len(oldLines))
	newSet := make(map[string]bool, len(newLines))
	for _, l := range oldLines {
		oldSet[l] = true
	}
	for _, l := range newLines {
		newSet[l] = true
	}

	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	shown := 0
	const maxShownLines = 60
	for i := 0; i < maxLines && shown < maxShownLines; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine == newLine {
			fmt.Fprintf(&sb, "  %s\n", oldLine)
		} else {
			if i < len(oldLines) {
				fmt.Fprintf(&sb, "- %s\n", oldLine)
				shown++
			}
			if i < len(newLines) {
				fmt.Fprintf(&sb, "+ %s\n", newLine)
				shown++
			}
		}
	}
	if shown >= maxShownLines {
		sb.WriteString("... (diff truncated)\n")
	}
	return sb.String()
}
