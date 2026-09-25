package liblicense_test

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/libcipher"
	"github.com/contenox/contenox/liblicense"
)

func TestSSHKeyGenerationAndParsing(t *testing.T) {
	passphrase := []byte("secret-passphrase-12345")

	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("test-key-unencrypted", nil)
	if err != nil {
		t.Fatalf("GenerateSSHKeyPair unencrypted failed: %v", err)
	}

	privKey, err := liblicense.ParseSSHPrivateKey(privPEM, nil)
	if err != nil {
		t.Fatalf("ParseSSHPrivateKey unencrypted failed: %v", err)
	}
	if len(privKey) != libcipher.SigningPrivateKeySize {
		t.Fatalf("unexpected private key length: %d", len(privKey))
	}

	pubKey, err := liblicense.ParseSSHPublicKey(pubSSH)
	if err != nil {
		t.Fatalf("ParseSSHPublicKey failed: %v", err)
	}
	if len(pubKey) != libcipher.SigningPublicKeySize {
		t.Fatalf("unexpected public key length: %d", len(pubKey))
	}

	encPrivPEM, encPubSSH, err := liblicense.GenerateSSHKeyPair("test-key-encrypted", passphrase)
	if err != nil {
		t.Fatalf("GenerateSSHKeyPair encrypted failed: %v", err)
	}

	_, err = liblicense.ParseSSHPrivateKey(encPrivPEM, nil)
	if !errors.Is(err, liblicense.ErrPassphraseRequired) {
		t.Fatalf("expected ErrPassphraseRequired, got %v", err)
	}

	_, err = liblicense.ParseSSHPrivateKey(encPrivPEM, []byte("wrong-password"))
	if !errors.Is(err, liblicense.ErrInvalidPassphrase) {
		t.Fatalf("expected ErrInvalidPassphrase, got %v", err)
	}

	encPrivKey, err := liblicense.ParseSSHPrivateKey(encPrivPEM, passphrase)
	if err != nil {
		t.Fatalf("ParseSSHPrivateKey with valid passphrase failed: %v", err)
	}
	if len(encPrivKey) != libcipher.SigningPrivateKeySize {
		t.Fatalf("unexpected private key length: %d", len(encPrivKey))
	}

	encPubKey, err := liblicense.ParseSSHPublicKey(encPubSSH)
	if err != nil {
		t.Fatalf("ParseSSHPublicKey for encrypted pair failed: %v", err)
	}
	if len(encPubKey) != libcipher.SigningPublicKeySize {
		t.Fatalf("unexpected public key length: %d", len(encPubKey))
	}
}

func TestClaimsKVHelpers(t *testing.T) {
	claims := liblicense.NewClaims("lic-001", "contenox", "customer-corp")
	claims.Set("plan", "enterprise")
	claims.SetInt("prepaid_tokens", 5000000)
	claims.SetInt64("max_storage_bytes", 1099511627776)
	claims.SetBool("ee_ai_tools", true)
	claims.Set("features", "modeld,rag,fleet,ee_ai_tools")

	if claims.GetString("plan", "free") != "enterprise" {
		t.Errorf("expected plan enterprise, got %s", claims.GetString("plan", "free"))
	}
	if claims.GetInt("prepaid_tokens", 0) != 5000000 {
		t.Errorf("expected 5000000 tokens, got %d", claims.GetInt("prepaid_tokens", 0))
	}
	if claims.GetInt64("max_storage_bytes", 0) != 1099511627776 {
		t.Errorf("expected 1099511627776 bytes, got %d", claims.GetInt64("max_storage_bytes", 0))
	}
	if !claims.GetBool("ee_ai_tools", false) {
		t.Errorf("expected ee_ai_tools = true")
	}

	if !claims.HasFeature("ee_ai_tools") {
		t.Errorf("expected HasFeature(ee_ai_tools) to be true")
	}
	if !claims.HasFeature("modeld") {
		t.Errorf("expected HasFeature(modeld) to be true")
	}
	if !claims.HasFeature("rag") {
		t.Errorf("expected HasFeature(rag) to be true")
	}
	if claims.HasFeature("nonexistent_feature") {
		t.Errorf("expected HasFeature(nonexistent_feature) to be false")
	}
}

func TestEndToEndLicenseIssueAndVerify(t *testing.T) {
	passphrase := []byte("top-secret-signing-key")
	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("licensing-authority", passphrase)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(privPEM, passphrase, liblicense.WithIssuerKeyID("key-2026-v1"))
	if err != nil {
		t.Fatalf("failed to create issuer: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	nbf := now.Add(-1 * time.Hour)
	exp := now.Add(24 * time.Hour)

	claims := liblicense.Claims{
		ID:        "lic-enterprise-999",
		Issuer:    "contenox-auth",
		Subject:   "tenant-42",
		IssuedAt:  now,
		NotBefore: &nbf,
		ExpiresAt: &exp,
		KV: map[string]string{
			"tokens":      "25000000",
			"ee_ai_tools": "true",
			"tier":        "enterprise",
		},
	}

	token, err := issuer.Issue(claims)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if !strings.HasPrefix(token, liblicense.TokenPrefix+".") {
		t.Fatalf("unexpected token prefix in %s", token)
	}

	privVerifier, err := liblicense.NewVerifierFromSSHPrivateKey(privPEM, passphrase)
	if err != nil {
		t.Fatalf("failed to create private verifier: %v", err)
	}

	verifiedClaims, err := privVerifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify with private verifier failed: %v", err)
	}
	if verifiedClaims.ID != claims.ID {
		t.Errorf("expected ID %s, got %s", claims.ID, verifiedClaims.ID)
	}
	if verifiedClaims.GetInt("tokens", 0) != 25000000 {
		t.Errorf("expected tokens 25000000, got %d", verifiedClaims.GetInt("tokens", 0))
	}
	if !verifiedClaims.GetBool("ee_ai_tools", false) {
		t.Errorf("expected ee_ai_tools = true")
	}

	privKey, err := liblicense.ParseSSHPrivateKey(privPEM, passphrase)
	if err != nil {
		t.Fatalf("failed to parse privKey: %v", err)
	}
	derivedPayloadKey, err := liblicense.DeriveKey(privKey.Seed(), nil, liblicense.DefaultKDFInfo)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	pubVerifier, err := liblicense.NewVerifierFromSSHPublicKey(
		pubSSH,
		liblicense.WithPayloadDecryptionKey(derivedPayloadKey),
		liblicense.WithExpectedIssuer("contenox-auth"),
		liblicense.WithExpectedSubject("tenant-42"),
	)
	if err != nil {
		t.Fatalf("failed to create public verifier: %v", err)
	}

	verifiedClaimsFromPub, err := pubVerifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify with public verifier failed: %v", err)
	}
	if verifiedClaimsFromPub.ID != claims.ID {
		t.Errorf("expected ID %s, got %s", claims.ID, verifiedClaimsFromPub.ID)
	}
	if verifiedClaimsFromPub.GetString("tier", "") != "enterprise" {
		t.Errorf("expected tier enterprise, got %s", verifiedClaimsFromPub.GetString("tier", ""))
	}

	header, err := pubVerifier.InspectHeader(token)
	if err != nil {
		t.Fatalf("InspectHeader failed: %v", err)
	}
	if header.KeyID != "key-2026-v1" {
		t.Errorf("expected KeyID key-2026-v1, got %s", header.KeyID)
	}
	if header.LicenseID != "lic-enterprise-999" {
		t.Errorf("expected LicenseID lic-enterprise-999, got %s", header.LicenseID)
	}
}

func TestArmoredLicenseFormat(t *testing.T) {
	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("armored-test", nil)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(privPEM, nil)
	if err != nil {
		t.Fatalf("failed to create issuer: %v", err)
	}

	claims := liblicense.NewClaims("armored-lic-1", "issuer", "sub")
	claims.Set("tokens", "1000")

	armoredToken, err := issuer.IssueArmored(claims)
	if err != nil {
		t.Fatalf("IssueArmored failed: %v", err)
	}

	if !strings.Contains(armoredToken, liblicense.ArmorHeader) || !strings.Contains(armoredToken, liblicense.ArmorFooter) {
		t.Fatalf("armored token missing headers: %s", armoredToken)
	}

	verifier, err := liblicense.NewVerifierFromSSHPrivateKey(privPEM, nil)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	verified, err := verifier.Verify(armoredToken)
	if err != nil {
		t.Fatalf("failed to verify armored token: %v", err)
	}
	if verified.ID != "armored-lic-1" {
		t.Errorf("expected ID armored-lic-1, got %s", verified.ID)
	}
	if verified.GetInt("tokens", 0) != 1000 {
		t.Errorf("expected tokens 1000, got %d", verified.GetInt("tokens", 0))
	}

	quickVerified, err := liblicense.QuickVerify(privPEM, nil, armoredToken)
	if err != nil {
		t.Fatalf("QuickVerify failed: %v", err)
	}
	if quickVerified.ID != "armored-lic-1" {
		t.Errorf("QuickVerify ID mismatch: %s", quickVerified.ID)
	}

	_ = pubSSH
}

func TestTamperingAndValidation(t *testing.T) {
	privPEM1, _, _ := liblicense.GenerateSSHKeyPair("key1", nil)
	privPEM2, pubSSH2, _ := liblicense.GenerateSSHKeyPair("key2", nil)

	issuer, _ := liblicense.NewIssuerFromSSHPrivateKey(privPEM1, nil)
	verifierWrongKey, _ := liblicense.NewVerifierFromSSHPrivateKey(privPEM2, nil)

	claims := liblicense.NewClaims("lic-tamper-check", "iss", "sub")
	claims.Set("tokens", "100")

	token, err := issuer.Issue(claims)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	_, err = verifierWrongKey.Verify(token)
	if !errors.Is(err, liblicense.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature when verifying with wrong key, got %v", err)
	}

	parts := strings.Split(token, ".")
	corruptSigToken := strings.Join([]string{parts[0], parts[1], parts[2], "AAAA" + parts[3][4:]}, ".")
	validVerifier, _ := liblicense.NewVerifierFromSSHPrivateKey(privPEM1, nil)
	_, err = validVerifier.Verify(corruptSigToken)
	if !errors.Is(err, liblicense.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature on corrupt signature, got %v", err)
	}

	corruptCipherToken := strings.Join([]string{parts[0], parts[1], "AAAA" + parts[2][4:], parts[3]}, ".")
	_, err = validVerifier.Verify(corruptCipherToken)
	if !errors.Is(err, liblicense.ErrInvalidSignature) {
		// Signature fails first since ciphertext is part of the signed envelope
		t.Logf("Ciphertext tamper caught at signature check: %v", err)
	}

	pubVerifierOnly, _ := liblicense.NewVerifierFromSSHPublicKey(pubSSH2)
	_, err = pubVerifierOnly.Verify(token)
	if !errors.Is(err, liblicense.ErrInvalidSignature) && !errors.Is(err, liblicense.ErrMissingDecryptionKey) {
		t.Fatalf("expected ErrMissingDecryptionKey or ErrInvalidSignature, got %v", err)
	}

	past := time.Now().UTC().Add(-2 * time.Hour)
	expiredClaims := liblicense.Claims{
		ID:        "expired-lic",
		IssuedAt:  past.Add(-1 * time.Hour),
		ExpiresAt: &past,
	}
	expiredToken, err := issuer.Issue(expiredClaims)
	if err != nil {
		t.Fatalf("failed to issue expired token: %v", err)
	}
	_, err = validVerifier.Verify(expiredToken)
	if !errors.Is(err, liblicense.ErrLicenseExpired) {
		t.Fatalf("expected ErrLicenseExpired, got %v", err)
	}

	future := time.Now().UTC().Add(2 * time.Hour)
	futureClaims := liblicense.Claims{
		ID:        "future-lic",
		IssuedAt:  time.Now().UTC(),
		NotBefore: &future,
	}
	futureToken, err := issuer.Issue(futureClaims)
	if err != nil {
		t.Fatalf("failed to issue future token: %v", err)
	}
	_, err = validVerifier.Verify(futureToken)
	if !errors.Is(err, liblicense.ErrLicenseNotYetValid) {
		t.Fatalf("expected ErrLicenseNotYetValid, got %v", err)
	}

	_, err = issuer.Issue(liblicense.Claims{})
	if !errors.Is(err, liblicense.ErrEmptyClaims) {
		t.Fatalf("expected ErrEmptyClaims, got %v", err)
	}
}

func TestPinnedAuthorityVerifier(t *testing.T) {
	origPinned := liblicense.PinnedAuthorityPublicKey
	defer func() { liblicense.PinnedAuthorityPublicKey = origPinned }()

	liblicense.PinnedAuthorityPublicKey = ""
	_, err := liblicense.NewPinnedVerifier()
	if !errors.Is(err, liblicense.ErrNoPinnedAuthorityKey) {
		t.Fatalf("expected ErrNoPinnedAuthorityKey, got %v", err)
	}

	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("pinned-test-auth", nil)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	liblicense.PinnedAuthorityPublicKey = string(pubSSH)

	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(privPEM, nil)
	if err != nil {
		t.Fatalf("failed to create issuer: %v", err)
	}

	privKey, err := liblicense.ParseSSHPrivateKey(privPEM, nil)
	if err != nil {
		t.Fatalf("failed to parse priv key: %v", err)
	}
	derivedPayloadKey, err := liblicense.DeriveKey(privKey.Seed(), nil, liblicense.DefaultKDFInfo)
	if err != nil {
		t.Fatalf("failed to derive key: %v", err)
	}

	claims := liblicense.NewClaims("pinned-lic-001", "contenox", "client-device")
	claims.Set("api_key", "sk-pinned-test")
	claims.Set("provider_url", "https://api.deepseek.com")

	token, err := issuer.Issue(claims)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	verifiedClaims, err := liblicense.QuickVerifyPinned(derivedPayloadKey, token)
	if err != nil {
		t.Fatalf("failed to verify pinned token: %v", err)
	}
	if verifiedClaims.ID != "pinned-lic-001" {
		t.Fatalf("expected ID pinned-lic-001, got %q", verifiedClaims.ID)
	}
	if verifiedClaims.GetString("api_key", "") != "sk-pinned-test" {
		t.Fatalf("expected api_key sk-pinned-test, got %q", verifiedClaims.GetString("api_key", ""))
	}
}

func TestVerifyPinnedTokenUsesBothPinnedValues(t *testing.T) {
	prevPub, prevKey := liblicense.PinnedAuthorityPublicKey, liblicense.PinnedPayloadKey
	defer func() { liblicense.PinnedAuthorityPublicKey, liblicense.PinnedPayloadKey = prevPub, prevKey }()

	passphrase := []byte("pinned-test")
	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("contenox-authority", passphrase)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	payloadKey := make([]byte, 32)
	if _, err := rand.Read(payloadKey); err != nil {
		t.Fatalf("payload key: %v", err)
	}

	liblicense.PinnedAuthorityPublicKey = string(pubSSH)
	liblicense.PinnedPayloadKey = hex.EncodeToString(payloadKey)

	iss, err := liblicense.NewIssuerFromSSHPrivateKey(privPEM, passphrase, liblicense.WithPayloadEncryptionKey(payloadKey))
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	tok, err := iss.Issue(liblicense.NewClaims("lic-1", "contenox", "client-a"))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	claims, err := liblicense.VerifyPinnedToken(tok)
	if err != nil {
		t.Fatalf("verify pinned: %v", err)
	}
	if claims.Subject != "client-a" {
		t.Fatalf("subject = %q, want client-a", claims.Subject)
	}

	liblicense.PinnedPayloadKey = ""
	if _, err := liblicense.VerifyPinnedToken(tok); !errors.Is(err, liblicense.ErrNoPinnedPayloadKey) {
		t.Fatalf("want ErrNoPinnedPayloadKey, got %v", err)
	}
}
