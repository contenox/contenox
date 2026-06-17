// Command modeld is the contenox model daemon: the per-user, per-data-root
// owner of resident model state. It claims a cross-platform lease, renews it,
// and while it holds the lease it serves the runtime/transport.Service contract
// over gRPC (fenced by the owner instance id) so the runtime can open sessions
// and probe health. The served inference backend is selected at build time
// (-tags 'openvino openvino_genai' or -tags llamanode); a build with no local
// backend still owns the lease and answers health probes.
//
// Usage:
//
//	modeld serve  [--data-root DIR] [--ttl DURATION]
//	modeld status [--data-root DIR] [--json]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/modeld/owner"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "modeld:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("modeld "+cmd, flag.ContinueOnError)
	dataRoot := fs.String("data-root", "", "contenox data root (default ~/.contenox)")
	ttl := fs.Duration("ttl", 30*time.Second, "lease duration; renewed at ttl/3")
	listen := fs.String("listen", "127.0.0.1:0", "gRPC listen address for serve")
	asJSON := fs.Bool("json", false, "machine-readable JSON output (status)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	leasePath, err := resolveLeasePath(*dataRoot)
	if err != nil {
		return err
	}

	switch cmd {
	case "serve":
		return serve(leasePath, *ttl, *listen)
	case "status":
		return status(leasePath, *asJSON)
	default:
		return fmt.Errorf("unknown command %q (want: serve | status)", cmd)
	}
}

func resolveLeasePath(dataRoot string) (string, error) {
	if dataRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
		dataRoot = filepath.Join(home, ".contenox")
	}
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return "", fmt.Errorf("create data root %q: %w", dataRoot, err)
	}
	return filepath.Join(dataRoot, "modeld.lease"), nil
}

func serve(leasePath string, ttl time.Duration, listen string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	lis, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listen %q: %w", listen, err)
	}
	endpoint := lis.Addr().String()

	o, err := owner.Join(ctx, owner.Config{LeasePath: leasePath, TTL: ttl, Endpoint: endpoint})
	if err != nil {
		_ = lis.Close()
		return err
	}
	if !o.IsOwner() {
		_ = lis.Close()
		h := o.Holder()
		fmt.Printf("another modeld owns this data root: instance=%s pid=%d endpoint=%s until=%s\n",
			h.InstanceID, h.PID, h.Meta[owner.EndpointMetaKey], h.ExpiresAt().Format(time.RFC3339))
		return nil
	}
	fmt.Printf("modeld owner started: instance=%s pid=%d endpoint=%s ttl=%s\n", o.InstanceID(), os.Getpid(), endpoint, ttl)

	// Serve the runtime/transport.Service contract over gRPC, fenced by the owner
	// instance id, while we hold the lease. The served backend is selected at
	// build time; a build with no local backend still answers health probes so
	// the runtime's detector reports the daemon running.
	svc, backend := selectBackend()
	fmt.Printf("modeld serving transport: backend=%s\n", backend)
	serveCtx, serveCancel := context.WithCancel(ctx)
	defer serveCancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- transportgrpc.Serve(serveCtx, lis, svc, o.InstanceID()) }()

	select {
	case <-ctx.Done(): // signal -> graceful shutdown
	case <-o.Lost(): // self-fenced: lost the lease, stop touching state
		fmt.Fprintln(os.Stderr, "modeld: lost lease, shutting down:", o.LostErr())
	case err := <-serveErr: // the transport server stopped on its own
		if err != nil {
			return fmt.Errorf("transport server: %w", err)
		}
	}

	// Stop serving (GracefulStop closes the listener), then release the lease so a
	// successor can take over immediately rather than waiting out the TTL.
	serveCancel()
	if err := o.Release(); err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	fmt.Println("modeld owner stopped")
	return nil
}

func status(leasePath string, asJSON bool) error {
	rec, err := liblease.Inspect(leasePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if asJSON {
				fmt.Println(`{"running":false}`)
			} else {
				fmt.Println("no modeld owner (no lease)")
			}
			return nil
		}
		return err
	}
	expired := time.Now().After(rec.ExpiresAt())
	if asJSON {
		out := struct {
			liblease.Record
			Expired bool `json:"expired"`
		}{Record: rec, Expired: expired}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	state := "valid"
	if expired {
		state = "expired (stale)"
	}
	fmt.Printf("modeld lease: instance=%s pid=%d host=%s endpoint=%s expires=%s [%s]\n",
		rec.InstanceID, rec.PID, rec.Host, rec.Meta[owner.EndpointMetaKey], rec.ExpiresAt().Format(time.RFC3339), state)
	return nil
}
