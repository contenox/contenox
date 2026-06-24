package setupcheck

import (
	"testing"

	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/statetype"
)

func hasIssueCode(r Result, code string) bool {
	for _, iss := range r.Issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}

// localFamilyInput models a host where modeld is live in the llama engine (llama
// backend reachable with a chat model) while the openvino backend is dormant
// (reconciled to an error). The user's default-provider names the *other* local
// engine — which, with the one-local-provider fix, must still be satisfied.
func localFamilyInput(defaultProvider, defaultModel string) Input {
	llamaID, ovID := "llama-be", "openvino-be"
	return Input{
		DefaultProvider: defaultProvider,
		DefaultModel:    defaultModel,
		RegisteredBackends: []runtimetypes.Backend{
			{ID: llamaID, Name: "llama", Type: "llama", BaseURL: "/models/llama"},
			{ID: ovID, Name: "openvino", Type: "openvino", BaseURL: "/models/openvino"},
		},
		States: []statetype.BackendRuntimeState{
			{
				ID:      llamaID,
				Name:    "llama",
				Backend: runtimetypes.Backend{ID: llamaID, Name: "llama", Type: "llama", BaseURL: "/models/llama"},
				PulledModels: []statetype.ModelPullStatus{
					{Name: "coder", Model: "coder", CanChat: true, CanPrompt: true, CanStream: true, ContextLength: 8192},
				},
			},
			{
				ID:      ovID,
				Name:    "openvino",
				Backend: runtimetypes.Backend{ID: ovID, Name: "openvino", Type: "openvino", BaseURL: "/models/openvino"},
				Error:   "modeld is running the \"llama\" engine; \"openvino\" models are dormant until modeld runs that engine",
			},
		},
	}
}

// The default-provider names openvino, but modeld is live in llama mode serving
// the default-model. Readiness for the local family must be satisfied — no
// provider/chat/model issues — because the engine is autodetected.
func TestUnit_DefaultProvider_LocalFamily_SatisfiedByLiveEngine(t *testing.T) {
	r := Evaluate(localFamilyInput("openvino", "coder"))

	for _, code := range []string{
		"default_provider_backend_missing",
		"default_provider_unreachable",
		"default_provider_not_synced",
		"no_chat_models",
		"default_model_not_available",
	} {
		if hasIssueCode(r, code) {
			t.Fatalf("unexpected issue %q for live-engine local default:\n%+v", code, r.Issues)
		}
	}
}

// The flip side: a default-model that the live engine cannot serve (an openvino
// model while modeld runs llama) is still honestly reported as unavailable — the
// fix corrects the engine-name mismatch, it does not invent models.
func TestUnit_DefaultProvider_LocalFamily_HonestWhenModelAbsent(t *testing.T) {
	r := Evaluate(localFamilyInput("openvino", "phi-4-mini-ov"))
	if !hasIssueCode(r, "default_model_not_available") {
		t.Fatalf("expected default_model_not_available for a model the live engine lacks:\n%+v", r.Issues)
	}
}

// A remote provider with no reachable backend still reports a problem — the
// local-family relaxation must not leak into non-local providers.
func TestUnit_DefaultProvider_Remote_Unaffected(t *testing.T) {
	in := localFamilyInput("openai", "gpt-5")
	in.RegisteredBackends = append(in.RegisteredBackends, runtimetypes.Backend{ID: "oa", Name: "openai", Type: "openai", BaseURL: "https://api.openai.com/v1"})
	in.States = append(in.States, statetype.BackendRuntimeState{
		ID:      "oa",
		Name:    "openai",
		Backend: runtimetypes.Backend{ID: "oa", Name: "openai", Type: "openai", BaseURL: "https://api.openai.com/v1"},
		Error:   "API key not configured",
	})
	r := Evaluate(in)
	if r.Ready() {
		t.Fatalf("openai default with an errored backend must not be ready:\n%+v", r.Issues)
	}
}
