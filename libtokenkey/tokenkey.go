// Package libtokenkey holds the credential-hashing key: the server-side secret
// under which an instance token, a pairing key and a proxy key are HMAC'd
// before any of them is written down.
//
// # Why an HMAC and not a bare digest
//
// The stored value used to be base64(SHA-256(secret)). A bare digest is
// computable by anyone, so a leaked database alone lets an attacker confirm a
// candidate token offline — which matters the moment a token reaches a log, a
// backup, a screenshot or a support ticket. Under an HMAC they must also hold
// this key, which lives in the deployment's secret store and never in the
// database. libcipher.NewHash is the only construction used here.
//
// # Why the key must be its own secret
//
// libcipher's rule: keys must be secret and DISTINCT per purpose. This key is
// therefore not the deployment's Ed25519 identity — that one's public half is
// published to every enrolled machine, which is the opposite of what an HMAC key
// needs. [FromEnv] refuses if the two are configured to the same value.
//
// # Where it comes from
//
// Configuration or a mounted secret, never the repository and never a default.
// [FromEnv] reads RELAY_TOKEN_KEY_FILE (a path, which is how a Kubernetes Secret
// arrives) or RELAY_TOKEN_KEY, and returns [ErrNoSigningKey] rather than
// inventing one: a host that generated this key at startup would silently
// invalidate every instance token in the fleet on every rollout, and the symptom
// is indistinguishable from a wrong token. [Generate] exists for tests and for
// minting the secret once by hand.
package libtokenkey

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/contenox/contenox/libcipher"
)

// Environment variables carrying the credential-hashing key. The file form is
// preferred where both are set, because a mounted secret does not appear in the
// process listing or in a crash dump of the environment.
const (
	EnvSigningKeyFile = "CONTENOX_TOKEN_KEY_FILE"
	EnvSigningKey     = "CONTENOX_TOKEN_KEY"
)

// The names these carried before the gateway was its own product. They are still
// read, so a deployment that set them keeps working and a host that shares this
// secret with a relay is configured once.
const (
	legacyEnvSigningKeyFile = "RELAY_TOKEN_KEY_FILE"
	legacyEnvSigningKey     = "RELAY_TOKEN_KEY"
)

// Environment variables carrying the Ed25519 identity secret, read only to
// refuse configuring it as the hashing key.
const (
	EnvIdentityKeyFile = "RELAY_PRIVATE_KEY_FILE"
	EnvIdentityKey     = "RELAY_PRIVATE_KEY"
)

// MinKeyLen is the shortest key accepted. libcipher.GenerateKey(32) produces 64
// hex characters, so a correctly minted secret is never near this bound and
// anything that is, is a placeholder somebody meant to replace.
const MinKeyLen = 32

// Configuration failures. They are all fatal at startup; see the package comment
// on why a generated fallback would be worse than not starting.
var (
	ErrNoSigningKey = errors.New("tokenkey: no credential hashing key configured")
	ErrKeyTooShort  = errors.New("tokenkey: credential hashing key is too short")
	// ErrKeyReused reports the key configured here is also the Ed25519
	// identity secret. Refused rather than warned about: libcipher's rule is
	// that keys are distinct per purpose, and the identity key's public half
	// is handed to every enrolled machine.
	ErrKeyReused = errors.New("tokenkey: credential hashing key must not be the identity key")
)

// Purpose is the domain separator mixed into a digest, so that a value computed
// for one kind of credential can never equal one computed for another under the
// same key. It is passed to libcipher as the salt.
//
// Fixed per kind rather than random per row, deliberately. The digest IS the
// lookup key, so a per-row random salt would make the row unfindable without
// scanning and comparing every one of them. The salt's usual job — stopping one
// precomputation from covering many rows — is already done by the input being
// 256 bits of crypto/rand.
type Purpose string

// The credential kinds. The version suffix is what lets the construction change
// later without a value minted under the old one silently verifying.
const (
	// PurposeInstanceToken is the long-lived bearer credential a connector
	// presents on every dial.
	PurposeInstanceToken Purpose = "contenox-relay/instance-token/v1"
	// PurposePairingKey is the short-lived activation key an account holder
	// mints in the app and types into a machine to enrol it.
	PurposePairingKey Purpose = "contenox-relay/pairing-key/v1"
	// PurposeSession is the opaque secret carried in the browser's session
	// cookie, of which only the digest is written down.
	//
	// Separated from the instance token because without the separator a value
	// that resolved to a session would also resolve to an instance, and one
	// leaked digest column would be usable against the other table.
	PurposeSession Purpose = "contenox-relay/session/v1"
	// PurposeProxyKey is the bearer a license carries for the model gateway
	// (the api_key claim minted at checkin). Only its digest is ever at rest:
	// the ledger row is what lets the gateway verify a presented key is one
	// this host issued, and to whom, without the plaintext token existing
	// anywhere on the server.
	PurposeProxyKey Purpose = "contenox-relay/proxy-key/v1"
	// PurposeRequestSource pseudonymises a caller's address for a log line.
	//
	// NOT a credential, unlike every purpose above it, and the only one whose
	// input is not 256 bits of crypto/rand — an IPv4 address is 32 bits and the
	// whole space is enumerable, so this digest is unlinkable only to somebody
	// who does not hold the key. That is the property being bought: an operator
	// can still tell two failures apart and correlate a burst, and a leaked log
	// alone names nobody. Rotating the key invalidates every digest at once.
	PurposeRequestSource Purpose = "contenox-relay/request-source/v1"
)

// scheme prefixes every stored digest, which makes a stale row fail closed:
// a digest written under the previous bare-SHA-256 scheme is plain base64
// carrying no '$', so it cannot equal anything this package produces.
const scheme = "h1$"

// Hasher computes and compares stored credential digests under one key.
//
// It is immutable after construction and safe for concurrent use: every dial in
// the fleet hashes through the same value.
type Hasher struct {
	key []byte
}

// FromSecret builds a Hasher from the configured key text.
//
// The bytes are used as configured rather than decoded: an HMAC key is opaque
// bytes, and a decode step would only add a way for two deployments to disagree
// about what the secret was. Surrounding whitespace and a trailing newline are
// trimmed.
func FromSecret(s string) (*Hasher, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrNoSigningKey
	}
	if len(s) < MinKeyLen {
		return nil, fmt.Errorf("%w: %d bytes, want at least %d", ErrKeyTooShort, len(s), MinKeyLen)
	}
	return &Hasher{key: []byte(s)}, nil
}

// Generate mints a fresh key. For tests and for the one-off creation of the
// deployment secret — a host never calls this at startup.
func Generate() (*Hasher, error) {
	key, err := libcipher.GenerateKey(MinKeyLen)
	if err != nil {
		return nil, fmt.Errorf("tokenkey: generate: %w", err)
	}
	return FromSecret(key)
}

// FromEnv loads the key from the environment, preferring [EnvSigningKeyFile]
// over [EnvSigningKey] and falling back to the names those carried before the
// gateway was its own product. It returns [ErrNoSigningKey] when none is set, so
// a caller can distinguish "not configured" from "configured and wrong" — the
// first is a deployment that has not been finished, the second is a deployment
// that will reject every token it ever issued.
//
// It also refuses a key that is the Ed25519 identity secret; see
// [ErrKeyReused]. An UNREADABLE identity key is not reuse — its own loader
// reports that failure, and refusing here would take a host down over a file it
// may not even use.
func FromEnv() (*Hasher, error) {
	secret, err := readEnvSecret(EnvSigningKeyFile, EnvSigningKey)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		if secret, err = readEnvSecret(legacyEnvSigningKeyFile, legacyEnvSigningKey); err != nil {
			return nil, err
		}
	}
	if secret == "" {
		return nil, ErrNoSigningKey
	}
	identity, err := readEnvSecret(EnvIdentityKeyFile, EnvIdentityKey)
	if err != nil {
		identity = ""
	}
	if identity != "" && secret == identity {
		return nil, ErrKeyReused
	}
	return FromSecret(secret)
}

// readEnvSecret returns the trimmed value of a path-or-value pair, preferring
// the path; empty means neither was set.
func readEnvSecret(fileVar, valueVar string) (string, error) {
	if path := strings.TrimSpace(os.Getenv(fileVar)); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("tokenkey: read %s: %w", fileVar, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(os.Getenv(valueVar)), nil
}

// Hash returns the stored form of a credential: the scheme tag and the base64
// (std, padded) libcipher digest of secret under this key and purpose.
//
// The result is deterministic for a given (key, purpose, secret), which is what
// lets it be the indexed column a lookup matches on; see [Purpose] for why that
// costs nothing here. The plaintext is not recoverable from it.
func (h *Hasher) Hash(p Purpose, secret string) (string, error) {
	if h == nil || len(h.key) == 0 {
		return "", ErrNoSigningKey
	}
	if secret == "" {
		return "", errors.New("tokenkey: refusing to hash an empty secret")
	}
	digest, err := libcipher.NewHash(libcipher.GenerateHashArgs{
		Payload:    []byte(secret),
		SigningKey: h.key,
		Salt:       []byte(p),
	}, sha256.New)
	if err != nil {
		return "", fmt.Errorf("tokenkey: hash credential: %w", err)
	}
	return scheme + base64.StdEncoding.EncodeToString(digest), nil
}

// Equal compares two stored digests in constant time.
//
// The digests, not the secrets: the plaintext of the stored one does not exist.
// libcipher.Equal is hmac.Equal, so an unequal-length input — a malformed or a
// stale-scheme row — is false rather than a panic.
func Equal(a, b string) bool {
	return libcipher.Equal([]byte(a), []byte(b))
}
