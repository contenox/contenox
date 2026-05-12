package libacp

import "context"

type Agent interface {
	Initialize(ctx context.Context, req InitializeRequest) (InitializeResponse, error)
	Authenticate(ctx context.Context, req AuthenticateRequest) (AuthenticateResponse, error)
	NewSession(ctx context.Context, req NewSessionRequest) (NewSessionResponse, error)
	LoadSession(ctx context.Context, req LoadSessionRequest) (LoadSessionResponse, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResponse, error)
	Prompt(ctx context.Context, req PromptRequest) (PromptResponse, error)
	Cancel(ctx context.Context, req CancelNotification) error
}

type AgentFactory func(conn *AgentSideConnection) Agent

type UnimplementedAgent struct{}

func (UnimplementedAgent) Initialize(context.Context, InitializeRequest) (InitializeResponse, error) {
	return InitializeResponse{}, MethodNotFound(MethodInitialize)
}
func (UnimplementedAgent) Authenticate(context.Context, AuthenticateRequest) (AuthenticateResponse, error) {
	return AuthenticateResponse{}, MethodNotFound(MethodAuthenticate)
}
func (UnimplementedAgent) NewSession(context.Context, NewSessionRequest) (NewSessionResponse, error) {
	return NewSessionResponse{}, MethodNotFound(MethodSessionNew)
}
func (UnimplementedAgent) LoadSession(context.Context, LoadSessionRequest) (LoadSessionResponse, error) {
	return LoadSessionResponse{}, MethodNotFound(MethodSessionLoad)
}
func (UnimplementedAgent) ListSessions(context.Context, ListSessionsRequest) (ListSessionsResponse, error) {
	return ListSessionsResponse{}, MethodNotFound(MethodSessionList)
}
func (UnimplementedAgent) Prompt(context.Context, PromptRequest) (PromptResponse, error) {
	return PromptResponse{}, MethodNotFound(MethodSessionPrompt)
}
func (UnimplementedAgent) Cancel(context.Context, CancelNotification) error {
	return nil
}
