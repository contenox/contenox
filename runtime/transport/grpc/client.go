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
			chunk := transport.StreamChunk{Text: w.Text}
			if w.Error != "" {
				chunk.Error = errors.New(w.Error)
			}
			out <- chunk
			if w.Error != "" {
				return
			}
		}
	}()
	return out, nil
}
