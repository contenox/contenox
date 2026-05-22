package contenoxcli

import (
	"context"
	"log/slog"
	"os"

	"github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/enginesvc"
	"github.com/contenox/agent/runtime/localtools"
	"github.com/contenox/agent/runtime/taskengine"
	"github.com/contenox/agent/runtime/vfsservice"
)

type Engine = enginesvc.Engine

func BuildEngine(ctx context.Context, db libdbexec.DBManager, opts chatOpts, vfs vfsservice.Service) (*Engine, error) {
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
		VFS:                vfs,
		FallbackVFS:        globalContenoxVFS(),
	})
}

func globalContenoxVFS() vfsservice.Service {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return vfsservice.NewLocalFS(home + "/.contenox")
}
