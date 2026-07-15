package libacp

const ProtocolVersion = 1

const (
	MethodInitialize   = "initialize"
	MethodAuthenticate = "authenticate"

	MethodSessionNew             = "session/new"
	MethodSessionLoad            = "session/load"
	MethodSessionResume          = "session/resume"
	MethodSessionClose           = "session/close"
	MethodSessionDelete          = "session/delete"
	MethodSessionList            = "session/list"
	MethodSessionPrompt          = "session/prompt"
	MethodSessionCancel          = "session/cancel"
	MethodSessionUpdate          = "session/update"
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
