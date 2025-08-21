package libbus

import (
	"context"
	"errors"
)

var (
	// ErrConnectionClosed is returned when an operation is attempted on a closed connection.
	ErrConnectionClosed = errors.New("connection closed")
	// ErrStreamSubscriptionFail is returned when a stream subscription fails.
	ErrStreamSubscriptionFail = errors.New("stream subscription failed")
	// ErrMessagePublish is returned when publishing a message fails for reasons other than a closed connection.
	ErrMessagePublish = errors.New("message publishing failed")
	// ErrRequestTimeout is returned when a request-reply operation times out.
	ErrRequestTimeout = errors.New("request timed out")
)

// Handler is a function that processes a request and returns a response.
// It is used by the Serve method to handle incoming requests.
type Handler func(ctx context.Context, data []byte) ([]byte, error)

// Messenger defines a high-level interface for various messaging patterns.
// It is designed for real-time event notifications, triggering ephemeral tasks,
// and distributing lightweight messages between services.
type Messenger interface {
	// Publish sends a fire-and-forget message to a given subject.
	Publish(ctx context.Context, subject string, data []byte) error

	// Stream creates a subscription to a subject and delivers messages asynchronously
	// to the provided channel. The subscription is automatically managed and will
	// be closed when the provided context is canceled.
	Stream(ctx context.Context, subject string, ch chan<- []byte) (Subscription, error)

	// Request sends a request message and waits for a reply. The context can be
	// used to set a timeout or to cancel the request.
	Request(ctx context.Context, subject string, data []byte) ([]byte, error)

	// Serve registers a handler for a given subject to respond to requests.
	// It starts a worker that listens for requests and executes the handler.
	// The returned Subscription can be used to stop serving.
	Serve(ctx context.Context, subject string, handler Handler) (Subscription, error)

	// Close disconnects from the messaging server and cleans up any underlying resources.
	Close() error
}

// Subscription represents an active subscription to a subject.
type Subscription interface {
	// Unsubscribe removes the subscription, stopping the delivery of messages.
	Unsubscribe() error
}
