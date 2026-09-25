package liblicense

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// TokenPrefix is the prefix for compact license tokens.
	TokenPrefix = "CLIC1"

	// CurrentTokenVersion is the current format version.
	CurrentTokenVersion = 1

	// DefaultAlgorithm is the cryptographic suite name.
	DefaultAlgorithm = "ED25519-AES256GCM"

	// ArmorHeader is the header line for PEM/armored license blocks.
	ArmorHeader = "-----BEGIN CONTENOX LICENSE-----"

	// ArmorFooter is the footer line for PEM/armored license blocks.
	ArmorFooter = "-----END CONTENOX LICENSE-----"
)

// Header holds the unencrypted metadata stored in the envelope.
type Header struct {
	Version   int    `json:"v"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid,omitempty"`
	LicenseID string `json:"id,omitempty"`
	ExpiresAt *int64 `json:"exp,omitempty"`
}

// EncodeToken serializes the header, ciphertext, and signature into a compact token string.
func EncodeToken(h Header, ciphertext []byte, signature []byte) (string, error) {
	headerJSON, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("liblicense: failed to marshal header: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	cipherB64 := base64.RawURLEncoding.EncodeToString(ciphertext)
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return fmt.Sprintf("%s.%s.%s.%s", TokenPrefix, headerB64, cipherB64, sigB64), nil
}

// DecodeToken parses a compact or armored token string into its constituent parts:
// Header, Ciphertext, Signature, and the signing message bytes.
func DecodeToken(rawToken string) (*Header, []byte, []byte, []byte, error) {
	token := strings.TrimSpace(rawToken)
	if strings.Contains(token, ArmorHeader) {
		dearmored, err := DearmorToken(token)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		token = dearmored
	}

	parts := strings.Split(token, ".")
	if len(parts) != 4 {
		return nil, nil, nil, nil, fmt.Errorf("%w: expected 4 segments, got %d", ErrInvalidToken, len(parts))
	}

	if parts[0] != TokenPrefix {
		return nil, nil, nil, nil, fmt.Errorf("%w: unknown token prefix %q", ErrInvalidToken, parts[0])
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid header base64: %w", ErrInvalidToken, err)
	}

	var h Header
	if err := json.Unmarshal(headerJSON, &h); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid header json: %w", ErrInvalidToken, err)
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid ciphertext base64: %w", ErrInvalidToken, err)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid signature base64: %w", ErrInvalidToken, err)
	}

	signingData := []byte(fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2]))

	return &h, ciphertext, signature, signingData, nil
}

// ArmorToken wraps a compact token in human-readable PEM-like armor lines.
func ArmorToken(token string) string {
	var buf bytes.Buffer
	buf.WriteString(ArmorHeader)
	buf.WriteString("\n")

	for len(token) > 64 {
		buf.WriteString(token[:64])
		buf.WriteString("\n")
		token = token[64:]
	}
	if len(token) > 0 {
		buf.WriteString(token)
		buf.WriteString("\n")
	}

	buf.WriteString(ArmorFooter)
	buf.WriteString("\n")
	return buf.String()
}

// DearmorToken extracts the compact token string from an armored block.
func DearmorToken(armored string) (string, error) {
	lines := strings.Split(armored, "\n")
	var tokenLines []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ArmorHeader {
			inBlock = true
			continue
		}
		if trimmed == ArmorFooter {
			inBlock = false
			break
		}
		if inBlock && trimmed != "" {
			tokenLines = append(tokenLines, trimmed)
		}
	}

	res := strings.Join(tokenLines, "")
	if res == "" {
		return "", fmt.Errorf("%w: no valid armored license block found", ErrInvalidToken)
	}
	return res, nil
}

// SigningMessage constructs the canonical byte slice to be signed for given header and ciphertext base64 strings.
func SigningMessage(headerB64, cipherB64 string) []byte {
	return []byte(fmt.Sprintf("%s.%s.%s", TokenPrefix, headerB64, cipherB64))
}
