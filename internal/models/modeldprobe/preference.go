package modeldprobe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PreferredBackend reads the native backend selected by onboarding; absent state returns empty.
func PreferredBackend(root string) (string, error) {
	if root == "" {
		root = DefaultDataRoot()
	}
	data, err := os.ReadFile(filepath.Join(root, "modeld-backend"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	backend := strings.TrimSpace(string(data))
	if backend != "llama" && backend != "openvino" {
		return "", fmt.Errorf("invalid managed modeld backend preference")
	}
	return backend, nil
}

// SetBackendPreference records the native backend used by successful onboarding.
func SetBackendPreference(root, backend string) error {
	if backend != "llama" && backend != "openvino" {
		return fmt.Errorf("unsupported modeld backend preference %q", backend)
	}
	if root == "" {
		root = DefaultDataRoot()
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".modeld-backend-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(backend + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(root, "modeld-backend"))
}
