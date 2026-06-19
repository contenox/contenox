package llama

import "fmt"

const (
	reasoningProtocolCommonChat = "llama:common_chat_reasoning_parser"
)

func reasoningProtocolKnown(protocol string) bool {
	return validateReasoningProtocol(protocol) == nil
}

func validateReasoningProtocol(protocol string) error {
	switch protocol {
	case "", reasoningProtocolCommonChat:
		return nil
	default:
		return fmt.Errorf("%w: reasoning protocol %q", ErrUnsupportedFeature, protocol)
	}
}
