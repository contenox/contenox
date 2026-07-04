package enginesvc

import (
	"context"

	"github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/execservice"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/mcpworker"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/taskengine"
)

type Config struct {
	DefaultModel       string
	DefaultProvider    string
	AltDefaultModel    string
	AltDefaultProvider string

	// ReadinessDefaultModel/ReadinessDefaultProvider are effective defaults to
	// credit during readiness evaluation when the persisted KV config leaves them
	// unset — e.g. the CLI's --model/--provider flags, which configure a single
	// invocation without writing config. Empty means "no override"; server/ACP/
	// editor paths leave these blank and rely solely on persisted config.
	ReadinessDefaultModel    string
	ReadinessDefaultProvider string

	ContextLength int

	NoDeleteModels bool

	LocalTools map[string]taskengine.ToolsRepo

	EnableHITL            bool
	AskApproval           localtools.AskApproval
	HITLService           hitlservice.Service
	HITLDefaultPolicyName string

	Bus     libbus.Messenger
	KVStore libkvstore.KVManager
	// State is the runtime backend/model snapshot to use. When nil, Build
	// creates one. Supplying it lets HTTP routes and the engine share exactly
	// one observed runtime state.
	State           *runtimestate.State
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
	State         *runtimestate.State
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
