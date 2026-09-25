package contenoxcli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/contenox/contenox/liblease"
)

type gatewayOwnership struct {
	lease      *liblease.Lease
	path       string
	ctx        context.Context
	cancel     context.CancelCauseFunc
	done       chan struct{}
	validUntil atomic.Int64
}

func acquireGatewayOwnership(ctx context.Context, path string, ttl time.Duration) (*gatewayOwnership, error) {
	if ttl < time.Second {
		return nil, fmt.Errorf("gateway lease TTL must be at least one second")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	held, err := liblease.AcquireContext(ctx, path, ttl, liblease.WithMeta(map[string]string{"role": "contenox-gateway"}))
	if err != nil {
		return nil, fmt.Errorf("gateway ownership: %w", err)
	}
	ownerCtx, cancel := context.WithCancelCause(context.Background())
	o := &gatewayOwnership{lease: held, path: path, ctx: ownerCtx, cancel: cancel, done: make(chan struct{})}
	o.validUntil.Store(held.Record().ExpiresAt().UnixNano())
	go o.renew(ttl)
	return o, nil
}

func (o *gatewayOwnership) renew(ttl time.Duration) {
	defer close(o.done)
	ticker := time.NewTicker(ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(o.ctx, ttl/6)
			err := o.lease.RenewContext(ctx)
			cancel()
			if err != nil {
				o.cancel(fmt.Errorf("gateway ownership lost: %w", err))
				return
			}
			o.validUntil.Store(o.lease.Record().ExpiresAt().UnixNano())
		}
	}
}

func (o *gatewayOwnership) release() {
	o.cancel(context.Canceled)
	<-o.done
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = o.lease.ReleaseContext(ctx)
}

func (o *gatewayOwnership) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record, err := liblease.InspectContext(o.ctx, o.path)
		if o.ctx.Err() != nil || time.Now().UnixNano() >= o.validUntil.Load() || err != nil || record.InstanceID != o.lease.InstanceID() || !time.Now().Before(record.ExpiresAt()) {
			o.cancel(fmt.Errorf("gateway ownership lost before request admission: %v", err))
			http.Error(w, "gateway is not the active lease holder", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}
