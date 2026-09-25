package liblicense

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/contenox/contenox/libcipher"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/ssh"
)

const (
	// DefaultKDFInfo is the HKDF domain separation tag for payload encryption keys.
	DefaultKDFInfo = "contenox/liblicense/payload-encryption/v1"

	// EncryptionKeySize is the default AES-256 key size in bytes.
	EncryptionKeySize = 32
)

// ParseSSHPrivateKey parses an OpenSSH or PEM-encoded private key, decrypting with
// passphrase if provided. Returns an Ed25519 private key compatible with libcipher.
func ParseSSHPrivateKey(pemBytes []byte, passphrase []byte) (libcipher.SigningPrivateKey, error) {
	if len(pemBytes) == 0 {
		return nil, fmt.Errorf("%w: private key is empty", ErrInvalidToken)
	}

	var rawKey any
	var err error

	if len(passphrase) > 0 {
		rawKey, err = ssh.ParseRawPrivateKeyWithPassphrase(pemBytes, passphrase)
		if err != nil {
			if errors.Is(err, x509.IncorrectPasswordError) || strings.Contains(strings.ToLower(err.Error()), "passphrase") || strings.Contains(strings.ToLower(err.Error()), "password") {
				return nil, fmt.Errorf("%w: %v", ErrInvalidPassphrase, err)
			}
			return nil, fmt.Errorf("liblicense: failed to parse encrypted ssh private key: %w", err)
		}
	} else {
		rawKey, err = ssh.ParseRawPrivateKey(pemBytes)
		if err != nil {
			var missingPassErr *ssh.PassphraseMissingError
			if errors.As(err, &missingPassErr) || strings.Contains(strings.ToLower(err.Error()), "passphrase") {
				return nil, ErrPassphraseRequired
			}
			return nil, fmt.Errorf("liblicense: failed to parse ssh private key: %w", err)
		}
	}

	switch k := rawKey.(type) {
	case *ed25519.PrivateKey:
		if len(*k) != libcipher.SigningPrivateKeySize {
			return nil, fmt.Errorf("%w: invalid key length %d", ErrBadPrivateKey, len(*k))
		}
		return libcipher.SigningPrivateKey(*k), nil
	case ed25519.PrivateKey:
		if len(k) != libcipher.SigningPrivateKeySize {
			return nil, fmt.Errorf("%w: invalid key length %d", ErrBadPrivateKey, len(k))
		}
		return libcipher.SigningPrivateKey(k), nil
	default:
		return nil, fmt.Errorf("%w: expected ed25519, got %T", ErrUnsupportedKeyType, rawKey)
	}
}

// ParseSSHPublicKey parses an OpenSSH authorized_keys line or PEM/base64 public key.
// Returns an Ed25519 public key compatible with libcipher.
func ParseSSHPublicKey(pubKeyBytes []byte) (libcipher.SigningPublicKey, error) {
	trimmed := bytes.TrimSpace(pubKeyBytes)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: public key is empty", ErrBadPublicKey)
	}

	sshPub, _, _, _, err := ssh.ParseAuthorizedKey(trimmed)
	if err == nil {
		if cryptoPub, ok := sshPub.(ssh.CryptoPublicKey); ok {
			switch k := cryptoPub.CryptoPublicKey().(type) {
			case ed25519.PublicKey:
				if len(k) == libcipher.SigningPublicKeySize {
					return libcipher.SigningPublicKey(k), nil
				}
			case *ed25519.PublicKey:
				if len(*k) == libcipher.SigningPublicKeySize {
					return libcipher.SigningPublicKey(*k), nil
				}
			default:
				return nil, fmt.Errorf("%w: OpenSSH key type %s is not Ed25519", ErrUnsupportedKeyType, sshPub.Type())
			}
		}
	}

	if block, _ := pem.Decode(trimmed); block != nil {
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err == nil {
			if edPub, ok := pub.(ed25519.PublicKey); ok {
				return libcipher.SigningPublicKey(edPub), nil
			}
			return nil, fmt.Errorf("%w: PEM public key is %T, not Ed25519", ErrUnsupportedKeyType, pub)
		}
	}

	libPub, err := libcipher.ParsePublicKey(string(trimmed))
	if err == nil {
		return libPub, nil
	}

	return nil, fmt.Errorf("%w: unrecognized SSH public key format", ErrBadPublicKey)
}

// GenerateSSHKeyPair generates a new Ed25519 SSH keypair, returning the private key PEM
// (optionally encrypted if passphrase is provided) and the OpenSSH public key line.
func GenerateSSHKeyPair(comment string, passphrase []byte) (privPEM []byte, pubSSH []byte, err error) {
	pub, priv, err := libcipher.GenerateSigningKey()
	if err != nil {
		return nil, nil, err
	}

	sshPub, err := ssh.NewPublicKey(ed25519.PublicKey(pub))
	if err != nil {
		return nil, nil, fmt.Errorf("liblicense: failed to create SSH public key: %w", err)
	}
	pubSSH = ssh.MarshalAuthorizedKey(sshPub)
	if comment != "" {
		pubSSH = bytes.TrimSpace(pubSSH)
		pubSSH = append(pubSSH, []byte(" "+comment+"\n")...)
	}

	var pemBlock *pem.Block
	if len(passphrase) > 0 {
		pemBlock, err = ssh.MarshalPrivateKeyWithPassphrase(crypto.PrivateKey(ed25519.PrivateKey(priv)), comment, passphrase)
		if err != nil {
			return nil, nil, fmt.Errorf("liblicense: failed to marshal encrypted private key: %w", err)
		}
	} else {
		pemBlock, err = ssh.MarshalPrivateKey(crypto.PrivateKey(ed25519.PrivateKey(priv)), comment)
		if err != nil {
			return nil, nil, fmt.Errorf("liblicense: failed to marshal private key: %w", err)
		}
	}

	privPEM = pem.EncodeToMemory(pemBlock)
	return privPEM, pubSSH, nil
}

// DeriveKey derives a 32-byte symmetric AES-256 key from a seed or secret using HKDF-SHA256.
func DeriveKey(secret []byte, salt []byte, info string) ([]byte, error) {
	if len(secret) == 0 {
		return nil, errors.New("liblicense: secret cannot be empty for key derivation")
	}
	if info == "" {
		info = DefaultKDFInfo
	}
	reader := hkdf.New(sha256.New, secret, salt, []byte(info))
	key := make([]byte, EncryptionKeySize)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("liblicense: key derivation error: %w", err)
	}
	return key, nil
}
