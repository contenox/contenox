package contenoxcli

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/contenox/runtime/runtime/workspacegrants"
)

// workspaceRootReloader recomputes and applies the effective workspace-root set
// when a grant changes, so `contenox serve` picks up an added or removed root
// without a restart.
//
// # base + grants
//
// The effective set is base ∪ grants:
//
//   - base is the LAUNCH-time root set — serve's default (its served project
//     directory, or home for a bare `serve`) first, then positional args,
//     --workspace-root flags, and WORKSPACE_ROOTS. These are process-launch
//     inputs; they do not change while serve runs, so base is captured once and
//     is ALWAYS FIRST, which is what keeps serve's default root (base[0]) stable
//     across every reload — a grant can add roots but never displace the default.
//   - grants are the DURABLE, hot-reloadable additions
//     (workspacegrants.ReadGrants), re-read fresh from the shared config DB on
//     every apply. They are the only part that changes at runtime.
//
// apply re-reads the grants and swaps the whole set into the live Factory via
// SetRoots. The reloader is the single place that knows how the two sources
// combine, so the boot path (serve_cmd) and the runtime path (the doorbell
// subscriber, and the REST mutators) all produce identical effective sets.
type workspaceRootReloader struct {
	factory *vfs.Factory
	base    []string
	store   runtimetypes.Store
}

func newWorkspaceRootReloader(factory *vfs.Factory, base []string, store runtimetypes.Store) *workspaceRootReloader {
	return &workspaceRootReloader{factory: factory, base: base, store: store}
}

// effectiveRoots returns base ∪ grants in order (base first, grants appended),
// re-reading the durable grants each call. Duplicates are collapsed by the
// Factory, so a grant that repeats a base root is harmless.
func (r *workspaceRootReloader) effectiveRoots(ctx context.Context) []string {
	roots := make([]string, 0, len(r.base)+4)
	roots = append(roots, r.base...)
	roots = append(roots, workspacegrants.ReadGrants(ctx, r.store)...)
	return roots
}

// apply recomputes the effective set and swaps it into the live Factory.
func (r *workspaceRootReloader) apply(ctx context.Context) error {
	return r.factory.SetRoots(r.effectiveRoots(ctx))
}

// mutators builds the write half of the REST /workspace/roots surface. Each verb
// persists the durable grant, applies it to the live Factory (so the operator's
// very next GET reflects it with no bus delay), and rings the reload doorbell (so
// any OTHER process on the same DB — and the convention — is served too). The
// doorbell is best-effort: the grant is already durable, so a publish failure is
// logged, never surfaced as a failed grant.
func (r *workspaceRootReloader) mutators(bus libbus.Messenger) *localfileapi.RootsMutators {
	return &localfileapi.RootsMutators{
		Add: func(ctx context.Context, path string) error {
			// Control-plane isolation (vfs-invariant slice): a client may not grant
			// the runtime's own state dir as a workspace root. serve set the global
			// denylist at boot, so consult it here — a bad grant is a client input
			// fault (wraps ErrInvalidGrant -> 422 via localfileapi.grantError),
			// never a silent widening. See runtime/vfs/controlplane.go.
			if denied, ok := vfs.IsControlPlanePath(path); ok {
				return fmt.Errorf("%w: %q is inside the runtime's control plane (%s) and can never be a workspace root — the runtime never lets a session reach its own config, database, or policies", workspacegrants.ErrInvalidGrant, path, denied)
			}
			if _, err := workspacegrants.Add(ctx, r.store, path); err != nil {
				return err
			}
			return r.applyAndRing(ctx, bus)
		},
		Remove: func(ctx context.Context, path string) error {
			if _, err := workspacegrants.Remove(ctx, r.store, path); err != nil {
				return err
			}
			return r.applyAndRing(ctx, bus)
		},
	}
}

// applyAndRing swaps the new set into the live Factory and rings the doorbell.
// An apply failure IS surfaced (the grant is durable but the live set did not
// take — the caller should know); a doorbell failure is only logged.
func (r *workspaceRootReloader) applyAndRing(ctx context.Context, bus libbus.Messenger) error {
	if err := r.apply(ctx); err != nil {
		return err
	}
	// Audit trail: a grant change is a trust decision, so it leaves a line in
	// serve's log naming the resulting root set. There is no ActivityTracker
	// threaded into this REST/CLI seam, so this is the "else log" the slice's brief
	// calls for.
	roots := r.factory.Roots()
	slog.Info("contenox serve: workspace roots changed (authenticated grant verb)",
		"count", len(roots), "roots", roots)
	if err := workspacegrants.PublishChanged(ctx, bus, r.effectiveRoots(ctx)); err != nil {
		slog.Warn("contenox serve: workspace-root grant applied but reload doorbell publish failed",
			"error", err)
	}
	return nil
}

// startWorkspaceRootReloader subscribes to the reload doorbell and applies the
// durable config on every signal, until the returned stop is called. It mirrors
// reportrouter.Start's shape (subscribe, loop in a goroutine, stop cancels +
// unsubscribes + joins).
//
// The event is a DOORBELL, not the source of truth: on each signal the reloader
// RE-READS the durable grant config and applies THAT, rather than trusting the
// event's payload. This is deliberate — the SQLite bus is durable so a signal is
// not lost, but re-reading the single durable value is what makes two racing
// writers, a reordered delivery, or a serve that booted mid-change all converge
// on the same set. No slow-poll fallback is added: the durable bus does not drop
// events (proven cross-process by the report-routing slice) and serve re-reads
// the config at boot, so the two convergence points — boot and doorbell — cover
// the field without a constant background DB read.
func startWorkspaceRootReloader(ctx context.Context, bus libbus.Messenger, reloader *workspaceRootReloader) (func(), error) {
	ch := make(chan []byte, 8)
	sub, err := bus.Stream(ctx, workspacegrants.RootsChangedSubject, ch)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-runCtx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
				if err := reloader.apply(runCtx); err != nil {
					slog.Warn("contenox serve: workspace-root reload failed; keeping current roots", "error", err)
					continue
				}
				// Audit trail for the cross-process path: a grant made by another
				// process (the `contenox workspace` CLI) that serve just applied live.
				roots := reloader.factory.Roots()
				slog.Info("contenox serve: workspace roots reloaded from doorbell",
					"count", len(roots), "roots", roots)
			}
		}
	}()
	return func() {
		cancel()
		_ = sub.Unsubscribe()
		wg.Wait()
	}, nil
}
