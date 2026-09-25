package gateway

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type factsVerifier struct{}

func (factsVerifier) Verify(string) (*liblicense.Claims, error) {
	return nil, errors.New("facts tests do not authenticate")
}

func factsService(t *testing.T) *service {
	t.Helper()
	svc, _ := factsServiceOn(t, "")
	return svc
}

// factsServiceOn builds two services over one database and one bus, which is what
// a deployment of several is: they contend for the same rows and they hear each
// other. Two buses would make every cross-instance test pass trivially by never
// delivering anything.
func factsServiceOn(t *testing.T, meterLeasePath string) (*service, *service) {
	t.Helper()
	ctx := context.Background()

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "gateway.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	bus := libbus.NewInMem()
	rt, err := runtimestate.New(ctx, db, bus)
	require.NoError(t, err)
	models := NewTestModelRepo(t, rt)

	build := func() *service {
		svc, err := New(Config{
			DB: db, Verifier: factsVerifier{}, Runtime: rt, Bus: bus,
			Models: models, MeterLeasePath: meterLeasePath,
		})
		require.NoError(t, err)
		return svc.(*service)
	}
	return build(), build()
}

func TestUnit_ReportFacts_DeclaredCapabilitiesReportedVerbatim(t *testing.T) {
	capabilities, info, parameters := reportFacts(&runtimestate.ModelPullStatus{
		Model:                "deepseek-flash",
		ContextLength:        1000000,
		MaxOutputTokens:      393216,
		DeclaredCapabilities: []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityTools},
	})

	assert.Equal(t, []string{"completion", "tools"}, capabilities,
		"tools is not expressible as a routing boolean, so the declaration is what the wire carries")
	assert.Equal(t, map[string]any{contextLengthKey: 1000000}, info)
	assert.Equal(t, "num_predict 393216", parameters)
}

func TestUnit_ReportFacts_ObservedFallback(t *testing.T) {
	capabilities, info, parameters := reportFacts(&runtimestate.ModelPullStatus{
		Model: "qwen3:8b", CanChat: true, CanEmbed: true, CanVision: true, CanThink: true,
		ContextLength: 4096, MaxOutputTokens: 8192,
	})

	assert.Equal(t, []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityEmbedding, runtimetypes.CapabilityVision, runtimetypes.CapabilityThinking}, capabilities)
	assert.Equal(t, map[string]any{contextLengthKey: 4096}, info)
	assert.Equal(t, "num_predict 8192", parameters)
}

func TestUnit_ReportFacts_NothingStatedIsLeftOut(t *testing.T) {
	capabilities, info, parameters := reportFacts(&runtimestate.ModelPullStatus{Model: "mystery"})
	assert.Nil(t, capabilities)
	assert.Nil(t, info, "a reader treats an absent window as unknown, a made-up one as true")
	assert.Empty(t, parameters)
}
