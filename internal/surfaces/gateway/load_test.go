package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

// upstreamOllama is a real HTTP upstream speaking NDJSON, so a measurement here
// covers the HTTP client, the per-frame decode and the re-encode.
func upstreamOllama(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"qwen3:8b","model":"qwen3:8b","modified_at":"2026-01-01T00:00:00Z","size":1,"digest":"d"}]}`)
	})
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"capabilities":["completion"],"model_info":{"llama.context_length":8192}}`)
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i := 0; i < 8; i++ {
			fmt.Fprintf(w, "{\"model\":\"qwen3:8b\",\"message\":{\"role\":\"assistant\",\"content\":\"chunk-%d \"},\"done\":false}\n", i)
		}
		fmt.Fprint(w, "{\"model\":\"qwen3:8b\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":40,\"eval_count\":20}\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func loadGateway(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	up := upstreamOllama(t)

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(dir, "load.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	bus := libbus.NewInMem()
	rt, err := runtimestate.New(ctx, db, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(ctx, &runtimetypes.Backend{
		ID: "up", Name: "up", Type: "ollama", BaseURL: up.URL,
	}))
	require.NoError(t, rt.RunBackendCycle(ctx))

	svc, err := New(Config{
		DB: db, Verifier: tolerantVerifier{}, Runtime: rt, Bus: bus,
		Models: NewTestModelRepo(t, rt),
	})
	require.NoError(t, err)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	return mux, "load-token"
}

// TestLoadThroughput measures the gateway's own request path end to end against
// an upstream that answers over a real socket: auth, the ledger lookup, the
// allow/deny checks, the HTTP client, the NDJSON decode and the meter append.
//
// It is opt-in (LOAD=1) because it issues thousands of turns; a default run
// stays fast. The number it prints is the gateway's ceiling, which is worth
// knowing against the model behind it: an upstream generating 20-100 output
// tokens a second finishes a turn every 1.5-7s, so the gateway is orders of
// magnitude ahead of what it proxies and is never the binding constraint.
func TestLoadThroughput(t *testing.T) {
	if os.Getenv("LOAD") == "" {
		t.Skip("set LOAD=1 to run the throughput measurement")
	}
	conc := 16
	perWorker := 200
	if v := os.Getenv("LOAD_CONC"); v != "" {
		fmt.Sscanf(v, "%d", &conc)
	}
	h, token := loadGateway(t)
	body := []byte(`{"model":"qwen3:8b","stream":false,"messages":[{"role":"user","content":"hi"}]}`)

	var wg sync.WaitGroup
	errs := make(chan string, conc*perWorker)
	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					errs <- fmt.Sprintf("%d: %s", rec.Code, rec.Body.String())
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	wall := time.Since(start)
	n := conc * perWorker
	failed := 0
	for e := range errs {
		if failed < 3 {
			t.Logf("failure: %s", e)
		}
		failed++
	}
	t.Logf("RESULT: concurrency %d, %d requests, %d failed, %v wall, %.0f req/s",
		conc, n, failed, wall, float64(n-failed)/wall.Seconds())
}
