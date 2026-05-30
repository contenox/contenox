package acpsvc

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantArgs string
		wantOk   bool
	}{
		{name: "bare command", input: "/help", wantName: "help", wantOk: true},
		{name: "command with args", input: "/model qwen2.5:7b", wantName: "model", wantArgs: "qwen2.5:7b", wantOk: true},
		{name: "command with trailing space", input: "/clear   ", wantName: "clear", wantOk: true},
		{name: "leading whitespace", input: "  /provider ollama", wantName: "provider", wantArgs: "ollama", wantOk: true},
		{name: "compact with keep", input: "/compact 12", wantName: "compact", wantArgs: "12", wantOk: true},
		{name: "args with extra spaces collapse to trimmed", input: "/model   gpt-4o  ", wantName: "model", wantArgs: "gpt-4o", wantOk: true},

		// Not commands:
		{name: "plain text", input: "hello there", wantOk: false},
		{name: "unknown slash word", input: "/foobar", wantOk: false},
		{name: "absolute path", input: "/home/user/file.go", wantOk: false},
		{name: "mid-sentence slash path", input: "what does /etc/passwd do", wantOk: false},
		{name: "empty", input: "", wantOk: false},
		{name: "just a slash", input: "/", wantOk: false},
		{name: "command not leading", input: "please run /help", wantOk: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, args, ok := parseCommand(tc.input)
			if ok != tc.wantOk {
				t.Fatalf("parseCommand(%q) ok = %v, want %v", tc.input, ok, tc.wantOk)
			}
			if !tc.wantOk {
				return
			}
			if name != tc.wantName {
				t.Errorf("parseCommand(%q) name = %q, want %q", tc.input, name, tc.wantName)
			}
			if args != tc.wantArgs {
				t.Errorf("parseCommand(%q) args = %q, want %q", tc.input, args, tc.wantArgs)
			}
		})
	}
}

func TestAcpCommandsCoverDispatch(t *testing.T) {
	// Every advertised command must be recognized by parseCommand, so the menu
	// the client shows never offers a command Prompt would pass through as text.
	for _, c := range acpCommands() {
		if _, _, ok := parseCommand("/" + c.Name); !ok {
			t.Errorf("advertised command %q is not recognized by parseCommand", c.Name)
		}
	}
}
