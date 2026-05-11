package taskengine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnit_BuildUnavailableToolsPrelude_EmptyReturnsNil(t *testing.T) {
	if got := buildUnavailableToolsPrelude(nil); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
	if got := buildUnavailableToolsPrelude([]UnavailableToolsProvider{}); got != nil {
		t.Fatalf("expected nil for empty slice, got %v", got)
	}
}

func TestUnit_BuildUnavailableToolsPrelude_RoleAndContent(t *testing.T) {
	msgs := buildUnavailableToolsPrelude([]UnavailableToolsProvider{
		{Name: "brave", Reason: "HTTP 429: rate limited"},
		{Name: "notion", Reason: "connect failed: oauth client registration: HTTP 404"},
	})
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one prelude message, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected system role, got %q", msgs[0].Role)
	}
	var got struct {
		Unavailable []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"unavailable_tools_providers"`
	}
	if err := json.Unmarshal([]byte(msgs[0].Content), &got); err != nil {
		t.Fatalf("prelude content is not valid JSON: %v; body=%q", err, msgs[0].Content)
	}
	if len(got.Unavailable) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got.Unavailable))
	}
	if got.Unavailable[0].Name != "brave" || got.Unavailable[0].Error != "HTTP 429: rate limited" {
		t.Errorf("entry 0 wrong: %+v", got.Unavailable[0])
	}
	if got.Unavailable[1].Name != "notion" || !strings.Contains(got.Unavailable[1].Error, "oauth") {
		t.Errorf("entry 1 wrong: %+v", got.Unavailable[1])
	}
}

func TestUnit_ShortenChainErr_TrimsAndCaps(t *testing.T) {
	long := strings.Repeat("x", 500)
	out := shortenChainErr(&fakeErr{msg: long})
	if len(out) > 200 {
		t.Fatalf("expected <= 200 chars, got %d", len(out))
	}
	if !strings.HasSuffix(out, "...") {
		t.Errorf("expected truncation marker, got %q", out[len(out)-10:])
	}
	if shortenChainErr(nil) != "" {
		t.Errorf("expected empty string for nil error")
	}
	if got := shortenChainErr(&fakeErr{msg: "  hello\nworld  "}); got != "hello world" {
		t.Errorf("expected newline collapse and trim, got %q", got)
	}
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
