package taskengine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	libkv "github.com/contenox/contenox/libkvstore"
	"github.com/contenox/contenox/libtracker"
)

const (
	kvStateRequestsSet = "state:requests"
	kvStatePrefix      = "state:"
	kvStateMaxEntries  = 1000
)

type KVInspector struct {
	inner Inspector
	kv    libkv.KVManager
}

func NewKVInspector(inner Inspector, kv libkv.KVManager) *KVInspector {
	return &KVInspector{inner: inner, kv: kv}
}

func (i *KVInspector) Start(ctx context.Context) StackTrace {
	inner := i.inner.Start(ctx)
	reqID, ok := ctx.Value(libtracker.ContextKeyRequestID).(string)
	if !ok || reqID == "" {
		return inner
	}

	op, err := i.kv.Executor(ctx)
	if err != nil {
		log.Printf("inspector(kv): executor on Start: %v", err)
		return inner
	}
	if err := op.SetAdd(ctx, kvStateRequestsSet, []byte(reqID)); err != nil {
		log.Printf("inspector(kv): track requestID: %v", err)
	}

	return &kvStackTrace{
		inner: inner,
		kv:    i.kv,
		reqID: reqID,
		ctx:   ctx,
	}
}

func (i *KVInspector) GetExecutionStateByRequestID(ctx context.Context, reqID string) ([]CapturedStateUnit, error) {
	if reqID == "" {
		return nil, nil
	}
	op, err := i.kv.Executor(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := op.ListRange(ctx, kvStatePrefix+reqID, 0, -1)
	if err != nil {
		return nil, err
	}
	out := make([]CapturedStateUnit, 0, len(raw))
	for _, b := range raw {
		var u CapturedStateUnit
		if err := json.Unmarshal(b, &u); err != nil {
			log.Printf("inspector(kv): unmarshal step: %v", err)
			continue
		}
		out = append(out, u)
	}
	return out, nil
}

func (i *KVInspector) GetStatefulRequests(ctx context.Context) ([]string, error) {
	op, err := i.kv.Executor(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := op.SetMembers(ctx, kvStateRequestsSet)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, b := range raw {
		out = append(out, string(b))
	}
	return out, nil
}

type kvStackTrace struct {
	inner StackTrace
	kv    libkv.KVManager
	reqID string
	ctx   context.Context
}

func (s *kvStackTrace) RecordStep(step CapturedStateUnit) {
	s.inner.RecordStep(step)

	data, err := json.Marshal(step)
	if err != nil {
		log.Printf("inspector(kv): marshal step: %v", err)
		return
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 10*time.Second)
	defer cancel()

	op, err := s.kv.Executor(writeCtx)
	if err != nil {
		log.Printf("inspector(kv): executor on RecordStep: %v", err)
		return
	}

	key := kvStatePrefix + s.reqID
	if err := op.ListPush(writeCtx, key, data); err != nil {
		log.Printf("inspector(kv): list push: %v", err)
		return
	}
	n, err := op.ListLength(writeCtx, key)
	if err != nil {
		log.Printf("inspector(kv): list length: %v", err)
		return
	}
	if n > kvStateMaxEntries {
		if err := op.ListTrim(writeCtx, key, 0, kvStateMaxEntries-1); err != nil {
			log.Printf("inspector(kv): list trim: %v", err)
		}
	}
}

func (s *kvStackTrace) GetExecutionHistory() []CapturedStateUnit {
	return s.inner.GetExecutionHistory()
}
