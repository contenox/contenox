package acpsvc

import (
	"context"

	libacp "github.com/contenox/contenox/libacp"
)

const terminalAuthMethodID = "terminal"

func (t *Transport) Authenticate(_ context.Context, req libacp.AuthenticateRequest) (libacp.AuthenticateResponse, error) {
	if req.MethodID == terminalAuthMethodID && clientSupportsTerminalAuth(t.getClientCaps()) {
		return libacp.AuthenticateResponse{}, nil
	}
	return libacp.AuthenticateResponse{}, libacp.NewErrorf(libacp.ErrInvalidParams, "auth method %q is not supported; use the terminal method to run --setup", req.MethodID)
}
