package liblicense

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/contenox/contenox/libcipher"
)

// VerifierOption allows customizing the Verifier.
type VerifierOption func(*Verifier)

// WithPayloadDecryptionKey sets the explicit 32-byte symmetric decryption key for the payload.
func WithPayloadDecryptionKey(key []byte) VerifierOption {
	return func(v *Verifier) {
		v.decryptionKey = key
	}
}

// WithVerifierKDFInfo customizes the HKDF info string used when deriving the decryption key.
func WithVerifierKDFInfo(info string) VerifierOption {
	return func(v *Verifier) {
		v.kdfInfo = info
	}
}

// WithExpectedIssuer validates that the verified claims match the expected issuer string.
func WithExpectedIssuer(expectedIssuer string) VerifierOption {
	return func(v *Verifier) {
		v.expectedIssuer = expectedIssuer
	}
}

// WithExpectedSubject validates that the verified claims match the expected subject string.
func WithExpectedSubject(expectedSubject string) VerifierOption {
	return func(v *Verifier) {
		v.expectedSubject = expectedSubject
	}
}

// Verifier validates signatures and decrypts license payloads using an SSH/Ed25519 public or private key.
type Verifier struct {
	pubKey          libcipher.SigningPublicKey
	privKey         libcipher.SigningPrivateKey // Optional, for symmetric auto-derivation
	decryptionKey   []byte
	kdfInfo         string
	expectedIssuer  string
	expectedSubject string
}

// NewVerifier creates a Verifier with an Ed25519 public key.
func NewVerifier(pubKey libcipher.SigningPublicKey, opts ...VerifierOption) (*Verifier, error) {
	if len(pubKey) != libcipher.SigningPublicKeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrBadPublicKey, libcipher.SigningPublicKeySize, len(pubKey))
	}

	v := &Verifier{
		pubKey:  pubKey,
		kdfInfo: DefaultKDFInfo,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(v)
		}
	}

	return v, nil
}

// NewVerifierFromSSHPublicKey creates a Verifier by parsing an OpenSSH or PEM public key.
func NewVerifierFromSSHPublicKey(pubKeyBytes []byte, opts ...VerifierOption) (*Verifier, error) {
	pubKey, err := ParseSSHPublicKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	return NewVerifier(pubKey, opts...)
}

// NewVerifierFromSSHPrivateKey creates a Verifier using an SSH private key (which contains both the public key
// and allows automatic decryption key derivation).
func NewVerifierFromSSHPrivateKey(pemBytes []byte, passphrase []byte, opts ...VerifierOption) (*Verifier, error) {
	privKey, err := ParseSSHPrivateKey(pemBytes, passphrase)
	if err != nil {
		return nil, err
	}

	pubKey := libcipher.SigningPublicKey(privKey.Public().(libcipher.SigningPublicKey))
	v := &Verifier{
		pubKey:  pubKey,
		privKey: privKey,
		kdfInfo: DefaultKDFInfo,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(v)
		}
	}

	return v, nil
}

// InspectHeader parses and returns the unencrypted token header without verifying or decrypting.
func (v *Verifier) InspectHeader(token string) (*Header, error) {
	h, _, _, _, err := DecodeToken(token)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// Verify validates the signature, decrypts the payload, and validates timestamps at the current time.
func (v *Verifier) Verify(token string) (*Claims, error) {
	return v.VerifyAt(token, time.Now().UTC())
}

// VerifyAt validates the signature, decrypts the payload, and validates timestamps against the given time `now`.
func (v *Verifier) VerifyAt(token string, now time.Time) (*Claims, error) {
	h, ciphertext, signature, signingData, err := DecodeToken(token)
	if err != nil {
		return nil, err
	}

	if h.Version != CurrentTokenVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidToken, h.Version)
	}
	if h.Algorithm != DefaultAlgorithm {
		return nil, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidToken, h.Algorithm)
	}

	if !libcipher.Verify(v.pubKey, signingData, signature) {
		return nil, ErrInvalidSignature
	}

	decKey := v.decryptionKey
	if len(decKey) == 0 && len(v.privKey) > 0 {
		derived, err := DeriveKey(v.privKey.Seed(), nil, v.kdfInfo)
		if err != nil {
			return nil, fmt.Errorf("liblicense: key derivation failed: %w", err)
		}
		decKey = derived
	}

	if len(decKey) == 0 {
		return nil, ErrMissingDecryptionKey
	}

	decryptor, err := libcipher.NewGCMDecryptor(decKey)
	if err != nil {
		return nil, fmt.Errorf("liblicense: failed to initialize GCM decryptor: %w", err)
	}

	plaintext, _, err := decryptor.Crypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	var claims Claims
	if err := json.Unmarshal(plaintext, &claims); err != nil {
		return nil, fmt.Errorf("%w: corrupt claims payload", ErrInvalidToken)
	}

	if h.LicenseID != "" && claims.ID != "" && h.LicenseID != claims.ID {
		return nil, fmt.Errorf("%w: license ID mismatch", ErrInvalidToken)
	}

	if v.expectedIssuer != "" && claims.Issuer != v.expectedIssuer {
		return nil, fmt.Errorf("%w: expected issuer %q, got %q", ErrInvalidToken, v.expectedIssuer, claims.Issuer)
	}
	if v.expectedSubject != "" && claims.Subject != v.expectedSubject {
		return nil, fmt.Errorf("%w: expected subject %q, got %q", ErrInvalidToken, v.expectedSubject, claims.Subject)
	}

	if err := claims.ValidateTimestamps(now); err != nil {
		return nil, err
	}

	return &claims, nil
}
