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
	ContextLength   int              `json:"context_length,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	CanChat         *bool            `json:"can_chat,omitempty"`
	CanEmbed        *bool            `json:"can_embed,omitempty"`
	CanPrompt       *bool            `json:"can_prompt,omitempty"`
	CanStream       *bool            `json:"can_stream,omitempty"`
	CanThink        bool             `json:"can_think,omitempty"`
	Device          string           `json:"device,omitempty"`
	GenAI           json.RawMessage  `json:"genai,omitempty"`
	ToolCalls       toolCallsProfile `json:"tool_calls,omitempty"`
	Reasoning       reasoningProfile `json:"reasoning,omitempty"`
}

// toolCallsProfile declares the OpenVINO GenAI parser or structured-output
// protocol certified for this model's tool-call output. Empty means the model is
// not certified for tool calls and the provider reports them unsupported rather
// than guessing.
type toolCallsProfile struct {
	Protocol string `json:"protocol,omitempty"`
}

type reasoningProfile struct {
	Protocol       string `json:"protocol,omitempty"`
	StreamProtocol string `json:"stream_protocol,omitempty"`
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
	if p.ToolCalls.Protocol != "" && !toolCallProtocolKnown(p.ToolCalls.Protocol) {
		return fmt.Errorf("openvino profile %s: unknown tool_calls.protocol %q", path, p.ToolCalls.Protocol)
	}
	if p.Reasoning.Protocol != "" && !reasoningProtocolKnown(p.Reasoning.Protocol) {
		return fmt.Errorf("openvino profile %s: unknown reasoning.protocol %q", path, p.Reasoning.Protocol)
	}
	if p.Reasoning.StreamProtocol != "" && !reasoningIncrementalProtocolKnown(p.Reasoning.StreamProtocol) {
		return fmt.Errorf("openvino profile %s: unknown reasoning.stream_protocol %q", path, p.Reasoning.StreamProtocol)
	}
	return nil
}

func (p modelProfile) capabilityConfig() modelrepo.CapabilityConfig {
	canChat := boolDefault(p.CanChat, true)
	return modelrepo.CapabilityConfig{
		ContextLength:   p.ContextLength,
		MaxOutputTokens: p.MaxOutputTokens,
		CanChat:         canChat,
		CanEmbed:        boolDefault(p.CanEmbed, false),
		CanPrompt:       boolDefault(p.CanPrompt, canChat),
		CanStream:       boolDefault(p.CanStream, canChat),
		CanThink:        p.CanThink || p.Reasoning.hasProtocol(),
	}
}

func boolDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func (r reasoningProfile) hasProtocol() bool {
	_, stream := r.protocols()
	return stream != ""
}

func (r reasoningProfile) protocols() (string, string) {
	protocol := r.Protocol
	stream := r.StreamProtocol
	if stream == "" {
		stream = reasoningStreamProtocolFor(protocol)
	}
	if protocol != "" && reasoningIncrementalProtocolKnown(protocol) {
		if stream == "" {
			stream = protocol
		}
		protocol = ""
	}
	return protocol, stream
}

func reasoningProtocolKnown(protocol string) bool {
	return reasoningCompleteProtocolKnown(protocol) || reasoningIncrementalProtocolKnown(protocol)
}

func reasoningCompleteProtocolKnown(protocol string) bool {
	switch protocol {
	case "openvino:reasoning_parser",
		"openvino:deepseek_r1_reasoning_parser",
		"openvino:phi4_reasoning_parser":
		return true
	default:
		return false
	}
}

func reasoningIncrementalProtocolKnown(protocol string) bool {
	switch protocol {
	case "openvino:reasoning_incremental_parser",
		"openvino:deepseek_r1_reasoning_incremental_parser",
		"openvino:phi4_reasoning_incremental_parser":
		return true
	default:
		return false
	}
}

func reasoningStreamProtocolFor(protocol string) string {
	switch protocol {
	case "openvino:reasoning_parser":
		return "openvino:reasoning_incremental_parser"
	case "openvino:deepseek_r1_reasoning_parser":
		return "openvino:deepseek_r1_reasoning_incremental_parser"
	case "openvino:phi4_reasoning_parser":
		return "openvino:phi4_reasoning_incremental_parser"
	default:
		return ""
	}
}
