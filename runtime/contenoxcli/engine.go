package contenoxcli

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/enginesvc"
	"github.com/contenox/agent/runtime/hitlservice"
	"github.com/contenox/agent/runtime/localtools"
	"github.com/contenox/agent/runtime/taskengine"
)

type Engine = enginesvc.Engine

func BuildEngine(ctx context.Context, db libdbexec.DBManager, opts chatOpts) (*Engine, error) {
	var tracker libtracker.ActivityTracker = libtracker.NoopTracker{}
	if opts.EffectiveTracing {
		tracker = libtracker.NewLogActivityTracker(slog.Default())
	}

	tools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(tracker),
		"local_fs": localtools.NewLocalFSTools(opts.EffectiveLocalExecAllowedDir, db),
	}
	if opts.EffectiveEnableLocalExec {
		execOpts := []localtools.LocalExecOption{}
		if opts.EffectiveLocalExecAllowedDir != "" {
			execOpts = append(execOpts, localtools.WithLocalExecAllowedDir(opts.EffectiveLocalExecAllowedDir))
		}
		tools["local_shell"] = localtools.NewLocalExecTools(execOpts...)

		if !opts.EffectiveHITL && opts.EffectiveLocalExecAllowedDir == "" {
			slog.Warn("local_shell is enabled with no HITL and no allowed-dir; chain-level tools_policies is the only safety gate")
		}
	}

	return enginesvc.Build(ctx, db, enginesvc.Config{
		DefaultModel:       opts.EffectiveDefaultModel,
		DefaultProvider:    opts.EffectiveDefaultProvider,
		AltDefaultModel:    opts.EffectiveAltDefaultModel,
		AltDefaultProvider: opts.EffectiveAltDefaultProvider,
		ContextLength:      opts.EffectiveContext,
		NoDeleteModels:     opts.EffectiveNoDeleteModels,
		LocalTools:         tools,
		EnableHITL:         opts.EffectiveHITL,
		AskApproval:        NewCLIAskApproval(os.Stderr),
		Tracker:            tracker,
		Tracing:            opts.EffectiveTracing,
		SkipBackendCycle:   opts.EffectiveSkipBackendCycle,
		WorkspaceID:        ResolveWorkspaceID(opts.ContenoxDir),
		HITLPolicySource:   hitlPolicySource(opts.ContenoxDir),
	})
}

// hitlPolicySource builds the CLI's HITL policy lookup: the workspace .contenox
// dir first, then the user's ~/.contenox as fallback.
func hitlPolicySource(primaryDir string) hitlservice.PolicySource {
	dirs := []string{primaryDir}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".contenox"))
	}
	return hitlservice.NewFSPolicySource(dirs...)
}
