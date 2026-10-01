package llmretry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/llmresolver"
	"github.com/contenox/contenox/internal/kernel/taskengine/llmretry"
	"github.com/contenox/contenox/internal/models/modelrepo"
)

func TestUnit_ClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want llmretry.ErrorClass
	}{
		{"nil", nil, llmretry.ClassNone},
		{"resolver capacity containing 504", fmt.Errorf("%w: minicpm5-2b provides only 67504", llmresolver.ErrNoSatisfactoryModel), llmretry.ClassPermanent},
		{"HTTP 400 containing 500", &modelrepo.HTTPError{StatusCode: 400, Message: "invalid item rs_500"}, llmretry.ClassPermanent},
		{"HTTP 503 without status in message", &modelrepo.HTTPError{StatusCode: 503, Message: "try later"}, llmretry.ClassServerError},
		{"canceled", context.Canceled, llmretry.ClassCanceled},
		{"deadline", context.DeadlineExceeded, llmretry.ClassTimeout},
		{"openai 429", fmt.Errorf("OpenAI API returned non-200 status: 429, body: rate limited for model gpt-4"), llmretry.ClassRateLimit},
		{"anthropic 529", fmt.Errorf("anthropic 529 overloaded"), llmretry.ClassRateLimit},
		{"openai 503", fmt.Errorf("OpenAI API returned non-200 status: 503 service unavailable"), llmretry.ClassServerError},
		{"401 unauthorized", fmt.Errorf("status: 401 unauthorized"), llmretry.ClassAuth},
		{"invalid api key", fmt.Errorf("invalid api key supplied"), llmretry.ClassAuth},
		{"capacity exceeded", fmt.Errorf("input token count 200000 exceeds context length 128000"), llmretry.ClassCapacity},
		{"timeout", fmt.Errorf("Post \"https://api/x\": net/http: request canceled (i/o timeout)"), llmretry.ClassTimeout},
		{"unknown", fmt.Errorf("totally unexpected provider error"), llmretry.ClassPermanent},

		// The provider clients classify with typed sentinels, and that decision
		// outranks the text. Each of these messages matches no marker below, so
		// the text path would call them permanent and never retry.
		{"typed rate limit", fmt.Errorf("provider refused: %w", modelrepo.ErrRateLimited), llmretry.ClassRateLimit},
		{"typed auth", fmt.Errorf("provider refused: %w", modelrepo.ErrModelAccessDenied), llmretry.ClassAuth},
		{"typed capacity", fmt.Errorf("provider refused: %w", modelrepo.ErrContextLengthExceeded), llmretry.ClassCapacity},
		{"typed model missing", fmt.Errorf("provider refused: %w", modelrepo.ErrModelNotFoundOnBackend), llmretry.ClassPermanent},
		// And a sentinel outranks text that would classify the other way: this
		// message says 503, the typed decision says the key is dead.
		{"sentinel over text", fmt.Errorf("upstream returned 503: %w", modelrepo.ErrModelAccessDenied), llmretry.ClassAuth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := llmretry.ClassifyError(tc.err)
			if got != tc.want {
				t.Fatalf("ClassifyError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestUnit_ErrorClass_IsRetryable(t *testing.T) {
	retryable := []llmretry.ErrorClass{llmretry.ClassRateLimit, llmretry.ClassServerError, llmretry.ClassTimeout}
	for _, c := range retryable {
		if !c.IsRetryable() {
			t.Errorf("expected %q retryable", c)
		}
	}
	notRetryable := []llmretry.ErrorClass{llmretry.ClassNone, llmretry.ClassAuth, llmretry.ClassCapacity, llmretry.ClassCanceled, llmretry.ClassPermanent}
	for _, c := range notRetryable {
		if c.IsRetryable() {
			t.Errorf("expected %q not retryable", c)
		}
	}
}

func fastPolicy(p llmretry.RetryPolicy) llmretry.RetryPolicy {
	if p.InitialBackoff == 0 {
		p.InitialBackoff = llmretry.Duration(time.Millisecond)
	}
	if p.MaxBackoff == 0 {
		p.MaxBackoff = llmretry.Duration(2 * time.Millisecond)
	}
	return p
}

func TestUnit_Do_NoRetryOnAuth(t *testing.T) {
	calls := 0
	_, out, err := llmretry.Do(context.Background(), fastPolicy(llmretry.RetryPolicy{MaxAttempts: 5}), "primary", func(model string) (any, error) {
		calls++
		return nil, fmt.Errorf("status: 401 unauthorized")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected single call, got %d", calls)
	}
	if out.LastErrorClass != llmretry.ClassAuth {
		t.Fatalf("class = %s", out.LastErrorClass)
	}
	if out.UsedFallback {
		t.Fatalf("fallback should not trigger on auth")
	}
}

func TestUnit_Do_NoRetryOnCapacity(t *testing.T) {
	calls := 0
	_, out, err := llmretry.Do(context.Background(), fastPolicy(llmretry.RetryPolicy{MaxAttempts: 5}), "primary", func(model string) (any, error) {
		calls++
		return nil, fmt.Errorf("input token count 200000 exceeds context length 128000")
	})
	if err == nil || calls != 1 || out.LastErrorClass != llmretry.ClassCapacity {
		t.Fatalf("calls=%d class=%s err=%v", calls, out.LastErrorClass, err)
	}
}

func TestUnit_Do_RetriesOnRateLimitThenSucceeds(t *testing.T) {
	seq := []error{
		fmt.Errorf("status: 429 too many requests"),
		fmt.Errorf("status: 429 too many requests"),
		nil,
	}
	calls := 0
	res, out, err := llmretry.Do(context.Background(), fastPolicy(llmretry.RetryPolicy{MaxAttempts: 5}), "primary", func(model string) (any, error) {
		defer func() { calls++ }()
		if e := seq[calls]; e != nil {
			return nil, e
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.(string) != "ok" {
		t.Fatalf("res = %v", res)
	}
	if out.Attempts != 3 {
		t.Fatalf("attempts = %d", out.Attempts)
	}
	if out.UsedFallback {
		t.Fatalf("did not configure fallback")
	}
}

func TestUnit_Do_FallbackAfterThreshold(t *testing.T) {
	calls := 0
	models := []string{}
	_, out, err := llmretry.Do(context.Background(), fastPolicy(llmretry.RetryPolicy{
		MaxAttempts:     5,
		FallbackModelID: "fallback",
		FallbackAfter:   2,
	}), "primary", func(model string) (any, error) {
		models = append(models, model)
		calls++
		// Fail twice on primary, then succeed on whatever model is asked.
		if calls < 3 {
			return nil, fmt.Errorf("status: 503 service unavailable")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected success after fallback, got %v", err)
	}
	if !out.UsedFallback {
		t.Fatalf("expected fallback used")
	}
	if models[0] != "primary" || models[len(models)-1] != "fallback" {
		t.Fatalf("expected primary→fallback sequence, got %v", models)
	}
}

func TestUnit_Do_ExhaustsAttempts(t *testing.T) {
	calls := 0
	_, out, err := llmretry.Do(context.Background(), fastPolicy(llmretry.RetryPolicy{MaxAttempts: 3}), "primary", func(model string) (any, error) {
		calls++
		return nil, fmt.Errorf("status: 503")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if calls != 3 || out.Attempts != 3 {
		t.Fatalf("calls=%d attempts=%d", calls, out.Attempts)
	}
	if out.LastErrorClass != llmretry.ClassServerError {
		t.Fatalf("class = %s", out.LastErrorClass)
	}
}

func TestUnit_Do_ContextCanceledBeforeFirstCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := llmretry.Do(ctx, llmretry.RetryPolicy{MaxAttempts: 5}, "primary", func(model string) (any, error) {
		t.Fatalf("call should not run after cancel")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestUnit_Do_ContextCanceledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, _, err := llmretry.Do(ctx, llmretry.RetryPolicy{MaxAttempts: 5, InitialBackoff: llmretry.Duration(50 * time.Millisecond), MaxBackoff: llmretry.Duration(50 * time.Millisecond)}, "primary", func(model string) (any, error) {
		calls++
		// Cancel after first attempt; sleep should observe cancel.
		go func() {
			time.Sleep(5 * time.Millisecond)
			cancel()
		}()
		return nil, fmt.Errorf("status: 503")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v (calls=%d)", err, calls)
	}
}

func TestUnit_Duration_JSON(t *testing.T) {
	type wrap struct {
		B llmretry.Duration `json:"b"`
	}
	cases := []struct {
		in   string
		want time.Duration
	}{
		{`{"b":"1s"}`, time.Second},
		{`{"b":"500ms"}`, 500 * time.Millisecond},
		{`{"b":1000000000}`, time.Second},
		{`{"b":""}`, 0},
		{`{"b":null}`, 0},
		{`{}`, 0},
	}
	for _, tc := range cases {
		var w wrap
		if err := json.Unmarshal([]byte(tc.in), &w); err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if w.B.D() != tc.want {
			t.Fatalf("%s: got %v want %v", tc.in, w.B.D(), tc.want)
		}
	}
	// Round-trip a policy using string form.
	pin := llmretry.RetryPolicy{
		MaxAttempts:      3,
		InitialBackoff:   llmretry.Duration(500 * time.Millisecond),
		MaxBackoff:       llmretry.Duration(30 * time.Second),
		Jitter:           0.25,
		RateLimitMinWait: llmretry.Duration(10 * time.Second),
	}
	b, err := json.Marshal(pin)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pout llmretry.RetryPolicy
	if err := json.Unmarshal(b, &pout); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, string(b))
	}
	if pout.InitialBackoff.D() != 500*time.Millisecond || pout.MaxBackoff.D() != 30*time.Second {
		t.Fatalf("roundtrip mismatch: %+v", pout)
	}
}

func TestUnit_Duration_InvalidString(t *testing.T) {
	var d llmretry.Duration
	if err := json.Unmarshal([]byte(`"not-a-duration"`), &d); err == nil {
		t.Fatalf("expected error for invalid duration string")
	}
}

func TestUnit_Do_ZeroPolicyMakesOneAttempt(t *testing.T) {
	calls := 0
	_, out, err := llmretry.Do(context.Background(), llmretry.RetryPolicy{}, "primary", func(model string) (any, error) {
		calls++
		return nil, fmt.Errorf("status: 503")
	})
	if err == nil || calls != 1 || out.Attempts != 1 {
		t.Fatalf("zero policy should make 1 attempt; calls=%d attempts=%d err=%v", calls, out.Attempts, err)
	}
}

func TestUnit_NoRetryVetoesARetryableClass(t *testing.T) {
	cause := fmt.Errorf("status: 503 service unavailable")
	if got := llmretry.ClassifyError(llmretry.NoRetry(cause)); got != llmretry.ClassPermanent {
		t.Fatalf("NoRetry(503) classified %q, want %q", got, llmretry.ClassPermanent)
	}
	if got := llmretry.NoRetry(cause).Error(); got != cause.Error() {
		t.Fatalf("NoRetry message = %q, want the cause's %q", got, cause.Error())
	}

	calls := 0
	_, out, err := llmretry.Do(context.Background(), llmretry.RetryPolicy{MaxAttempts: 3}, "primary", func(model string) (any, error) {
		calls++
		return nil, llmretry.NoRetry(cause)
	})
	if err == nil || calls != 1 || out.Attempts != 1 {
		t.Fatalf("a vetoed error should stop after 1 attempt; calls=%d attempts=%d err=%v", calls, out.Attempts, err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("a vetoed error should still unwrap to its cause, got %v", err)
	}
}
