package contenoxcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/models/backendservice"
	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestUnit_PromptCatalogModel(t *testing.T) {
	models := []modelrepo.ObservedModel{{Name: "first-model"}, {Name: "second-model"}}
	for _, tc := range []struct {
		name, input, want string
		cancelled         bool
	}{
		{name: "first", input: "1\n", want: "first-model"},
		{name: "second", input: "2\n", want: "second-model"},
		{name: "retry", input: "0\n3\ninvalid\n2\n", want: "second-model"},
		{name: "quit", input: "q\n", cancelled: true},
		{name: "eof", cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := promptCatalogModel(&out, bufio.NewScanner(strings.NewReader(tc.input)), models)
			if tc.cancelled {
				require.ErrorContains(t, err, "inference defaults unchanged")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
			require.Contains(t, out.String(), "1. first-model")
			require.Contains(t, out.String(), "2. second-model")
			require.Contains(t, out.String(), "Model (1-2, q to quit)")
		})
	}
}

func TestUnit_BackendSubscriptionCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscription.db")
	ctx := context.Background()
	db, err := OpenDBAt(ctx, path)
	require.NoError(t, err)
	defer db.Close()
	b := &runtimetypes.Backend{ID: "test-subscription", Name: "chatgpt", Type: modelauth.ProviderType, BaseURL: modelauth.BaseURL}
	require.NoError(t, backendservice.New(db).Create(ctx, b))
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.Flags().String("db", path, "")
	require.NoError(t, backendShowCmd.RunE(cmd, []string{"chatgpt"}))
	var shown struct {
		Authentication modelauth.Status `json:"authentication"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &shown))
	require.Equal(t, "login_required", shown.Authentication.State)
	require.NoError(t, backendLogout(cmd, []string{"chatgpt"}))
	require.NoError(t, backendservice.New(db).Delete(ctx, b.ID))
	var stored any
	require.ErrorIs(t, runtimetypes.New(db.WithoutTransaction()).GetKV(ctx, "model-oauth:"+b.ID, &stored), libdb.ErrNotFound)
	for _, flag := range []string{"api-key", "api-key-env", "url"} {
		c := &cobra.Command{}
		c.Flags().String("type", modelauth.ProviderType, "")
		for _, f := range []string{"api-key", "api-key-env", "url", "script"} {
			c.Flags().String(f, "", "")
		}
		require.NoError(t, c.Flags().Set(flag, "forbidden"))
		err := backendAddCmd.RunE(c, []string{"chatgpt"})
		require.ErrorContains(t, err, "omit --api-key")
	}
}
