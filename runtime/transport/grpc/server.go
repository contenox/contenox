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
	if gen := sessionGeneration(sess); gen > 0 {
		handle = s.instanceID + "/" + strconv.FormatUint(gen, 10) + "/" + strconv.FormatUint(s.seq, 10)
	}
	s.sessions[handle] = sess
	return handle
}

type generationSession interface {
	SlotGeneration() uint64
}

func sessionGeneration(sess transport.Session) uint64 {
	if gs, ok := sess.(generationSession); ok {
		return gs.SlotGeneration()
	}
	return 0
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

func (s *Server) controller() (transport.ModelController, error) {
	ctrl, ok := s.svc.(transport.ModelController)
	if !ok {
		return nil, transport.ErrUnsupportedFeature
	}
	return ctrl, nil
}

func (s *Server) status(ctx context.Context, _ *statusReq) (*transport.DaemonStatus, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	ctrl, err := s.controller()
	if err != nil {
		return nil, encodeError(err)
	}
	st, err := ctrl.Status(ctx)
	if err != nil {
		return nil, encodeError(err)
	}
	if st.OwnerInstanceID == "" {
		st.OwnerInstanceID = s.instanceID
	}
	if st.Backend == "" {
		st.Backend = s.backend
	}
	return &st, nil
}

func (s *Server) loadModel(ctx context.Context, in *loadModelReq) (*transport.ActiveModel, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	ctrl, err := s.controller()
	if err != nil {
		return nil, encodeError(err)
	}
	owner := in.OwnerInstanceID
	if owner == "" {
		owner = s.instanceID
	}
	active, err := ctrl.LoadModel(ctx, transport.LoadModelRequest{
		Fence:              transport.Fence{OwnerInstanceID: owner},
		ModelName:          in.ModelName,
		Type:               in.Type,
		Digest:             in.Digest,
		Path:               in.Path,
		Config:             in.Config,
		ExpectedGeneration: in.ExpectedGeneration,
	})
	if err != nil {
		return nil, encodeError(err)
	}
	return &active, nil
}

func (s *Server) unloadModel(ctx context.Context, in *unloadModelReq) (*unloadModelResp, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	ctrl, err := s.controller()
	if err != nil {
		return nil, encodeError(err)
	}
	owner := in.OwnerInstanceID
	if owner == "" {
		owner = s.instanceID
	}
	if err := ctrl.UnloadModel(ctx, transport.UnloadModelRequest{
		Fence:              transport.Fence{OwnerInstanceID: owner},
		ExpectedGeneration: in.ExpectedGeneration,
	}); err != nil {
		return nil, encodeError(err)
	}
	return &unloadModelResp{}, nil
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
		Config:    in.Config,
	})
	if err != nil {
		return nil, encodeError(err)
	}
	return &describeResp{
		ModelMaxContext:         info.ModelMaxContext,
		EffectiveContext:        info.EffectiveContext,
		MemoryContextTokens:     info.MemoryContextTokens,
		HotContextTokens:        info.HotContextTokens,
		PlannerEffectiveContext: info.PlannerEffectiveContext,
		KVBytesPerToken:         info.KVBytesPerToken,
		FreeBytes:               info.FreeBytes,
		WeightsBytes:            info.WeightsBytes,
		OverheadBytes:           info.OverheadBytes,
		ReservedBytes:           info.ReservedBytes,
		UserLimitBytes:          info.UserLimitBytes,
		MinFreeBytes:            info.MinFreeBytes,
		HostColdBudgetBytes:     info.HostColdBudgetBytes,
		UsableBytes:             info.UsableBytes,
		RequiredBytes:           info.RequiredBytes,
		Clamped:                 info.Clamped,
		Reason:                  info.Reason,
		DeviceKind:              info.DeviceKind,
		DeviceID:                info.DeviceID,
		DeviceTotalBytes:        info.DeviceTotalBytes,
		SharedWithDisplay:       info.SharedWithDisplay,
		RequestedGpuLayers:      info.RequestedGpuLayers,
		ResolvedGpuLayers:       info.ResolvedGpuLayers,
		RuntimeName:             info.RuntimeName,
		RuntimeDigest:           info.RuntimeDigest,
		RuntimeSystemInfo:       info.RuntimeSystemInfo,
		SupportsGPUOffload:      info.SupportsGPUOffload,
		Devices:                 info.Devices,
	}, nil
}

func (s *Server) embed(ctx context.Context, in *embedReq) (*embedResp, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	res, err := s.svc.Embed(ctx, transport.EmbedRequest{
		Fence:     transport.Fence{OwnerInstanceID: in.OwnerInstanceID},
		ModelName: in.ModelName,
		Type:      in.Type,
		Digest:    in.Digest,
		Path:      in.Path,
		Config:    in.Config,
		Text:      in.Text,
	})
	if err != nil {
		return nil, encodeError(err)
	}
	return &embedResp{Vector: res.Vector}, nil
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

func (s *Server) snapshot(ctx context.Context, in *snapshotReq) (*transport.SessionSnapshot, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	sess, err := s.lookup(in.Handle)
	if err != nil {
		return nil, encodeError(err)
	}
	snap, err := sess.Snapshot(ctx)
	if err != nil {
		return nil, encodeError(err)
	}
	return &snap, nil
}

func (s *Server) restore(ctx context.Context, in *restoreReq) (*restoreResp, error) {
	if err := checkFence(ctx, s.instanceID); err != nil {
		return nil, encodeError(err)
	}
	sess, err := s.lookup(in.Handle)
	if err != nil {
		return nil, encodeError(err)
	}
	if err := sess.Restore(ctx, in.Snapshot); err != nil {
		return nil, encodeError(err)
	}
	return &restoreResp{}, nil
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
		if err := sess.Close(); err != nil {
			return nil, encodeError(err)
		}
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
		w := &wireChunk{Text: chunk.Text, Thinking: chunk.Thinking, ToolCalls: chunk.ToolCalls}
		if chunk.Error != nil {
			w.Error = chunk.Error.Error()
			w.ErrorToken = errorToken(chunk.Error)
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
