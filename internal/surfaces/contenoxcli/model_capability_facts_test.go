package contenoxcli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/models/modelcapability"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func seedCapabilityCLIBackend(t *testing.T, dbPath, id, name, backendType string) {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(context.Background(), &runtimetypes.Backend{
		ID: id, Name: name, Type: backendType, BaseURL: "https://" + name + ".example/v1",
	}))
}

func TestUnit_ModelCapabilitySetCmd_PersistsBackendModelFacts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "capability-facts-cli.db")
	seedCapabilityCLIBackend(t, dbPath, "reseller", "reseller", "openai")

	cmd := testCobraCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Root().PersistentFlags().Set("db", dbPath))
	for name, value := range map[string]string{
		"context": "1m", "max-output": "128k", "capabilities": "completion,vision",
		"input-price": "1.25", "output-price": "10", "backend": "reseller",
	} {
		cmd.Flags().String(name, "", "")
		require.NoError(t, cmd.Flags().Set(name, value))
	}

	require.NoError(t, modelCapabilitySetCmd.RunE(cmd, []string{"OpenAI", "gpt-5-mini"}))

	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()

	declared, err := runtimetypes.New(db.WithoutTransaction()).GetLLMProviderModelFacts(context.Background(), "reseller")
	require.NoError(t, err)
	facts, ok := declared["gpt-5-mini"]
	require.True(t, ok)
	require.Equal(t, 1000000, facts.ContextLength)
	require.Equal(t, 128000, facts.MaxOutputTokens)
	require.Equal(t, []string{"completion", "vision"}, facts.Capabilities)
	require.InDelta(t, 1.25, facts.Pricing.InputPerMillion, 1e-9)
	require.InDelta(t, 10.0, facts.Pricing.OutputPerMillion, 1e-9)
}

func TestUnit_ModelCapabilitySetCmd_FansOutToEveryBackendOfTheType(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "capability-fanout-cli.db")
	seedCapabilityCLIBackend(t, dbPath, "b1", "openai-eu", "openai")
	seedCapabilityCLIBackend(t, dbPath, "b2", "openai-us", "openai")
	seedCapabilityCLIBackend(t, dbPath, "b3", "local", "ollama")

	cmd := testCobraCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Root().PersistentFlags().Set("db", dbPath))
	cmd.Flags().String("context", "", "")
	require.NoError(t, cmd.Flags().Set("context", "400k"))

	require.NoError(t, modelCapabilitySetCmd.RunE(cmd, []string{"openai", "gpt-5-mini"}))

	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()
	store := runtimetypes.New(db.WithoutTransaction())

	for _, id := range []string{"b1", "b2"} {
		declared, err := store.GetLLMProviderModelFacts(context.Background(), id)
		require.NoError(t, err)
		require.Equal(t, 400000, declared["gpt-5-mini"].ContextLength)
	}

	other, err := store.GetLLMProviderModelFacts(context.Background(), "b3")
	require.NoError(t, err)
	require.Empty(t, other, "a backend of another type is never written")
}

func TestUnit_ModelCapabilitySetCmd_RefusesBooleansScopedToOneBackend(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "capability-refuse-cli.db")
	seedCapabilityCLIBackend(t, dbPath, "reseller", "reseller", "openai")

	cmd := testCobraCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Root().PersistentFlags().Set("db", dbPath))
	for name, value := range map[string]string{"think": "true", "backend": "reseller"} {
		cmd.Flags().String(name, "", "")
		require.NoError(t, cmd.Flags().Set(name, value))
	}

	err := modelCapabilitySetCmd.RunE(cmd, []string{"openai", "gpt-5-mini"})
	require.Error(t, err, "a provider-level boolean cannot be scoped to one backend")
	require.Contains(t, err.Error(), "--capabilities")
}

func TestUnit_ModelCapabilityUnsetCmd_RemovesFactsAndProviderOverride(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "capability-unset-cli.db")
	seedCapabilityCLIBackend(t, dbPath, "reseller", "reseller", "openai")

	setCmd := testCobraCmd()
	setCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, setCmd.Root().PersistentFlags().Set("db", dbPath))
	for name, value := range map[string]string{"think": "true", "context": "1m"} {
		setCmd.Flags().String(name, "", "")
		require.NoError(t, setCmd.Flags().Set(name, value))
	}
	require.NoError(t, modelCapabilitySetCmd.RunE(setCmd, []string{"openai", "gpt-5-mini"}))

	unsetCmd := testCobraCmd()
	unsetCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, unsetCmd.Root().PersistentFlags().Set("db", dbPath))
	require.NoError(t, modelCapabilityUnsetCmd.RunE(unsetCmd, []string{"openai", "gpt-5-mini"}))

	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()
	store := runtimetypes.New(db.WithoutTransaction())

	declared, err := store.GetLLMProviderModelFacts(context.Background(), "reseller")
	require.NoError(t, err)
	require.Empty(t, declared)

	_, ok, err := modelcapability.New(store).Get(context.Background(), "openai", "gpt-5-mini")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestUnit_ModelCapabilitySetCmd_PersistsThePerImageRate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "image-price-cli.db")
	seedCapabilityCLIBackend(t, dbPath, "reseller", "reseller", "openai")

	cmd := testCobraCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Root().PersistentFlags().Set("db", dbPath))
	for name, value := range map[string]string{
		"image-price": "0.021", "audio-price": "1.5", "input-price": "1.25", "backend": "reseller",
	} {
		cmd.Flags().String(name, "", "")
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	require.NoError(t, modelCapabilitySetCmd.RunE(cmd, []string{"openai", "gpt-5-mini"}))

	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()
	declared, err := runtimetypes.New(db.WithoutTransaction()).GetLLMProviderModelFacts(context.Background(), "reseller")
	require.NoError(t, err)
	require.InDelta(t, 0.021, declared["gpt-5-mini"].Pricing.PerImage, 1e-9)
	require.InDelta(t, 1.5, declared["gpt-5-mini"].Pricing.PerAudioMebibyte, 1e-9)
}
