package runtimetypes_test

import (
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

func TestUnit_ProxyKeys_MintGetListRevoke(t *testing.T) {
	ctx, st, _ := runtimetypes.SetupStoreExec(t)
	expires := time.Now().UTC().Add(time.Hour)

	k, err := st.RecordProxyKey(ctx, runtimetypes.ProxyKey{KeyHash: "h1", ClientID: "laptop", Tier: "pro", ExpiresAt: expires})
	require.NoError(t, err)
	require.NotEmpty(t, k.ID)
	require.True(t, k.IssuedAt.After(time.Time{}), "issued_at defaults to now")

	got, err := st.GetProxyKeyByHash(ctx, "h1")
	require.NoError(t, err)
	require.Equal(t, "laptop", got.ClientID)
	require.Equal(t, "pro", got.Tier)
	require.Nil(t, got.RevokedAt)

	_, err = st.GetProxyKeyByHash(ctx, "never-issued")
	require.Error(t, err, "the lookup must fail closed")

	list, err := st.ListProxyKeys(ctx, nil, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)

	active, err := st.CountActiveProxyKeys(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 1, active)

	revoked, err := st.RevokeProxyKey(ctx, "h1")
	require.NoError(t, err)
	require.True(t, revoked)
	revoked, err = st.RevokeProxyKey(ctx, "h1")
	require.NoError(t, err)
	require.False(t, revoked, "revocation is idempotent")

	active, err = st.CountActiveProxyKeys(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 0, active)
}

func TestUnit_ProxyKeys_RequireHashClientAndExpiry(t *testing.T) {
	ctx, st, _ := runtimetypes.SetupStoreExec(t)
	later := time.Now().UTC().Add(time.Hour)

	_, err := st.RecordProxyKey(ctx, runtimetypes.ProxyKey{ClientID: "laptop", ExpiresAt: later})
	require.Error(t, err, "a key with no digest cannot be looked up")

	_, err = st.RecordProxyKey(ctx, runtimetypes.ProxyKey{KeyHash: "h", ExpiresAt: later})
	require.Error(t, err, "a key with no client cannot be attributed")

	_, err = st.RecordProxyKey(ctx, runtimetypes.ProxyKey{KeyHash: "h", ClientID: "laptop"})
	require.Error(t, err, "a key with no expiry is unbounded")
}
