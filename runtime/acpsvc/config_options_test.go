package acpsvc

import (
	"context"
	"path/filepath"
	"testing"

	libacp "github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

func setupConfigOptionsDB(t *testing.T) (context.Context, libdb.DBManager) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "acp-config-options.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return ctx, db
}

func TestUnit_SessionConfigOptionsExposeModelPolicyAndThink(t *testing.T) {
	ctx, db := setupConfigOptionsDB(t)
	require.NoError(t, clikv.SetHITLPolicy(ctx, runtimetypes.New(db.WithoutTransaction()), "dev"))

	tr := &Transport{
		deps: Deps{
			DB:                    db,
			KnownPolicies:         []string{"strict", "dev"},
			HITLDefaultPolicyName: "strict",
		},
		defaultProvider:    "openai",
		defaultModel:       "gpt-5-mini",
		defaultAltProvider: "anthropic",
		defaultAltModel:    "claude-sonnet-4",
	}
	sess := &sessionEntry{Think: "medium"}

	options := tr.sessionConfigOptions(ctx, sess)
	require.Len(t, options, 4)

	model := optionByID(t, options, configIDModel)
	require.Equal(t, configCategoryModel, model.Category)
	require.Equal(t, "openai/gpt-5-mini", model.CurrentValue)
	require.Len(t, model.Options.Groups, 2)
	require.Equal(t, "openai", model.Options.Groups[0].Group)
	require.Equal(t, "gpt-5-mini", model.Options.Groups[0].Options[0].Name)
	require.Equal(t, "anthropic", model.Options.Groups[1].Group)
	require.Equal(t, "claude-sonnet-4", model.Options.Groups[1].Options[0].Name)
	require.True(t, configOptionHasValue(model, "openai/gpt-5-mini"))
	require.True(t, configOptionHasValue(model, "anthropic/claude-sonnet-4"))

	policy := optionByID(t, options, configIDHITLPolicy)
	require.Equal(t, configCategoryHITLPolicy, policy.Category)
	require.Equal(t, "dev", policy.CurrentValue)
	require.True(t, configOptionHasValue(policy, hitlPolicyDefaultValue))
	require.True(t, configOptionHasValue(policy, "strict"))
	require.True(t, configOptionHasValue(policy, "dev"))

	think := optionByID(t, options, configIDThink)
	require.Equal(t, configCategoryThink, think.Category)
	require.Equal(t, "medium", think.CurrentValue)
	require.True(t, configOptionHasValue(think, "auto"))
	require.True(t, configOptionHasValue(think, "off"))
	require.True(t, configOptionHasValue(think, "xhigh"))

	limit := optionByID(t, options, configIDTokenLimit)
	require.Equal(t, "context", limit.Category)
	require.Equal(t, "0", limit.CurrentValue) // default
	require.Contains(t, limit.Description, "token limit")
}

func TestUnit_HITLPolicyDisplayNameShortensPresetFilenames(t *testing.T) {
	require.Equal(t, "strict", hitlPolicyDisplayName("hitl-policy-strict.json"))
	require.Equal(t, "acpx", hitlPolicyDisplayName("hitl-policy-acpx.json"))
	require.Equal(t, "custom", hitlPolicyDisplayName("custom.json"))
}

func TestUnit_ModelConfigDisplayNameStripsGeminiResourcePrefix(t *testing.T) {
	require.Equal(t, "gemini-3.1-pro-preview", modelConfigDisplayName("models/gemini-3.1-pro-preview"))
	require.Equal(t, "gpt-5", modelConfigDisplayName("gpt-5"))
}

func TestUnit_SetSessionConfigOptionUpdatesSessionAndPolicyConfig(t *testing.T) {
	ctx, db := setupConfigOptionsDB(t)

	sid := libacp.SessionID("sess-1")
	sess := &sessionEntry{Provider: "openai", Model: "gpt-5-mini", Think: "low"}
	tr := &Transport{
		deps: Deps{
			DB:                    db,
			KnownPolicies:         []string{"strict", "dev"},
			HITLDefaultPolicyName: "strict",
		},
		sessions:           map[libacp.SessionID]*sessionEntry{sid: sess},
		contenoxToACPID:    map[string]libacp.SessionID{},
		defaultProvider:    "openai",
		defaultModel:       "gpt-5-mini",
		defaultAltProvider: "anthropic",
		defaultAltModel:    "claude-sonnet-4",
	}

	resp, err := tr.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: sid,
		ConfigID:  configIDModel,
		Value:     "anthropic/claude-sonnet-4",
	})
	require.NoError(t, err)
	require.Equal(t, "openai", tr.provider(), "ACP selector changes the session, not the persistent default provider")
	require.Equal(t, "gpt-5-mini", tr.model(), "ACP selector changes the session, not the persistent default model")
	require.Empty(t, ReadConfigValue(ctx, db, "default-provider"))
	require.Empty(t, ReadConfigValue(ctx, db, "default-model"))
	require.Equal(t, "anthropic", sess.providerOrDefault(""))
	require.Equal(t, "claude-sonnet-4", sess.modelOrDefault(""))
	require.Equal(t, "anthropic/claude-sonnet-4", optionByID(t, resp.ConfigOptions, configIDModel).CurrentValue)

	resp, err = tr.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: sid,
		ConfigID:  configIDHITLPolicy,
		Value:     "dev",
	})
	require.NoError(t, err)
	require.Equal(t, "dev", clikv.ReadHITLPolicy(ctx, runtimetypes.New(db.WithoutTransaction())))
	require.Equal(t, "dev", optionByID(t, resp.ConfigOptions, configIDHITLPolicy).CurrentValue)

	resp, err = tr.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: sid,
		ConfigID:  configIDHITLPolicy,
		Value:     hitlPolicyDefaultValue,
	})
	require.NoError(t, err)
	require.Empty(t, clikv.ReadHITLPolicy(ctx, runtimetypes.New(db.WithoutTransaction())))
	require.Equal(t, hitlPolicyDefaultValue, optionByID(t, resp.ConfigOptions, configIDHITLPolicy).CurrentValue)

	resp, err = tr.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: sid,
		ConfigID:  configIDThink,
		Value:     "xhigh",
	})
	require.NoError(t, err)
	require.Equal(t, "xhigh", sess.think())
	require.Equal(t, "xhigh", optionByID(t, resp.ConfigOptions, configIDThink).CurrentValue)
}

func TestUnit_SetSessionConfigOptionRejectsUnknownValue(t *testing.T) {
	ctx, db := setupConfigOptionsDB(t)

	sid := libacp.SessionID("sess-1")
	tr := &Transport{
		deps:               Deps{DB: db},
		sessions:           map[libacp.SessionID]*sessionEntry{sid: &sessionEntry{Provider: "openai", Model: "gpt-5-mini", Think: "low"}},
		defaultProvider:    "openai",
		defaultModel:       "gpt-5-mini",
		defaultAltProvider: "anthropic",
		defaultAltModel:    "claude-sonnet-4",
	}

	_, err := tr.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: sid,
		ConfigID:  configIDModel,
		Value:     "openai/not-advertised",
	})
	require.Error(t, err)
	var rpcErr *libacp.Error
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, libacp.ErrInvalidParams, rpcErr.Code)
	require.Equal(t, "openai", tr.provider())
	require.Equal(t, "gpt-5-mini", tr.model())
}

func optionByID(t *testing.T, options []libacp.SessionConfigOption, id string) libacp.SessionConfigOption {
	t.Helper()
	for _, option := range options {
		if option.ID == id {
			return option
		}
	}
	t.Fatalf("option %q not found in %#v", id, options)
	return libacp.SessionConfigOption{}
}
