package llama

import "testing"

func TestUnit_LlamaParseHermesToolCalls(t *testing.T) {
	out := "Let me check.\n<tool_call>\n{\"name\": \"get_weather\", \"arguments\": {\"city\": \"SF\"}}\n</tool_call>"
	calls, content, err := parseHermesToolCalls(out)
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

func TestUnit_LlamaParseToolCalls_MultipleAndNone(t *testing.T) {
	multi := "<tool_call>{\"name\":\"a\",\"arguments\":{}}</tool_call><tool_call>{\"name\":\"b\",\"arguments\":{\"x\":1}}</tool_call>"
	calls, _, err := parseHermesToolCalls(multi)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Function.Name != "a" || calls[1].Function.Name != "b" {
		t.Fatalf("multi parse = %+v", calls)
	}

	calls, content, err := parseHermesToolCalls("just a plain answer")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 || content != "just a plain answer" {
		t.Fatalf("plain parse calls=%+v content=%q", calls, content)
	}
}

func TestUnit_LlamaToolCallParserFor(t *testing.T) {
	if p, err := toolCallParserFor(""); err != nil || p != nil {
		t.Fatalf("blank protocol should be (nil,nil), got p=%v err=%v", p, err)
	}
	if p, err := toolCallParserFor("hermes"); err != nil || p == nil {
		t.Fatalf("hermes protocol should resolve, got p=%v err=%v", p, err)
	}
	if _, err := toolCallParserFor("does-not-exist"); err == nil {
		t.Fatal("unknown protocol must error, not fall back")
	}
}
