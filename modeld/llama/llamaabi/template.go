//go:build llamanode && llama_unsafe_abi

package llamaabi

/*
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>

struct llama_model;
struct llama_vocab;
struct llama_chat_message { const char * role; const char * content; };

extern const char * llama_model_chat_template(const struct llama_model * model, const char * name);
extern const struct llama_vocab * llama_model_get_vocab(const struct llama_model * model);
extern _Bool llama_vocab_get_add_bos(const struct llama_vocab * vocab);
extern int32_t llama_chat_apply_template(const char * tmpl, const struct llama_chat_message * chat, size_t n_msg, _Bool add_ass, char * buf, int32_t length);

typedef int32_t llama_token;
extern llama_token llama_vocab_bos(const struct llama_vocab * vocab);
extern llama_token llama_vocab_eos(const struct llama_vocab * vocab);
extern const char * llama_vocab_get_text(const struct llama_vocab * vocab, llama_token token);

// Thin wrappers so the Go side never touches the C _Bool ABI directly.
static int cx_vocab_add_bos(const struct llama_vocab * vocab) {
    return llama_vocab_get_add_bos(vocab) ? 1 : 0;
}

// cx_bos_text / cx_eos_text return the model's BOS/EOS token text (e.g.
// "<|endoftext|>"), which the Jinja chat template references via its bos_token /
// eos_token variables. "" when the model declares none.
static const char * cx_bos_text(const struct llama_model * model) {
    const struct llama_vocab * v = llama_model_get_vocab(model);
    llama_token t = llama_vocab_bos(v);
    if (t < 0) return "";
    const char * s = llama_vocab_get_text(v, t);
    return s ? s : "";
}
static const char * cx_eos_text(const struct llama_model * model) {
    const struct llama_vocab * v = llama_model_get_vocab(model);
    llama_token t = llama_vocab_eos(v);
    if (t < 0) return "";
    const char * s = llama_vocab_get_text(v, t);
    return s ? s : "";
}
static int cx_apply_chat_template(const char * tmpl, const struct llama_chat_message * chat, size_t n_msg, int add_ass, char * buf, int length) {
    return (int) llama_chat_apply_template(tmpl, chat, n_msg, add_ass != 0, buf, (int32_t) length);
}
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"

	"github.com/contenox/runtime/modeld/llama/chattmpl"
	"github.com/ollama/ollama/llama"
)

// ApplyChatTemplateTools renders the model's OWN chat template (the GGUF Jinja),
// including tool definitions, via the minja engine — the model-native path the
// legacy llama_chat_apply_template cannot provide. messagesJSON is a JSON array of
// chat messages; toolsJSON is a JSON array of tool definitions ("" for none).
func ApplyChatTemplateTools(m *llama.Model, messagesJSON, toolsJSON string, addAssistant bool) (string, error) {
	source := ModelChatTemplate(m)
	if source == "" {
		return "", errors.New("llamaabi: model declares no chat template")
	}
	var bos, eos string
	if cm := modelPtr(m); cm != nil {
		bos = C.GoString(C.cx_bos_text(cm))
		eos = C.GoString(C.cx_eos_text(cm))
	}
	runtime.KeepAlive(m)
	return chattmpl.Render(source, bos, eos, messagesJSON, toolsJSON, addAssistant)
}

// ChatMessage is one role/content turn for chat-template application.
// ToolCalls is a raw JSON string (the model's own tool_calls array) and is
// only populated for role=="assistant" turns that triggered a tool call.
// ToolCallID is only populated for role=="tool" result turns.
// Both fields are passed through to minja as-is; the C llama_chat_apply_template
// path (ApplyChatTemplate) ignores them because that API only accepts role/content.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  string // raw JSON array, e.g. [{"id":"...","type":"function",...}]
	ToolCallID string // for role=="tool" result turns
}

// modelPtr extracts the upstream *llama_model from the ollama wrapper, whose
// first field is the C handle (same layout trick as contextPtr).
func modelPtr(m *llama.Model) *C.struct_llama_model {
	if m == nil {
		return nil
	}
	return (*C.struct_llama_model)((*contextPrefix)(unsafe.Pointer(m)).c)
}

// ModelChatTemplate returns the model's OWN chat template baked into the GGUF
// (tokenizer.chat_template metadata), or "" if the model declares none. This is
// what makes templating model-driven instead of a hardcoded format.
func ModelChatTemplate(m *llama.Model) string {
	cm := modelPtr(m)
	if cm == nil {
		return ""
	}
	t := C.llama_model_chat_template(cm, nil)
	runtime.KeepAlive(m)
	if t == nil {
		return ""
	}
	return C.GoString(t)
}

// AddBOS reports whether the model's tokenizer adds a BOS token, read from the
// GGUF metadata — so BOS policy is model-driven, not a config guess.
func AddBOS(m *llama.Model) bool {
	cm := modelPtr(m)
	if cm == nil {
		return false
	}
	v := C.llama_model_get_vocab(cm)
	on := C.cx_vocab_add_bos(v) != 0
	runtime.KeepAlive(m)
	return on
}

// ApplyChatTemplate renders the turns with the model's own chat template via
// llama.cpp. llama.cpp matches the template against its pre-defined formats
// (chatml, llama2/3, mistral, ...) — not arbitrary Jinja — and reports an error
// when the model's template is unsupported. addAssistant appends the assistant
// generation prefix.
func ApplyChatTemplate(m *llama.Model, messages []ChatMessage, addAssistant bool) (string, error) {
	tmpl := ModelChatTemplate(m)
	if tmpl == "" {
		return "", errors.New("llamaabi: model declares no chat template")
	}
	return applyChatTemplate(tmpl, messages, addAssistant)
}

func applyChatTemplate(tmpl string, messages []ChatMessage, addAssistant bool) (string, error) {
	ctmpl := C.CString(tmpl)
	defer C.free(unsafe.Pointer(ctmpl))

	cmsgs := make([]C.struct_llama_chat_message, len(messages))
	for i, msg := range messages {
		cr := C.CString(msg.Role)
		cc := C.CString(msg.Content)
		defer C.free(unsafe.Pointer(cr))
		defer C.free(unsafe.Pointer(cc))
		cmsgs[i].role = cr
		cmsgs[i].content = cc
	}
	var msgPtr *C.struct_llama_chat_message
	if len(cmsgs) > 0 {
		msgPtr = (*C.struct_llama_chat_message)(unsafe.Pointer(&cmsgs[0]))
	}
	addAss := C.int(0)
	if addAssistant {
		addAss = 1
	}

	// First call sizes the output, second fills it.
	need := C.cx_apply_chat_template(ctmpl, msgPtr, C.size_t(len(messages)), addAss, nil, 0)
	if need < 0 {
		return "", errors.New("llamaabi: chat template not supported by llama.cpp")
	}
	buf := make([]byte, int(need)+1)
	got := C.cx_apply_chat_template(ctmpl, msgPtr, C.size_t(len(messages)), addAss, (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if got < 0 {
		return "", errors.New("llamaabi: chat template apply failed")
	}
	if int(got) > len(buf) {
		got = C.int(len(buf))
	}
	return string(buf[:int(got)]), nil
}
