package acpsvc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/modelcapability"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

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

func TestUnit_HandleThinkStatusSetAndInvalid(t *testing.T) {
	sess := &sessionEntry{Think: "medium"}
	tr := &Transport{}

	out, err := tr.handleThink(sess, "")
	if err != nil {
		t.Fatalf("handleThink status: %v", err)
	}
	if out != "Think: medium" {
		t.Fatalf("status = %q, want Think: medium", out)
	}

	out, err = tr.handleThink(sess, "true")
	if err != nil {
		t.Fatalf("handleThink set alias: %v", err)
	}
	if out != "Think set to high for this session." {
		t.Fatalf("set output = %q", out)
	}
	if got := sess.think(); got != "high" {
		t.Fatalf("session think = %q, want high", got)
	}

	_, err = tr.handleThink(sess, "nonsense")
	if err == nil {
		t.Fatal("invalid think level should error")
	}
	if !strings.Contains(err.Error(), "invalid think level") {
		t.Fatalf("invalid error = %q", err.Error())
	}
	if got := sess.think(); got != "high" {
		t.Fatalf("invalid /think mutated session value to %q", got)
	}
}

func TestUnit_HandleCapabilitySetShowUnset(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "capability-acp.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tr := &Transport{deps: Deps{DB: db}}
	out, err := tr.handleCapability(ctx, "set OpenAI gpt-5-mini --think true")
	if err != nil {
		t.Fatalf("set capability: %v", err)
	}
	if out != "Capability override set for openai/gpt-5-mini: think=true." {
		t.Fatalf("set output = %q", out)
	}

	override, ok, err := modelcapability.New(runtimetypes.New(db.WithoutTransaction())).Get(ctx, "openai", "gpt-5-mini")
	if err != nil || !ok || override.CanThink == nil || !*override.CanThink {
		t.Fatalf("stored override = %#v ok=%v err=%v", override, ok, err)
	}

	out, err = tr.handleCapability(ctx, "show openai gpt-5-mini")
	if err != nil {
		t.Fatalf("show capability: %v", err)
	}
	if out != "Capability override for openai/gpt-5-mini: think=true." {
		t.Fatalf("show output = %q", out)
	}

	out, err = tr.handleCapability(ctx, "unset openai gpt-5-mini")
	if err != nil {
		t.Fatalf("unset capability: %v", err)
	}
	if out != "Capability override removed for openai/gpt-5-mini." {
		t.Fatalf("unset output = %q", out)
	}
}

func TestUnit_ParseCapabilitySetArgs(t *testing.T) {
	provider, model, canThink, err := parseCapabilitySetArgs([]string{"set", "VLLM", "Qwen/Qwen3-32B", "--think=false"})
	if err != nil {
		t.Fatalf("parse inline flag: %v", err)
	}
	if provider != "VLLM" || model != "Qwen/Qwen3-32B" || canThink {
		t.Fatalf("parsed = provider=%q model=%q canThink=%v", provider, model, canThink)
	}

	_, _, _, err = parseCapabilitySetArgs([]string{"set", "openai", "gpt-5", "--think", "maybe"})
	if err == nil || !strings.Contains(err.Error(), "--think must be true or false") {
		t.Fatalf("invalid think error = %v", err)
	}
}
