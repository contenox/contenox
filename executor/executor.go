package executor

import (
	"context"

	"github.com/contenox/runtime/eventstore"
)

// Executor defines the interface for executing functions with an event as input.
type Executor interface {
	// ExecuteFunction executes a function with the given code and event.
	// It returns a result as a JSON-like map and any error encountered.
	ExecuteFunction(
		ctx context.Context,
		code string,
		functionName string,
		event *eventstore.Event,
	) (map[string]interface{}, error)
}
