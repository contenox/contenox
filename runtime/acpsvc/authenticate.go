package acpsvc

import (
	"context"

	libacp "github.com/contenox/contenox/libacp"
)

func (t *Transport) Authenticate(_ context.Context, req libacp.AuthenticateRequest) (libacp.AuthenticateResponse, error) {
	return libacp.AuthenticateResponse{}, libacp.NewErrorf(libacp.ErrInvalidParams, "auth method %q is not supported; use the terminal method to run --setup", req.MethodID)
}
