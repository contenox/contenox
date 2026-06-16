// Command modeld is the contenox model daemon: the per-user, per-data-root
// owner of resident model state. This first cut establishes single-owner
// lifecycle only — it claims a cross-platform lease, renews it, and shuts down
// cleanly. The wire transport that serves the modelrepo API mounts in a later
// phase (see modeld/transport); until then the process owns the lease and the
// in-process Daemon but exposes no API.
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
	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/owner"
	"github.com/contenox/runtime/modeld/transport"
	modeldgrpc "github.com/contenox/runtime/modeld/transport/grpc"
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

	daemon := modeld.Default()
	grpcServer := modeldgrpc.NewServer(transport.FromDaemon(daemon), modeldgrpc.WithOwnerToken(o.InstanceID()))
	serveErr := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			serveErr <- err
		}
		close(serveErr)
	}()

	var runErr error
	select {
	case <-ctx.Done(): // signal -> graceful shutdown
	case <-o.Lost(): // self-fenced: lost the lease, stop touching state
		fmt.Fprintln(os.Stderr, "modeld: lost lease, shutting down:", o.LostErr())
	case err := <-serveErr:
		if err != nil {
			runErr = fmt.Errorf("serve grpc: %w", err)
		}
	}

	// Drain backend resources, then release the lease so a successor can take
	// over immediately rather than waiting out the TTL.
	grpcServer.Stop()
	if err := daemon.Stop(); err != nil {
		fmt.Fprintln(os.Stderr, "modeld: shutdown hooks:", err)
	}
	if err := o.Release(); err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	if runErr != nil {
		return runErr
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
