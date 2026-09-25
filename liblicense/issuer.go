package liblicense

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/contenox/contenox/libcipher"
)

// IssuerOption allows customizing the Issuer.
type IssuerOption func(*Issuer)

// WithPayloadEncryptionKey sets an explicit 32-byte symmetric encryption key for the payload.
func WithPayloadEncryptionKey(key []byte) IssuerOption {
	return func(iss *Issuer) {
		iss.encryptionKey = key
	}
}

// WithIssuerKeyID sets a key ID (kid) in the token header.
func WithIssuerKeyID(keyID string) IssuerOption {
	return func(iss *Issuer) {
		iss.keyID = keyID
	}
}

// WithIssuerKDFInfo customizes the HKDF info string used to derive the encryption key.
func WithIssuerKDFInfo(info string) IssuerOption {
	return func(iss *Issuer) {
		iss.kdfInfo = info
	}
}

// Issuer creates, encrypts, and signs license tokens using an SSH/Ed25519 private key.
type Issuer struct {
	privKey       libcipher.SigningPrivateKey
	pubKey        libcipher.SigningPublicKey
	encryptionKey []byte
	keyID         string
	kdfInfo       string
}

// NewIssuer creates a new Issuer using an Ed25519 signing private key.
func NewIssuer(privKey libcipher.SigningPrivateKey, opts ...IssuerOption) (*Issuer, error) {
	if len(privKey) != libcipher.SigningPrivateKeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrBadPrivateKey, libcipher.SigningPrivateKeySize, len(privKey))
	}

	pubKey := libcipher.SigningPublicKey(privKey.Public().(libcipher.SigningPublicKey))

	fingerprint := sha256.Sum256(pubKey)
	defaultKeyID := hex.EncodeToString(fingerprint[:8])

	iss := &Issuer{
		privKey: privKey,
		pubKey:  pubKey,
		keyID:   defaultKeyID,
		kdfInfo: DefaultKDFInfo,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(iss)
		}
	}

	return iss, nil
}

// NewIssuerFromSSHPrivateKey creates an Issuer by parsing an SSH private key (with optional passphrase).
func NewIssuerFromSSHPrivateKey(pemBytes []byte, passphrase []byte, opts ...IssuerOption) (*Issuer, error) {
	privKey, err := ParseSSHPrivateKey(pemBytes, passphrase)
	if err != nil {
		return nil, err
	}
	return NewIssuer(privKey, opts...)
}

// Issue generates, encrypts, and signs a license token in compact format.
func (iss *Issuer) Issue(claims Claims) (string, error) {
	if claims.ID == "" {
		return "", fmt.Errorf("%w: ID is required", ErrEmptyClaims)
	}
	if claims.IssuedAt.IsZero() {
		claims.IssuedAt = time.Now().UTC()
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("liblicense: failed to marshal claims: %w", err)
	}

	encKey := iss.encryptionKey
	if len(encKey) == 0 {
		derived, err := DeriveKey(iss.privKey.Seed(), nil, iss.kdfInfo)
		if err != nil {
			return "", fmt.Errorf("liblicense: failed to derive encryption key: %w", err)
		}
		encKey = derived
	}

	encryptor, err := libcipher.NewGCMEncryptor(encKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("liblicense: failed to initialize GCM encryptor: %w", err)
	}

	var exp *int64
	if claims.ExpiresAt != nil {
		unixExp := claims.ExpiresAt.Unix()
		exp = &unixExp
	}

	h := Header{
		Version:   CurrentTokenVersion,
		Algorithm: DefaultAlgorithm,
		KeyID:     iss.keyID,
		LicenseID: claims.ID,
		ExpiresAt: exp,
	}

	headerJSON, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("liblicense: failed to marshal header: %w", err)
	}

	ciphertext, err := encryptor.Crypt(claimsJSON, headerJSON)
	if err != nil {
		return "", fmt.Errorf("liblicense: payload encryption failed: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	cipherB64 := base64.RawURLEncoding.EncodeToString(ciphertext)

	msg := SigningMessage(headerB64, cipherB64)

	signature, err := libcipher.Sign(iss.privKey, msg)
	if err != nil {
		return "", fmt.Errorf("liblicense: signing failed: %w", err)
	}

	return EncodeToken(h, ciphertext, signature)
}

// IssueArmored generates, encrypts, signs, and formats a license token as an armored text block.
func (iss *Issuer) IssueArmored(claims Claims) (string, error) {
	token, err := iss.Issue(claims)
	if err != nil {
		return "", err
	}
	return ArmorToken(token), nil
}

// PublicKey returns the issuer's public key.
func (iss *Issuer) PublicKey() libcipher.SigningPublicKey {
	return iss.pubKey
}
