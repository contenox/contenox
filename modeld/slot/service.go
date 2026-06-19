// Package slot enforces modeld's single active local model invariant.
package slot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/contenox/runtime/runtime/transport"
)

// Service wraps a backend transport.Service and exposes it as one active model
// slot. The wrapped backend still owns native inference; this layer owns
// residency cardinality, generation fencing, and switch/unload safety.
type Service struct {
	backend transport.Service

	owner       string
	backendName string

	op chan struct{}
	mu sync.Mutex

	active     *activeSlot
	generation uint64
	state      transport.SlotState
	busyOp     string
	lastErr    string
}

type activeSlot struct {
	req        transport.OpenSessionRequest
	sess       transport.Session
	generation uint64
	refs       int
	explicit   bool
}

// Option configures a slot Service.
type Option func(*Service)

// WithOwner records the owner instance expected on direct calls. The gRPC layer
// also fences requests; this keeps direct tests and future in-process callers
// honest.
func WithOwner(owner string) Option {
	return func(s *Service) { s.owner = owner }
}

// WithBackend records the backend mode for Status.
func WithBackend(backend string) Option {
	return func(s *Service) { s.backendName = backend }
}

// New returns a transport service that allows only one active resident model.
func New(backend transport.Service, opts ...Option) *Service {
	s := &Service{backend: backend, op: make(chan struct{}, 1), state: transport.SlotEmpty}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

var _ transport.Service = (*Service)(nil)
var _ transport.ModelController = (*Service)(nil)

func (s *Service) Status(ctx context.Context) (transport.DaemonStatus, error) {
	if err := ctxErr(ctx); err != nil {
		return transport.DaemonStatus{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(), nil
}

func (s *Service) LoadModel(ctx context.Context, req transport.LoadModelRequest) (transport.ActiveModel, error) {
	if err := ctxErr(ctx); err != nil {
		return transport.ActiveModel{}, err
	}
	if err := s.checkFence(req.Fence.OwnerInstanceID); err != nil {
		return transport.ActiveModel{}, err
	}
	openReq := transport.OpenSessionRequest{
		Fence:     req.Fence,
		ModelName: req.ModelName,
		Type:      req.Type,
		Digest:    req.Digest,
		Path:      req.Path,
		Config:    req.Config,
	}

	unlock, err := s.lockOperation(ctx)
	if err != nil {
		return transport.ActiveModel{}, err
	}
	defer unlock()
	if err := ctxErr(ctx); err != nil {
		return transport.ActiveModel{}, err
	}

	s.mu.Lock()
	if req.ExpectedGeneration != 0 && req.ExpectedGeneration != s.generation {
		s.mu.Unlock()
		return transport.ActiveModel{}, transport.ErrSlotGenerationStale
	}
	if s.active != nil && sameIdentity(s.active.req, openReq) {
		s.active.explicit = true
		active := activeModel(s.active.req, s.active.generation)
		s.state = transport.SlotReady
		s.busyOp = ""
		s.lastErr = ""
		s.mu.Unlock()
		return active, nil
	}
	if s.active != nil && s.active.refs > 0 {
		s.mu.Unlock()
		return transport.ActiveModel{}, transport.ErrModelBusy
	}
	old := s.active
	if old != nil {
		s.active = nil
		s.generation++
		s.state = transport.SlotSwitching
		s.busyOp = "switch"
	} else {
		s.state = transport.SlotLoading
		s.busyOp = "load"
	}
	s.lastErr = ""
	s.mu.Unlock()

	if old != nil {
		if err := old.sess.Close(); err != nil {
			return transport.ActiveModel{}, s.finishCloseError(err)
		}
	}

	sess, err := s.backend.OpenSession(ctx, openReq)
	if err != nil {
		if sess != nil {
			_ = sess.Close()
		}
		return transport.ActiveModel{}, s.finishLoadError(loadError(err))
	}

	s.mu.Lock()
	s.generation++
	gen := s.generation
	s.active = &activeSlot{req: openReq, sess: sess, generation: gen, explicit: true}
	s.state = transport.SlotReady
	s.busyOp = ""
	s.lastErr = ""
	active := activeModel(openReq, gen)
	s.mu.Unlock()
	return active, nil
}

func (s *Service) UnloadModel(ctx context.Context, req transport.UnloadModelRequest) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if err := s.checkFence(req.Fence.OwnerInstanceID); err != nil {
		return err
	}

	unlock, err := s.lockOperation(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctxErr(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	if req.ExpectedGeneration != 0 && req.ExpectedGeneration != s.generation {
		s.mu.Unlock()
		return transport.ErrSlotGenerationStale
	}
	if s.active == nil {
		s.state = transport.SlotEmpty
		s.busyOp = ""
		s.lastErr = ""
		s.mu.Unlock()
		return nil
	}
	if s.active.refs > 0 {
		s.mu.Unlock()
		return transport.ErrModelBusy
	}
	old := s.active
	s.active = nil
	s.generation++
	s.state = transport.SlotUnloading
	s.busyOp = "unload"
	s.lastErr = ""
	s.mu.Unlock()

	err = old.sess.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.state = transport.SlotFailed
		s.busyOp = ""
		s.lastErr = err.Error()
		return err
	}
	s.state = transport.SlotEmpty
	s.busyOp = ""
	s.lastErr = ""
	return nil
}

func (s *Service) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if err := s.checkFence(req.Fence.OwnerInstanceID); err != nil {
		return nil, err
	}

	unlock, err := s.lockOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.active != nil {
		switch {
		case sameIdentity(s.active.req, req):
			if s.active.refs > 0 {
				s.mu.Unlock()
				return nil, transport.ErrModelBusy
			}
			s.active.refs = 1
			s.state = transport.SlotReady
			s.busyOp = ""
			s.lastErr = ""
			gen := s.active.generation
			s.mu.Unlock()
			return &slotSession{svc: s, generation: gen}, nil
		case s.active.refs > 0:
			s.mu.Unlock()
			return nil, transport.ErrModelBusy
		default:
			s.mu.Unlock()
			return nil, transport.ErrModelSwitchRequired
		}
	}
	s.state = transport.SlotLoading
	s.busyOp = "load"
	s.lastErr = ""
	s.mu.Unlock()

	sess, err := s.backend.OpenSession(ctx, req)
	if err != nil {
		if sess != nil {
			_ = sess.Close()
		}
		return nil, s.finishLoadError(loadError(err))
	}

	s.mu.Lock()
	s.generation++
	gen := s.generation
	s.active = &activeSlot{req: req, sess: sess, generation: gen, refs: 1}
	s.state = transport.SlotReady
	s.busyOp = ""
	s.lastErr = ""
	s.mu.Unlock()
	return &slotSession{svc: s, generation: gen}, nil
}

func (s *Service) Describe(ctx context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	if err := ctxErr(ctx); err != nil {
		return transport.ModelInfo{}, err
	}
	if err := s.checkFence(req.Fence.OwnerInstanceID); err != nil {
		return transport.ModelInfo{}, err
	}
	return s.backend.Describe(ctx, req)
}

func (s *Service) Embed(ctx context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	if err := ctxErr(ctx); err != nil {
		return transport.EmbedResult{}, err
	}
	if err := s.checkFence(req.Fence.OwnerInstanceID); err != nil {
		return transport.EmbedResult{}, err
	}
	unlock, err := s.lockOperation(ctx)
	if err != nil {
		return transport.EmbedResult{}, err
	}
	defer unlock()
	if err := ctxErr(ctx); err != nil {
		return transport.EmbedResult{}, err
	}
	s.mu.Lock()
	if s.active != nil {
		s.mu.Unlock()
		return transport.EmbedResult{}, transport.ErrModelBusy
	}
	s.state = transport.SlotBusy
	s.busyOp = "embed"
	s.mu.Unlock()
	res, err := s.backend.Embed(ctx, req)
	s.markReadyOrError(err)
	return res, err
}

func (s *Service) lockOperation(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case s.op <- struct{}{}:
		return func() { <-s.op }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func loadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %w", transport.ErrModelLoadFailed, err)
}

func (s *Service) finishLoadError(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busyOp = ""
	s.lastErr = err.Error()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		s.state = transport.SlotEmpty
		return err
	}
	s.state = transport.SlotFailed
	return err
}

func (s *Service) finishCloseError(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = transport.SlotFailed
	s.busyOp = ""
	s.lastErr = err.Error()
	return err
}

func (s *Service) statusLocked() transport.DaemonStatus {
	state := s.state
	if state == "" {
		state = transport.SlotEmpty
	}
	st := transport.DaemonStatus{
		OwnerInstanceID: s.owner,
		Backend:         s.backendName,
		State:           state,
		BusyOperation:   s.busyOp,
		LastError:       s.lastErr,
	}
	if s.active != nil {
		active := activeModel(s.active.req, s.active.generation)
		st.Active = &active
	}
	return st
}

func (s *Service) checkFence(owner string) error {
	if s.owner == "" {
		return nil
	}
	if owner != s.owner {
		return transport.ErrStaleFence
	}
	return nil
}

func (s *Service) release(gen uint64) error {
	unlock, err := s.lockOperation(context.Background())
	if err != nil {
		return err
	}
	defer unlock()

	s.mu.Lock()
	if s.active == nil || s.active.generation != gen {
		s.mu.Unlock()
		return nil
	}
	if s.active.refs > 0 {
		s.active.refs--
	}
	if s.active.refs > 0 || s.active.explicit {
		s.state = transport.SlotReady
		s.busyOp = ""
		s.mu.Unlock()
		return nil
	}
	old := s.active
	s.active = nil
	s.generation++
	s.state = transport.SlotUnloading
	s.busyOp = "unload"
	s.lastErr = ""
	s.mu.Unlock()

	err = old.sess.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.state = transport.SlotFailed
		s.busyOp = ""
		s.lastErr = err.Error()
		return err
	}
	s.state = transport.SlotEmpty
	s.busyOp = ""
	s.lastErr = ""
	return nil
}

func (s *Service) withSession(ctx context.Context, gen uint64, op string, fn func(transport.Session) error) error {
	unlock, err := s.lockOperation(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctxErr(ctx); err != nil {
		return err
	}

	sess, err := s.beginOperation(gen, op)
	if err != nil {
		return err
	}
	err = fn(sess)
	s.markReadyOrError(err)
	return err
}

func (s *Service) beginOperation(gen uint64, op string) (transport.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		if gen != 0 && gen != s.generation {
			return nil, transport.ErrSlotGenerationStale
		}
		return nil, transport.ErrModelNotActive
	}
	if s.active.generation != gen {
		return nil, transport.ErrSlotGenerationStale
	}
	s.state = transport.SlotBusy
	s.busyOp = op
	s.lastErr = ""
	return s.active.sess, nil
}

func (s *Service) markReadyOrError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busyOp = ""
	if err != nil && !errors.Is(err, context.Canceled) {
		s.lastErr = err.Error()
		if errors.Is(err, transport.ErrSessionClosed) || errors.Is(err, transport.ErrSlotGenerationStale) {
			s.state = transport.SlotFailed
			return
		}
	}
	if s.active == nil {
		s.state = transport.SlotEmpty
		return
	}
	s.state = transport.SlotReady
}

func sameIdentity(a, b transport.OpenSessionRequest) bool {
	return a.ModelName == b.ModelName &&
		a.Type == b.Type &&
		a.Digest == b.Digest &&
		a.Path == b.Path &&
		reflect.DeepEqual(a.Config, b.Config)
}

func activeModel(req transport.OpenSessionRequest, gen uint64) transport.ActiveModel {
	return transport.ActiveModel{
		ModelName:  req.ModelName,
		Type:       req.Type,
		Digest:     req.Digest,
		Path:       req.Path,
		Config:     req.Config,
		Generation: gen,
	}
}

type slotSession struct {
	svc        *Service
	generation uint64

	mu     sync.Mutex
	closed bool
}

var _ transport.Session = (*slotSession)(nil)

func (s *slotSession) SlotGeneration() uint64 { return s.generation }

func (s *slotSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *slotSession) EnsurePrefix(ctx context.Context, prefix transport.PrefixInput) (transport.PrefixStatus, error) {
	if s.isClosed() {
		return transport.PrefixStatus{}, transport.ErrSessionClosed
	}
	var out transport.PrefixStatus
	err := s.svc.withSession(ctx, s.generation, "ensure_prefix", func(sess transport.Session) error {
		var err error
		out, err = sess.EnsurePrefix(ctx, prefix)
		return err
	})
	return out, err
}

func (s *slotSession) PrefillSuffix(ctx context.Context, suffix transport.SuffixInput) (transport.SuffixStatus, error) {
	if s.isClosed() {
		return transport.SuffixStatus{}, transport.ErrSessionClosed
	}
	var out transport.SuffixStatus
	err := s.svc.withSession(ctx, s.generation, "prefill_suffix", func(sess transport.Session) error {
		var err error
		out, err = sess.PrefillSuffix(ctx, suffix)
		return err
	})
	return out, err
}

func (s *slotSession) Decode(ctx context.Context, cfg transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	if s.isClosed() {
		return nil, transport.ErrSessionClosed
	}
	unlock, err := s.svc.lockOperation(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctxErr(ctx); err != nil {
		unlock()
		return nil, err
	}
	sess, err := s.svc.beginOperation(s.generation, "decode")
	if err != nil {
		unlock()
		return nil, err
	}
	src, err := sess.Decode(ctx, cfg)
	if err != nil {
		s.svc.markReadyOrError(err)
		unlock()
		return nil, err
	}
	out := make(chan transport.StreamChunk, 16)
	go func() {
		defer unlock()
		defer close(out)
		var streamErr error
		for chunk := range src {
			if chunk.Error != nil {
				streamErr = chunk.Error
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				streamErr = ctx.Err()
				s.svc.markReadyOrError(streamErr)
				return
			}
		}
		s.svc.markReadyOrError(streamErr)
	}()
	return out, nil
}

func (s *slotSession) ExplainContext() transport.ContextReport {
	if s.isClosed() {
		return transport.ContextReport{Closed: true}
	}
	var out transport.ContextReport
	err := s.svc.withSession(context.Background(), s.generation, "explain_context", func(sess transport.Session) error {
		out = sess.ExplainContext()
		return nil
	})
	if err != nil {
		return transport.ContextReport{Closed: true}
	}
	return out
}

func (s *slotSession) Snapshot(ctx context.Context) (transport.SessionSnapshot, error) {
	if s.isClosed() {
		return transport.SessionSnapshot{}, transport.ErrSessionClosed
	}
	var out transport.SessionSnapshot
	err := s.svc.withSession(ctx, s.generation, "snapshot", func(sess transport.Session) error {
		var err error
		out, err = sess.Snapshot(ctx)
		return err
	})
	return out, err
}

func (s *slotSession) Restore(ctx context.Context, snap transport.SessionSnapshot) error {
	if s.isClosed() {
		return transport.ErrSessionClosed
	}
	return s.svc.withSession(ctx, s.generation, "restore", func(sess transport.Session) error {
		return sess.Restore(ctx, snap)
	})
}

func (s *slotSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.svc.release(s.generation)
}
