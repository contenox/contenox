package liblicense

import "errors"

// Package-level error definitions for liblicense.
var (
	// ErrInvalidToken is returned when a license token string is malformed or unparseable.
	ErrInvalidToken = errors.New("liblicense: invalid license token format")

	// ErrInvalidSignature is returned when a license's signature verification fails.
	ErrInvalidSignature = errors.New("liblicense: invalid license signature")

	// ErrLicenseExpired is returned when a license is past its expiration time.
	ErrLicenseExpired = errors.New("liblicense: license has expired")

	// ErrLicenseNotYetValid is returned when a license's NotBefore timestamp is in the future.
	ErrLicenseNotYetValid = errors.New("liblicense: license is not valid yet")

	// ErrPassphraseRequired is returned when an encrypted SSH private key is provided without a passphrase.
	ErrPassphraseRequired = errors.New("liblicense: ssh private key is encrypted; passphrase required")

	// ErrInvalidPassphrase is returned when an incorrect passphrase is supplied for an encrypted SSH private key.
	ErrInvalidPassphrase = errors.New("liblicense: incorrect passphrase for ssh private key")

	// ErrUnsupportedKeyType is returned when the key is not an Ed25519 key.
	ErrUnsupportedKeyType = errors.New("liblicense: unsupported key type; ed25519 is required")

	// ErrBadPublicKey is returned when a public key is invalid or malformed.
	ErrBadPublicKey = errors.New("liblicense: invalid public key")

	// ErrBadPrivateKey is returned when a private key is invalid or malformed.
	ErrBadPrivateKey = errors.New("liblicense: invalid private key")

	// ErrEmptyClaims is returned when attempting to issue a license with no ID or empty claims.
	ErrEmptyClaims = errors.New("liblicense: claims cannot be empty")

	// ErrDecryptionFailed is returned when the ciphertext cannot be decrypted.
	ErrDecryptionFailed = errors.New("liblicense: failed to decrypt license payload")

	// ErrMissingDecryptionKey is returned when verifying a license without an available decryption key.
	ErrMissingDecryptionKey = errors.New("liblicense: decryption key not provided")

	// ErrNoPinnedAuthorityKey is returned when attempting to use the pinned verifier without a configured authority public key.
	ErrNoPinnedAuthorityKey = errors.New("liblicense: no pinned authority public key configured")

	// ErrNoPinnedPayloadKey is returned when attempting to use the pinned
	// verifier without a configured payload decryption key.
	ErrNoPinnedPayloadKey = errors.New("liblicense: no pinned payload key configured")
)
