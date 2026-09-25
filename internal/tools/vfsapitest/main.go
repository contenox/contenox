// Command vfsapitest serves the VFS HTTP surface for the apitests in
// internal/apitests, and for anyone who wants to poke at the surface without
// standing up a relay.
//
// It is a host, not a product: SQLite in a file, the access list's callbacks
// wired into the VFS, and an actor read from a header instead of a session.
// A real deployment substitutes its own [vfsapi.Authenticator] and its own
// database; everything below it is the same surface either way.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/vfsapi"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
)

// actorHeader carries the acting identity. Tests set it; nothing else does.
const actorHeader = "X-Actor"

type headerActor struct{}

func (headerActor) Actor(r *http.Request) (vfs.Actor, error) {
	raw := strings.TrimSpace(r.Header.Get(actorHeader))
	if raw == "" {
		return vfs.Actor{}, vfsapi.ErrNoActor
	}
	var actor vfs.Actor
	if err := json.Unmarshal([]byte(raw), &actor); err != nil {
		return vfs.Actor{}, fmt.Errorf("vfsapitest: malformed %s header: %w", actorHeader, err)
	}
	return actor, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "address to listen on; port 0 picks a free one")
	dbPath := flag.String("db", "", "SQLite file for the VFS; empty uses a file in the temp dir")
	prefix := flag.String("prefix", "/v1/vfs", "path prefix the surface is mounted under")
	flag.Parse()

	ctx := context.Background()
	path := *dbPath
	if path == "" {
		dir, err := os.MkdirTemp("", "vfsapitest-")
		if err != nil {
			log.Fatalf("vfsapitest: temp dir: %v", err)
		}
		path = filepath.Join(dir, "vfs.db")
	}

	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	if err != nil {
		log.Fatalf("vfsapitest: open %s: %v", path, err)
	}
	defer db.Close()

	acl, err := vfs.NewACL(ctx, db)
	if err != nil {
		log.Fatalf("vfsapitest: access list: %v", err)
	}
	store, err := vfs.New(ctx, db, acl.Callbacks(vfs.WithCallbackTracker(libtracker.NoopTracker{})))
	if err != nil {
		log.Fatalf("vfsapitest: vfs: %v", err)
	}
	api, err := vfsapi.New(vfsapi.Config{VFS: store, ACL: acl, Actor: headerActor{}})
	if err != nil {
		log.Fatalf("vfsapitest: surface: %v", err)
	}

	sub := http.NewServeMux()
	api.AddRoutes(sub)

	root := http.NewServeMux()
	root.Handle(*prefix+"/", http.StripPrefix(*prefix, sub))
	root.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// The apitests need a clean store per test; a truncate through the API is
	// the honest way to get one without reaching into the file.
	root.HandleFunc("POST /__reset", func(w http.ResponseWriter, r *http.Request) {
		if err := reset(r.Context(), db); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("vfsapitest: listen on %s: %v", *addr, err)
	}
	// The port is printed so a client that asked for 0 can find it.
	fmt.Printf("vfsapitest listening on http://%s%s\n", listener.Addr().String(), *prefix)
	os.Stdout.Sync()
	if err := http.Serve(listener, root); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("vfsapitest: serve: %v", err)
	}
}

func reset(ctx context.Context, db libdb.DBManager) error {
	exec := db.WithoutTransaction()
	for _, table := range []string{"vfs_files", "vfs_filestree", "vfs_blobs", "vfs_access_lists"} {
		if _, err := exec.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	return nil
}
