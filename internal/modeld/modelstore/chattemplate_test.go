package modelstore

import "testing"

// TestUnit_ExtractChatTemplate pins both shapes publishers use: a template file,
// and the template buried in tokenizer_config.json — the second is how the
// Nanbeige repos ship it, and writing that JSON to chat_template.jinja would
// hand llama.cpp a tokenizer config as its Jinja program.
func TestUnit_ExtractChatTemplate(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"a template file", "{{ bos_token }}{% for m in messages %}{{ m['content'] }}{% endfor %}", "{{ bos_token }}{% for m in messages %}{{ m['content'] }}{% endfor %}"},
		{"trailing newline", "\n{{ messages }}\n", "{{ messages }}"},
		{"tokenizer config with one template", `{"add_bos_token":true,"chat_template":"{{ bos_token }}{{ messages }}"}`, "{{ bos_token }}{{ messages }}"},
		{"tokenizer config with named templates", `{"chat_template":{"default":"{{ d }}","tool_use":"{{ t }}"}}`, "{{ d }}"},
		{"tokenizer config without a template", `{"add_bos_token":true}`, ""},
		{"not json and empty", "   ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractChatTemplate([]byte(tc.raw)); got != tc.want {
				t.Fatalf("extractChatTemplate(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
