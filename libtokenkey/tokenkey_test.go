package libtokenkey_test

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/contenox/libtokenkey"
	"github.com/stretchr/testify/require"
)

const aToken = "Zm9vYmFyLXRoaXMtaXMtYS1mYWtlLXRva2Vu"

// The digest is deterministic for one key and purpose, which is what lets it be
// the indexed column a lookup matches on, and it is not the bare SHA-256 the
// service used to store — the key is what a leaked dump would still be missing.
func TestUnit_TokenKey_HashIsKeyedAndDeterministic(t *testing.T) {
	h, err := libtokenkey.Generate()
	require.NoError(t, err)

	first, err := h.Hash(libtokenkey.PurposeInstanceToken, aToken)
	require.NoError(t, err)
	second, err := h.Hash(libtokenkey.PurposeInstanceToken, aToken)
	require.NoError(t, err)
	require.Equal(t, first, second, "the digest is the lookup key and must be stable")
	require.True(t, libtokenkey.Equal(first, second))

	sum := sha256.Sum256([]byte(aToken))
	require.NotEqual(t, base64.StdEncoding.EncodeToString(sum[:]), first,
		"a stored digest must not be computable without the key")
	require.NotContains(t, first, aToken)

	require.False(t, libtokenkey.Equal(base64.StdEncoding.EncodeToString(sum[:]), first),
		"a stale bare-SHA-256 row carries no scheme tag and can never match")
}

// Another key gives another digest. That is the property the whole change buys:
// the database alone is not enough to verify a candidate token.
func TestUnit_TokenKey_DifferentKeysDisagree(t *testing.T) {
	a, err := libtokenkey.Generate()
	require.NoError(t, err)
	b, err := libtokenkey.Generate()
	require.NoError(t, err)

	fromA, err := a.Hash(libtokenkey.PurposeInstanceToken, aToken)
	require.NoError(t, err)
	fromB, err := b.Hash(libtokenkey.PurposeInstanceToken, aToken)
	require.NoError(t, err)
	require.False(t, libtokenkey.Equal(fromA, fromB))
}

// The purpose is domain separation, not decoration: the same secret under two
// kinds of credential must never produce one stored value, or a pairing key
// could be presented as an instance token.
func TestUnit_TokenKey_PurposeSeparatesCredentials(t *testing.T) {
	h, err := libtokenkey.Generate()
	require.NoError(t, err)

	asToken, err := h.Hash(libtokenkey.PurposeInstanceToken, aToken)
	require.NoError(t, err)
	asCode, err := h.Hash(libtokenkey.PurposePairingKey, aToken)
	require.NoError(t, err)
	require.False(t, libtokenkey.Equal(asToken, asCode))
}

func TestUnit_TokenKey_RefusesDegenerateInput(t *testing.T) {
	h, err := libtokenkey.Generate()
	require.NoError(t, err)
	_, err = h.Hash(libtokenkey.PurposeInstanceToken, "")
	require.Error(t, err)

	var missing *libtokenkey.Hasher
	_, err = missing.Hash(libtokenkey.PurposeInstanceToken, aToken)
	require.ErrorIs(t, err, libtokenkey.ErrNoSigningKey,
		"a nil hasher must fail rather than hash under an empty key")

	_, err = libtokenkey.FromSecret("")
	require.ErrorIs(t, err, libtokenkey.ErrNoSigningKey)
	_, err = libtokenkey.FromSecret(strings.Repeat("x", libtokenkey.MinKeyLen-1))
	require.ErrorIs(t, err, libtokenkey.ErrKeyTooShort)
}

// The key comes from configuration or a mounted secret. A host that finds
// neither must say so rather than invent one: a generated key would reject every
// token the host had ever issued after the next rollout, and the symptom is
// indistinguishable from a fleet presenting wrong tokens.
func TestUnit_TokenKey_FromEnv(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKeyFile, "")
	t.Setenv(libtokenkey.EnvSigningKey, "")
	t.Setenv(libtokenkey.EnvIdentityKeyFile, "")
	t.Setenv(libtokenkey.EnvIdentityKey, "")
	_, err := libtokenkey.FromEnv()
	require.ErrorIs(t, err, libtokenkey.ErrNoSigningKey)

	value := strings.Repeat("a", libtokenkey.MinKeyLen)
	t.Setenv(libtokenkey.EnvSigningKey, value)
	fromValue, err := libtokenkey.FromEnv()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "token.key")
	fileValue := strings.Repeat("b", libtokenkey.MinKeyLen)
	require.NoError(t, os.WriteFile(path, []byte(fileValue+"\n"), 0o600))
	t.Setenv(libtokenkey.EnvSigningKeyFile, path)
	fromFile, err := libtokenkey.FromEnv()
	require.NoError(t, err)

	a, err := fromValue.Hash(libtokenkey.PurposePairingKey, aToken)
	require.NoError(t, err)
	b, err := fromFile.Hash(libtokenkey.PurposePairingKey, aToken)
	require.NoError(t, err)
	require.False(t, libtokenkey.Equal(a, b), "the file form must have supplied the key")

	t.Setenv(libtokenkey.EnvSigningKeyFile, filepath.Join(t.TempDir(), "missing"))
	_, err = libtokenkey.FromEnv()
	require.Error(t, err)
	require.NotErrorIs(t, err, libtokenkey.ErrNoSigningKey,
		"configured-and-wrong must not read as not-configured")
}

// libcipher's own rule: keys are per purpose. Configuring the Ed25519 identity
// secret as the credential-hashing key is refused, not warned about — that key's
// public half is handed to every enrolled machine.
func TestUnit_TokenKey_RefusesTheIdentityKey(t *testing.T) {
	seed := strings.Repeat("d", libtokenkey.MinKeyLen)

	t.Setenv(libtokenkey.EnvIdentityKeyFile, "")
	t.Setenv(libtokenkey.EnvIdentityKey, seed)
	t.Setenv(libtokenkey.EnvSigningKeyFile, "")
	t.Setenv(libtokenkey.EnvSigningKey, seed)
	_, err := libtokenkey.FromEnv()
	require.ErrorIs(t, err, libtokenkey.ErrKeyReused)

	path := filepath.Join(t.TempDir(), "identity.key")
	require.NoError(t, os.WriteFile(path, []byte(seed+"\n"), 0o600))
	t.Setenv(libtokenkey.EnvIdentityKey, "")
	t.Setenv(libtokenkey.EnvIdentityKeyFile, path)
	_, err = libtokenkey.FromEnv()
	require.ErrorIs(t, err, libtokenkey.ErrKeyReused,
		"the mounted-file form is the deployed shape and must collide too")

	t.Setenv(libtokenkey.EnvSigningKey, strings.Repeat("c", libtokenkey.MinKeyLen))
	_, err = libtokenkey.FromEnv()
	require.NoError(t, err, "a distinct key is accepted")
}

func TestUnit_FromEnv_ReadsTheLegacyNamesToo(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, "")
	t.Setenv(libtokenkey.EnvSigningKeyFile, "")
	t.Setenv("RELAY_TOKEN_KEY", strings.Repeat("l", 32))

	legacy, err := libtokenkey.FromEnv()
	require.NoError(t, err)

	t.Setenv(libtokenkey.EnvSigningKey, strings.Repeat("n", 32))
	current, err := libtokenkey.FromEnv()
	require.NoError(t, err)

	first, err := legacy.Hash(libtokenkey.PurposeProxyKey, "token")
	require.NoError(t, err)
	second, err := current.Hash(libtokenkey.PurposeProxyKey, "token")
	require.NoError(t, err)
	require.NotEqual(t, first, second, "the current name wins over the legacy one")
}
