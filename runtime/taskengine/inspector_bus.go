package taskengine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/contenox/contenox/libbus"
	"github.com/contenox/contenox/libtracker"
)

func StateSubject(reqID string) string {
	return "state." + reqID
}

type BusInspector struct {
	inner Inspector
	bus   libbus.Messenger
}

func NewBusInspector(inner Inspector, bus libbus.Messenger) *BusInspector {
	return &BusInspector{inner: inner, bus: bus}
}

func (i *BusInspector) Start(ctx context.Context) StackTrace {
	inner := i.inner.Start(ctx)
	reqID, ok := ctx.Value(libtracker.ContextKeyRequestID).(string)
	if !ok || reqID == "" {
		return inner
	}
	return &busStackTrace{
		inner:   inner,
		bus:     i.bus,
		subject: StateSubject(reqID),
		ctx:     ctx,
	}
}

type busStackTrace struct {
	inner   StackTrace
	bus     libbus.Messenger
	subject string
	ctx     context.Context
}

func (s *busStackTrace) RecordStep(step CapturedStateUnit) {
	s.inner.RecordStep(step)

	data, err := json.Marshal(step)
	if err != nil {
		log.Printf("inspector(bus): marshal step: %v", err)
		return
	}

	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 5*time.Second)
	defer cancel()

	if err := s.bus.Publish(pubCtx, s.subject, data); err != nil {
		log.Printf("inspector(bus): publish %s: %v", s.subject, err)
	}
}

func (s *busStackTrace) GetExecutionHistory() []CapturedStateUnit {
	return s.inner.GetExecutionHistory()
}
