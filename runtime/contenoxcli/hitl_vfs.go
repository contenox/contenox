package contenoxcli

import (
	"context"
	"errors"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/runtime/vfsservice"
)

type layeredHITLVFS struct {
	vfsservice.Service
	fallback vfsservice.Service
}

func newLayeredHITLVFS(primary vfsservice.Service) vfsservice.Service {
	homeDir, err := globalContenoxDir()
	if err != nil {
		return primary
	}
	return &layeredHITLVFS{
		Service:  primary,
		fallback: vfsservice.NewLocalFS(homeDir),
	}
}

func (l *layeredHITLVFS) GetFileByID(ctx context.Context, id string) (*vfsservice.File, error) {
	f, err := l.Service.GetFileByID(ctx, id)
	if err == nil {
		return f, nil
	}
	if errors.Is(err, libdb.ErrNotFound) && l.fallback != nil {
		return l.fallback.GetFileByID(ctx, id)
	}
	return nil, err
}
