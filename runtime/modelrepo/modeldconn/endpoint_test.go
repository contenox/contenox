package modeldconn

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/modeld/slot"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// fakeNode is a real gRPC-served modeld-shaped server (slot.Service wrapping
// a MemoryService) on a loopback port — the same shape production modeld
// serves, so Endpoint's Health-probe-then-fence dial sequence exercises the
// real wire path rather than a mock.
type fakeNode struct {
	addr     string
	instance string
	cancel   context.CancelFunc
}

func startFakeNode(t *testing.T, addr, instance, backend, modelsDir string) *fakeNode {
	t.Helper()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err)
	}
	svc := slot.New(
		transport.NewMemoryService(transport.WithOwnerFence(instance)),
		slot.WithOwner(instance),
		slot.WithBackend(backend),
		slot.WithModelsDir(modelsDir),
	)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = transportgrpc.Serve(ctx, lis, svc, instance, backend) }()
	t.Cleanup(cancel)
	return &fakeNode{addr: lis.Addr().String(), instance: instance, cancel: cancel}
}

func TestUnit_Endpoint_DialsHealthProbesAndCaches(t *testing.T) {
	node := startFakeNode(t, "127.0.0.1:0", "instance-a", "llama", t.TempDir())
	t.Cleanup(func() { CloseEndpoint("backend-1") })

	ec, err := Endpoint(context.Background(), "backend-1", node.addr)
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if ec.InstanceID != "instance-a" || ec.Backend != "llama" || !ec.Ready {
		t.Fatalf("EndpointHealth = %+v, want instance-a/llama/ready", ec.EndpointHealth)
	}

	// The embedded *transportgrpc.Client's NodeAdmin/ModelController methods
	// must be reachable through EndpointClient by promotion.
	if _, err := ec.ListModels(context.Background()); err != nil {
		t.Fatalf("ListModels via EndpointClient: %v", err)
	}
	if _, err := ec.Status(context.Background()); err != nil {
		t.Fatalf("Status via EndpointClient: %v", err)
	}

	// A second call for the same backend must succeed via the cached
	// connection (still healthy, same instance) without error.
	ec2, err := Endpoint(context.Background(), "backend-1", node.addr)
	if err != nil {
		t.Fatalf("second Endpoint call: %v", err)
	}
	if ec2.InstanceID != "instance-a" {
		t.Fatalf("cached EndpointHealth = %+v, want instance-a", ec2.EndpointHealth)
	}
}

func TestUnit_Endpoint_DifferentBackendsGetIndependentConnections(t *testing.T) {
	nodeA := startFakeNode(t, "127.0.0.1:0", "instance-a", "llama", t.TempDir())
	nodeB := startFakeNode(t, "127.0.0.1:0", "instance-b", "openvino", t.TempDir())
	t.Cleanup(func() { CloseEndpoint("backend-a"); CloseEndpoint("backend-b") })

	ecA, err := Endpoint(context.Background(), "backend-a", nodeA.addr)
	if err != nil {
		t.Fatalf("Endpoint A: %v", err)
	}
	ecB, err := Endpoint(context.Background(), "backend-b", nodeB.addr)
	if err != nil {
		t.Fatalf("Endpoint B: %v", err)
	}
	if ecA.Backend != "llama" || ecB.Backend != "openvino" {
		t.Fatalf("A=%+v B=%+v, want distinct backends untangled", ecA.EndpointHealth, ecB.EndpointHealth)
	}

	// Dialing B must not have disturbed A's cached connection: A must still work.
	if _, err := ecA.Status(context.Background()); err != nil {
		t.Fatalf("Status on A after dialing B: %v", err)
	}
}

func TestUnit_Endpoint_RedialsAfterNodeRestartWithNewInstance(t *testing.T) {
	nodeA := startFakeNode(t, "127.0.0.1:0", "instance-a", "llama", t.TempDir())
	t.Cleanup(func() { CloseEndpoint("backend-1") })

	first, err := Endpoint(context.Background(), "backend-1", nodeA.addr)
	if err != nil {
		t.Fatalf("Endpoint (first): %v", err)
	}
	if first.InstanceID != "instance-a" {
		t.Fatalf("first InstanceID = %q, want instance-a", first.InstanceID)
	}

	// Simulate a daemon restart on the exact same address: stop node A, start
	// a fresh one bound to the identical addr with a different instance id.
	nodeA.cancel()
	waitForPortFree(t, nodeA.addr)
	nodeB := startFakeNode(t, nodeA.addr, "instance-b", "openvino", t.TempDir())

	second, err := Endpoint(context.Background(), "backend-1", nodeB.addr)
	if err != nil {
		t.Fatalf("Endpoint (after restart): %v", err)
	}
	if second.InstanceID != "instance-b" || second.Backend != "openvino" {
		t.Fatalf("after restart EndpointHealth = %+v, want instance-b/openvino (stale cache not detected)", second.EndpointHealth)
	}
}

func TestUnit_Endpoint_UnreachableAddrReturnsError(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close() // nothing listens here now

	_, err = Endpoint(context.Background(), "backend-dead", addr)
	if err == nil {
		t.Fatal("Endpoint against a dead address = nil error, want failure")
	}
}

func TestUnit_CloseEndpoint_DropsCacheWithoutPanicking(t *testing.T) {
	node := startFakeNode(t, "127.0.0.1:0", "instance-a", "llama", t.TempDir())

	if _, err := Endpoint(context.Background(), "backend-1", node.addr); err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	CloseEndpoint("backend-1")
	CloseEndpoint("backend-1") // idempotent: closing twice must not panic

	// A fresh Endpoint call after close must redial cleanly.
	if _, err := Endpoint(context.Background(), "backend-1", node.addr); err != nil {
		t.Fatalf("Endpoint after CloseEndpoint: %v", err)
	}
	CloseEndpoint("backend-1")
}

// LocalEndpointAddr reads the SAME lease file the local hot-path dial()
// reads (via modeldprobe); a real listener is required so Probe's
// reachability check succeeds.
func TestUnit_LocalEndpointAddr_FromLiveLease(t *testing.T) {
	dir := t.TempDir()
	SetDataRoot(dir)
	t.Cleanup(func() { SetDataRoot("") })

	node := startFakeNode(t, "127.0.0.1:0", "instance-local", "llama", t.TempDir())

	lease, err := liblease.Acquire(
		filepath.Join(dir, "modeld.lease"), 30*time.Second,
		liblease.WithMeta(map[string]string{"backend": "llama", "endpoint": node.addr}),
		func(r *liblease.Record) { r.InstanceID = "instance-local" },
	)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })

	addr, err := LocalEndpointAddr(context.Background())
	if err != nil {
		t.Fatalf("LocalEndpointAddr: %v", err)
	}
	if addr != node.addr {
		t.Fatalf("LocalEndpointAddr = %q, want %q", addr, node.addr)
	}

	// The resolved address must be usable through the SAME Endpoint() path a
	// remote node uses — proving the local-via-registry-row case (BaseURL ==
	// LocalSentinel) can reach the identical daemon the hot chat-serving path
	// already talks to, without disturbing that path's own connection cache.
	ec, err := Endpoint(context.Background(), "local-backend-row", addr)
	if err != nil {
		t.Fatalf("Endpoint(local): %v", err)
	}
	if ec.InstanceID != "instance-local" {
		t.Fatalf("Endpoint(local).InstanceID = %q, want instance-local", ec.InstanceID)
	}
	CloseEndpoint("local-backend-row")
}

func waitForPortFree(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		lis, err := net.Listen("tcp", addr)
		if err == nil {
			lis.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("port %s never freed: %v", addr, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
