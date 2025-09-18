package eventstore

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/contenox/runtime/libdbexec"
)

// Event represents a stored event without exposing partition details
type Event struct {
	ID            string          `json:"id" example:"event-uuid"`
	CreatedAt     time.Time       `json:"created_at" example:"2023-01-01T00:00:00Z"`
	EventType     string          `json:"event_type" example:"github.pull_request"`
	EventSource   string          `json:"event_source" example:"github.com"`
	AggregateID   string          `json:"aggregate_id" example:"aggregate-uuid"`
	AggregateType string          `json:"aggregate_type" example:"github.webhook"`
	Version       int             `json:"version" example:"1"`
	Data          json.RawMessage `json:"data" example:"{}"`
	Metadata      json.RawMessage `json:"metadata" example:"{}"`
}

// Store provides methods for storing and retrieving events
type Store interface {
	AppendEvent(ctx context.Context, event *Event) error
	GetEventsByAggregate(ctx context.Context, eventType string, from, to time.Time, aggregateType, aggregateID string, limit int) ([]Event, error)
	GetEventsByType(ctx context.Context, eventType string, from, to time.Time, limit int) ([]Event, error)
	GetEventsBySource(ctx context.Context, eventType string, from, to time.Time, eventSource string, limit int) ([]Event, error)
	GetEventTypesInRange(ctx context.Context, from, to time.Time, limit int) ([]string, error)
	DeleteEventsByTypeInRange(ctx context.Context, eventType string, from, to time.Time) error

	EnsurePartitionExists(ctx context.Context, ts time.Time) error
}

// internalEvent represents the database structure with partition key
type internalEvent struct {
	*Event
	PartitionKey string `json:"-"`
}

// store implements EventStore using libdbexec
type store struct {
	Exec     libdbexec.Exec
	pManager *partitionManager
}

type partitionManager struct {
	lastExecuted *time.Time
	lock         sync.Mutex
}

// New creates a new event store instance
func New(exec libdbexec.Exec) Store {
	return &store{Exec: exec, pManager: &partitionManager{lock: sync.Mutex{}, lastExecuted: nil}}
}
