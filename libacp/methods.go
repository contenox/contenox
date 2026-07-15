package libacp

import (
	"context"
	"encoding/json"
	"strings"
)

const ProtocolVersion = 1

const (
	MethodInitialize   = "initialize"
	MethodAuthenticate = "authenticate"
	MethodLogout       = "logout"

	MethodSessionNew             = "session/new"
	MethodSessionLoad            = "session/load"
	MethodSessionResume          = "session/resume"
	MethodSessionClose           = "session/close"
	MethodSessionDelete          = "session/delete"
	MethodSessionList            = "session/list"
	MethodSessionPrompt          = "session/prompt"
	MethodSessionCancel          = "session/cancel"
	MethodSessionUpdate          = "session/update"
	MethodSessionSetMode         = "session/set_mode"
	MethodSessionSetConfigOption = "session/set_config_option"

	MethodSessionRequestPermission = "session/request_permission"

	// MethodCancelRequest is the protocol-level "$/cancel_request"
	// notification: either side may signal it no longer awaits the response to
	// an in-flight request. "$/"-prefixed methods are always safe to ignore.
	MethodCancelRequest = "$/cancel_request"

	MethodFSReadTextFile  = "fs/read_text_file"
	MethodFSWriteTextFile = "fs/write_text_file"

	MethodTerminalCreate      = "terminal/create"
	MethodTerminalOutput      = "terminal/output"
	MethodTerminalWaitForExit = "terminal/wait_for_exit"
	MethodTerminalKill        = "terminal/kill"
	MethodTerminalRelease     = "terminal/release"
)

// ExtensionMethodPrefix is the reserved namespace for custom "extension"
// methods and notifications. Per extensibility.mdx: "The protocol reserves
// any method name starting with an underscore (_) for custom extensions."
// "$/"-prefixed methods (MethodCancelRequest) are a separate, protocol-owned
// namespace and are never extension-eligible.
const ExtensionMethodPrefix = "_"

// IsExtensionMethod reports whether method is eligible for dispatch through
// an ExtRequestHandler/ExtNotificationHandler: non-empty and starting with
// ExtensionMethodPrefix.
func IsExtensionMethod(method string) bool {
	return strings.HasPrefix(method, ExtensionMethodPrefix)
}

// ExtRequestHandler handles an inbound extension request: a JSON-RPC request
// whose method is not part of the core ACP method set but is extension-
// eligible (IsExtensionMethod). params is the request's raw, unparsed params
// exactly as received (nil if the request omitted them) — extension methods
// define their own wire schema, so libacp does not attempt to interpret them.
// A handler returns either a raw JSON result or an *Error, mirroring how a
// core method handler returns (response, error): both are written back
// through the same JSON-RPC result/error machinery, and the request
// participates in "$/cancel_request" cancellation via ctx like any other
// inbound request.
//
// See ExtRequest/ExtResponse in the ACP schema (v1) and protocol docs:
// https://agentclientprotocol.com/protocol/extensibility
type ExtRequestHandler func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *Error)

// ExtNotificationHandler handles an inbound extension notification —
// fire-and-forget, matching the spec's "implementations SHOULD ignore
// unrecognized notifications" for anything it doesn't recognize itself.
//
// See ExtNotification in the ACP schema (v1) and protocol docs:
// https://agentclientprotocol.com/protocol/extensibility
type ExtNotificationHandler func(ctx context.Context, method string, params json.RawMessage)
