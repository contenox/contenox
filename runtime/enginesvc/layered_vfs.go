package enginesvc

import (
	"context"
	"errors"

	libdb "github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/runtime/vfsservice"
)

type layeredVFS struct {
	vfsservice.Service
	fallback vfsservice.Service
}

func newLayered(primary, fallback vfsservice.Service) vfsservice.Service {
	return &layeredVFS{Service: primary, fallback: fallback}
}

func (l *layeredVFS) GetFileByID(ctx context.Context, tenantID, id string) (*vfsservice.File, error) {
	f, err := l.Service.GetFileByID(ctx, tenantID, id)
	if err == nil {
		return f, nil
	}
	if errors.Is(err, libdb.ErrNotFound) && l.fallback != nil {
		return l.fallback.GetFileByID(ctx, tenantID, id)
	}
	return nil, err
}
