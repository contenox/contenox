package grpc

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/contenox/runtime/runtime/transport"
	grpclib "google.golang.org/grpc"
)

// Server adapts a transport.Service to the gRPC wire. It is generic: modeld
// supplies the concrete (CGO-backed) Service and the owner instance id from the
// lease. Sessions live here keyed by an opaque handle that encodes the owner
// instance, so a handle minted by a previous owner is rejected after takeover.
type Server struct {
	svc        transport.Service
	instanceID string
	backend    string

	mu       sync.Mutex
	seq      uint64
	sessions map[string]transport.Session
}

// NewServer wraps a transport.Service. instanceID is the owner's lease instance
// id used for fencing; pass "" to disable fencing (the unwired/local path).
// backend is the served inference backend ("llama"/"openvino"/"none"/"") echoed
// on the health probe so the runtime learns the daemon's mode over the wire.
func NewServer(svc transport.Service, instanceID, backend string) *Server {
	return &Server{svc: svc, instanceID: instanceID, backend: backend, sessions: map[string]transport.Session{}}
}

// Register mounts the service on a gRPC server.
func (s *Server) Register(reg grpclib.ServiceRegistrar) { reg.RegisterService(&serviceDesc, s) }

// Serve runs a gRPC server for svc on lis until ctx is cancelled, then stops it
// gracefully. This is the entry point modeld calls once it holds the lease.
func Serve(ctx context.Context, lis net.Listener, svc transport.Service, instanceID, backend string) error {
	gs := grpclib.NewServer()
	NewServer(svc, instanceID, backend).Register(gs)
	go func() {
		<-ctx.Done()
		gs.GracefulStop()
	}()
	return gs.Serve(lis)
}

func (s *Server) register(sess transport.Session) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	handle := s.instanceID + "/" + strconv.FormatUint(s.seq, 10)
	s.sessions[handle] = sess
	return handle
}

// lookup returns the live session for a handle. A handle minted by a different
// owner is a stale fence; an unknown handle for this owner is a closed session.
func (s *Server) lookup(handle string) (transport.Session, error) {
	if s.instanceID != "" && !strings.HasPrefix(handle, s.instanceID+"/") {
		return nil, transport.ErrStaleFence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[handle]
	if !ok {
		return nil, transport.ErrSessionClosed
	}
	return sess, nil
}

// health is the unfenced liveness probe: it answers regardless of the caller's
// owner token so a detector can learn which instance is actually serving (and
// compare it against the lease holder).
func (s *Server) health(_ context.Context, _ *healthReq) (*healthResp, error) {
	return &healthResp{InstanceID: s.instanceID, Ready: true, Backend: s.backend}, nil
}

func (s *Server) openSession(ctx context.Context, in *openSessionReq) (*openSessionResp, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	sess, err := s.svc.OpenSession(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: in.OwnerInstanceID},
		ModelName: in.ModelName,
		Type:      in.Type,
		Digest:    in.Digest,
		Path:      in.Path,
		Config:    in.Config,
	})
	if err != nil {
		return nil, encodeError(err)
	}
	return &openSessionResp{Handle: s.register(sess)}, nil
}

func (s *Server) describe(ctx context.Context, in *openSessionReq) (*describeResp, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	info, err := s.svc.Describe(ctx, transport.OpenSessionRequest{
		Fence:     transport.Fence{OwnerInstanceID: in.OwnerInstanceID},
		ModelName: in.ModelName,
		Type:      in.Type,
		Digest:    in.Digest,
		Path:      in.Path,
	})
	if err != nil {
		return nil, encodeError(err)
	}
	return &describeResp{
		ModelMaxContext:  info.ModelMaxContext,
		EffectiveContext: info.EffectiveContext,
		KVBytesPerToken:  info.KVBytesPerToken,
		FreeBytes:        info.FreeBytes,
		WeightsBytes:     info.WeightsBytes,
	}, nil
}

func (s *Server) ensurePrefix(ctx context.Context, in *ensurePrefixReq) (*transport.PrefixStatus, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	sess, err := s.lookup(in.Handle)
	if err != nil {
		return nil, encodeError(err)
	}
	st, err := sess.EnsurePrefix(ctx, in.Prefix)
	if err != nil {
		return nil, encodeError(err)
	}
	return &st, nil
}

func (s *Server) prefillSuffix(ctx context.Context, in *prefillSuffixReq) (*transport.SuffixStatus, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	sess, err := s.lookup(in.Handle)
	if err != nil {
		return nil, encodeError(err)
	}
	st, err := sess.PrefillSuffix(ctx, in.Suffix)
	if err != nil {
		return nil, encodeError(err)
	}
	return &st, nil
}

func (s *Server) explainContext(ctx context.Context, in *explainReq) (*transport.ContextReport, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	sess, err := s.lookup(in.Handle)
	if err != nil {
		return nil, encodeError(err)
	}
	report := sess.ExplainContext()
	return &report, nil
}

func (s *Server) closeSession(ctx context.Context, in *closeReq) (*closeResp, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	s.mu.Lock()
	sess := s.sessions[in.Handle]
	delete(s.sessions, in.Handle)
	s.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
	return &closeResp{}, nil
}

func (s *Server) decode(ctx context.Context, in *decodeReq, stream grpclib.ServerStream) error {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return encodeError(err)
	}
	sess, err := s.lookup(in.Handle)
	if err != nil {
		return encodeError(err)
	}
	ch, err := sess.Decode(ctx, in.Config)
	if err != nil {
		return encodeError(err)
	}
	for chunk := range ch {
		w := &wireChunk{Text: chunk.Text}
		if chunk.Error != nil {
			w.Error = chunk.Error.Error()
		}
		if err := stream.SendMsg(w); err != nil {
			return err
		}
		if chunk.Error != nil {
			return nil
		}
	}
	return nil
}
