// Package grpc is the gRPC transport for modeld. It wraps a *grpc.Server and
// binds it to a transport.Service.
package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/transport"
	"github.com/contenox/runtime/modeld/transport/grpc/modeldpb"
	"google.golang.org/grpc"
)

// Server is the gRPC transport for modeld.
type Server struct {
	svc transport.Service
	srv *grpc.Server
}

// NewServer constructs a gRPC server bound to svc.
func NewServer(svc transport.Service, opts ...grpc.ServerOption) *Server {
	s := &Server{
		svc: svc,
		srv: grpc.NewServer(opts...),
	}
	s.register()
	return s
}

// register wires the modeld service handlers onto the gRPC server.
func (s *Server) register() {
	modeldpb.RegisterModelRepoServer(s.srv, handler{svc: s.svc})
}

// Serve accepts connections on lis until Stop is called. It blocks.
func (s *Server) Serve(lis net.Listener) error {
	return s.srv.Serve(lis)
}

// Start binds to addr, serves in the background, and returns the resolved
// listen address (useful with ":0" in tests).
func (s *Server) Start(addr string) (net.Addr, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("modeld/grpc: listen %q: %w", addr, err)
	}
	go func() { _ = s.Serve(lis) }()
	return lis.Addr(), nil
}

// Stop gracefully stops the server, waiting for in-flight RPCs to finish.
func (s *Server) Stop() {
	s.srv.GracefulStop()
}

type handler struct {
	modeldpb.UnimplementedModelRepoServer
	svc transport.Service
}

func (h handler) RegisterBackend(ctx context.Context, req *modeldpb.RegisterBackendRequest) (*modeldpb.RegisterBackendResponse, error) {
	if err := h.svc.RegisterBackend(ctx, req.GetBackendId(), backendSpecFromPB(req.GetSpec())); err != nil {
		return nil, err
	}
	return &modeldpb.RegisterBackendResponse{}, nil
}

func (h handler) RemoveBackend(ctx context.Context, req *modeldpb.RemoveBackendRequest) (*modeldpb.RemoveBackendResponse, error) {
	if err := h.svc.RemoveBackend(ctx, req.GetBackendId()); err != nil {
		return nil, err
	}
	return &modeldpb.RemoveBackendResponse{}, nil
}

func (h handler) ListBackends(ctx context.Context, _ *modeldpb.ListBackendsRequest) (*modeldpb.ListBackendsResponse, error) {
	ids, err := h.svc.ListBackends(ctx)
	if err != nil {
		return nil, err
	}
	return &modeldpb.ListBackendsResponse{BackendIds: ids}, nil
}

func (h handler) ListModels(ctx context.Context, req *modeldpb.ListModelsRequest) (*modeldpb.ListModelsResponse, error) {
	models, err := h.svc.ListModels(ctx, req.GetBackendId())
	if err != nil {
		return nil, err
	}
	out := make([]*modeldpb.ObservedModel, 0, len(models))
	for _, m := range models {
		out = append(out, observedModelToPB(m))
	}
	return &modeldpb.ListModelsResponse{Models: out}, nil
}

func observedModelToPB(m modeld.ObservedModel) *modeldpb.ObservedModel {
	var modified int64
	if !m.ModifiedAt.IsZero() {
		modified = m.ModifiedAt.UnixNano()
	}
	return &modeldpb.ObservedModel{
		Name:               m.Name,
		ContextLength:      int32(m.ContextLength),
		ModifiedAtUnixNano: modified,
		Size:               m.Size,
		Digest:             m.Digest,
		MaxOutputTokens:    int32(m.MaxOutputTokens),
		CanChat:            m.CanChat,
		CanEmbed:           m.CanEmbed,
		CanStream:          m.CanStream,
		CanPrompt:          m.CanPrompt,
		CanThink:           m.CanThink,
		Meta:               m.Meta,
	}
}

func backendSpecToPB(spec modeld.BackendSpec) *modeldpb.BackendSpec {
	return &modeldpb.BackendSpec{
		Type:    spec.Type,
		BaseUrl: spec.BaseURL,
		ApiKey:  spec.APIKey,
	}
}

func backendSpecFromPB(spec *modeldpb.BackendSpec) modeld.BackendSpec {
	if spec == nil {
		return modeld.BackendSpec{}
	}
	return modeld.BackendSpec{
		Type:    spec.Type,
		BaseURL: spec.BaseUrl,
		APIKey:  spec.ApiKey,
	}
}

func observedModelFromPB(m *modeldpb.ObservedModel) modeld.ObservedModel {
	if m == nil {
		return modeld.ObservedModel{}
	}
	var modified time.Time
	if m.ModifiedAtUnixNano != 0 {
		modified = time.Unix(0, m.ModifiedAtUnixNano)
	}
	return modeld.ObservedModel{
		Name:          m.Name,
		ContextLength: int(m.ContextLength),
		ModifiedAt:    modified,
		Size:          m.Size,
		Digest:        m.Digest,
		CapabilityConfig: modeld.CapabilityConfig{
			ContextLength:   int(m.ContextLength),
			MaxOutputTokens: int(m.MaxOutputTokens),
			CanChat:         m.CanChat,
			CanEmbed:        m.CanEmbed,
			CanStream:       m.CanStream,
			CanPrompt:       m.CanPrompt,
			CanThink:        m.CanThink,
		},
		Meta: m.Meta,
	}
}
