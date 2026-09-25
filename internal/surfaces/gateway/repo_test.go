package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/stretchr/testify/require"
)

const repoScript = `{
  "model": "small-model",
  "capabilities": {"chat": true},
  "turns": [
    {"text": "one", "usage": {"prompt_tokens": 10, "completion_tokens": 60}},
    {"text": "two", "usage": {"prompt_tokens": 10, "completion_tokens": 60}},
    {"text": "three", "usage": {"prompt_tokens": 10, "completion_tokens": 60}},
    {"text": "four", "usage": {"prompt_tokens": 10, "completion_tokens": 60}}
  ]
}`

// fixedVerifier hands back the licence the test issued, so the ceiling stated for
// the harness is the ceiling the repo spends against.
type fixedVerifier struct{ claims *liblicense.Claims }

func (v fixedVerifier) Verify(string) (*liblicense.Claims, error) { return v.claims, nil }

// repoService is a gateway whose ledger and meter work, which is what makes a
// turn chargeable: an unledgered deployment has no client id to meter against.
func repoService(t *testing.T, claims *liblicense.Claims) (*service, string) {
	t.Helper()
	svc := turnService(t, repoScript)
	svc.verifier = fixedVerifier{claims: claims}
	hasher, err := libtokenkey.Generate()
	require.NoError(t, err)
	svc.hasher = hasher

	privPEM, _, err := liblicense.GenerateSSHKeyPair("repo-test", nil)
	require.NoError(t, err)
	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(privPEM, nil)
	require.NoError(t, err)
	bearer, err := issuer.Issue(*claims)
	require.NoError(t, err)

	digest, err := hasher.Hash(libtokenkey.PurposeProxyKey, bearer)
	require.NoError(t, err)
	_, err = svc.keys.RecordProxyKey(context.Background(), runtimetypes.ProxyKey{
		KeyHash: digest, ClientID: claims.Subject, ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	return svc, bearer
}

func harnessClaims() liblicense.Claims {
	claims := liblicense.NewClaims("lic-repo", "issuer", "harness")
	claims.Set("allowed_models", "small-model")
	claims.SetInt64(liblicense.OutputAllowanceKey("small-model"), 100)
	return claims
}

// The harness reaches models through this repo, so a ceiling stated for a client
// has to bite here exactly as it does on the HTTP route, and the turn has to land
// on the same meter: the agent's own missions are bounded by the allowance its
// key states, and the operator can see what they spent.
func TestSystem_RepoEnforcesTheCeilingLikeTheRoute(t *testing.T) {
	ctx := context.Background()
	claims := harnessClaims()
	svc, bearer := repoService(t, &claims)

	repo, err := NewRepo(svc, bearer)
	require.NoError(t, err)
	req := llmrepo.Request{ModelNames: []string{"small-model"}}
	msgs := []modelrepo.Message{{Role: "user", Content: "hi"}}

	for i := 1; i <= 2; i++ {
		_, _, err = repo.Chat(ctx, req, msgs)
		require.NoError(t, err, "call %d: %d output tokens are under the 100 ceiling", i, i*60)
	}

	snapshot, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: claims.Subject,
		Model: "small-model", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 120, snapshot.CompletionTokens,
		"an in-process turn lands on the same meter a proxied one does")

	_, _, err = repo.Chat(ctx, req, msgs)
	require.ErrorIs(t, err, ErrAllowanceExhausted,
		"the third call would cross the ceiling, so the repo refuses it")
}

// A model the licence does not name is refused on this path too: the harness does
// not get to spend on models its own key cannot reach.
func TestSystem_RepoRefusesAModelTheLicenceDoesNotName(t *testing.T) {
	claims := harnessClaims()
	svc, bearer := repoService(t, &claims)

	repo, err := NewRepo(svc, bearer)
	require.NoError(t, err)

	_, _, err = repo.Chat(context.Background(),
		llmrepo.Request{ModelNames: []string{"other-model"}},
		[]modelrepo.Message{{Role: "user", Content: "hi"}})
	require.ErrorIs(t, err, ErrModelNotAllowed)
}
