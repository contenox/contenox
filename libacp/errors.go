package libacp

import (
	"encoding/json"
	"fmt"
)

const (
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternalError  = -32603

	ErrAuthRequired = -32000
)

type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil libacp.Error>"
	}
	return fmt.Sprintf("libacp: rpc error %d: %s", e.Code, e.Message)
}

func NewError(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

func NewErrorf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func ParseError(msg string) *Error     { return NewError(ErrParseError, msg) }
func InvalidRequest(msg string) *Error { return NewError(ErrInvalidRequest, msg) }
func MethodNotFound(method string) *Error {
	return NewError(ErrMethodNotFound, "method not found: "+method)
}
func InvalidParams(msg string) *Error { return NewError(ErrInvalidParams, msg) }
func InternalError(msg string) *Error { return NewError(ErrInternalError, msg) }

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return InternalError(err.Error())
}
