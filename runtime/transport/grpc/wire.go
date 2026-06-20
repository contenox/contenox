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
	status(context.Context, *statusReq) (*transport.DaemonStatus, error)
	loadModel(context.Context, *loadModelReq) (*transport.ActiveModel, error)
	unloadModel(context.Context, *unloadModelReq) (*unloadModelResp, error)
	openSession(context.Context, *openSessionReq) (*openSessionResp, error)
	describe(context.Context, *openSessionReq) (*describeResp, error)
	embed(context.Context, *embedReq) (*embedResp, error)
	ensurePrefix(context.Context, *ensurePrefixReq) (*transport.PrefixStatus, error)
	prefillSuffix(context.Context, *prefillSuffixReq) (*transport.SuffixStatus, error)
	explainContext(context.Context, *explainReq) (*transport.ContextReport, error)
	snapshot(context.Context, *snapshotReq) (*transport.SessionSnapshot, error)
	restore(context.Context, *restoreReq) (*restoreResp, error)
	closeSession(context.Context, *closeReq) (*closeResp, error)
	decode(context.Context, *decodeReq, grpclib.ServerStream) error
}

// Wire request/response payloads. Status/report responses reuse the transport.*
// types directly (they are JSON-friendly); only stream chunks need a wire form
// because StreamChunk carries an error value.

type openSessionReq struct {
	OwnerInstanceID string           `json:"owner_instance_id,omitempty"`
	ModelName       string           `json:"model_name,omitempty"`
	Type            string           `json:"type,omitempty"`
	Digest          string           `json:"digest,omitempty"`
	Path            string           `json:"path,omitempty"`
	Config          transport.Config `json:"config"`
}

type openSessionResp struct {
	Handle string `json:"handle"`
}

type statusReq struct {
	OwnerInstanceID string `json:"owner_instance_id,omitempty"`
}

type loadModelReq struct {
	OwnerInstanceID    string           `json:"owner_instance_id,omitempty"`
	ModelName          string           `json:"model_name,omitempty"`
	Type               string           `json:"type,omitempty"`
	Digest             string           `json:"digest,omitempty"`
	Path               string           `json:"path,omitempty"`
	Config             transport.Config `json:"config"`
	ExpectedGeneration uint64           `json:"expected_generation,omitempty"`
}

type unloadModelReq struct {
	OwnerInstanceID    string `json:"owner_instance_id,omitempty"`
	ExpectedGeneration uint64 `json:"expected_generation,omitempty"`
}

type unloadModelResp struct{}

// describeReq reuses the open-session request shape: Type + Path identify the
// model, and Config carries the requested context/runtime knobs for capacity
// planning. describeResp carries the model capabilities the daemon read from
// model metadata plus the capacity decision.
type describeResp struct {
	ModelMaxContext         int                    `json:"model_max_context"`
	EffectiveContext        int                    `json:"effective_context"`
	MemoryContextTokens     int                    `json:"memory_context_tokens,omitempty"`
	HotContextTokens        int                    `json:"hot_context_tokens,omitempty"`
	PlannerEffectiveContext int                    `json:"planner_effective_context,omitempty"`
	KVBytesPerToken         int64                  `json:"kv_bytes_per_token,omitempty"`
	FreeBytes               int64                  `json:"free_bytes,omitempty"`
	WeightsBytes            int64                  `json:"weights_bytes,omitempty"`
	OverheadBytes           int64                  `json:"overhead_bytes,omitempty"`
	ReservedBytes           int64                  `json:"reserved_bytes,omitempty"`
	UserLimitBytes          int64                  `json:"user_limit_bytes,omitempty"`
	MinFreeBytes            int64                  `json:"min_free_bytes,omitempty"`
	UsableBytes             int64                  `json:"usable_bytes,omitempty"`
	RequiredBytes           int64                  `json:"required_bytes,omitempty"`
	Clamped                 bool                   `json:"clamped,omitempty"`
	Reason                  string                 `json:"reason,omitempty"`
	DeviceKind              string                 `json:"device_kind,omitempty"`
	DeviceID                string                 `json:"device_id,omitempty"`
	DeviceTotalBytes        int64                  `json:"device_total_bytes,omitempty"`
	SharedWithDisplay       bool                   `json:"shared_with_display,omitempty"`
	RequestedGpuLayers      int                    `json:"requested_gpu_layers,omitempty"`
	ResolvedGpuLayers       int                    `json:"resolved_gpu_layers,omitempty"`
	RuntimeName             string                 `json:"runtime_name,omitempty"`
	RuntimeDigest           string                 `json:"runtime_digest,omitempty"`
	RuntimeSystemInfo       string                 `json:"runtime_system_info,omitempty"`
	SupportsGPUOffload      bool                   `json:"supports_gpu_offload,omitempty"`
	Devices                 []transport.DeviceInfo `json:"devices,omitempty"`
}

type embedReq struct {
	OwnerInstanceID string           `json:"owner_instance_id,omitempty"`
	ModelName       string           `json:"model_name,omitempty"`
	Type            string           `json:"type,omitempty"`
	Digest          string           `json:"digest,omitempty"`
	Path            string           `json:"path,omitempty"`
	Config          transport.Config `json:"config"`
	Text            string           `json:"text"`
}

type embedResp struct {
	Vector []float32 `json:"vector,omitempty"`
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

type snapshotReq struct {
	Handle string `json:"handle"`
}

type restoreReq struct {
	Handle   string                    `json:"handle"`
	Snapshot transport.SessionSnapshot `json:"snapshot"`
}

type restoreResp struct{}

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
	Backend    string `json:"backend,omitempty"`
}

// wireChunk is the JSON-safe form of transport.StreamChunk (error -> string).
type wireChunk struct {
	Text       string               `json:"text,omitempty"`
	Thinking   string               `json:"thinking,omitempty"`
	ToolCalls  []transport.ToolCall `json:"tool_calls,omitempty"`
	Error      string               `json:"error,omitempty"`
	ErrorToken string               `json:"error_token,omitempty"`
}

// decodeStreamDesc is the client-side stream descriptor for Decode.
var decodeStreamDesc = grpclib.StreamDesc{StreamName: "Decode", ServerStreams: true}

// serviceDesc registers the unary methods plus the Decode server stream against
// a *Server.
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
		{MethodName: "Status", Handler: unaryHandler("Status", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(statusReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.status(ctx, in)
		})},
		{MethodName: "LoadModel", Handler: unaryHandler("LoadModel", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(loadModelReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.loadModel(ctx, in)
		})},
		{MethodName: "UnloadModel", Handler: unaryHandler("UnloadModel", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(unloadModelReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.unloadModel(ctx, in)
		})},
		{MethodName: "OpenSession", Handler: unaryHandler("OpenSession", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(openSessionReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.openSession(ctx, in)
		})},
		{MethodName: "Describe", Handler: unaryHandler("Describe", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(openSessionReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.describe(ctx, in)
		})},
		{MethodName: "Embed", Handler: unaryHandler("Embed", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(embedReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.embed(ctx, in)
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
		{MethodName: "Snapshot", Handler: unaryHandler("Snapshot", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(snapshotReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.snapshot(ctx, in)
		})},
		{MethodName: "Restore", Handler: unaryHandler("Restore", func(s *Server, ctx context.Context, dec func(any) error) (any, error) {
			in := new(restoreReq)
			if err := dec(in); err != nil {
				return nil, err
			}
			return s.restore(ctx, in)
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
