package taskengine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/contenox/runtime/libtracker"
	libkv "github.com/contenox/runtime/libkvstore"
	"github.com/google/uuid"
)

type AlertSink interface {
	SendAlert(ctx context.Context, message string, kvPairMetadata ...string) error
	FetchAlerts(ctx context.Context, limit int) ([]*Alert, error)
}

type SimpleAlertSink struct {
	kvManager libkv.KVManager
}

func NewAlertSink(kvManager libkv.KVManager) AlertSink {
	return &SimpleAlertSink{
		kvManager: kvManager,
	}
}

type Alert struct {
	ID        string            `json:"id"`
	Message   string            `json:"message"`
	RequestID string            `json:"requestID"`
	Metadata  map[string]string `json:"metadata"`
	Timestamp time.Time         `json:"Timestamp"`
}

func (as *SimpleAlertSink) SendAlert(ctx context.Context, message string, kvPairMetadata ...string) error {
	opCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	op, err := as.kvManager.Executor(opCtx)
	if err != nil {
		return err
	}
	meta := make(map[string]string)
	for i, kv := range kvPairMetadata {
		if i%2 == 0 {
			meta[kv] = ""
		} else {
			meta[kvPairMetadata[i-1]] = kv
		}
	}
	event := &Alert{
		Message:   message,
		RequestID: "",
		Metadata:  meta,
		ID:        uuid.NewString(),
		Timestamp: time.Now().UTC(),
	}
	if reqID, ok := ctx.Value(libtracker.ContextKeyRequestID).(string); ok {
		event.RequestID = reqID
	}
	ev, err := json.Marshal(event)
	if err != nil {
		return err
	}
	err = op.ListPush(ctx, "alert", ev)
	if err != nil {
		return err
	}
	err = op.ListTrim(ctx, "alert", 0, 99)
	if err != nil {
		return err
	}

	return nil
}

func (as *SimpleAlertSink) FetchAlerts(ctx context.Context, limit int) ([]*Alert, error) {
	opCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	op, err := as.kvManager.Executor(opCtx)
	if err != nil {
		return nil, err
	}
	alerts := []*Alert{}
	l, err := op.ListRange(ctx, "alert", 0, int64(limit))
	if err != nil {
		return nil, err
	}
	for _, v := range l {
		var alert Alert
		if err := json.Unmarshal(v, &alert); err != nil {
			return nil, err
		}
		alerts = append(alerts, &alert)
	}
	return alerts, nil
}
