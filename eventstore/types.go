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
	ID            string          `json:"id"`
	CreatedAt     time.Time       `json:"created_at"`
	EventType     string          `json:"event_type"`
	AggregateID   string          `json:"aggregate_id"`
	AggregateType string          `json:"aggregate_type"`
	Version       int             `json:"version"`
	Data          json.RawMessage `json:"data"`
	Metadata      json.RawMessage `json:"metadata"`
}

// EventStore provides methods for storing and retrieving events
type EventStore interface {
	AppendEvent(ctx context.Context, event *Event) error
	GetEventsByAggregate(ctx context.Context, eventType string, from, to time.Time, aggregateType, aggregateID string, limit int) ([]Event, error)
	GetEventsByType(ctx context.Context, eventType string, from, to time.Time, limit int) ([]Event, error)
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

// NewEventStore creates a new event store instance
func NewEventStore(exec libdbexec.Exec) EventStore {
	return &store{Exec: exec, pManager: &partitionManager{lock: sync.Mutex{}, lastExecuted: nil}}
}
