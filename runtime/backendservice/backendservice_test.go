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
