package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/require"
)

func testKey(id string) *runtimetypes.ProxyKey {
	return &runtimetypes.ProxyKey{ID: id, KeyHash: "hash-" + id, ClientID: "client-" + id}
}

func user(text string) modelrepo.Message {
	return modelrepo.Message{Role: "user", Content: text}
}

func assistant(text string) modelrepo.Message {
	return modelrepo.Message{Role: "assistant", Content: text}
}

// A client resends its history on every turn, so the key has to be derived from
// the part that does not move. Deriving it from the whole request would mint a
// new session per turn and pin nothing, which is the defect this guards.
func TestUnit_SessionKeySurvivesTheHistoryGrowing(t *testing.T) {
	key := testKey("k1")
	system := modelrepo.Message{Role: "system", Content: "you are terse"}
	turn1 := []modelrepo.Message{system, user("plan a trip")}
	turn2 := []modelrepo.Message{system, user("plan a trip"), assistant("where to?"), user("Lisbon")}
	turn3 := []modelrepo.Message{system, user("plan a trip"), assistant("where to?"), user("Lisbon"), assistant("when?"), user("May")}

	first := sessionKeyFor(key, nil, "llama3", "", turn1)
	require.NotEmpty(t, first)
	require.Equal(t, first, sessionKeyFor(key, nil, "llama3", "", turn2))
	require.Equal(t, first, sessionKeyFor(key, nil, "llama3", "", turn3))
}

// A request that is short because it is early is not one whose whole self is
// stable: the session would change identity the moment it outgrew the boundary.
func TestUnit_SessionKeyIsTheOpeningNotTheRequestsLength(t *testing.T) {
	key := testKey("k1")
	system := modelrepo.Message{Role: "system", Content: "you are terse"}
	short := []modelrepo.Message{system, user("hi"), assistant("hello"), user("more")}
	long := append(append([]modelrepo.Message{}, short...), assistant("sure"), user("and more"))

	require.Equal(t,
		sessionKeyFor(key, nil, "llama3", "", short),
		sessionKeyFor(key, nil, "llama3", "", long),
	)
}

func TestUnit_SessionKeySeparatesConversationsClientsAndModels(t *testing.T) {
	key := testKey("k1")
	system := modelrepo.Message{Role: "system", Content: "you are terse"}
	trip := []modelrepo.Message{system, user("plan a trip")}
	base := sessionKeyFor(key, nil, "llama3", "", trip)
	require.NotEmpty(t, base)

	require.NotEqual(t, base, sessionKeyFor(key, nil, "llama3", "", []modelrepo.Message{system, user("write a parser")}))
	require.NotEqual(t, base, sessionKeyFor(testKey("k2"), nil, "llama3", "", trip))
	require.NotEqual(t, base, sessionKeyFor(key, nil, "qwen3", "", trip))
}

func TestUnit_SessionKeyPrefersTheClientsOwn(t *testing.T) {
	key := testKey("k1")
	require.Equal(t, "client-stated", sessionKeyFor(key, nil, "llama3", "client-stated", []modelrepo.Message{user("anything")}))
}

// No identity and no opening is no session: the request resolves the way it did
// before a session existed.
func TestUnit_SessionKeyIsEmptyWithoutSomethingToIdentifyIt(t *testing.T) {
	require.Empty(t, sessionKeyFor(nil, nil, "llama3", "", []modelrepo.Message{user("hello")}))
	require.Empty(t, sessionKeyFor(testKey("k1"), nil, "llama3", "", nil), "nothing said yet")
	require.Empty(t, sessionKeyFor(testKey("k1"), nil, "llama3", "", []modelrepo.Message{user("plan a trip")}), "a first turn has no opening a later turn repeats")
}

// A key is the identity when the ledger row carries no id yet, so a deployment
// that hashes but does not mint still pins.
func TestUnit_ClientIdentityFallsBackToTheHashThenTheSubject(t *testing.T) {
	claims := liblicense.NewClaims("lic", "issuer", "alex")
	require.Equal(t, "id-1", clientIdentity(&runtimetypes.ProxyKey{ID: "id-1", KeyHash: "h", ClientID: "c"}, &claims))
	require.Equal(t, "h", clientIdentity(&runtimetypes.ProxyKey{KeyHash: "h", ClientID: "c"}, &claims))
	require.Equal(t, "alex", clientIdentity(nil, &claims))
	require.Empty(t, clientIdentity(&runtimetypes.ProxyKey{ClientID: "c"}, nil))
}

// The key and the assertion must name the same prefix. A key derived from the
// opening while the assertion covers a longer prefix is a breakpoint on a
// message nothing has watched hold still.
func TestUnit_OpeningBoundaryIsWhatBothTheKeyAndTheHintUse(t *testing.T) {
	system := modelrepo.Message{Role: "system", Content: "you are terse"}
	opening := []modelrepo.Message{system, user("a")}
	later := []modelrepo.Message{system, user("a"), assistant("b"), user("c")}

	require.Equal(t, 2, openingBoundary(opening))
	require.Equal(t, 2, openingBoundary(later))
	require.Equal(t, 0, openingBoundary([]modelrepo.Message{user("a")}), "no system, nothing repeated yet")
	require.Equal(t, 0, openingBoundary([]modelrepo.Message{assistant("hi")}))

	require.Nil(t, sessionHints("", later), "no session, no assertion")

	hints := sessionHints("s", later)
	require.NotNil(t, hints)
	require.True(t, hints.StableSystem)
	require.True(t, hints.StableTools)
	require.Equal(t, openingBoundary(later), hints.StableHistoryLen)
}

type capturedRequest struct {
	req      llmrepo.Request
	messages []modelrepo.Message
}

type capturingRepo struct {
	llmrepo.ModelRepo
	seen chan capturedRequest
}

func (r capturingRepo) Stream(ctx context.Context, req llmrepo.Request, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, llmrepo.Meta, error) {
	r.seen <- capturedRequest{req: req, messages: messages}
	ch := make(chan *modelrepo.StreamParcel, 1)
	ch <- &modelrepo.StreamParcel{Terminal: &modelrepo.StreamTerminal{FinishReason: "stop"}}
	close(ch)
	return ch, llmrepo.Meta{BackendID: "scripted", ModelName: req.ModelNames[0]}, nil
}

func (r capturingRepo) Chat(ctx context.Context, req llmrepo.Request, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, llmrepo.Meta, error) {
	r.seen <- capturedRequest{req: req, messages: messages}
	return modelrepo.ChatResult{Message: modelrepo.Message{Role: "assistant", Content: "ok"}}, llmrepo.Meta{BackendID: "scripted", ModelName: req.ModelNames[0]}, nil
}

// The gateway is the surface that fronts several backends, so a turn that names
// a conversation has to reach the model layer carrying it. Nothing observed the
// session key before it was set here, which is how gateway traffic resolved
// randomly while the harness resolved sticky.
func TestSystem_AChatTurnCarriesItsSessionToTheModelLayer(t *testing.T) {
	svc, bearer := repoService(t, harnessClaimsPtr())
	seen := make(chan capturedRequest, 1)
	svc.models = capturingRepo{seen: seen}

	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	body := `{"model":"small-model","stream":false,"contenox_session":"conversation-7","messages":[{"role":"system","content":"be terse"},{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"again"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got := <-seen
	require.Equal(t, "conversation-7", got.req.SessionKey)
	require.NotNil(t, got.req.CacheHints)
	require.Equal(t, 2, got.req.CacheHints.StableHistoryLen)
}

// Without the client naming one, the conversation's opening is what pins it:
// the same opening twice is the same session, and the key the model layer sees
// is the one derived here rather than the empty string.
func TestSystem_AChatTurnDerivesItsSessionFromTheConversation(t *testing.T) {
	svc, bearer := repoService(t, harnessClaimsPtr())
	seen := make(chan capturedRequest, 1)
	svc.models = capturingRepo{seen: seen}

	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	call := func() capturedRequest {
		body := `{"model":"small-model","stream":false,"messages":[{"role":"system","content":"be terse"},{"role":"user","content":"hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bearer)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return <-seen
	}

	first, second := call(), call()
	require.NotEmpty(t, first.req.SessionKey)
	require.Equal(t, first.req.SessionKey, second.req.SessionKey)
}

func harnessClaimsPtr() *liblicense.Claims {
	claims := harnessClaims()
	return &claims
}

// The generated key is a digest, never the caller's identity in the clear: it
// reaches a log line and an upstream header on some routes.
func TestUnit_SessionKeyIsOpaque(t *testing.T) {
	key := testKey("k1")
	got := sessionKeyFor(key, nil, "llama3", "", []modelrepo.Message{{Role: "system", Content: "be terse"}, user("the secret prompt")})
	require.NotContains(t, got, "the secret prompt")
	require.NotContains(t, got, "k1")
	require.Len(t, got, 64)
}
