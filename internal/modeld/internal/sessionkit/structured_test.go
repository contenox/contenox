package sessionkit

import (
	"testing"
)

// TestUnit_QwenToolCallTags_XMLDialect pins the dialect the Qwen 3.x templates
// document and train the model to emit: <function=…> with <parameter=…> blocks.
// The JSON dialect below it is what some Qwen variants emit instead; both must
// resolve to the same transport tool call.
func TestUnit_QwenToolCallTags_XMLDialect(t *testing.T) {
	raw := "I'll look at the repository first.\n\n" +
		"<tool_call>\n<function=local_fs.find_files>\n" +
		"<parameter=path>\n.\n</parameter>\n" +
		"<parameter=pattern>\n*.go\n</parameter>\n" +
		"</function>\n</tool_call>"

	chunk, err := StructuredToolCallChunk(raw)
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if chunk.Text != "I'll look at the repository first." {
		t.Fatalf("content = %q", chunk.Text)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v, want one", chunk.ToolCalls)
	}
	call := chunk.ToolCalls[0]
	if call.ID != "call_1" || call.Type != "function" {
		t.Fatalf("call identity = %+v", call)
	}
	if call.Function.Name != "local_fs.find_files" {
		t.Fatalf("name = %q", call.Function.Name)
	}
	if want := `{"path":".","pattern":"*.go"}`; call.Function.Arguments != want {
		t.Fatalf("arguments = %q, want %q", call.Function.Arguments, want)
	}
}

// TestUnit_QwenToolCallTags_XMLDialectValueFidelity pins that only the template's
// own delimiter newlines are dropped: whitespace the value carries — a code
// block's indentation — is content.
func TestUnit_QwenToolCallTags_XMLDialectValueFidelity(t *testing.T) {
	raw := "<tool_call>\n<function=native-fs-write.write>\n" +
		"<parameter=path>\nmain.go\n</parameter>\n" +
		"<parameter=content>\nfunc main() {\n\tprintln(\"hi\")\n}\n</parameter>\n" +
		"</function>\n</tool_call>"

	chunk, err := StructuredToolCallChunk(raw)
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v, want one", chunk.ToolCalls)
	}
	want := `{"content":"func main() {\n\tprintln(\"hi\")\n}","path":"main.go"}`
	if got := chunk.ToolCalls[0].Function.Arguments; got != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

// TestUnit_QwenToolCallTags_XMLDialectVariants covers the shapes models actually
// drift into: no surrounding newlines, several calls in one output, and a
// typed parameter the tool layer coerces from text.
func TestUnit_QwenToolCallTags_XMLDialectVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "no delimiter newlines",
			raw:  "<tool_call><function=fs.read><parameter=path>main.go</parameter></function></tool_call>",
			want: []string{`fs.read|{"path":"main.go"}`},
		},
		{
			name: "typed parameter stays text",
			raw: "<tool_call>\n<function=local_fs.list_dir>\n" +
				"<parameter=path>\n.\n</parameter>\n" +
				"<parameter=recursive>\ntrue\n</parameter>\n" +
				"<parameter=max_depth>\n2\n</parameter>\n" +
				"</function>\n</tool_call>",
			want: []string{`local_fs.list_dir|{"max_depth":"2","path":".","recursive":"true"}`},
		},
		{
			name: "two calls with surrounding prose",
			raw: "first\n<tool_call>\n<function=a.one>\n<parameter=x>\n1\n</parameter>\n</function>\n</tool_call>\n" +
				"middle\n<tool_call>\n<function=b.two>\n<parameter=y>\n2\n</parameter>\n</function>\n</tool_call>\ntail",
			want: []string{`a.one|{"x":"1"}`, `b.two|{"y":"2"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := StructuredToolCallChunk(tc.raw)
			if err != nil {
				t.Fatalf("StructuredToolCallChunk: %v", err)
			}
			if len(chunk.ToolCalls) != len(tc.want) {
				t.Fatalf("tool calls = %+v, want %d", chunk.ToolCalls, len(tc.want))
			}
			for i, want := range tc.want {
				got := chunk.ToolCalls[i].Function.Name + "|" + chunk.ToolCalls[i].Function.Arguments
				if got != want {
					t.Fatalf("call %d = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestUnit_QwenToolCallTags_JSONDialectStillParses guards the dialect that was
// already supported, and the JSON-arguments form of the same tag.
func TestUnit_QwenToolCallTags_JSONDialectStillParses(t *testing.T) {
	chunk, err := StructuredToolCallChunk("<tool_call>\n{\"name\":\"echo\",\"arguments\":{\"input\":\"hi\"}}\n</tool_call>")
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 || chunk.ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("tool calls = %+v", chunk.ToolCalls)
	}
	if want := `{"input":"hi"}`; chunk.ToolCalls[0].Function.Arguments != want {
		t.Fatalf("arguments = %q, want %q", chunk.ToolCalls[0].Function.Arguments, want)
	}
}

// TestUnit_QwenToolCallTags_UnknownBlockStaysContent pins the failure direction:
// a <tool_call> block in neither dialect is content, because losing the call is
// cheaper than losing the turn.
func TestUnit_QwenToolCallTags_UnknownBlockStaysContent(t *testing.T) {
	raw := "before <tool_call>not a call at all</tool_call> after"
	chunk, err := StructuredToolCallChunk(raw)
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 0 {
		t.Fatalf("tool calls = %+v, want none", chunk.ToolCalls)
	}
	if chunk.Text != raw {
		t.Fatalf("content = %q, want the block preserved", chunk.Text)
	}
}

// TestUnit_HasToolCallMarker pins the guard's trigger: a complete block in either
// dialect counts, and so does an opening marker whose payload never arrived — a
// truncated generation is a lost call, not prose.
func TestUnit_HasToolCallMarker(t *testing.T) {
	present := []string{
		"<tool_call>\n<function=a.b>\n</function>\n</tool_call>",
		"prose before <tool_call>{\"name\":\"a.b\"}</tool_call> prose after",
		"<tool_call>\n<function=a.b>\n<parameter=x>",
		"text ending mid-marker <tool_call>",
	}
	for _, text := range present {
		if !HasToolCallMarker(text) {
			t.Fatalf("HasToolCallMarker(%q) = false, want true", text)
		}
	}
	absent := []string{
		"",
		"no markers here",
		"a bare <function=a.b> without the envelope",
		"tool_call and </tool_call> without the opening bracket",
	}
	for _, text := range absent {
		if HasToolCallMarker(text) {
			t.Fatalf("HasToolCallMarker(%q) = true, want false", text)
		}
	}
}

// TestUnit_StripToolCallClosers pins the tail-of-a-call case: a turn that opens
// nothing but closes a block is emitting markup, not prose, and raw dialect in
// front of the reader is what the guard exists to prevent.
func TestUnit_StripToolCallClosers(t *testing.T) {
	for in, want := range map[string]string{
		`plain text`:                          `plain text`,
		`</tool_call>The next thing happened`: `The next thing happened`,
		"</function>\n</tool_call>done":       "\ndone",
		`a </parameter> b`:                    `a  b`,
		`x</tool_call>y`:                      `xy`,
		`<tool_call>`:                         `<tool_call>`,
		`x </Tool_Call> y`:                    `x </Tool_Call> y`,
	} {
		if got := StripToolCallClosers(in); got != want {
			t.Errorf("StripToolCallClosers(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUnit_MarkupGuard_WithholdsTheBlockAndNamesTheAttempt pins the last line of
// defence: whatever produced the delta, markup never leaves as prose, and an
// attempt with no call behind it is a failed turn rather than an answer.
func TestUnit_MarkupGuard_WithholdsTheBlockAndNamesTheAttempt(t *testing.T) {
	var g MarkupGuard
	text, thinking := g.Filter("I'll look. <tool_call>\n<function=x.y>\n<parameter=p>\n1\n", "", 0)
	if text != "I'll look. " {
		t.Fatalf("text = %q, want the prose before the marker and no markup", text)
	}
	if thinking != "" {
		t.Fatalf("thinking = %q, want empty", thinking)
	}
	if !g.AttemptWithoutCall() {
		t.Fatal("an attempt that delivered no call must fail the turn")
	}

	var withCall MarkupGuard
	withCall.Filter("<tool_call>", "", 1)
	if withCall.AttemptWithoutCall() {
		t.Fatal("a delta that carried the call itself is not a failed turn")
	}
}

// TestUnit_MarkupGuard_HoldsAPendingMarker pins the split case: a delta ending in
// the first bytes of a marker is held, so the marker cannot arrive in two pieces
// with the first one already shown.
func TestUnit_MarkupGuard_HoldsAPendingMarker(t *testing.T) {
	var g MarkupGuard
	text, _ := g.Filter("prose then <tool", "", 0)
	if text != "prose then " {
		t.Fatalf("text = %q, want the pending marker held back", text)
	}
	if !g.AttemptWithoutCall() {
		t.Fatal("a held marker with no call is an attempt")
	}
}

// TestUnit_MarkupGuard_CoversThinkingToo pins the channel the live leak used: the
// same block in reasoning is markup as well.
func TestUnit_MarkupGuard_CoversThinkingToo(t *testing.T) {
	var g MarkupGuard
	text, thinking := g.Filter("", "reasoning <tool_call><function=a.b>", 0)
	if text != "" || thinking != "reasoning " {
		t.Fatalf("text = %q thinking = %q, want the marker withheld from thinking", text, thinking)
	}
	if !g.AttemptWithoutCall() {
		t.Fatal("a withheld block in thinking with no call is an attempt")
	}
}

// TestUnit_MarkupGuard_LeavesPlainTextAlone pins that the guard is not a filter
// that edits ordinary output.
func TestUnit_MarkupGuard_LeavesPlainTextAlone(t *testing.T) {
	var g MarkupGuard
	if text, thinking := g.Filter("plain ", "thought", 0); text != "plain " || thinking != "thought" {
		t.Fatalf("text = %q thinking = %q, want both untouched", text, thinking)
	}
	if g.AttemptWithoutCall() {
		t.Fatal("plain output is not an attempt")
	}
}

// TestUnit_Minicpm5ToolCalls_AttributeDialect pins the second native dialect we
// read: MiniCPM5 writes a call as a bare <function name="…"> block with
// <param name="…"> children, and escapes a value that carries markup as CDATA.
// Nothing here is inside a <tool_call> envelope, so the reader has to find the
// calls in ordinary output.
func TestUnit_Minicpm5ToolCalls_AttributeDialect(t *testing.T) {
	raw := "I should list the directory and check the log.\n" +
		"<function name=\"local_fs.list_dir\"><param name=\"path\">/tmp</param></function>\n" +
		"<function name=\"local_shell\"><param name=\"command\"><![CDATA[cd /tmp && git log --oneline -10]]></param></function>"

	chunk, err := StructuredToolCallChunk(raw)
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if want := "I should list the directory and check the log."; chunk.Text != want {
		t.Fatalf("content = %q, want %q", chunk.Text, want)
	}
	if len(chunk.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v, want two", chunk.ToolCalls)
	}
	first, second := chunk.ToolCalls[0], chunk.ToolCalls[1]
	if first.ID != "call_1" || second.ID != "call_2" {
		t.Fatalf("call identity = %+v", chunk.ToolCalls)
	}
	if first.Function.Name != "local_fs.list_dir" || first.Function.Arguments != `{"path":"/tmp"}` {
		t.Fatalf("first call = %+v", first)
	}
	if second.Function.Name != "local_shell" || second.Function.Arguments != `{"command":"cd /tmp && git log --oneline -10"}` {
		t.Fatalf("second call = %+v", second)
	}
}

// TestUnit_Minicpm5ToolCalls_ValueFidelity pins that CDATA is an escape, not
// content: the wrapper goes, the bytes inside it stay exactly as written — a
// command's quoting and a body's indentation are what the tool has to run or
// write.
func TestUnit_Minicpm5ToolCalls_ValueFidelity(t *testing.T) {
	raw := "<function name=\"write_file\">" +
		"<param name=\"path\">main.go</param>" +
		"<param name=\"content\"><![CDATA[func main() {\n\tif a < b && c > d {\n\t\tprintln(\"hi\")\n\t}\n}]]></param>" +
		"</function>"

	chunk, err := StructuredToolCallChunk(raw)
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v, want one", chunk.ToolCalls)
	}
	want := `{"content":"func main() {\n\tif a < b && c > d {\n\t\tprintln(\"hi\")\n\t}\n}","path":"main.go"}`
	if got := chunk.ToolCalls[0].Function.Arguments; got != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

// TestUnit_Minicpm5ToolCalls_InsideAToolCallStaysOneCall pins the boundary
// between the dialects: text that looks like a bare function block but sits
// inside a <tool_call> payload is an argument, not a second call.
func TestUnit_Minicpm5ToolCalls_InsideAToolCallStaysOneCall(t *testing.T) {
	raw := "<tool_call>\n{\"name\":\"write_file\",\"arguments\":{\"content\":\"<function name=\\\"x\\\"><param name=\\\"y\\\">z</param></function>\"}}\n</tool_call>"

	chunk, err := StructuredToolCallChunk(raw)
	if err != nil {
		t.Fatalf("StructuredToolCallChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 || chunk.ToolCalls[0].Function.Name != "write_file" {
		t.Fatalf("tool calls = %+v, want the one envelope call", chunk.ToolCalls)
	}
}

// TestUnit_HasToolCallMarker_CoversBothDialects pins the gate the adapters use to
// decide an output was a call attempt: it must fire for MiniCPM5's opener too, or
// a call in that dialect is delivered as prose nobody runs.
func TestUnit_HasToolCallMarker_CoversBothDialects(t *testing.T) {
	for _, raw := range []string{
		"<tool_call>{\"name\":\"a\"}</tool_call>",
		`<function name="local_fs.list_dir"><param name="path">.</param></function>`,
	} {
		if !HasToolCallMarker(raw) {
			t.Fatalf("HasToolCallMarker(%q) = false", raw)
		}
	}
	if HasToolCallMarker("the function name=\"x\" is documented") {
		t.Fatal("prose that merely names a function is not an attempt")
	}
}

// TestUnit_MarkupGuard_HoldsTheAttributeDialect pins that a call opening in the
// attribute dialect is held from its own keyword: the parse that would rescue it
// runs at the end of the turn, and bytes delivered before then cannot be taken
// back. Holding from "<f" would eat ordinary prose, so the hold starts at the
// full keyword.
func TestUnit_MarkupGuard_HoldsTheAttributeDialect(t *testing.T) {
	var g MarkupGuard
	if text, _ := g.Filter("let me check <f", "", 0); text != "let me check <f" {
		t.Fatalf("text = %q, want an ordinary angle bracket left alone", text)
	}
	if text, _ := g.Filter("let me check <function", "", 0); text != "let me check " {
		t.Fatalf("text = %q, want the dialect keyword withheld", text)
	}
	if !g.AttemptWithoutCall() {
		t.Fatal("a withheld call opener with no call is an attempt")
	}
}
