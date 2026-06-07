//go:build windows

package terminalservice

import "context"

func (s *service) Create(context.Context, string, CreateRequest) (*CreateResponse, error) {
	return nil, ErrNotImplemented
}
