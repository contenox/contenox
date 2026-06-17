package acpsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	libacp "github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/localtools"
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
