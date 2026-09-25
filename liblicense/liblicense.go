// Package liblicense provides cryptographically signed and encrypted software license
// generation, issuance, and verification with arbitrary Key-Value (KV) payloads.
// It is built on top of libcipher (Ed25519 signatures, AES-256-GCM authenticated encryption)
// and uses standard SSH key pairs (with or without passphrases) as the root of trust / seed.
package liblicense

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/contenox/contenox/libcipher"
)

// PinnedAuthorityPublicKey is the official Contenox licensing authority Ed25519 public key.
// It is empty in the OSS source tree and is injected at compile-time via -ldflags during official release builds.
var PinnedAuthorityPublicKey string

// PinnedPayloadKey is the 32-byte payload encryption key every license the
// relay mints is AES-256-GCM encrypted with. Empty in the OSS source tree and
// injected at compile-time via -ldflags alongside [PinnedAuthorityPublicKey]
// in official release builds: a client that cannot decrypt the license it
// received at checkin cannot build the model provider its claims describe.
var PinnedPayloadKey string

// init keeps the two injected pins alive for -ldflags -X: with no live
// reference the linker strips them and injection silently writes nothing. An
// empty pin stays inert and a set-but-unusable one panics at start.
func init() {
	if strings.TrimSpace(PinnedAuthorityPublicKey) != "" {
		if _, err := ParseAuthorityPublicKey(PinnedAuthorityPublicKey); err != nil {
			panic("liblicense: pinned authority public key is unusable: " + err.Error())
		}
	}
	if strings.TrimSpace(PinnedPayloadKey) != "" {
		if _, err := pinnedPayloadKey(); err != nil {
			panic("liblicense: pinned payload key is unusable: " + err.Error())
		}
	}
}

// ParseAuthorityPublicKey parses an authority public key from OpenSSH wire format, PEM block, or base64 raw bytes.
func ParseAuthorityPublicKey(s string) (libcipher.SigningPublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrNoPinnedAuthorityKey
	}

	if strings.HasPrefix(s, "ssh-ed25519") || strings.Contains(s, "-----BEGIN") {
		return ParseSSHPublicKey([]byte(s))
	}

	pubKey, err := libcipher.ParsePublicKey(s)
	if err == nil {
		return pubKey, nil
	}

	return ParseSSHPublicKey([]byte(s))
}

// NewPinnedVerifier constructs a Verifier configured with the build-time PinnedAuthorityPublicKey.
func NewPinnedVerifier(opts ...VerifierOption) (*Verifier, error) {
	pubKey, err := ParseAuthorityPublicKey(PinnedAuthorityPublicKey)
	if err != nil {
		return nil, err
	}
	return NewVerifier(pubKey, opts...)
}

// QuickIssue issues an encrypted and signed license token from an SSH private key PEM.
func QuickIssue(privPEM []byte, passphrase []byte, claims Claims) (string, error) {
	iss, err := NewIssuerFromSSHPrivateKey(privPEM, passphrase)
	if err != nil {
		return "", err
	}
	return iss.Issue(claims)
}

// QuickVerify verifies and decrypts a license token using an SSH private key (which contains the full seed).
func QuickVerify(privPEM []byte, passphrase []byte, token string) (*Claims, error) {
	v, err := NewVerifierFromSSHPrivateKey(privPEM, passphrase)
	if err != nil {
		return nil, err
	}
	return v.Verify(token)
}

// QuickVerifyWithPublicKey verifies the signature using an SSH public key and decrypts with the payload key.
func QuickVerifyWithPublicKey(pubKeyBytes []byte, payloadKey []byte, token string) (*Claims, error) {
	v, err := NewVerifierFromSSHPublicKey(pubKeyBytes, WithPayloadDecryptionKey(payloadKey))
	if err != nil {
		return nil, err
	}
	return v.Verify(token)
}

// QuickVerifyPinned verifies and decrypts a token using the build-time PinnedAuthorityPublicKey.
func QuickVerifyPinned(payloadKey []byte, token string) (*Claims, error) {
	v, err := NewPinnedVerifier(WithPayloadDecryptionKey(payloadKey))
	if err != nil {
		return nil, err
	}
	return v.Verify(token)
}

// QuickVerifyPinnedAt verifies a token using the build-time PinnedAuthorityPublicKey at an explicit timestamp.
func QuickVerifyPinnedAt(payloadKey []byte, token string, at time.Time) (*Claims, error) {
	v, err := NewPinnedVerifier(WithPayloadDecryptionKey(payloadKey))
	if err != nil {
		return nil, err
	}
	return v.VerifyAt(token, at)
}

// VerifyPinnedToken verifies and decrypts a token with both build-time pinned
// values: [PinnedAuthorityPublicKey] for the signature and [PinnedPayloadKey]
// for the payload. This is the client entry point the contenox model provider
// uses to turn a checked-in license into a configured provider.
func VerifyPinnedToken(token string) (*Claims, error) {
	payloadKey, err := pinnedPayloadKey()
	if err != nil {
		return nil, err
	}
	v, err := NewPinnedVerifier(WithPayloadDecryptionKey(payloadKey))
	if err != nil {
		return nil, err
	}
	return v.Verify(token)
}

// pinnedPayloadKey resolves [PinnedPayloadKey] as raw bytes, hex or base64.
func pinnedPayloadKey() ([]byte, error) {
	raw := strings.TrimSpace(PinnedPayloadKey)
	if raw == "" {
		return nil, ErrNoPinnedPayloadKey
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	if b, err := hex.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, ErrNoPinnedPayloadKey
}

// QuickVerifyAt verifies the token at an explicit timestamp.
func QuickVerifyAt(pubKeyBytes []byte, payloadKey []byte, token string, at time.Time) (*Claims, error) {
	v, err := NewVerifierFromSSHPublicKey(pubKeyBytes, WithPayloadDecryptionKey(payloadKey))
	if err != nil {
		return nil, err
	}
	return v.VerifyAt(token, at)
}
