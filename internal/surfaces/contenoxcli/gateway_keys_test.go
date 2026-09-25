package contenoxcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/stretchr/testify/require"
)

func writeAuthority(t *testing.T) (privPath, pubPath string) {
	t.Helper()
	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("gateway-keys-test", nil)
	require.NoError(t, err)
	dir := t.TempDir()
	privPath = filepath.Join(dir, "authority")
	pubPath = filepath.Join(dir, "authority.pub")
	require.NoError(t, os.WriteFile(privPath, privPEM, 0o600))
	require.NoError(t, os.WriteFile(pubPath, pubSSH, 0o600))
	return privPath, pubPath
}

func testKeyCreateCmd(t *testing.T, dbPath string, flags map[string]string) (*bytes.Buffer, *bytes.Buffer, func() error) {
	t.Helper()
	cmd := testCobraCmd()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	require.NoError(t, cmd.Root().PersistentFlags().Set("db", dbPath))
	for name, value := range flags {
		cmd.Flags().String(name, "", "")
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	return out, errOut, func() error { return gatewayKeyCreateCmd.RunE(cmd, nil) }
}

func openLedger(t *testing.T, dbPath string) runtimetypes.Store {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return runtimetypes.New(db.WithoutTransaction())
}

func TestUnit_GatewayKeyCreateCmd_MintsAKeyTheLedgerKnows(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, strings.Repeat("k", 32))
	privPath, pubPath := writeAuthority(t)
	dbPath := filepath.Join(t.TempDir(), "gateway-keys.db")

	out, errOut, run := testKeyCreateCmd(t, dbPath, map[string]string{
		"client":                     "laptop-alex",
		"models":                     "qwen3:8b,llama3.1:8b",
		"output-allowance":           "5m",
		"input-allowance":            "25m",
		"five-hour-allowance":        "400k",
		"monthly-budget":             "20",
		"cache-discount":             "0.25",
		"thinking-discount":          "0",
		"image-allowance":            "100",
		"audio-allowance":            "10",
		"tier":                       "team",
		"ttl":                        "30d",
		"authority-private-key-file": privPath,
	})
	require.NoError(t, run())

	bearer := strings.TrimSpace(out.String())
	require.NotEmpty(t, bearer)
	require.Contains(t, errOut.String(), "shown once")

	store := openLedger(t, dbPath)
	keys, err := store.ListProxyKeys(context.Background(), nil, 10)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, "laptop-alex", keys[0].ClientID)
	require.Equal(t, "team", keys[0].Tier)
	require.WithinDuration(t, keys[0].IssuedAt.Add(30*24*time.Hour), keys[0].ExpiresAt, time.Minute)

	hasher, err := libtokenkey.FromSecret(strings.Repeat("k", 32))
	require.NoError(t, err)
	digest, err := hasher.Hash(libtokenkey.PurposeProxyKey, bearer)
	require.NoError(t, err)
	require.Equal(t, digest, keys[0].KeyHash, "the recorded digest must be the one the gateway will hash the token to")

	verifier, err := liblicense.NewVerifierFromSSHPrivateKey(mustRead(t, privPath), nil)
	require.NoError(t, err)
	claims, err := verifier.Verify(bearer)
	require.NoError(t, err)
	require.Equal(t, "laptop-alex", claims.Subject)
	require.Equal(t, "qwen3:8b,llama3.1:8b", claims.GetString("allowed_models", ""))
	require.EqualValues(t, 5_000_000, claims.GetInt64(liblicense.OutputAllowanceKey("qwen3:8b"), 0))
	require.EqualValues(t, 25_000_000, claims.GetInt64(liblicense.InputAllowanceKey("llama3.1:8b"), 0))
	require.EqualValues(t, 400_000, claims.GetInt64(liblicense.FiveHourAllowanceKey("qwen3:8b"), 0))
	require.EqualValues(t, 20_000_000, claims.GetInt64(liblicense.MonthlyBudgetUSDKey("qwen3:8b"), 0))
	require.EqualValues(t, 2500, claims.GetInt64(liblicense.CacheDiscountMultiplierKey("llama3.1:8b"), 0),
		"the flag states a share and the claim carries basis points")
	require.EqualValues(t, 0, claims.GetInt64(liblicense.ThinkingDiscountMultiplierKey("llama3.1:8b"), -1))
	require.EqualValues(t, 100, liblicense.ImageAllowanceFor(claims, "qwen3:8b"),
		"the image ceiling is declared per model like every other allowance")
	require.EqualValues(t, 10, liblicense.AudioAllowanceFor(claims, "llama3.1:8b"),
		"the audio ceiling is in mebibytes, the unit the wire can be counted in")
	require.InDelta(t, 0.25, liblicense.CacheDiscountMultiplierFor(claims, "qwen3:8b"), 1e-9)
	require.Zero(t, liblicense.ThinkingDiscountMultiplierFor(claims, "qwen3:8b"))
	pub, err := os.ReadFile(pubPath)
	require.NoError(t, err)
	publicOnly, err := liblicense.NewVerifierFromSSHPublicKey(pub)
	require.NoError(t, err)
	_, err = publicOnly.Verify(bearer)
	require.ErrorIs(t, err, liblicense.ErrMissingDecryptionKey,
		"a public key alone verifies the signature but cannot read the claims, which is why the gateway also needs the payload key")
}

func TestUnit_GatewayKeyCreateCmd_RefusesToMintWhatItCannotRevoke(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, "")
	t.Setenv(libtokenkey.EnvSigningKeyFile, "")
	privPath, _ := writeAuthority(t)

	_, _, run := testKeyCreateCmd(t, filepath.Join(t.TempDir(), "no-key.db"), map[string]string{
		"client":                     "laptop",
		"authority-private-key-file": privPath,
	})
	err := run()
	require.Error(t, err)
	require.Contains(t, err.Error(), libtokenkey.EnvSigningKey)
}

func TestUnit_GatewayKeyCreateCmd_RefusesAnAllowanceWithoutModels(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, strings.Repeat("k", 32))
	privPath, _ := writeAuthority(t)

	_, _, run := testKeyCreateCmd(t, filepath.Join(t.TempDir(), "no-models.db"), map[string]string{
		"client":                     "laptop",
		"output-allowance":           "5m",
		"authority-private-key-file": privPath,
	})
	err := run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "--models")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

func TestUnit_GatewayKeyCreateCmd_RefusesACacheDiscountOverTheWholeRead(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, strings.Repeat("k", 32))
	privPath, _ := writeAuthority(t)

	for _, raw := range []string{"1.5", "-0.1", "quarter"} {
		_, _, run := testKeyCreateCmd(t, filepath.Join(t.TempDir(), "discount.db"), map[string]string{
			"client":                     "laptop",
			"models":                     "qwen3:8b",
			"cache-discount":             raw,
			"authority-private-key-file": privPath,
		})
		err := run()
		require.Error(t, err, "%q is not a share of a cached token", raw)
	}
}

func TestUnit_DiscountClaim_AcceptsAShareOrAPercentage(t *testing.T) {
	share, err := discountClaim("cache-discount", "0.25")
	require.NoError(t, err)
	percent, err := discountClaim("cache-discount", "25%")
	require.NoError(t, err)
	require.Equal(t, share, percent)
	require.EqualValues(t, 2500, share)
}

// A key that states an output allowance and no input cap gets the input cap the
// plan implies; minting one without it left half a turn unmetered.
func TestUnit_GatewayKeyCreateCmd_DerivesTheInputCapFromTheOutputOne(t *testing.T) {
	t.Setenv(libtokenkey.EnvSigningKey, strings.Repeat("k", 32))
	privPath, _ := writeAuthority(t)

	verify := func(t *testing.T, flags map[string]string) *liblicense.Claims {
		t.Helper()
		out, _, run := testKeyCreateCmd(t, filepath.Join(t.TempDir(), "mint.db"), flags)
		require.NoError(t, run())
		verifier, err := liblicense.NewVerifierFromSSHPrivateKey(mustRead(t, privPath), nil)
		require.NoError(t, err)
		claims, err := verifier.Verify(strings.TrimSpace(out.String()))
		require.NoError(t, err)
		return claims
	}

	derived := verify(t, map[string]string{
		"client":                     "laptop",
		"models":                     "qwen3:8b",
		"output-allowance":           "5m",
		"authority-private-key-file": privPath,
	})
	require.EqualValues(t, 5_000_000, liblicense.OutputAllowanceFor(derived, "qwen3:8b"))
	require.EqualValues(t, 5_000_000*liblicense.DefaultInputAllowanceMultiplier,
		liblicense.InputAllowanceFor(derived, "qwen3:8b"),
		"an unstated input cap follows from the output cap, as a plan has always meant it")

	stated := verify(t, map[string]string{
		"client":                     "laptop",
		"models":                     "qwen3:8b",
		"output-allowance":           "5m",
		"input-allowance":            "1m",
		"authority-private-key-file": privPath,
	})
	require.EqualValues(t, 1_000_000, liblicense.InputAllowanceFor(stated, "qwen3:8b"),
		"a stated input cap is never overridden by the default")

	unlimited := verify(t, map[string]string{
		"client":                     "laptop",
		"models":                     "qwen3:8b",
		"authority-private-key-file": privPath,
	})
	require.Zero(t, liblicense.InputAllowanceFor(unlimited, "qwen3:8b"),
		"a key that states no allowance states none")
}
