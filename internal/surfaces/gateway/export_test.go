package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/ollamatokenizer"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/libroutine"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

// NewTestModelRepo builds the model repo a gateway fixture has to hand to New.
// It is exported because the package's black-box tests are in gateway_test and
// cannot see an unexported fixture helper.
func NewTestModelRepo(t *testing.T, rt *runtimestate.State) llmrepo.ModelRepo {
	t.Helper()
	repo, err := llmrepo.NewModelManager(rt, ollamatokenizer.NewEstimateTokenizer(), llmrepo.ModelManagerConfig{}, libtracker.NoopTracker{})
	require.NoError(t, err)
	return repo
}

// SnapshotHolds reports whether this gateway is holding a cutoff for a client,
// which is how a fixture observes what a peer's answer did without driving a
// turn through the meter.
func SnapshotHolds(s Service, clientID string) (bool, string) {
	return s.(*service).revocations.IsBlocked(clientID, "", "", time.Now().UTC())
}

// Quarantined reports whether this gateway is holding a backend out of service.
// A half-open circuit is not quarantined: it is the state that lets one request
// through to find out whether the backend is back.
func Quarantined(s Service, backendID string) bool {
	return s.(*service).breaker.pool.Get(backendID).GetState() == libroutine.Open
}

// SetBreakerCooldown shortens the quarantine window so a fixture can watch a
// backend come back without waiting the deployment's two minutes.
func SetBreakerCooldown(s Service, d time.Duration) {
	s.(*service).breaker.pool = libroutine.NewPool(breakerThreshold, d)
}

// TripBreaker takes every backend out of service, which is what a run of
// refusals does.
func TripBreaker(s Service, model string) {
	svc := s.(*service)
	for _, st := range svc.runtime.Get(context.Background()) {
		state := st
		svc.recordBackendHealth(backendRefusal{backend: state.Backend.ID, terminal: true})
	}
}

// WouldServe reports whether the read-only health view would let this model be
// attempted, without consuming the half-open probe.
func WouldServe(s Service, model string) bool {
	svc := s.(*service)
	for _, st := range svc.runtime.Get(context.Background()) {
		state := st
		if !svc.breaker.WouldServe(state.Backend.ID, time.Now().UTC()) {
			return false
		}
	}
	return true
}

// QuarantinedWholeModel reports whether every backend serving a model is out of
// service, which is the state a turn is refused in.
func QuarantinedWholeModel(s Service, model string) bool {
	return s.(*service).quarantined(context.Background(), model)
}
