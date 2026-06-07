//go:build windows

package terminalservice

import (
	"context"
	"io"
)

func (s *service) Attach(context.Context, string, string, io.ReadWriteCloser, <-chan ResizeMsg) error {
	return ErrNotImplemented
}
