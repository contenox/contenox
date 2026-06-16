package backendservice

import (
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/runtimetypes"
)

func TestUnit_BackendService_ValidateRejectsUnknownTypes(t *testing.T) {
	invalidBackend := &runtimetypes.Backend{
		Name:    "my-backend",
		BaseURL: "http://localhost:8000",
		Type:    "unsupported-type",
	}

	err := validate(invalidBackend)
	if err == nil || !strings.Contains(err.Error(), "Type must be ollama") {
		t.Fatalf("expected validation error for unknown type, got: %v", err)
	}
}

func TestUnit_BackendService_ValidateAcceptsLlama(t *testing.T) {
	err := validate(&runtimetypes.Backend{
		Name:    "llama",
		BaseURL: "/tmp/models",
		Type:    "llama",
	})
	if err != nil {
		t.Fatalf("expected llama backend type to be accepted, got: %v", err)
	}
}

func TestUnit_BackendService_ValidateAcceptsLocalAsLlamaAlias(t *testing.T) {
	err := validate(&runtimetypes.Backend{
		Name:    "local",
		BaseURL: "/tmp/models",
		Type:    "local",
	})
	if err != nil {
		t.Fatalf("expected local backend type alias to be accepted, got: %v", err)
	}
}

func TestUnit_BackendService_ValidateRejectsRetiredLocalNodeType(t *testing.T) {
	for _, typ := range []string{"localnode"} {
		err := validate(&runtimetypes.Backend{
			Name:    typ,
			BaseURL: "/tmp/models",
			Type:    typ,
		})
		if err == nil {
			t.Fatalf("expected retired backend type %q to be rejected", typ)
		}
	}
}

func TestUnit_BackendService_ValidateRequiresNameAndURL(t *testing.T) {
	err := validate(&runtimetypes.Backend{BaseURL: "http://host", Type: "ollama"})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name validation error, got: %v", err)
	}

	err = validate(&runtimetypes.Backend{Name: "my-backend", Type: "ollama"})
	if err == nil || !strings.Contains(err.Error(), "baseURL is required") {
		t.Fatalf("expected baseURL validation error, got: %v", err)
	}
}
