package contenoxcli

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/stretchr/testify/require"
)

func writeAuthorityKeyPair(t *testing.T) (privPath, pubPath string, payloadKey []byte) {
	t.Helper()
	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("gateway-cli-test", nil)
	require.NoError(t, err)
	priv, err := liblicense.ParseSSHPrivateKey(privPEM, nil)
	require.NoError(t, err)
	payloadKey, err = liblicense.DeriveKey(priv.Seed(), nil, liblicense.DefaultKDFInfo)
	require.NoError(t, err)

	dir := t.TempDir()
	privPath = filepath.Join(dir, "authority")
	pubPath = filepath.Join(dir, "authority.pub")
	require.NoError(t, os.WriteFile(privPath, privPEM, 0o600))
	require.NoError(t, os.WriteFile(pubPath, pubSSH, 0o600))
	return privPath, pubPath, payloadKey
}

func mintToken(t *testing.T, privPath string) string {
	t.Helper()
	pem, err := os.ReadFile(privPath)
	require.NoError(t, err)
	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(pem, nil)
	require.NoError(t, err)
	claims := liblicense.NewClaims("lic-1", "contenox", "laptop")
	claims.Set("allowed_models", "qwen3:8b")
	token, err := issuer.Issue(claims)
	require.NoError(t, err)
	return token
}

func TestUnit_BuildGatewayVerifier_PublicKeyAndPayloadKeyVerifyAMintedToken(t *testing.T) {
	privPath, pubPath, payloadKey := writeAuthorityKeyPair(t)
	token := mintToken(t, privPath)

	for name, encoded := range map[string]string{
		"hex":    hex.EncodeToString(payloadKey),
		"base64": base64.StdEncoding.EncodeToString(payloadKey),
	} {
		t.Run(name, func(t *testing.T) {
			verifier, err := buildGatewayVerifier(gatewayServeConfig{
				authorityKeyFile: pubPath,
				payloadKey:       encoded,
			})
			require.NoError(t, err)

			claims, err := verifier.Verify(token)
			require.NoError(t, err)
			require.Equal(t, "laptop", claims.Subject)
			require.Equal(t, "qwen3:8b", claims.GetString("allowed_models", ""))
		})
	}
}

func TestUnit_BuildGatewayVerifier_PrivateKeyCarriesThePayloadKey(t *testing.T) {
	privPath, _, _ := writeAuthorityKeyPair(t)

	verifier, err := buildGatewayVerifier(gatewayServeConfig{privateKeyFile: privPath})
	require.NoError(t, err)
	_, err = verifier.Verify(mintToken(t, privPath))
	require.NoError(t, err)
}

func TestUnit_BuildGatewayVerifier_RefusesAnIncompleteAuthority(t *testing.T) {
	_, pubPath, _ := writeAuthorityKeyPair(t)

	_, err := buildGatewayVerifier(gatewayServeConfig{authorityKeyFile: pubPath})
	require.Error(t, err, "a public key alone cannot decrypt a license")
	require.Contains(t, err.Error(), "--payload-key")

	_, err = buildGatewayVerifier(gatewayServeConfig{})
	require.Error(t, err, "this build pins no authority, so one must be configured")
	require.Contains(t, err.Error(), "--authority-private-key-file")
}

func TestUnit_GatewayHasher_AbsentKeyIsNotAnErrorButABadOneIs(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, "")
	t.Setenv(libtokenkey.EnvSigningKeyFile, "")
	hasher, err := gatewayHasher()
	require.NoError(t, err)
	require.Nil(t, hasher, "an unfinished ledger is a deployment without revocation, reported at startup")

	t.Setenv(libtokenkey.EnvSigningKey, "too-short")
	_, err = gatewayHasher()
	require.Error(t, err, "a key that is present and unusable would reject every token the host minted")

	_, err = libtokenkey.FromSecret("placeholder")
	require.ErrorIs(t, err, libtokenkey.ErrKeyTooShort)
}
