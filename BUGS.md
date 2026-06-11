# Bug Analysis - Contenox Runtime Engine

---

## 🔴 CRITICAL BUGS

### BUG-001: Context Length Overflow in `classify_request` Route Step

**Status:** Open | **Severity:** CRITICAL | **Impact:** Chain fails completely

**Error:**
```
step "classify_request" (route) failed: task classify_request: route task classify_request: exceeds context length: input token count 1803876 > 131072
```

**Root Cause:**
- The `classify_request` route step receives the **entire accumulated ChatHistory** as input
- No truncation, summarization, or sliding-window is applied before routing
- After many conversation turns, the serialized history balloons to ~1.8M tokens
- The route handler (in `taskexec.go:564`) calls `exe.Prompt()` with the full history
- Token counting in `countChatHistoryTokens()` (line 118) validates against `ctxLength` and fails

**Location:**
- Chain: `~/.contenox/default-acp-chain.json` (task: `classify_request`)
- Code: `runtime/taskengine/taskexec.go:547-568` (HandleRoute case)

**Fix Required:**
1. Add history pruning before the classify_request step
2. Implement sliding window in `shiftMessagesToFit()` for route tasks
3. Or: Add a chain-level `max_input_tokens` that trims history before routing

---

### BUG-002: Chain Does Not Short-Circuit on Step Failure - Cascading 'any' Type Errors

**Status:** Open | **Severity:** CRITICAL | **Impact:** Misleading error messages

**Error Pattern:**
```
step "acp_chat" failed: handler 'chat_completion' requires input of type 'chat_history' or 'string', used var:  but got 'any'
step "recovery_chat" failed: handler 'chat_completion' requires input of type 'chat_history' or 'string', used var: acp_chat but got 'any'
step "summarise_failure" failed: handler 'chat_completion' requires input of type 'chat_history' or 'string', used var: recovery_chat but got 'any'
```

**Root Cause:**
- When `classify_request` fails (BUG-001), its output variable is **not written** or is written as `nil`
- The chain execution continues to the next step (`acp_chat`)
- `acp_chat` tries to resolve `{{var:classify_request}}` via `resolveVar()` 
- Returns `nil` with type `DataTypeAny` (since var doesn't exist or is nil)
- In `taskexec.go:578-613` (HandleChatCompletion), the type switch hits default:
  ```go
  default:
      return nil, DataTypeAny, "", fmt.Errorf("input data for handler %s claimed to be %s but was %T", currentTask.Handler, dataType.String(), input)
  ```
- Each subsequent step reads the prior failed step's unset/nil output → same error

**Location:**
- Chain execution: `runtime/taskengine/taskexec.go` (TaskExec function)
- Error handling: Missing short-circuit logic after step failure

**Fix Required:**
1. **Engine should stop chain execution** when a step returns an error and there's no `on_failure` handler
2. OR: Ensure failed steps write a proper error value (not nil) with a known type
3. In `HandleChatCompletion`, add explicit nil check before type assertion:
   ```go
   if input == nil {
       return nil, DataTypeAny, "", fmt.Errorf("input is nil for task %s", currentTask.ID)
   }
   ```

---

### BUG-003: Tool Names Sent to Anthropic API Are Not Validated

**Status:** Open | **Severity:** HIGH | **Impact:** API rejection (400 error)

**Error:**
```
tools.0.custom.name: String should match pattern '^[a-zA-Z0-9_-]{1,128}$'
```

**Root Cause:**
- Tool names from MCP servers can contain special characters (dots, slashes, etc.)
- The codebase **does have** sanitization in `RegisterMCPTools` (toolregistry.go)
- BUT: The sanitization uses inline regex and truncates to **64 chars**
- There's **dead code** with `sanitizeToolName()` that truncates to **128 chars** (inconsistent)
- More critically: `RegisterTool()` (direct registration path) bypasses ALL sanitization
- `toAnthropicTools()` in the OLD code passed names directly - but the NEW code doesn't have this file

**Location:**
- **OLD code path:** `runtime/modelrepo/anthropic/anthropictools.go` (file doesn't exist in current codebase)
- **NEW code path:** Need to find where tools are serialized for Anthropic

**Fix Required:**
1. Find where Anthropic tools are built in current codebase
2. Add validation: `regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)`
3. Apply sanitization consistently across ALL registration paths
4. Delete dead utility functions or use them

---

## 🟠 HIGH SEVERITY BUGS

### BUG-004: Missing Token Count Tracking in Telemetry

**Status:** Open | **Severity:** HIGH (Observability)

**Observed:**
```json
{"msg":"step completed","model":"gemini-2.5-flash-lite-preview-06-17","input_tokens":null,"output_tokens":null}
```

**Root Cause:**
- Telemetry logging in `taskexec.go` doesn't extract or propagate token usage
- Anthropic provider returns usage metadata in response but it's not captured
- Need to thread `InputTokens` and `OutputTokens` from provider response back to telemetry

**Location:**
- `runtime/taskengine/taskexec.go` (telemetry logging)
- All provider implementations (Anthropic, Vertex, etc.)

**Fix Required:**
1. Extract usage from provider responses
2. Pass token counts through return chain
3. Log in telemetry step-completed events

---

### BUG-005: Chain Definition References Unknown Task in `on_failure`

**Status:** Open | **Severity:** HIGH | **Found in:** `~/.contenox/hubspot-revops.yaml`

**Error:**
```
chain execution failed: task "verify": on_failure references unknown task "end" bad request
```

**Root Cause:**
- Chain YAML defines `on_failure: end`
- But there's no task with ID `end` in the chain
- The chain should use `TermEnd` constant or define an `end` task

**Location:**
- `~/.contenox/hubspot-revops.yaml` (and potentially other chain files)

**Fix Required:**
1. Use `TermEnd` constant: `on_failure: ""` (empty string means end)
2. OR define an explicit `end` noop task
3. Add chain validation to catch unknown task references

---

## 🟡 MEDIUM SEVERITY BUGS

### BUG-006: Terminal Session Connection Management Issues

**Status:** Open | **Severity:** MEDIUM | **Impact:** User confusion

**Errors Observed in Logs:**
```
time=2026-06-10T18:18:20.383+02:00 level=ERROR msg="terminal attach error" session=ccf88a25-32ec-418b-b3d5-86cbf8af331c error="session already has an active connection"
```

**Root Cause:**
- Multiple attempts to attach to the same terminal session
- No proper cleanup of active connections
- Race condition in terminal service

**Location:**
- `runtime/terminalservice/`

**Fix Required:**
1. Implement proper session locking
2. Return error clearly when session is already attached
3. Add cleanup on disconnect

---

### BUG-007: Vertex Backend Configuration Issues

**Status:** Open | **Severity:** MEDIUM | **Impact:** Model not accessible

**Errors Observed:**
```
vertex API returned non-200 status for stream: 404
Publisher Model `projects/contenox/locations/us-central1/publishers/google/models/gemini-3.1-pro-preview` was not found
```

**Root Cause:**
- GOOGLE_CLOUD_PROJECT environment variable not set when creating backend
- Backend URL construction fails with consecutive slashes

**Location:**
- Backend registration: CLI and runtime configuration

**Fix Required:**
1. Validate GOOGLE_CLOUD_PROJECT is set before creating Vertex backend
2. Better error messages for malformed URLs
3. Documentation update

---

### BUG-008: Local Shell Enabled Without Safety Constraints

**Status:** Open | **Severity:** MEDIUM | **Impact:** Security risk

**Warnings in Logs:**
```
time=2026-06-04T23:00:40.810+02:00 level=WARN msg="local_shell is enabled with no HITL and no allowed-dir; chain-level tools_policies is the only safety gate"
```

**Root Cause:**
- `local_shell` tool is enabled in chains without:
  - Human-in-the-loop (HITL) supervision
  - `allowed-dir` restriction
- Only chain-level `tools_policies` provides safety

**Location:**
- Chain definitions (e.g., `default-acp-chain.json`)
- `runtime/taskengine/` tool execution

**Fix Required:**
1. Add validation warnings when local_shell is enabled without constraints
2. Consider making HITL or allowed-dir mandatory for local_shell
3. Audit all chains for this issue

---

## 🟢 LOW SEVERITY / CODE QUALITY

### BUG-009: Dead Code in Tool Registry

**Status:** Open | **Severity:** LOW | **Impact:** Maintainability

**Location:** `runtime/taskengine/toolregistry.go` (in OLD code path)

**Functions Never Called:**
- `sanitizeToolName(name string) string`
- `buildToolName(serverName, toolName string) string`
- `formatToolParameters(params any) string`
- `truncate(s string, max int) string`
- `isValidToolName(name string) bool`
- `normalizeWhitespace(s string) string`

**Fix Required:**
1. Delete dead functions
2. OR wire them into the active code paths properly
3. Ensure consistent behavior (64 vs 128 char limit)

---

### BUG-010: Model List Typo in CLI

**Status:** Open | **Severity:** LOW | **Impact:** User confusion

**Error Observed:**
```
argv="model lsit"
error="unknown subcommand \"lsit\""
```

**Root Cause:**
- User typed `lsit` instead of `list`
- But suggests the CLI could use better typo detection or suggestions

**Location:** CLI command parsing

**Fix Required:**
- Add did-you-mean suggestions for common typos
- Or just document the correct command

---

## 📊 SUMMARY BY CATEGORY

| Category | Count | Severity |
|----------|-------|----------|
| CRITICAL | 3 | 🔴 (Chain breaks completely) |
| HIGH | 2 | 🟠 (API errors, observability gaps) |
| MEDIUM | 4 | 🟡 (Security, configuration, UX) |
| LOW | 2 | 🟢 (Code quality, minor UX) |

---

## 🎯 IMMEDIATE ACTION ITEMS

1. **P0 - FIX NOW:** BUG-001 (Context overflow) - This is blocking all long conversations
2. **P0 - FIX NOW:** BUG-002 (Cascading errors) - Obscures root causes
3. **P1 - FIX SOON:** BUG-003 (Tool name validation) - Prevents Anthropic usage
4. **P1 - FIX SOON:** BUG-004 (Token tracking) - Essential for observability
5. **P2 - FIX LATER:** BUG-005 through BUG-010 - Quality improvements

---

## 🔍 INVESTIGATION NOTES

### Files Reviewed:
- `~/.contenox/default-acp-chain.json` - Main ACP chain definition
- `~/.contenox/chain-compact.json` - Compaction chain
- `~/.contenox/telemetry.log` - Runtime telemetry (250MB+)
- `runtime/taskengine/taskexec.go` - Task execution engine
- `runtime/taskengine/tasktype.go` - Type definitions
- `runtime/modelrepo/anthropic/provider.go` - Anthropic provider
- `runtime/modelrepo/anthropic/client.go` - Anthropic client

### Chain Analysis - default-acp-chain.json:
```
classify_request (route) 
  -> on_failure: acp_chat
  -> branches: coding_change -> coding_inspect, general -> acp_chat

acp_chat (chat_completion)
  -> ... (general conversation loop)

coding_* (inspect -> patch -> verify -> audit workflow)
```

The chain has **no history truncation** before `classify_request`, which is the root of BUG-001.

### Architecture Notes:
- The engine uses a **task-based workflow** with conditional transitions
- Each task has a handler: route, chat_completion, execute_tool_calls, noop, tools, raise_error
- The **route** handler uses a prompt to get a classification label
- **chat_completion** handles LLM chat with optional tool calls
- Token counting happens at execution time, not at chain definition time

---
