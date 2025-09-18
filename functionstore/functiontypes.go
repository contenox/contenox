package functionstore

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const MAXLIMIT = 1000

var ErrLimitParamExceeded = fmt.Errorf("limit exceeds maximum allowed value")
var ErrNotFound = errors.New("not found")

// EventTrigger defines the configuration for an event-driven action.
type EventTrigger struct {
	// A unique identifier for the trigger.
	Name string `json:"name" yaml:"name" example:"new_user_created"`
	// A user-friendly description of what the trigger does.
	Description string `json:"description" yaml:"description" example:"Send a welcome email to a new user"`
	// The event type to listen for.
	ListenFor Listener `json:"listenFor" yaml:"listenFor" example:"user_created"`
	// The type of the triggered action.
	Type string `json:"type" yaml:"type" example:"function"`
	// The name of the function to execute when the event is received.
	Function string `json:"function" yaml:"function" example:"new_user_created_event_handler"`
	// Timestamps for creation and updates
	CreatedAt time.Time `json:"createdAt" example:"2023-11-15T14:30:45Z"`
	UpdatedAt time.Time `json:"updatedAt" example:"2023-11-15T14:30:45Z"`
}

// Listener defines the event source.
type Listener struct {
	// The event type to listen for.
	Type string `json:"type" yaml:"type" example:"contenox.user_created"`
}

// Function represents a serverless-like function.
type Function struct {
	// A unique identifier for the function.
	Name string `json:"name" yaml:"name" example:"send_welcome_email_event_handler"`
	// A user-friendly description of what the function does.
	Description string `json:"description" yaml:"description"`
	// The type of script to execute.
	ScriptType string `json:"scriptType" yaml:"scriptType" example:"goja"`
	// The script code itself.
	Script string `json:"script" yaml:"script"`
	// The type of action to perform after the script.
	ActionType string `json:"actionType" yaml:"actionType" example:"chain"`
	// The target of the action.
	ActionTarget string `json:"actionTarget" yaml:"actionTarget" example:"welcome_email_chain"`
	// Timestamps for creation and updates
	CreatedAt time.Time `json:"createdAt" example:"2023-11-15T14:30:45Z"`
	UpdatedAt time.Time `json:"updatedAt" example:"2023-11-15T14:30:45Z"`
}

type FunctionScriptType string

const (
	GojaTerm FunctionScriptType = "goja"
)

type FunctionActionType string

const (
	ChainTerm FunctionActionType = "chain"
)

type FunctionType string

const (
	FunctionTerm FunctionType = "function"
)

// Store provides methods for storing and retrieving functions and event triggers
type Store interface {
	// Function management
	CreateFunction(ctx context.Context, function *Function) error
	GetFunction(ctx context.Context, name string) (*Function, error)
	UpdateFunction(ctx context.Context, function *Function) error
	DeleteFunction(ctx context.Context, name string) error
	ListFunctions(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*Function, error)
	ListAllFunctions(ctx context.Context) ([]*Function, error)
	EstimateFunctionCount(ctx context.Context) (int64, error)

	// Event trigger management
	CreateEventTrigger(ctx context.Context, trigger *EventTrigger) error
	GetEventTrigger(ctx context.Context, name string) (*EventTrigger, error)
	UpdateEventTrigger(ctx context.Context, trigger *EventTrigger) error
	DeleteEventTrigger(ctx context.Context, name string) error
	ListEventTriggers(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*EventTrigger, error)
	ListAllEventTriggers(ctx context.Context) ([]*EventTrigger, error)
	ListEventTriggersByEventType(ctx context.Context, eventType string) ([]*EventTrigger, error)
	ListEventTriggersByFunction(ctx context.Context, functionName string) ([]*EventTrigger, error)
	EstimateEventTriggerCount(ctx context.Context) (int64, error)
}
