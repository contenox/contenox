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

// afterResponseSink collects callbacks scheduled by a request handler to run
// once the handler's result has been written to the wire. handleRequest installs
// one per request and flushes it after the result, so notifications a handler
// emits are ordered AFTER the response.
type afterResponseSink struct {
	mu      sync.Mutex
	flushed bool
	fns     []func()
}

func (s *afterResponseSink) add(fn func()) {
	s.mu.Lock()
	if s.flushed {
		// The handler's result is already on the wire; run immediately instead
		// of appending to a sink that will never be flushed again.
		s.mu.Unlock()
		fn()
		return
	}
	s.fns = append(s.fns, fn)
	s.mu.Unlock()
}

func (s *afterResponseSink) run() {
	s.mu.Lock()
	s.flushed = true
	fns := s.fns
	s.fns = nil
	s.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

type afterResponseKey struct{}

// AfterResponse schedules fn to run after the result of the request currently
// being handled has been written to the wire. Use it from a request handler
// (NewSession, LoadSession, ...) to emit session/update notifications that must
// reach the client only once it can resolve the session — most importantly the
// available_commands_update after session/new, which a client (e.g. Zed) drops
// as an "unknown session" if it arrives before the session/new result.
//
// Called outside a request handler (no sink in ctx), fn runs immediately, so it
// is always safe to use regardless of caller context.
func AfterResponse(ctx context.Context, fn func()) {
	if sink, ok := ctx.Value(afterResponseKey{}).(*afterResponseSink); ok {
		sink.add(fn)
		return
	}
	fn()
}

type AgentSideConnection struct {
	reader *ndjsonReader
	writer *ndjsonWriter
	closer io.Closer

	agent Agent

	pendingMu sync.Mutex
	pending   map[int64]chan *Response

	nextID atomic.Int64

	cancelMu       sync.Mutex
	sessionCancels map[SessionID]*promptCancel

	// requestCancels tracks every in-flight incoming request's cancelable
	// context by JSON-RPC id, so "$/cancel_request" can abort it.
	reqCancelMu    sync.Mutex
	requestCancels map[string]context.CancelFunc

	// extRequest and extNotification, if set (via SetExtRequestHandler /
	// SetExtNotificationHandler), handle inbound extension methods and
	// notifications (see IsExtensionMethod). Nil preserves the connection's
	// behavior before extension support existed: MethodNotFound for requests,
	// silent drop for notifications. Set once from the AgentFactory before Run
	// starts reading, like c.agent itself; no separate synchronization.
	extRequest      ExtRequestHandler
	extNotification ExtNotificationHandler

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

// requestCancelKey disambiguates the JSON-RPC id namespace: a string id "1"
// and a numeric id 1 render identically via String_().
func requestCancelKey(id RequestID) string {
	return fmt.Sprintf("%d:%s", id.Kind, id.String_())
}

// promptCancel tracks the cancelable context of one in-flight session/prompt.
// Entries are compared by pointer identity so a prompt's cleanup can never
// remove a successor prompt's registration (context.CancelFunc values are not
// comparable — printing them with %p yields the shared code pointer, so any
// func-value comparison degenerates to "always equal").
type promptCancel struct {
	sessionID SessionID
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewAgentSideConnection(rw io.ReadWriteCloser, factory AgentFactory) *AgentSideConnection {
	c := &AgentSideConnection{
		reader:         newNDJSONReader(rw),
		writer:         newNDJSONWriter(rw),
		closer:         rw,
		pending:        make(map[int64]chan *Response),
		sessionCancels: make(map[SessionID]*promptCancel),
		requestCancels: make(map[string]context.CancelFunc),
		closed:         make(chan struct{}),
	}
	c.agent = factory(c)
	return c
}

// SetExtRequestHandler installs h to handle inbound extension requests
// (method names starting with ExtensionMethodPrefix that fall outside the
// core ACP method set). Call it from the AgentFactory, before Run starts
// reading. A nil h (the default) leaves extension requests answered with
// MethodNotFound, exactly as before this seam existed.
func (c *AgentSideConnection) SetExtRequestHandler(h ExtRequestHandler) {
	c.extRequest = h
}

// SetExtNotificationHandler installs h to handle inbound extension
// notifications. Call it from the AgentFactory, before Run starts reading. A
// nil h (the default) leaves extension notifications silently ignored,
// exactly as before this seam existed.
func (c *AgentSideConnection) SetExtNotificationHandler(h ExtNotificationHandler) {
	c.extNotification = h
}

func (c *AgentSideConnection) Run(ctx context.Context) error {
	defer c.shutdown(nil)

	go func() {
		select {
		case <-ctx.Done():
			c.shutdown(ctx.Err())
		case <-c.closed:
			// Connection ended on its own (EOF, transport error); exit instead
			// of leaking until the caller's ctx dies.
		}
	}()

	for {
		line, err := c.reader.Next()
		if err != nil {
			// A canceled ctx closes the transport out from under the reader, so
			// the reader's error is a side effect ("file already closed"), not
			// the cause. Report the cancellation itself.
			if ctxErr := ctx.Err(); ctxErr != nil {
				c.shutdown(ctxErr)
				return ctxErr
			}
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
		for sid, pc := range c.sessionCancels {
			pc.cancel()
			delete(c.sessionCancels, sid)
		}
		c.cancelMu.Unlock()

		c.reqCancelMu.Lock()
		for key, cancel := range c.requestCancels {
			cancel()
			delete(c.requestCancels, key)
		}
		c.reqCancelMu.Unlock()

		close(c.closed)
	})
}

func (c *AgentSideConnection) dispatch(ctx context.Context, line []byte) {
	msg, err := ParseIncoming(line)
	if err != nil {
		c.respondToMalformed(line, err)
		return
	}
	switch msg.Kind {
	case IncomingKindResponse:
		c.deliverResponse(msg.Response)
	case IncomingKindRequest:
		// Every request runs under its own cancelable context, registered by
		// JSON-RPC id before the handler goroutine spawns (read-loop order), so
		// a later $/cancel_request is guaranteed to observe it.
		reqCtx, cancelReq := context.WithCancel(ctx)
		key := requestCancelKey(msg.Request.ID)
		c.reqCancelMu.Lock()
		c.requestCancels[key] = cancelReq
		c.reqCancelMu.Unlock()

		var pc *promptCancel
		if msg.Request.Method == MethodSessionPrompt {
			pc = c.registerPromptCancel(reqCtx, msg.Request.Params)
		}
		go func() {
			defer func() {
				c.reqCancelMu.Lock()
				delete(c.requestCancels, key)
				c.reqCancelMu.Unlock()
				cancelReq()
			}()
			c.handleRequest(reqCtx, msg.Request, pc)
		}()
	case IncomingKindNotification:
		// session/cancel and $/cancel_request are applied inline on the read
		// loop so wire order is preserved: a cancel that arrives after its
		// request always observes the request's registration (both happen on
		// this goroutine), instead of racing it across handler goroutines.
		switch msg.Notification.Method {
		case MethodSessionCancel:
			c.applySessionCancel(msg.Notification.Params)
		case MethodCancelRequest:
			c.applyCancelRequest(msg.Notification.Params)
		}
		go c.handleNotification(ctx, msg.Notification)
	}
}

// applyCancelRequest aborts the in-flight request the peer no longer awaits.
// Unknown ids are ignored ("$/" methods are always safe to ignore) — the
// request may simply have completed already.
func (c *AgentSideConnection) applyCancelRequest(params json.RawMessage) {
	var p CancelRequestNotification
	if len(params) == 0 || json.Unmarshal(params, &p) != nil {
		return
	}
	c.reqCancelMu.Lock()
	cancel, ok := c.requestCancels[requestCancelKey(p.RequestID)]
	c.reqCancelMu.Unlock()
	if ok {
		cancel()
	}
}

// respondToMalformed answers input the dispatcher could not parse. Silently
// dropping it would leave a requesting peer waiting forever on a response that
// never comes; JSON-RPC 2.0 prescribes -32700 for invalid JSON and -32600 for
// structurally invalid messages (id null when it cannot be salvaged).
func (c *AgentSideConnection) respondToMalformed(line []byte, parseErr error) {
	if !json.Valid(line) {
		_ = c.writer.Write(NewErrorResponse(NewRequestIDNull(), ParseError(parseErr.Error())))
		return
	}
	id := NewRequestIDNull()
	var probe struct {
		ID *json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(line, &probe); err == nil && probe.ID != nil {
		var rid RequestID
		if rid.UnmarshalJSON(*probe.ID) == nil {
			id = rid
		}
	}
	_ = c.writer.Write(NewErrorResponse(id, InvalidRequest(parseErr.Error())))
}

// registerPromptCancel creates and registers the cancelable context for a
// session/prompt request at dispatch time — on the read loop, before the
// handler goroutine is spawned — so a later session/cancel is guaranteed to
// observe it. Returns nil when params carry no usable sessionId; the handler
// will reject those with InvalidParams anyway.
func (c *AgentSideConnection) registerPromptCancel(ctx context.Context, params json.RawMessage) *promptCancel {
	var probe struct {
		SessionID SessionID `json:"sessionId"`
	}
	if len(params) == 0 || json.Unmarshal(params, &probe) != nil || probe.SessionID == "" {
		return nil
	}
	promptCtx, cancel := context.WithCancel(ctx)
	pc := &promptCancel{sessionID: probe.SessionID, ctx: promptCtx, cancel: cancel}
	c.cancelMu.Lock()
	if prev, ok := c.sessionCancels[probe.SessionID]; ok {
		// Spec-discouraged but possible: a second prompt on a busy session
		// supersedes the first turn.
		prev.cancel()
	}
	c.sessionCancels[probe.SessionID] = pc
	c.cancelMu.Unlock()
	return pc
}

// unregisterPromptCancel removes pc's registration if — and only if — it is
// still the current one for its session (pointer identity), then cancels its
// context to release resources.
func (c *AgentSideConnection) unregisterPromptCancel(pc *promptCancel) {
	c.cancelMu.Lock()
	if existing, ok := c.sessionCancels[pc.sessionID]; ok && existing == pc {
		delete(c.sessionCancels, pc.sessionID)
	}
	c.cancelMu.Unlock()
	pc.cancel()
}

func (c *AgentSideConnection) applySessionCancel(params json.RawMessage) {
	var p CancelNotification
	if len(params) == 0 || json.Unmarshal(params, &p) != nil {
		return
	}
	c.cancelMu.Lock()
	if pc, ok := c.sessionCancels[p.SessionID]; ok {
		pc.cancel()
		delete(c.sessionCancels, p.SessionID)
	}
	c.cancelMu.Unlock()
}

func (c *AgentSideConnection) handleRequest(ctx context.Context, req Request, pc *promptCancel) {
	sink := &afterResponseSink{}
	ctx = context.WithValue(ctx, afterResponseKey{}, sink)

	result, rpcErr := c.safeCallMethod(ctx, req, pc)
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
	// Now that the result (and any session id it carries) is on the wire, flush
	// notifications the handler deferred via AfterResponse. They are ordered
	// after the response, so the client can resolve their session.
	sink.run()
}

// safeCallMethod converts a panicking Agent handler into an InternalError
// response instead of tearing down the whole process (and with it every other
// in-flight session on this connection).
func (c *AgentSideConnection) safeCallMethod(ctx context.Context, req Request, pc *promptCancel) (result any, rpcErr *Error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			rpcErr = InternalError(fmt.Sprintf("panic in %s handler: %v", req.Method, r))
		}
	}()
	return c.callMethod(ctx, req, pc)
}

func (c *AgentSideConnection) handleNotification(ctx context.Context, n Notification) {
	switch n.Method {
	case MethodSessionCancel:
		// The cancel itself was already applied inline by dispatch; this only
		// informs the agent.
		var p CancelNotification
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		_ = c.agent.Cancel(ctx, p)
	default:
		// "$/"-prefixed methods (MethodCancelRequest) never reach here as an
		// extension notification: IsExtensionMethod only matches the "_"
		// namespace, so they fall through and stay ignored, same as any other
		// unrecognized notification.
		if IsExtensionMethod(n.Method) && c.extNotification != nil {
			c.extNotification(ctx, n.Method, n.Params)
		}
	}
}

func (c *AgentSideConnection) callMethod(ctx context.Context, req Request, pc *promptCancel) (any, *Error) {
	params := req.Params
	if len(params) == 0 {
		// JSON-RPC allows omitting params entirely; treat that as {} so methods
		// whose params are all optional (session/list) don't fail to unmarshal.
		params = []byte("{}")
	}
	switch req.Method {
	case MethodInitialize:
		var p InitializeRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.Initialize(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodAuthenticate:
		var p AuthenticateRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.Authenticate(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodLogout:
		var p LogoutRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.Logout(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionNew:
		var p NewSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.NewSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionLoad:
		var p LoadSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.LoadSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionResume:
		var p ResumeSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.ResumeSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionClose:
		var p CloseSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.CloseSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionDelete:
		var p DeleteSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.DeleteSession(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionList:
		var p ListSessionsRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.ListSessions(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionSetMode:
		var p SetSessionModeRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.SetSessionMode(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionSetConfigOption:
		var p SetSessionConfigOptionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, InvalidParams(err.Error())
		}
		resp, err := c.agent.SetSessionConfigOption(ctx, p)
		if err != nil {
			return nil, AsError(err)
		}
		return resp, nil

	case MethodSessionPrompt:
		var p PromptRequest
		if err := json.Unmarshal(params, &p); err != nil {
			if pc != nil {
				c.unregisterPromptCancel(pc)
			}
			return nil, InvalidParams(err.Error())
		}
		// The cancelable context was registered by dispatch (read-loop order);
		// pc is nil only when the params were unusable, which the unmarshal
		// above already rejects for the sessionId-less case.
		promptCtx := ctx
		if pc != nil {
			promptCtx = pc.ctx
			defer c.unregisterPromptCancel(pc)
		}

		resp, err := c.agent.Prompt(promptCtx, p)
		if err != nil {
			// Spec: after session/cancel the prompt MUST resolve with the
			// cancelled stop reason, never a JSON-RPC error. Agents that return
			// their context's error are translated here as a safety net.
			if promptCtx.Err() == context.Canceled && errors.Is(err, context.Canceled) {
				return PromptResponse{StopReason: StopReasonCancelled}, nil
			}
			return nil, AsError(err)
		}
		return resp, nil

	default:
		// req.Params (not the []byte("{}") default above) so the handler sees
		// exactly what arrived on the wire, including a genuinely absent
		// params field — extension methods own their own params schema.
		if IsExtensionMethod(req.Method) && c.extRequest != nil {
			result, extErr := c.extRequest(ctx, req.Method, req.Params)
			if extErr != nil {
				return nil, extErr
			}
			return result, nil
		}
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
		// Tell the peer this response is no longer awaited so it can abort the
		// work — most visibly, tear down a permission dialog whose prompt turn
		// was cancelled. Best effort by design.
		_ = c.notify(MethodCancelRequest, CancelRequestNotification{RequestID: rid})
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

// CallExtMethod sends a custom extension request (method must satisfy
// IsExtensionMethod) to the client and returns its raw result. This is the
// outbound half of the extension-method seam; SetExtRequestHandler installs
// the inbound half. A canceled ctx aborts the wait and best-effort notifies
// the client with "$/cancel_request", exactly like any other outbound call
// (see call).
func (c *AgentSideConnection) CallExtMethod(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if !IsExtensionMethod(method) {
		return nil, fmt.Errorf("libacp: %q is not an extension method (must start with %q)", method, ExtensionMethodPrefix)
	}
	var paramsAny any
	if len(params) > 0 {
		paramsAny = params
	}
	var result json.RawMessage
	if err := c.call(ctx, method, paramsAny, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SendExtNotification sends a custom, fire-and-forget extension notification
// (method must satisfy IsExtensionMethod) to the client.
func (c *AgentSideConnection) SendExtNotification(method string, params json.RawMessage) error {
	if !IsExtensionMethod(method) {
		return fmt.Errorf("libacp: %q is not an extension method (must start with %q)", method, ExtensionMethodPrefix)
	}
	var paramsAny any
	if len(params) > 0 {
		paramsAny = params
	}
	return c.notify(method, paramsAny)
}
