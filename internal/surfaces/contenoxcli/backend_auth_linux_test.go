package contenoxcli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestUnit_CodexSetupInterrupt(t *testing.T) {
	if os.Getenv("CONTENOX_TEST_SETUP_INTERRUPT") == "1" {
		ctx := context.Background()
		path := filepath.Join(t.TempDir(), "setup.db")
		db, err := OpenDBAt(ctx, path)
		require.NoError(t, err)
		defer db.Close()
		require.NoError(t, registerSetupBackend(ctx, db, modelauth.ProviderType, "", modelauth.BaseURL))
		store := runtimetypes.New(db.WithoutTransaction())
		b, err := store.GetBackendByName(ctx, modelauth.ProviderType)
		require.NoError(t, err)
		raw, err := json.Marshal(map[string]any{"generation": "test", "account": "test", "token": &oauth2.Token{AccessToken: "test", RefreshToken: "test", Expiry: time.Now().Add(time.Hour)}})
		require.NoError(t, err)
		require.NoError(t, store.SetKV(ctx, "model-oauth:"+b.ID, raw))
		modelrepo.SharedHTTPClient = &http.Client{Transport: subscriptionTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"test-model","visibility":"list"}]}`))}, nil
		})}
		cmd := &cobra.Command{}
		cmd.SetContext(ctx)
		cmd.Flags().String("db", path, "")
		_ = runCodexSetup(cmd, os.Stdout, bufio.NewScanner(os.Stdin))
		t.Fatal("setup returned instead of terminating on SIGINT")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUnit_CodexSetupInterrupt$")
	cmd.Env = append(os.Environ(), "CONTENOX_TEST_SETUP_INTERRUPT=1")
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer input.Close()
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	scanner := bufio.NewScanner(output)
	scanner.Split(bufio.ScanWords)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "Model" {
			ready = true
			break
		}
	}
	if ready {
		require.NoError(t, cmd.Process.Signal(os.Interrupt))
	}
	err = cmd.Wait()
	require.True(t, ready, "child never reached model selection")
	require.NoError(t, ctx.Err(), "SIGINT did not terminate setup")
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, -1, exit.ExitCode(), "expected termination by signal")
}
