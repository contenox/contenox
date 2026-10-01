package modelauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func authFixture(t *testing.T) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "auth.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	b := &runtimetypes.Backend{ID: "backend", Name: "chatgpt", Type: ProviderType, BaseURL: BaseURL}
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(ctx, b))
	return New(db), b.ID
}

func jwt() string {
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"test-account"}}`)) + ".signature"
}

func seed(t *testing.T, s *Service, id string, expiry time.Time) {
	t.Helper()
	require.NoError(t, s.mutate(context.Background(), id, func(c *credential) error {
		*c = credential{Generation: "initial", Account: "test-account", Token: &oauth2.Token{AccessToken: jwt(), RefreshToken: "refresh-secret", Expiry: expiry}}
		return nil
	}))
}

func TestUnit_ModelAuth_StatusLogout(t *testing.T) {
	s, id := authFixture(t)
	ctx := context.Background()
	_, err := s.Headers(ctx, id)
	require.ErrorIs(t, err, ErrLoginRequired)
	seed(t, s, id, time.Now().Add(time.Hour))
	headers, err := s.Headers(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Bearer "+jwt(), headers.Get("Authorization"))
	status, err := s.Status(ctx, id)
	require.NoError(t, err)
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret")
	require.NotContains(t, string(raw), "test-account")
	require.NotContains(t, string(raw), jwt())
	require.NoError(t, s.Logout(ctx, id))
	_, err = s.Headers(ctx, id)
	require.ErrorIs(t, err, ErrLoginRequired)
	c, err := read(ctx, runtimetypes.New(s.db.WithoutTransaction()), id)
	require.NoError(t, err)
	require.Nil(t, c.Token)
	require.Empty(t, c.Account)
	require.NotEqual(t, "initial", c.Generation)
}

func TestUnit_ModelAuth_ConcurrentRefresh(t *testing.T) {
	s, id := authFixture(t)
	seed(t, s, id, time.Now().Add(-time.Minute))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh-secret", r.Form.Get("refresh_token"))
		require.Empty(t, r.Form.Get("client_secret"))
		json.NewEncoder(w).Encode(map[string]any{"access_token": jwt(), "refresh_token": "rotated", "expires_in": 3600})
	}))
	defer server.Close()
	s.authURL = server.URL
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		other := *s
		wg.Add(1)
		go func() { defer wg.Done(); _, err := other.Headers(context.Background(), id); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, calls.Load())
	c, err := read(context.Background(), runtimetypes.New(s.db.WithoutTransaction()), id)
	require.NoError(t, err)
	require.Equal(t, "rotated", c.Token.RefreshToken)
}

func TestUnit_ModelAuth_RefreshFailureRequiresLogin(t *testing.T) {
	s, id := authFixture(t)
	seed(t, s, id, time.Now().Add(-time.Minute))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(400)
		fmt.Fprint(w, "refresh-secret")
	}))
	defer server.Close()
	s.authURL = server.URL
	for i := 0; i < 2; i++ {
		_, err := s.Headers(context.Background(), id)
		require.ErrorIs(t, err, ErrLoginRequired)
		require.NotContains(t, err.Error(), "refresh-secret")
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestUnit_ModelAuth_DeviceLogin(t *testing.T) {
	s, id := authFixture(t)
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			fmt.Fprint(w, `{"device_auth_id":"device","user_code":"CODE","interval":"1"}`)
		case "/api/accounts/deviceauth/token":
			if polls.Add(1) == 1 {
				w.WriteHeader(403)
				return
			}
			fmt.Fprint(w, `{"authorization_code":"grant","code_verifier":"verifier"}`)
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "verifier", r.Form.Get("code_verifier"))
			require.Equal(t, clientID, r.Form.Get("client_id"))
			json.NewEncoder(w).Encode(map[string]any{"access_token": jwt(), "refresh_token": "refresh", "expires_in": 3600})
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	s.authURL = server.URL
	err := s.Login(context.Background(), id, func(code DeviceCode) error {
		require.Equal(t, authURL+"/codex/device", code.URL)
		require.Equal(t, "CODE", code.Code)
		return nil
	})
	require.NoError(t, err)
	status, err := s.Status(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "authenticated", status.State)
}

func TestUnit_ModelAuth_CancelPreservesLogin(t *testing.T) {
	s, id := authFixture(t)
	seed(t, s, id, time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"device_auth_id":"device","user_code":"CODE","interval":1}`)
	}))
	defer server.Close()
	s.authURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	err := s.Login(ctx, id, func(DeviceCode) error { cancel(); return nil })
	require.ErrorIs(t, err, context.Canceled)
	_, err = s.Headers(context.Background(), id)
	require.NoError(t, err)
}

func TestUnit_ModelAuth_LogoutDuringLoginWins(t *testing.T) {
	s, id := authFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			fmt.Fprint(w, `{"device_auth_id":"device","user_code":"CODE","interval":1}`)
		case "/api/accounts/deviceauth/token":
			fmt.Fprint(w, `{"authorization_code":"grant","code_verifier":"verifier"}`)
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": jwt(), "refresh_token": "refresh", "expires_in": 3600})
		}
	}))
	defer server.Close()
	s.authURL = server.URL
	err := s.Login(context.Background(), id, func(DeviceCode) error { return s.Logout(context.Background(), id) })
	require.ErrorContains(t, err, "superseded")
	_, err = s.Headers(context.Background(), id)
	require.ErrorIs(t, err, ErrLoginRequired)
}

func TestUnit_ModelAuth_LogoutDuringRefreshWins(t *testing.T) {
	s, id := authFixture(t)
	seed(t, s, id, time.Now().Add(-time.Minute))
	started, finish := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-finish
		json.NewEncoder(w).Encode(map[string]any{"access_token": jwt(), "refresh_token": "rotated", "expires_in": 3600})
	}))
	defer server.Close()
	s.authURL = server.URL
	refreshed, loggedOut := make(chan error, 1), make(chan error, 1)
	go func() { _, err := s.Headers(context.Background(), id); refreshed <- err }()
	<-started
	go func() { loggedOut <- s.Logout(context.Background(), id) }()
	close(finish)
	require.NoError(t, <-refreshed)
	require.NoError(t, <-loggedOut)
	_, err := s.Headers(context.Background(), id)
	require.ErrorIs(t, err, ErrLoginRequired)
}

func TestUnit_ModelAuth_CancelledRefreshCannotReuseToken(t *testing.T) {
	s, id := authFixture(t)
	seed(t, s, id, time.Now().Add(-time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancel(); w.WriteHeader(400) }))
	defer server.Close()
	s.authURL = server.URL
	_, err := s.Headers(ctx, id)
	require.ErrorIs(t, err, ErrLoginRequired)
	_, err = s.Headers(context.Background(), id)
	require.ErrorIs(t, err, ErrLoginRequired)
}
