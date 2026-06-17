package grpc

import (
	"context"

	"github.com/contenox/runtime/runtime/transport"
	grpclib "google.golang.org/grpc"
)

// serviceName is the fully-qualified gRPC service for the session contract.
const serviceName = "contenox.transport.v1.Compute"

func method(name string) string { return "/" + serviceName + "/" + name }

// computeServer is the method set the handlers dispatch to. grpc.RegisterService
// requires ServiceDesc.HandlerType to be an interface that the registered impl
// (*Server) satisfies; this is that interface.
type computeServer interface {
	health(context.Context, *healthReq) (*healthResp, error)
	openSession(context.Context, *openSessionReq) (*openSessionResp, error)
	ensurePrefix(context.Context, *ensurePrefixReq) (*transport.PrefixStatus, error)
	prefillSuffix(context.Context, *prefillSuffixReq) (*transport.SuffixStatus, error)
	explainContext(context.Context, *explainReq) (*transport.ContextReport, error)
	closeSession(context.Context, *closeReq) (*closeResp, error)
	decode(context.Context, *decodeReq, grpclib.ServerStream) error
}

// Wire request/response payloads. Status/report responses reuse the transport.*
// types directly (they are JSON-friendly); only stream chunks need a wire form
// because StreamChunk carries an error value.

type openSessionReq struct {
	OwnerInstanceID string           `json:"owner_instance_id,omitempty"`
	ModelID         string           `json:"model_id,omitempty"`
	Config          transport.Config `json:"config"`
}

type openSessionResp struct {
	Handle string `json:"handle"`
}

type ensurePrefixReq struct {
	Handle string                `json:"handle"`
	Prefix transport.PrefixInput `json:"prefix"`
}

type prefillSuffixReq struct {
	Handle string                `json:"handle"`
	Suffix transport.SuffixInput `json:"suffix"`
}

type decodeReq struct {
	Handle string                 `json:"handle"`
	Config transport.DecodeConfig `json:"config"`
}

type explainReq struct {
	Handle string `json:"handle"`
}

type closeReq struct {
	Handle string `json:"handle"`
}

type closeResp struct{}

// healthReq/healthResp back the unfenced liveness probe: it reports which owner
// instance is actually serving so a caller can confirm the lease holder is the
// process answering (and is ready), distinguishing a live owner from a wedged
// one that still holds a fresh lease.
type healthReq struct{}

type healthResp struct {
	InstanceID string `json:"instance_id,omitempty"`
	Ready      bool   `json:"ready"`
}

// wireChunk is the JSON-safe form of transport.StreamChunk (error -> string).
type wireChunk struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// decodeStreamDesc is the client-side stream descriptor for Decode.
var decodeStreamDesc = grpclib.StreamDesc{StreamName: "Decode", ServerStreams: true}

// serviceDesc registers the five unary methods plus the Decode server stream
// against a *Server.
var serviceDesc = grpclib.ServiceDesc{
	ServiceName: serviceName,
	HandlerType: (*computeServer)(nil),
	Methods: []grpclib.MethodDesc{
		{MethodName: "Health", Handler: unaryHandler("Health", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(healthReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.health(ctx, in)
		})},
		{MethodName: "OpenSession", Handler: unaryHandler("OpenSession", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(openSessionReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.openSession(ctx, in)
		})},
		{MethodName: "EnsurePrefix", Handler: unaryHandler("EnsurePrefix", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(ensurePrefixReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.ensurePrefix(ctx, in)
		})},
		{MethodName: "PrefillSuffix", Handler: unaryHandler("PrefillSuffix", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(prefillSuffixReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.prefillSuffix(ctx, in)
		})},
		{MethodName: "ExplainContext", Handler: unaryHandler("ExplainContext", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(explainReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.explainContext(ctx, in)
		})},
		{MethodName: "CloseSession", Handler: unaryHandler("CloseSession", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(closeReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.closeSession(ctx, in)
		})},
	},
	Streams: []grpclib.StreamDesc{
		{
			StreamName:    "Decode",
			ServerStreams: true,
			Handler: func(srv any, stream grpclib.ServerStream) error {
				in := new(decodeReq)
				if err := stream.RecvMsg(in); err != nil {
					return err
				}
				return srv.(*Server).decode(stream.Context(), in, stream)
			},
		},
	},
	Metadata: "contenox/transport",
}

// unaryHandler adapts a typed (*Server, ctx, dec) func to grpc's methodHandler
// signature. Fencing and error mapping live inside the server methods, so no
// unary interceptor is configured and the interceptor argument is unused.
func unaryHandler(_ string, call func(*Server, context.Context, func(any) error) (any, error)) func(any, context.Context, func(any) error, grpclib.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpclib.UnaryServerInterceptor) (any, error) {
		return call(srv.(*Server), ctx, dec)
	}
}
