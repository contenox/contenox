package openvino

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/transport"
)

// explainOverflow turns a raw transport context-overflow into an actionable
// message while staying errors.Is(transport.ErrContextOverflow)-true.
func TestUnit_ExplainOverflow_EnrichesContextOverflow(t *testing.T) {
	c := &client{
		modelName:  "qwen3-8b",
		cfg:        Config{NumCtx: 512},
		deviceKind: "GPU",
		freeBytes:  6 << 30, // 6 GiB
	}
	raw := fmt.Errorf("%w during suffix: resident_tokens=1 additional_tokens=404 num_ctx=128", transport.ErrContextOverflow)
	got := c.explainOverflow(raw)
	if !errors.Is(got, transport.ErrContextOverflow) {
		t.Fatalf("wrapped error must stay transport.ErrContextOverflow: %v", got)
	}
	msg := got.Error()
	for _, want := range []string{"qwen3-8b", "GPU", "6.0 GiB", "smaller model"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("overflow message missing %q:\n%s", want, msg)
		}
	}
}

func TestUnit_ExplainOverflow_PassThroughNonOverflow(t *testing.T) {
	c := &client{modelName: "qwen3-8b", cfg: Config{NumCtx: 512}}
	other := errors.New("some downstream failure")
	if got := c.explainOverflow(other); got != other {
		t.Fatalf("non-overflow error must pass through unchanged, got %v", got)
	}
	if c.explainOverflow(nil) != nil {
		t.Fatal("nil must pass through")
	}
}
