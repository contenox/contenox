package gateway

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblease"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/require"
)

func leasedService(t *testing.T, leasePath string) *service {
	t.Helper()
	first, _ := factsServiceOn(t, leasePath)
	return first
}

// leasedPair is two instances of one deployment: one database, one bus, one
// election. Building each instance with its own fixture would leave them on
// separate buses, which is a deployment that cannot exist.
func leasedPair(t *testing.T, leasePath string) (*service, *service) {
	t.Helper()
	return factsServiceOn(t, leasePath)
}

func recordOne(t *testing.T, svc *service, tokens int64) {
	t.Helper()
	ctx := context.Background()
	key := &runtimetypes.ProxyKey{
		KeyHash: "h-lease", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if _, err := svc.keys.RecordProxyKey(ctx, *key); err != nil {
		_, err = svc.keys.GetProxyKeyByHash(ctx, key.KeyHash)
		require.NoError(t, err)
	}
	svc.chargeTurn(ctx, key, &liblicense.Claims{}, turnUsage{
		model: "qwen3:8b", prompt: tokens, completion: 0,
		durationMs: 1, finishReason: "stop",
	})
}

func weekTotal(t *testing.T, svc *service) int64 {
	t.Helper()
	snapshot, err := svc.usage.ReadUsage(context.Background(), runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	return snapshot.TotalTokens
}

// One deployment, one writer. Two instances over one database that both write a
// snapshot built from the same predecessor lose one turn's charge; the lease is
// what makes the second instance a follower.
func TestSystem_ASecondInstanceDoesNotWriteTheMeter(t *testing.T) {
	ctx := context.Background()
	lease := filepath.Join(t.TempDir(), "meter-writer.lease")

	holder, follower := leasedPair(t, lease)
	held, err := holder.meter.Start(ctx)
	require.NoError(t, err)
	require.True(t, held, "the first instance takes the writer role")

	followed, err := follower.meter.Start(ctx)
	require.NoError(t, err)
	require.False(t, followed, "the second instance must not take a lease that is held")

	require.NoError(t, holder.StartUsageConsumer(ctx), "the writer also drains the bus")

	recordOne(t, holder, 100)
	require.EqualValues(t, 100, weekTotal(t, holder), "the writer's turn is metered once")

	// The follower's turn is not lost to the election: it goes to the writer on
	// the bus, and the meter converges on it. A window where both count is what
	// the election removes; a turn that is never counted is what it must not
	// introduce.
	recordOne(t, follower, 100)
	require.Eventually(t, func() bool { return weekTotal(t, holder) == 200 }, 5*time.Second, 20*time.Millisecond,
		"a follower's turn reaches the writer and is metered there")
}

// The fence is the TTL: a holder whose process died leaves a lease that lapses,
// and the lapsed record must not authorise a write. This is the state a killed
// replica leaves behind, so the test builds it rather than killing a goroutine.
func TestSystem_ALapsedLeaseFencesItsHolder(t *testing.T) {
	lease := filepath.Join(t.TempDir(), "meter-writer.lease")
	writer := leasedService(t, lease)

	expired, err := liblease.Acquire(lease, 250*time.Millisecond)
	require.NoError(t, err)

	writer.meter.mu.Lock()
	writer.meter.lease = expired
	writer.meter.mu.Unlock()
	require.True(t, writer.meter.holds(), "the role is held while the lease stands")

	require.Eventually(t, func() bool { return !writer.meter.holds() }, 2*time.Second, 10*time.Millisecond,
		"a lease that stopped being renewed must stop authorising writes")

	writes := weekTotal(t, writer)
	recordOne(t, writer, 40)
	require.EqualValues(t, writes, weekTotal(t, writer), "a lapsed holder writes nothing")

	wrote, err := writer.meter.Record(context.Background(), runtimetypes.ProxyUsage{
		KeyHash: "h-lease", ClientID: "laptop", Model: "qwen3:8b", PromptTokens: 5,
	})
	require.NoError(t, err)
	require.False(t, wrote, "and reports the turn as one to send to the writer instead")
}

// The takeover: the instance that asks after the record lapsed becomes the
// writer, and the previous holder stays fenced out.
func TestSystem_AnInstanceAsksAgainUntilItHoldsTheRole(t *testing.T) {
	ctx := context.Background()
	lease := filepath.Join(t.TempDir(), "meter-writer.lease")

	crashed, err := liblease.Acquire(lease, 200*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, crashed)

	original := meterLeaseTTL
	meterLeaseTTL = 200 * time.Millisecond
	t.Cleanup(func() { meterLeaseTTL = original })

	first, second := factsServiceOn(t, lease)
	heldFirst, err := first.meter.Start(ctx)
	require.NoError(t, err)
	require.False(t, heldFirst, "a live record is not this instance's to take")

	require.Eventually(t, func() bool { return first.meter.holds() }, 5*time.Second, 50*time.Millisecond,
		"an instance that keeps asking takes the role once the record lapses")

	second.meter.mu.Lock()
	second.meter.lease = crashed
	second.meter.mu.Unlock()
	require.False(t, second.meter.holds(), "the instance that lost the role is fenced out")

	recordOne(t, first, 30)
	require.EqualValues(t, 30, weekTotal(t, first), "the new holder writes")

	before := weekTotal(t, first)
	recordOne(t, second, 30)
	require.EqualValues(t, before, weekTotal(t, first), "the fenced instance writes nothing")
}

// Without a lease path there is nothing to elect, which is the one-process
// deployment the CLI serves.
func TestUnit_AnUnleasedMeterWritesEverywhere(t *testing.T) {
	svc := leasedService(t, "")
	held, err := svc.meter.Start(context.Background())
	require.NoError(t, err)
	require.True(t, held)
	recordOne(t, svc, 25)
	require.EqualValues(t, 25, weekTotal(t, svc))
}
