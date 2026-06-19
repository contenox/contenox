package slot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/runtime/transport"
)

type fakeBackend struct {
	mu       sync.Mutex
	opened   []string
	closed   []string
	openErr  error
	sessions []*fakeSession
}

func (b *fakeBackend) OpenSession(_ context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if b.openErr != nil {
		return nil, b.openErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &fakeSession{name: req.ModelName, owner: b}
	b.opened = append(b.opened, req.ModelName)
	b.sessions = append(b.sessions, s)
	return s, nil
}

func (b *fakeBackend) Describe(_ context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	return transport.ModelInfo{ModelMaxContext: req.Config.NumCtx, EffectiveContext: req.Config.NumCtx}, nil
}

func (b *fakeBackend) Embed(context.Context, transport.EmbedRequest) (transport.EmbedResult, error) {
	return transport.EmbedResult{}, transport.ErrUnsupportedFeature
}

func (b *fakeBackend) recordClosed(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = append(b.closed, name)
}

type fakeSession struct {
	name     string
	owner    *fakeBackend
	closed   bool
	decodeCh <-chan transport.StreamChunk
}

func (s *fakeSession) EnsurePrefix(context.Context, transport.PrefixInput) (transport.PrefixStatus, error) {
	if s.closed {
		return transport.PrefixStatus{}, transport.ErrSessionClosed
	}
	return transport.PrefixStatus{PrefixTokens: 1}, nil
}

func (s *fakeSession) PrefillSuffix(context.Context, transport.SuffixInput) (transport.SuffixStatus, error) {
	if s.closed {
		return transport.SuffixStatus{}, transport.ErrSessionClosed
	}
	return transport.SuffixStatus{SuffixTokens: 1}, nil
}

func (s *fakeSession) Decode(context.Context, transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	if s.closed {
		return nil, transport.ErrSessionClosed
	}
	if s.decodeCh != nil {
		return s.decodeCh, nil
	}
	ch := make(chan transport.StreamChunk)
	close(ch)
	return ch, nil
}

func (s *fakeSession) ExplainContext() transport.ContextReport {
	return transport.ContextReport{}
}

func (s *fakeSession) Snapshot(context.Context) (transport.SessionSnapshot, error) {
	if s.closed {
		return transport.SessionSnapshot{}, transport.ErrSessionClosed
	}
	return transport.SessionSnapshot{}, nil
}

func (s *fakeSession) Restore(context.Context, transport.SessionSnapshot) error {
	if s.closed {
		return transport.ErrSessionClosed
	}
	return nil
}

func (s *fakeSession) Close() error {
	s.closed = true
	if s.owner != nil {
		s.owner.recordClosed(s.name)
	}
	return nil
}

func req(name string) transport.OpenSessionRequest {
	return transport.OpenSessionRequest{
		ModelName: name,
		Type:      "llama",
		Digest:    "digest-" + name,
		Path:      "/models/" + name,
		Config:    transport.Config{NumCtx: 1024},
	}
}

func loadReq(name string) transport.LoadModelRequest {
	r := req(name)
	return transport.LoadModelRequest{
		ModelName: r.ModelName,
		Type:      r.Type,
		Digest:    r.Digest,
		Path:      r.Path,
		Config:    r.Config,
	}
}

func TestUnit_Slot_OpenSessionAutoLoadsAndCloseReleasesImplicitModel(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	svc := New(backend, WithBackend("llama"))

	sess, err := svc.OpenSession(ctx, req("a"))
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	st, _ := svc.Status(ctx)
	if st.State != transport.SlotReady || st.Active == nil || st.Active.ModelName != "a" {
		t.Fatalf("status after open = %+v", st)
	}
	if _, err := svc.OpenSession(ctx, req("b")); !errors.Is(err, transport.ErrModelBusy) {
		t.Fatalf("OpenSession different while held = %v, want ErrModelBusy", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	st, _ = svc.Status(ctx)
	if st.State != transport.SlotEmpty || st.Active != nil {
		t.Fatalf("status after close = %+v, want empty", st)
	}
	if len(backend.sessions) != 1 || !backend.sessions[0].closed {
		t.Fatalf("implicit session was not closed on release")
	}
}

func TestUnit_Slot_ExplicitSwitchRequiresReleasedSessionAndInvalidatesOldHandle(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	svc := New(backend, WithBackend("llama"))

	activeA, err := svc.LoadModel(ctx, loadReq("a"))
	if err != nil {
		t.Fatalf("LoadModel a: %v", err)
	}
	sessA, err := svc.OpenSession(ctx, req("a"))
	if err != nil {
		t.Fatalf("OpenSession a: %v", err)
	}
	if _, err := svc.LoadModel(ctx, loadReq("b")); !errors.Is(err, transport.ErrModelBusy) {
		t.Fatalf("LoadModel b while a held = %v, want ErrModelBusy", err)
	}
	if err := sessA.Close(); err != nil {
		t.Fatalf("Close a: %v", err)
	}
	activeB, err := svc.LoadModel(ctx, loadReq("b"))
	if err != nil {
		t.Fatalf("LoadModel b after release: %v", err)
	}
	if activeB.Generation <= activeA.Generation {
		t.Fatalf("generation did not advance: a=%d b=%d", activeA.Generation, activeB.Generation)
	}
	if _, err := sessA.EnsurePrefix(ctx, transport.PrefixInput{}); !errors.Is(err, transport.ErrSessionClosed) {
		t.Fatalf("old session after close/switch = %v, want ErrSessionClosed", err)
	}
	st, _ := svc.Status(ctx)
	if st.Active == nil || st.Active.ModelName != "b" {
		t.Fatalf("status after switch = %+v", st)
	}
	if len(backend.sessions) < 1 || !backend.sessions[0].closed {
		t.Fatalf("old backend session was not closed during switch")
	}
}

func TestUnit_Slot_UnloadExplicitModel(t *testing.T) {
	ctx := context.Background()
	svc := New(&fakeBackend{}, WithBackend("llama"))

	active, err := svc.LoadModel(ctx, loadReq("a"))
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if err := svc.UnloadModel(ctx, transport.UnloadModelRequest{ExpectedGeneration: active.Generation + 1}); !errors.Is(err, transport.ErrSlotGenerationStale) {
		t.Fatalf("Unload stale generation = %v, want ErrSlotGenerationStale", err)
	}
	if err := svc.UnloadModel(ctx, transport.UnloadModelRequest{ExpectedGeneration: active.Generation}); err != nil {
		t.Fatalf("UnloadModel: %v", err)
	}
	st, _ := svc.Status(ctx)
	if st.State != transport.SlotEmpty || st.Active != nil {
		t.Fatalf("status after unload = %+v", st)
	}
}

func TestUnit_Slot_LoadFailureLeavesFailedWithoutActiveModel(t *testing.T) {
	ctx := context.Background()
	svc := New(&fakeBackend{openErr: transport.ErrContextOverflow}, WithBackend("llama"))

	_, err := svc.LoadModel(ctx, loadReq("a"))
	if !errors.Is(err, transport.ErrModelLoadFailed) || !errors.Is(err, transport.ErrContextOverflow) {
		t.Fatalf("LoadModel error = %v, want load failed and context overflow", err)
	}
	st, _ := svc.Status(ctx)
	if st.State != transport.SlotFailed || st.Active != nil || st.LastError == "" {
		t.Fatalf("status after failed load = %+v", st)
	}
}

func TestUnit_Slot_CanceledLoadWhileBusyDoesNotRunLater(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	svc := New(backend, WithBackend("llama"))
	active, err := svc.LoadModel(ctx, loadReq("a"))
	if err != nil {
		t.Fatalf("LoadModel a: %v", err)
	}
	sess, err := svc.OpenSession(ctx, req("a"))
	if err != nil {
		t.Fatalf("OpenSession a: %v", err)
	}

	block := make(chan transport.StreamChunk)
	backend.sessions[0].decodeCh = block
	decodeCtx, cancelDecode := context.WithCancel(ctx)
	ch, err := sess.Decode(decodeCtx, transport.DecodeConfig{})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	defer func() {
		cancelDecode()
		close(block)
		for range ch {
		}
		_ = sess.Close()
	}()

	loadCtx, cancelLoad := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		_, err := svc.LoadModel(loadCtx, loadReq("b"))
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancelLoad()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled LoadModel while waiting = %v, want context.Canceled", err)
	}
	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Active == nil || st.Active.ModelName != "a" || st.Active.Generation != active.Generation {
		t.Fatalf("active changed after canceled load: %+v", st)
	}
}

func TestUnit_Slot_DecodeCancellationReleasesBusyWhenOutputNotDrained(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	svc := New(backend, WithBackend("llama"))
	if _, err := svc.LoadModel(ctx, loadReq("a")); err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	sess, err := svc.OpenSession(ctx, req("a"))
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer sess.Close()

	src := make(chan transport.StreamChunk, 32)
	for i := 0; i < cap(src); i++ {
		src <- transport.StreamChunk{Text: "x"}
	}
	backend.sessions[0].decodeCh = src
	decodeCtx, cancelDecode := context.WithCancel(ctx)
	out, err := sess.Decode(decodeCtx, transport.DecodeConfig{})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	_ = out // Intentionally do not drain it; cancellation must still unblock the slot.
	cancelDecode()

	deadline := time.After(2 * time.Second)
	for {
		st, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if st.State == transport.SlotReady {
			close(src)
			return
		}
		select {
		case <-deadline:
			t.Fatalf("slot stayed busy after decode cancellation: %+v", st)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
