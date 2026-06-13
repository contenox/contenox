package vscodeagent

import (
	"context"
	"errors"

	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/taskengine"
)

const Identity = "vscode"

var ErrSetupRequired = errors.New("vscodeagent: setup required")

type RuntimeHooks struct {
	AskApproval localtools.AskApproval
	EventSink   taskengine.TaskEventSink
}

type Runtime struct {
	Engine       *enginesvc.Engine
	Agent        agentservice.Agent
	Chain        *taskengine.TaskChainDefinition
	FIMChain     *taskengine.TaskChainDefinition
	CompactChain *taskengine.TaskChainDefinition
	Close        func()
}

type RuntimeBuilder func(ctx context.Context, hooks RuntimeHooks) (*Runtime, error)

func (r *Runtime) stop() {
	if r == nil {
		return
	}
	if r.Close != nil {
		r.Close()
		return
	}
	if r.Engine != nil && r.Engine.Stop != nil {
		r.Engine.Stop()
	}
}
