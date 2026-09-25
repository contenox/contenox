package modeldconn

import (
	"context"
	"fmt"
	"sync"

	"github.com/contenox/contenox/internal/models/modeldprobe"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
)

// LocalSentinel is the backend BaseURL value meaning "this modeld backend row
// is the local daemon" — reach it via the lease (LocalEndpointAddr), not a
// stored address.
const LocalSentinel = "local"

// LocalEndpointAddr resolves the local modeld owner's current advertised
// endpoint address via the lease. Every local-model operation funnels through
// here, so it is also the last-resort autostart seam: if no daemon is running
// (a host that ensures eagerly is gone, or a flow that never ran the
// host-level ensure), the daemon is started on first use. A failed autostart
// falls through to the probe error, which states the actual condition.
func LocalEndpointAddr(ctx context.Context) (string, error) {
	st := detector().Probe(ctx)
	if st.State != modeldprobe.StateRunning {
		_ = EnsureDaemon(ctx)
		st = detector().Probe(ctx)
		if st.State != modeldprobe.StateRunning {
			return "", st.Err()
		}
	}
	return st.Endpoint, nil
}

// EndpointHealth reports what a node's Health RPC returned, cached alongside
// its connection.
type EndpointHealth struct {
	InstanceID string
	Backend    string // "llama" | "openvino" | "none"
	Ready      bool
}

// EndpointClient is a fenced connection to one modeld node (local or remote),
// reached by address rather than lease discovery.
type EndpointClient struct {
	*transportgrpc.Client
	EndpointHealth
}

type endpointEntry struct {
	client   *transportgrpc.Client
	instance string
}

var (
	endpointsMu sync.Mutex
	endpoints   = map[string]*endpointEntry{}
)

// Endpoint dials (or reuses a cached connection to) the modeld node
// identified by backendID at addr.
func Endpoint(ctx context.Context, backendID, addr string) (EndpointClient, error) {
	if entry, ok := cachedEndpoint(backendID); ok {
		if health, err := entry.client.Health(ctx); err == nil && health.Ready && health.InstanceID == entry.instance {
			return EndpointClient{Client: entry.client, EndpointHealth: EndpointHealth{
				InstanceID: health.InstanceID, Backend: health.Backend, Ready: health.Ready,
			}}, nil
		}
		dropEndpoint(backendID, entry.client)
	}

	probe, err := transportgrpc.DialLeader(addr, "")
	if err != nil {
		return EndpointClient{}, fmt.Errorf("modeldconn: dial %s: %w", addr, err)
	}
	health, err := probe.Health(ctx)
	if err != nil {
		_ = probe.Close()
		return EndpointClient{}, fmt.Errorf("modeldconn: health probe %s: %w", addr, err)
	}
	if !health.Ready || health.InstanceID == "" {
		_ = probe.Close()
		return EndpointClient{}, fmt.Errorf("modeldconn: node %s reported not ready", addr)
	}

	_ = probe.Close()
	fenced, err := transportgrpc.DialLeader(addr, health.InstanceID)
	if err != nil {
		return EndpointClient{}, fmt.Errorf("modeldconn: dial %s: %w", addr, err)
	}

	endpointsMu.Lock()
	endpoints[backendID] = &endpointEntry{client: fenced, instance: health.InstanceID}
	endpointsMu.Unlock()

	return EndpointClient{Client: fenced, EndpointHealth: EndpointHealth{
		InstanceID: health.InstanceID, Backend: health.Backend, Ready: health.Ready,
	}}, nil
}

func cachedEndpoint(backendID string) (*endpointEntry, bool) {
	endpointsMu.Lock()
	defer endpointsMu.Unlock()
	entry, ok := endpoints[backendID]
	return entry, ok
}

func dropEndpoint(backendID string, stale *transportgrpc.Client) {
	endpointsMu.Lock()
	entry, ok := endpoints[backendID]
	if ok && entry.client == stale {
		delete(endpoints, backendID)
	}
	endpointsMu.Unlock()
	_ = stale.Close()
}

// CloseEndpoint drops and closes a cached endpoint connection.
func CloseEndpoint(backendID string) {
	endpointsMu.Lock()
	entry, ok := endpoints[backendID]
	delete(endpoints, backendID)
	endpointsMu.Unlock()
	if ok {
		_ = entry.client.Close()
	}
}
