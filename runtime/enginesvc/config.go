package enginesvc

import (
	"context"

	"github.com/contenox/agent/libbus"
	"github.com/contenox/agent/libkvstore"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/execservice"
	"github.com/contenox/agent/runtime/hitlservice"
	"github.com/contenox/agent/runtime/internal/setupcheck"
	"github.com/contenox/agent/runtime/localtools"
	"github.com/contenox/agent/runtime/mcpworker"
	"github.com/contenox/agent/runtime/taskengine"
)

type Config struct {
	DefaultModel       string
	DefaultProvider    string
	AltDefaultModel    string
	AltDefaultProvider string

	ContextLength int

	NoDeleteModels bool

	LocalTools map[string]taskengine.ToolsRepo

	EnableHITL            bool
	AskApproval           localtools.AskApproval
	HITLService           hitlservice.Service
	HITLDefaultPolicyName string

	Bus             libbus.Messenger
	KVStore         libkvstore.KVManager
	Tracker         libtracker.ActivityTracker
	ExtraInspectors []func(taskengine.Inspector) taskengine.Inspector
	TaskEventSink   taskengine.TaskEventSink

	Tracing bool

	SkipBackendCycle bool

	WorkspaceID string
	// TenantID is the tenant the engine operates under. When empty, defaults
	// to runtimetypes.LocalTenantID. Proprietary builds pass real tenant IDs.
	TenantID string
	// HITLPolicySource supplies HITL policy documents (used only when EnableHITL
	// is set and HITLService is nil). OSS passes a filesystem-backed source;
	// tenant-scoped builds inject their own.
	HITLPolicySource hitlservice.PolicySource
}

type Engine struct {
	TaskService   execservice.TasksEnvService
	Tracker       libtracker.ActivityTracker
	Bus           libbus.Messenger
	MCPManager    *mcpworker.Manager
	LocalTools    []string
	SetupCheck    setupcheck.Result
	TaskEventSink taskengine.TaskEventSink
	Stop          func()
	// SetupStatus recomputes current readiness from live runtime state (read-only:
	// reads synced backend state + config, never probes or runs a completion).
	// SetupCheck above is the build-time snapshot; this reflects the latest state.
	SetupStatus func(ctx context.Context) (setupcheck.Result, error)
}
