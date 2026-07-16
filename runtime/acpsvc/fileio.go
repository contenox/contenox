package acpsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	libacp "github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

var osFallback = localtools.NewOSFileIO()

type acpFileIO struct {
	transport func() *Transport
}

func NewACPFileIO(transport func() *Transport) localtools.FileIO {
	return &acpFileIO{transport: transport}
}

func (a *acpFileIO) ReadFile(ctx context.Context, path string) ([]byte, error) {
	t := a.transport()
	if t == nil {
		return nil, errors.New("acpsvc: no active ACP transport")
	}
	if !t.getClientCaps().FS.ReadTextFile {
		return osFallback.ReadFile(ctx, path)
	}
	req := libacp.ReadTextFileRequest{Path: path}
	if sid := resolveACPSessionID(ctx, t); sid != "" {
		req.SessionID = sid
	}
	resp, err := t.conn.ReadTextFile(ctx, req)
	if err != nil {
		return nil, mapACPNotExist(err)
	}
	return []byte(resp.Content), nil
}

func (a *acpFileIO) WriteFile(ctx context.Context, path string, data []byte) error {
	t := a.transport()
	if t == nil {
		return errors.New("acpsvc: no active ACP transport")
	}
	if !t.getClientCaps().FS.WriteTextFile {
		return osFallback.WriteFile(ctx, path, data)
	}
	req := libacp.WriteTextFileRequest{Path: path, Content: string(data)}
	if sid := resolveACPSessionID(ctx, t); sid != "" {
		req.SessionID = sid
	}
	if _, err := t.conn.WriteTextFile(ctx, req); err != nil {
		return mapACPNotExist(err)
	}
	return nil
}

func mapACPNotExist(err error) error {
	if err == nil {
		return nil
	}
	var e *libacp.Error
	if errors.As(err, &e) {
		if e.Code == libacp.ErrResourceNotFound || strings.Contains(strings.ToLower(e.Message), "not found") {
			return fmt.Errorf("%w: %v", os.ErrNotExist, err)
		}
		return err
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return fmt.Errorf("%w: %v", os.ErrNotExist, err)
	}
	return err
}

func NewACPCwdResolver(transport func() *Transport) func(context.Context) string {
	return func(ctx context.Context) string {
		t := transport()
		if t == nil {
			return ""
		}
		internalID := sessionIDFromCtx(ctx)
		if internalID == "" {
			return ""
		}
		t.sessionMu.Lock()
		defer t.sessionMu.Unlock()
		for _, entry := range t.sessions {
			if entry.InternalSessionID == internalID && entry.Cwd != "" {
				return entry.Cwd
			}
		}
		return ""
	}
}

// NewServeCwdResolver returns the cwd resolver for the serve path, where a
// single shared local_fs tool is consulted by many per-connection transports —
// so it cannot close over one transport the way the stdio path does. Instead it
// resolves the session's persisted workspace cwd from the database (keyed by the
// internal session id in ctx), falling back to defaultRoot when the session has
// none, its stored cwd is the legacy "/" sentinel, or there is no session in
// scope. The stored cwd is already validated against the allowlist at
// session/new time, so this read is trusted; defaultRoot is the Factory default.
func NewServeCwdResolver(db libdb.DBManager, defaultRoot string) func(context.Context) string {
	return func(ctx context.Context) string {
		if db == nil {
			return defaultRoot
		}
		internalID := sessionIDFromCtx(ctx)
		if internalID == "" {
			return defaultRoot
		}
		cwd := serveSessionCwd(ctx, db, internalID)
		if cwd == "" || cwd == "/" {
			return defaultRoot
		}
		return cwd
	}
}

// serveSessionCwd maps an internal session id to its persisted workspace cwd:
// message_indices.name is the ACP session id, under which persistSessionCwd
// stores the cwd in the KV store.
func serveSessionCwd(ctx context.Context, db libdb.DBManager, internalID string) string {
	exec := db.WithoutTransaction()
	var name string
	if err := exec.QueryRowContext(ctx,
		`SELECT name FROM message_indices WHERE id = $1`, internalID,
	).Scan(&name); err != nil || name == "" {
		return ""
	}
	var rec sessionCwdRecord
	if err := runtimetypes.New(exec).GetKV(ctx, acpSessionCwdKVPrefix+name, &rec); err != nil {
		return ""
	}
	return rec.Cwd
}
