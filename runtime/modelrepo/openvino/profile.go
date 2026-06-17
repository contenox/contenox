package openvino

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelrepo"
)

const profileFileName = "contenox-openvino.json"

// modelProfile is the declared, tested profile beside an OpenVINO IR model. The
// runtime only needs the capability limits and (optionally) a device hint;
// device selection and GenAI pipeline knobs are owned by modeld, so those fields
// are accepted for forward/backward compatibility but not consumed here.
type modelProfile struct {
	ContextLength   int             `json:"context_length,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	CanThink        bool            `json:"can_think,omitempty"`
	Device          string          `json:"device,omitempty"`
	GenAI           json.RawMessage `json:"genai,omitempty"`
	ToolCalls       json.RawMessage `json:"tool_calls,omitempty"`
	Reasoning       json.RawMessage `json:"reasoning,omitempty"`
}

func loadModelProfile(modelPath string) (modelProfile, error) {
	path := filepath.Join(modelPath, profileFileName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return modelProfile{}, nil
		}
		return modelProfile{}, fmt.Errorf("openvino profile open %s: %w", path, err)
	}
	defer f.Close()

	var profile modelProfile
	if err := json.NewDecoder(f).Decode(&profile); err != nil {
		return modelProfile{}, fmt.Errorf("openvino profile decode %s: %w", path, err)
	}
	if err := profile.validate(path); err != nil {
		return modelProfile{}, err
	}
	return profile, nil
}

func (p modelProfile) validate(path string) error {
	if p.ContextLength < 0 {
		return fmt.Errorf("openvino profile %s: context_length must be non-negative", path)
	}
	if p.MaxOutputTokens < 0 {
		return fmt.Errorf("openvino profile %s: max_output_tokens must be non-negative", path)
	}
	return nil
}

func (p modelProfile) capabilityConfig() modelrepo.CapabilityConfig {
	return modelrepo.CapabilityConfig{
		ContextLength:   p.ContextLength,
		MaxOutputTokens: p.MaxOutputTokens,
		CanChat:         true,
		CanPrompt:       true,
		CanStream:       true,
		CanThink:        p.CanThink,
	}
}
