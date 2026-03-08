package mcpserverservice

import (
	"context"
	"fmt"
	"time"

	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtimetypes"
)

type activityTrackerDecorator struct {
	service Service
	tracker libtracker.ActivityTracker
}

// WithActivityTracker wraps a Service with activity tracking.
func WithActivityTracker(service Service, tracker libtracker.ActivityTracker) Service {
	return &activityTrackerDecorator{service: service, tracker: tracker}
}

func (d *activityTrackerDecorator) Create(ctx context.Context, srv *runtimetypes.MCPServer) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(ctx, "create", "mcp_server",
		"name", srv.Name, "transport", srv.Transport)
	defer endFn()
	if err := d.service.Create(ctx, srv); err != nil {
		reportErrFn(err)
		return err
	}
	reportChangeFn(srv.ID, srv)
	return nil
}

func (d *activityTrackerDecorator) Get(ctx context.Context, id string) (*runtimetypes.MCPServer, error) {
	_, _, endFn := d.tracker.Start(ctx, "get", "mcp_server", "id", id)
	defer endFn()
	return d.service.Get(ctx, id)
}

func (d *activityTrackerDecorator) GetByName(ctx context.Context, name string) (*runtimetypes.MCPServer, error) {
	_, _, endFn := d.tracker.Start(ctx, "get_by_name", "mcp_server", "name", name)
	defer endFn()
	return d.service.GetByName(ctx, name)
}

func (d *activityTrackerDecorator) Update(ctx context.Context, srv *runtimetypes.MCPServer) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(ctx, "update", "mcp_server",
		"id", srv.ID, "name", srv.Name)
	defer endFn()
	if err := d.service.Update(ctx, srv); err != nil {
		reportErrFn(err)
		return err
	}
	reportChangeFn(srv.ID, srv)
	return nil
}

func (d *activityTrackerDecorator) Delete(ctx context.Context, id string) error {
	srv, err := d.service.Get(ctx, id)
	if err != nil {
		return err
	}
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(ctx, "delete", "mcp_server",
		"id", id, "name", srv.Name)
	defer endFn()
	if err := d.service.Delete(ctx, id); err != nil {
		reportErrFn(err)
		return fmt.Errorf("mcp server delete: %w", err)
	}
	reportChangeFn(id, nil)
	return nil
}

func (d *activityTrackerDecorator) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.MCPServer, error) {
	_, _, endFn := d.tracker.Start(ctx, "list", "mcp_servers")
	defer endFn()
	return d.service.List(ctx, createdAtCursor, limit)
}
