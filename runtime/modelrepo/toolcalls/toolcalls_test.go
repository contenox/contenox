package toolcalls

import (
	"errors"
	"testing"
)

func TestUnit_ParseHermesToolCalls(t *testing.T) {
	out := "Let me check.\n<tool_call>\n{\"name\": \"get_weather\", \"arguments\": {\"city\": \"SF\"}}\n</tool_call>"
	calls, content, err := ParseHermes(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("got %d tool calls, want 1: %+v", len(calls), calls)
	}
	if calls[0].Function.Name != "get_weather" {
		t.Fatalf("tool name = %q", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"city": "SF"}` {
		t.Fatalf("arguments = %q", calls[0].Function.Arguments)
	}
	if content != "Let me check." {
		t.Fatalf("content = %q", content)
	}
}

func TestUnit_ParseToolCalls_MultipleAndNone(t *testing.T) {
	multi := "<tool_call>{\"name\":\"a\",\"arguments\":{}}</tool_call><tool_call>{\"name\":\"b\",\"arguments\":{\"x\":1}}</tool_call>"
	calls, _, err := ParseHermes(multi)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Function.Name != "a" || calls[1].Function.Name != "b" {
		t.Fatalf("multi parse = %+v", calls)
	}

	calls, content, err := ParseHermes("just a plain answer")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 || content != "just a plain answer" {
		t.Fatalf("plain parse calls=%+v content=%q", calls, content)
	}
}

func TestUnit_ParserFor(t *testing.T) {
	if p, err := ParserFor(""); err != nil || p != nil {
		t.Fatalf("blank protocol should be (nil,nil), got p=%v err=%v", p, err)
	}
	if p, err := ParserFor("hermes"); err != nil || p == nil {
		t.Fatalf("hermes protocol should resolve, got p=%v err=%v", p, err)
	}
	if p, err := ParserFor("qwen"); err != nil || p == nil {
		t.Fatalf("qwen protocol should resolve, got p=%v err=%v", p, err)
	}
	if _, err := ParserFor("does-not-exist"); !errors.Is(err, ErrUnknownProtocol) {
		t.Fatalf("unknown protocol must return ErrUnknownProtocol, got %v", err)
	}
}
