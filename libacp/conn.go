package libacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

var (
	ErrConnectionClosed = errors.New("libacp: connection closed")
)

type AgentSideConnection struct {
	reader *ndjsonReader
	writer *ndjsonWriter
	closer io.Closer

	agent Agent

	pendingMu sync.Mutex
	pending   map[int64]chan *Response

	nextID atomic.Int64

	cancelMu       sync.Mutex
	sessionCancels map[SessionID]context.CancelFunc

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

func NewAgentSideConnection(rw io.ReadWriteCloser, factory AgentFactory) *AgentSideConnection {
	c := &AgentSideConnection{
		reader:         newNDJSONReader(rw),
		writer:         newNDJSONWriter(rw),
		closer:         rw,
		pending:        make(map[int64]chan *Response),
		sessionCancels: make(map[SessionID]context.CancelFunc),
		closed:         make(chan struct{}),
	}
	c.agent = factory(c)
	return c
}

func (c *AgentSideConnection) Run(ctx context.Context) error {
	defer c.shutdown(nil)

	go func() {
		<-ctx.Done()
		c.shutdown(ctx.Err())
	}()

	for {
		line, err := c.reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.shutdown(nil)
				return nil
			}
			c.shutdown(err)
			return err
		}
		c.dispatch(ctx, line)
	}
}

func (c *AgentSideConnection) Closed() <-chan struct{} { return c.closed }

func (c *AgentSideConnection) CloseErr() error {
	<-c.closed
	return c.closeErr
}

func (c *AgentSideConnection) shutdown(err error) {
	c.closeOnce.Do(func() {
		c.closeErr = err
		_ = c.closer.Close()

		c.pendingMu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()

		c.cancelMu.Lock()
		for sid, cancel := range c.sessionCancels {
			cancel()
			delete(c.sessionCancels, sid)
		}
		c.cancelMu.Unlock()

		close(c.closed)
	})
}

func (c *AgentSideConnection) dispatch(ctx context.Context, line []byte) {
	msg, err := ParseIncoming(line)
	if err != nil {
		return
	}
	switch msg.Kind {
	case IncomingKindResponse:
		c.deliverResponse(msg.Response)
	case IncomingKindRequest:
		go c.handleRequest(ctx, msg.Request)
	case IncomingKindNotification:
		go c.handleNotification(ctx, msg.Notification)
	}
}

func (c *AgentSideConnection) handleRequest(ctx context.Context, req Request) {
	result, rpcErr := c.callMethod(ctx, req)
	if rpcErr != nil {
		_ = c.writer.Write(NewErrorResponse(req.ID, rpcErr))
		return
	}
	resultRaw, err := json.Marshal(result)
	if err != nil {
		_ = c.writer.Write(NewErrorResponse(req.ID, InternalError(err.Error())))
		return
	}
	_ = c.writer.Write(NewResultResponse(req.ID, resultRaw))
}

func (c *AgentSideConnection) handleNotification(ctx context.Context, n Notification) {
	switch n.Method {
	case MethodSessionCancel:
		var p CancelNotification
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		c.cancelMu.Lock()
		if cancel, ok := c.sessionCancels[p.SessionID]; ok {
			cancel()
			delete(c.sessionCancels, p.SessionID)
		}
		c.cancelMu.Unlock()
		_ = c.agent.Cancel(ctx, p)
	}
}

func (c *AgentSideConnection) callMethod(ctx context.Context, req Request) (any, *Error) {
	switch req.Method {
	case MethodInitialize:
		var p InitializeRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.Initialize(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodAuthenticate:
		var p AuthenticateRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.Authenticate(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionNew:
		var p NewSessionRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.NewSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionLoad:
		var p LoadSessionRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.LoadSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionList:
		var p ListSessionsRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.ListSessions(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionPrompt:
		var p PromptRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		promptCtx, cancel := context.WithCancel(ctx)
		c.cancelMu.Lock()
		if prev, ok := c.sessionCancels[p.SessionID]; ok {
			prev()
		}
		c.sessionCancels[p.SessionID] = cancel
		c.cancelMu.Unlock()

		resp, err := c.agent.Prompt(promptCtx, p)

		c.cancelMu.Lock()
		if existing, ok := c.sessionCancels[p.SessionID]; ok && fmt.Sprintf("%p", existing) == fmt.Sprintf("%p", cancel) {
			delete(c.sessionCancels, p.SessionID)
		}
		c.cancelMu.Unlock()
		cancel()

		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	default:
		return nil, MethodNotFound(req.Method)
	}
}

func (c *AgentSideConnection) deliverResponse(resp Response) {
	if resp.ID.Kind != RequestIDKindNumber {
		return
	}
	c.pendingMu.Lock()
	ch, ok := c.pending[resp.ID.Number]
	if ok {
		delete(c.pending, resp.ID.Number)
	}
	c.pendingMu.Unlock()
	if !ok {
		return
	}
	ch <- &resp
	close(ch)
}

func (c *AgentSideConnection) call(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)
	rid := NewRequestIDNumber(id)

	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("libacp: marshal %s params: %w", method, err)
		}
		paramsRaw = b
	}

	ch := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.writer.Write(NewRequest(rid, method, paramsRaw)); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return fmt.Errorf("libacp: write %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return ctx.Err()
	case <-c.closed:
		return ErrConnectionClosed
	case resp, ok := <-ch:
		if !ok {
			return ErrConnectionClosed
		}
		if resp.Error != nil {
			return resp.Error
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("libacp: unmarshal %s result: %w", method, err)
		}
		return nil
	}
}

func (c *AgentSideConnection) notify(method string, params any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("libacp: marshal %s params: %w", method, err)
		}
		paramsRaw = b
	}
	return c.writer.Write(NewNotification(method, paramsRaw))
}

func (c *AgentSideConnection) SessionUpdate(n SessionNotification) error {
	return c.notify(MethodSessionUpdate, n)
}

func (c *AgentSideConnection) RequestPermission(ctx context.Context, req RequestPermissionRequest) (RequestPermissionResponse, error) {
	var resp RequestPermissionResponse
	if err := c.call(ctx, MethodSessionRequestPermission, req, &resp); err != nil {
		return RequestPermissionResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) ReadTextFile(ctx context.Context, req ReadTextFileRequest) (ReadTextFileResponse, error) {
	var resp ReadTextFileResponse
	if err := c.call(ctx, MethodFSReadTextFile, req, &resp); err != nil {
		return ReadTextFileResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) WriteTextFile(ctx context.Context, req WriteTextFileRequest) (WriteTextFileResponse, error) {
	var resp WriteTextFileResponse
	if err := c.call(ctx, MethodFSWriteTextFile, req, &resp); err != nil {
		return WriteTextFileResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) CreateTerminal(ctx context.Context, req CreateTerminalRequest) (CreateTerminalResponse, error) {
	var resp CreateTerminalResponse
	if err := c.call(ctx, MethodTerminalCreate, req, &resp); err != nil {
		return CreateTerminalResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) TerminalOutput(ctx context.Context, req TerminalOutputRequest) (TerminalOutputResponse, error) {
	var resp TerminalOutputResponse
	if err := c.call(ctx, MethodTerminalOutput, req, &resp); err != nil {
		return TerminalOutputResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) WaitForTerminalExit(ctx context.Context, req WaitForTerminalExitRequest) (WaitForTerminalExitResponse, error) {
	var resp WaitForTerminalExitResponse
	if err := c.call(ctx, MethodTerminalWaitForExit, req, &resp); err != nil {
		return WaitForTerminalExitResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) KillTerminal(ctx context.Context, req KillTerminalRequest) (KillTerminalResponse, error) {
	var resp KillTerminalResponse
	if err := c.call(ctx, MethodTerminalKill, req, &resp); err != nil {
		return KillTerminalResponse{}, err
	}
	return resp, nil
}

func (c *AgentSideConnection) ReleaseTerminal(ctx context.Context, req ReleaseTerminalRequest) (ReleaseTerminalResponse, error) {
	var resp ReleaseTerminalResponse
	if err := c.call(ctx, MethodTerminalRelease, req, &resp); err != nil {
		return ReleaseTerminalResponse{}, err
	}
	return resp, nil
}
