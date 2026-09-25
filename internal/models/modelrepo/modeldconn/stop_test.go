package modeldconn

import (
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/owner"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
	"github.com/stretchr/testify/require"
)

type stopWorker struct {
	*transport.MemoryService
	received chan transport.UnloadModelRequest
	reject   atomic.Bool
}

func (w *stopWorker) Status(context.Context) (transport.DaemonStatus, error) {
	return transport.DaemonStatus{Active: &transport.ActiveModel{ModelName: "resident", Generation: 42}}, nil
}
func (w *stopWorker) LoadModel(context.Context, transport.LoadModelRequest) (transport.ActiveModel, error) {
	return transport.ActiveModel{}, nil
}
func (w *stopWorker) UnloadModel(_ context.Context, req transport.UnloadModelRequest) error {
	w.received <- req
	if w.reject.Load() {
		return transport.ErrSlotGenerationStale
	}
	return nil
}

func TestUnit_StopModel_NameAndGenerationFence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	oldRoot := dataRoot
	SetDataRoot(root)
	defer SetDataRoot(oldRoot)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	o, err := owner.Join(ctx, owner.Config{LeasePath: filepath.Join(root, "modeld.lease"), TTL: time.Minute, Endpoint: listener.Addr().String(), Backend: "llama"})
	require.NoError(t, err)
	defer o.Release()
	w := &stopWorker{MemoryService: transport.NewMemoryService(), received: make(chan transport.UnloadModelRequest, 1)}
	w.reject.Store(true)
	go func() { _ = transportgrpc.Serve(ctx, listener, w, o.InstanceID(), "llama") }()
	require.ErrorContains(t, StopModel(ctx, "different"), "not resident")
	require.Empty(t, w.received)
	require.Error(t, StopModel(ctx, "resident"))
	req := <-w.received
	require.Equal(t, uint64(42), req.ExpectedGeneration)
	require.Equal(t, o.InstanceID(), req.OwnerInstanceID)
	w.reject.Store(false)
	require.NoError(t, StopModel(ctx, "resident"))
	require.Equal(t, uint64(42), (<-w.received).ExpectedGeneration)
}
