package grpc

import (
	"context"
	"errors"
	"io"

	"github.com/contenox/runtime/runtime/transport"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client dials a modeld owner endpoint and implements transport.Service over the
// wire. owner is the lease instance id it expects to be serving; it is fenced on
// every call so a client never acts on a stale owner after a takeover.
type Client struct {
	cc    *grpclib.ClientConn
	owner string
}

var _ transport.Service = (*Client)(nil)
var _ transport.ModelController = (*Client)(nil)

// DialLeader connects to the owner's advertised endpoint and fences every call
// with the expected owner instance id resolved from the lease record.
func DialLeader(endpoint, expectedOwner string, opts ...grpclib.DialOption) (*Client, error) {
	base := []grpclib.DialOption{grpclib.WithTransportCredentials(insecure.NewCredentials())}
	cc, err := grpclib.NewClient("passthrough:///"+endpoint, append(base, opts...)...)
	if err != nil {
		return nil, err
	}
	return &Client{cc: cc, owner: expectedOwner}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error { return c.cc.Close() }

// HealthStatus is the result of the unfenced liveness probe.
type HealthStatus struct {
	InstanceID string // the owner instance actually serving this endpoint
	Ready      bool
	Backend    string // inference backend the owner serves ("llama"/"openvino"/"none")
}

// Health pings the owner for liveness. It is not fenced — the caller uses the
// returned InstanceID to confirm the lease holder is the process answering.
func (c *Client) Health(ctx context.Context) (HealthStatus, error) {
	out := new(healthResp)
	if err := c.invoke(ctx, "Health", &healthReq{}, out); err != nil {
		return HealthStatus{}, err
	}
	return HealthStatus{InstanceID: out.InstanceID, Ready: out.Ready, Backend: out.Backend}, nil
}

func (c *Client) invoke(ctx context.Context, name string, in, out any) error {
	ctx = withOwner(ctx, c.owner)
	err := c.cc.Invoke(ctx, method(name), in, out, grpclib.CallContentSubtype(codecName))
	return decodeError(err)
}

func (c *Client) Status(ctx context.Context) (transport.DaemonStatus, error) {
	out := new(transport.DaemonStatus)
	if err := c.invoke(ctx, "Status", &statusReq{OwnerInstanceID: c.owner}, out); err != nil {
		return transport.DaemonStatus{}, err
	}
	return *out, nil
}

func (c *Client) LoadModel(ctx context.Context, req transport.LoadModelRequest) (transport.ActiveModel, error) {
	owner := req.Fence.OwnerInstanceID
	if owner == "" {
		owner = c.owner
	}
	out := new(transport.ActiveModel)
	in := &loadModelReq{
		OwnerInstanceID:    owner,
		ModelName:          req.ModelName,
		Type:               req.Type,
		Digest:             req.Digest,
		Path:               req.Path,
		Config:             req.Config,
		ExpectedGeneration: req.ExpectedGeneration,
	}
	if err := c.invoke(ctx, "LoadModel", in, out); err != nil {
		return transport.ActiveModel{}, err
	}
	return *out, nil
}

func (c *Client) UnloadModel(ctx context.Context, req transport.UnloadModelRequest) error {
	owner := req.Fence.OwnerInstanceID
	if owner == "" {
		owner = c.owner
	}
	return c.invoke(ctx, "UnloadModel", &unloadModelReq{
		OwnerInstanceID:    owner,
		ExpectedGeneration: req.ExpectedGeneration,
	}, new(unloadModelResp))
}

func (c *Client) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	owner := req.Fence.OwnerInstanceID
	if owner == "" {
		owner = c.owner
	}
	out := new(openSessionResp)
	in := &openSessionReq{
		OwnerInstanceID: owner,
		ModelName:       req.ModelName,
		Type:            req.Type,
		Digest:          req.Digest,
		Path:            req.Path,
		Config:          req.Config,
	}
	if err := c.invoke(ctx, "OpenSession", in, out); err != nil {
		return nil, err
	}
	return &grpcSession{client: c, handle: out.Handle}, nil
}

// Describe reports a model's capabilities as read by the daemon from the model
// metadata and capacity policy. The model is identified by req's typed handle
// (Type + Path); Config carries the requested context/runtime knobs for the
// capacity plan.
func (c *Client) Describe(ctx context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	owner := req.Fence.OwnerInstanceID
	if owner == "" {
		owner = c.owner
	}
	out := new(describeResp)
	in := &openSessionReq{
		OwnerInstanceID: owner,
		ModelName:       req.ModelName,
		Type:            req.Type,
		Digest:          req.Digest,
		Path:            req.Path,
		Config:          req.Config,
	}
	if err := c.invoke(ctx, "Describe", in, out); err != nil {
		return transport.ModelInfo{}, err
	}
	return transport.ModelInfo{
		ModelMaxContext:              out.ModelMaxContext,
		EffectiveContext:             out.EffectiveContext,
		MemoryContextTokens:          out.MemoryContextTokens,
		HotContextTokens:             out.HotContextTokens,
		PlannerEffectiveContext:      out.PlannerEffectiveContext,
		KVBytesPerToken:              out.KVBytesPerToken,
		FreeBytes:                    out.FreeBytes,
		WeightsBytes:                 out.WeightsBytes,
		OverheadBytes:                out.OverheadBytes,
		ReservedBytes:                out.ReservedBytes,
		UserLimitBytes:               out.UserLimitBytes,
		MinFreeBytes:                 out.MinFreeBytes,
		HostColdBudgetBytes:          out.HostColdBudgetBytes,
		UsableBytes:                  out.UsableBytes,
		RequiredBytes:                out.RequiredBytes,
		Clamped:                      out.Clamped,
		Reason:                       out.Reason,
		DeviceKind:                   out.DeviceKind,
		DeviceID:                     out.DeviceID,
		DeviceTotalBytes:             out.DeviceTotalBytes,
		SharedWithDisplay:            out.SharedWithDisplay,
		RequestedGpuLayers:           out.RequestedGpuLayers,
		ResolvedGpuLayers:            out.ResolvedGpuLayers,
		SparseAttention:              out.SparseAttention,
		SlidingWindowAttentionTokens: out.SlidingWindowAttentionTokens,
		RuntimeName:                  out.RuntimeName,
		RuntimeDigest:                out.RuntimeDigest,
		RuntimeSystemInfo:            out.RuntimeSystemInfo,
		SupportsGPUOffload:           out.SupportsGPUOffload,
		Devices:                      out.Devices,
	}, nil
}

// Embed computes a one-shot embedding through the owner without opening a
// persistent decode session.
func (c *Client) Embed(ctx context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	owner := req.Fence.OwnerInstanceID
	if owner == "" {
		owner = c.owner
	}
	out := new(embedResp)
	in := &embedReq{
		OwnerInstanceID: owner,
		ModelName:       req.ModelName,
		Type:            req.Type,
		Digest:          req.Digest,
		Path:            req.Path,
		Config:          req.Config,
		Text:            req.Text,
	}
	if err := c.invoke(ctx, "Embed", in, out); err != nil {
		return transport.EmbedResult{}, err
	}
	return transport.EmbedResult{Vector: out.Vector}, nil
}

// grpcSession is the client handle to a session resident in the owner; each
// method is a fenced RPC keyed by the opaque handle.
type grpcSession struct {
	client *Client
	handle string
}

var _ transport.Session = (*grpcSession)(nil)

func (s *grpcSession) EnsurePrefix(ctx context.Context, prefix transport.PrefixInput) (transport.PrefixStatus, error) {
	out := new(transport.PrefixStatus)
	if err := s.client.invoke(ctx, "EnsurePrefix", &ensurePrefixReq{Handle: s.handle, Prefix: prefix}, out); err != nil {
		return transport.PrefixStatus{}, err
	}
	return *out, nil
}

func (s *grpcSession) PrefillSuffix(ctx context.Context, suffix transport.SuffixInput) (transport.SuffixStatus, error) {
	out := new(transport.SuffixStatus)
	if err := s.client.invoke(ctx, "PrefillSuffix", &prefillSuffixReq{Handle: s.handle, Suffix: suffix}, out); err != nil {
		return transport.SuffixStatus{}, err
	}
	return *out, nil
}

func (s *grpcSession) ExplainContext() transport.ContextReport {
	out := new(transport.ContextReport)
	if err := s.client.invoke(context.Background(), "ExplainContext", &explainReq{Handle: s.handle}, out); err != nil {
		// The contract has no error channel here; report the session as closed.
		return transport.ContextReport{Closed: true}
	}
	return *out
}

func (s *grpcSession) Snapshot(ctx context.Context) (transport.SessionSnapshot, error) {
	out := new(transport.SessionSnapshot)
	if err := s.client.invoke(ctx, "Snapshot", &snapshotReq{Handle: s.handle}, out); err != nil {
		return transport.SessionSnapshot{}, err
	}
	return *out, nil
}

func (s *grpcSession) Restore(ctx context.Context, snap transport.SessionSnapshot) error {
	return s.client.invoke(ctx, "Restore", &restoreReq{Handle: s.handle, Snapshot: snap}, new(restoreResp))
}

func (s *grpcSession) Close() error {
	return s.client.invoke(context.Background(), "CloseSession", &closeReq{Handle: s.handle}, new(closeResp))
}

func (s *grpcSession) Decode(ctx context.Context, cfg transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	stream, err := s.client.cc.NewStream(withOwner(ctx, s.client.owner), &decodeStreamDesc, method("Decode"), grpclib.CallContentSubtype(codecName))
	if err != nil {
		return nil, decodeError(err)
	}
	if err := stream.SendMsg(&decodeReq{Handle: s.handle, Config: cfg}); err != nil {
		return nil, decodeError(err)
	}
	if err := stream.CloseSend(); err != nil {
		return nil, decodeError(err)
	}

	out := make(chan transport.StreamChunk, 16)
	go func() {
		defer close(out)
		for {
			var w wireChunk
			if err := stream.RecvMsg(&w); err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				out <- transport.StreamChunk{Error: decodeError(err)}
				return
			}
			chunk := transport.StreamChunk{Text: w.Text, Thinking: w.Thinking, ToolCalls: w.ToolCalls}
			if w.Error != "" {
				chunk.Error = decodeWireError(w.ErrorToken, w.Error)
			}
			out <- chunk
			if w.Error != "" {
				return
			}
		}
	}()
	return out, nil
}
