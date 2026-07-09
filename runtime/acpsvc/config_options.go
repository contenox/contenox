package acpsvc

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	libacp "github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/reasoning"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/statetype"
)

const (
	configIDModel      = "model"
	configIDHITLPolicy = "hitl-policy"
	configIDThink      = "think"
	configIDTokenLimit = "token-limit"

	configCategoryModel      = "model"
	configCategoryHITLPolicy = "_hitl_policy"
	configCategoryThink      = "thought_level"

	configTypeSelect = "select"

	hitlPolicyDefaultValue = "__contenox_default__"
)

func (t *Transport) sessionConfigOptions(ctx context.Context, sess *sessionEntry) []libacp.SessionConfigOption {
	return []libacp.SessionConfigOption{
		t.modelConfigOption(ctx, sess),
		t.hitlPolicyConfigOption(ctx),
		t.thinkConfigOption(sess),
		t.tokenLimitConfigOption(ctx, sess),
	}
}

func (t *Transport) modelConfigOption(ctx context.Context, sess *sessionEntry) libacp.SessionConfigOption {
	currentProvider := sess.providerOrDefault(t.provider())
	currentModel := sess.modelOrDefault(t.model())
	current := modelConfigValue(currentProvider, currentModel)
	return libacp.SessionConfigOption{
		ID:           configIDModel,
		Name:         "Model",
		Category:     configCategoryModel,
		Type:         configTypeSelect,
		CurrentValue: current,
		Options:      t.modelConfigValues(ctx, currentProvider, currentModel),
	}
}

func (t *Transport) hitlPolicyConfigOption(ctx context.Context) libacp.SessionConfigOption {
	return libacp.SessionConfigOption{
		ID:           configIDHITLPolicy,
		Name:         "HITL Policy",
		Description:  "Approval policy used for gated tool calls",
		Category:     configCategoryHITLPolicy,
		Type:         configTypeSelect,
		CurrentValue: t.hitlPolicyConfigValue(ctx),
		Options:      t.hitlPolicyConfigValues(ctx),
	}
}

func (t *Transport) thinkConfigOption(sess *sessionEntry) libacp.SessionConfigOption {
	return libacp.SessionConfigOption{
		ID:           configIDThink,
		Name:         "Think",
		Description:  "Reasoning level for this session",
		Category:     configCategoryThink,
		Type:         configTypeSelect,
		CurrentValue: sess.think(),
		Options: libacp.NewSessionConfigValues([]libacp.SessionConfigValue{
			{Value: reasoning.Auto, Name: "Auto"},
			{Value: reasoning.Off, Name: "Off"},
			{Value: reasoning.Minimal, Name: "Minimal"},
			{Value: reasoning.Low, Name: "Low"},
			{Value: reasoning.Medium, Name: "Medium"},
			{Value: reasoning.High, Name: "High"},
			{Value: reasoning.XHigh, Name: "XHigh"},
		}),
	}
}

func (t *Transport) tokenLimitConfigOption(ctx context.Context, sess *sessionEntry) libacp.SessionConfigOption {
	limit := sess.effectiveTokenLimit()
	current := "0"
	if limit > 0 {
		current = strconv.Itoa(limit)
	}
	cap := t.modelContextCap(ctx, sess)
	desc := "Session context budget (token limit for history). Controls shifting and usage indicator size. 0 = chain default / unlimited."
	if cap > 0 {
		desc += fmt.Sprintf(" Capped to model max %d if larger.", cap)
	}
	return libacp.SessionConfigOption{
		ID:           configIDTokenLimit,
		Name:         "Token Limit",
		Description:  desc,
		Category:     "context",
		Type:         "text",
		CurrentValue: current,
	}
}

// modelContextCap returns the hard cap for the session's current model (0 if unknown/unreported).
// Prefers declared/observed ContextLength from runtime pulled models.
func (t *Transport) modelContextCap(ctx context.Context, sess *sessionEntry) int {
	if sess == nil {
		return 0
	}
	prov := sess.providerOrDefault(t.provider())
	mod := sess.modelOrDefault(t.model())
	for _, st := range t.runtimeStates(ctx) {
		for _, pm := range st.PulledModels {
			if (pm.Model == mod || pm.Name == mod) && (prov == "" || strings.Contains(strings.ToLower(st.Backend.Type), strings.ToLower(prov)) || prov == "") {
				if pm.ContextLength > 0 {
					return pm.ContextLength
				}
			}
		}
	}
	// Fallback: any matching model name
	for _, st := range t.runtimeStates(ctx) {
		for _, pm := range st.PulledModels {
			if pm.Model == mod || pm.Name == mod {
				if pm.ContextLength > 0 {
					return pm.ContextLength
				}
			}
		}
	}
	return 0
}

func (t *Transport) modelConfigValues(ctx context.Context, currentProvider, currentModel string) libacp.SessionConfigValues {
	type modelValue struct {
		provider    string
		value       string
		name        string
		description string
		current     bool
	}

	seen := map[string]modelValue{}
	add := func(provider, model, description string, current bool) {
		value := modelConfigValue(provider, model)
		if strings.TrimSpace(value) == "" {
			return
		}
		name := modelConfigDisplayName(model)
		if existing, ok := seen[value]; ok {
			existing.current = existing.current || current
			if existing.description == "" {
				existing.description = description
			}
			seen[value] = existing
			return
		}
		seen[value] = modelValue{
			provider:    strings.TrimSpace(provider),
			value:       value,
			name:        name,
			description: description,
			current:     current,
		}
	}

	add(currentProvider, currentModel, "Current default model", true)
	if altModel := t.altModel(); altModel != "" {
		altProvider := t.altProvider()
		if altProvider == "" {
			altProvider = currentProvider
		}
		add(altProvider, altModel, "Configured alternate model", false)
	}

	for _, state := range t.runtimeStates(ctx) {
		if strings.TrimSpace(state.Error) != "" {
			continue
		}
		provider := strings.TrimSpace(state.Backend.Type)
		if provider == "" {
			continue
		}
		for _, pulled := range state.PulledModels {
			if !pulled.CanChat && !pulled.CanPrompt {
				continue
			}
			model := strings.TrimSpace(pulled.Model)
			if model == "" {
				model = strings.TrimSpace(pulled.Name)
			}
			add(provider, model, describePulledModel(pulled), false)
		}
	}

	groupsByProvider := map[string][]modelValue{}
	groupNames := map[string]string{}
	for _, value := range seen {
		groupID := value.provider
		if groupID == "" {
			groupID = "default"
		}
		groupsByProvider[groupID] = append(groupsByProvider[groupID], value)
		groupNames[groupID] = groupID
	}
	if len(groupsByProvider) == 0 {
		return libacp.NewSessionConfigValues(nil)
	}

	groupIDs := make([]string, 0, len(groupsByProvider))
	for groupID := range groupsByProvider {
		groupIDs = append(groupIDs, groupID)
	}
	sort.SliceStable(groupIDs, func(i, j int) bool {
		leftCurrent := groupIDs[i] == currentProvider
		rightCurrent := groupIDs[j] == currentProvider
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		return strings.ToLower(groupIDs[i]) < strings.ToLower(groupIDs[j])
	})

	groups := make([]libacp.SessionConfigGroup, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		values := groupsByProvider[groupID]
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].current != values[j].current {
				return values[i].current
			}
			return strings.ToLower(values[i].name) < strings.ToLower(values[j].name)
		})
		options := make([]libacp.SessionConfigValue, 0, len(values))
		for _, value := range values {
			options = append(options, libacp.SessionConfigValue{
				Value:       value.value,
				Name:        value.name,
				Description: value.description,
			})
		}
		groups = append(groups, libacp.SessionConfigGroup{
			Group:   groupID,
			Name:    groupNames[groupID],
			Options: options,
		})
	}
	return libacp.NewGroupedSessionConfigValues(groups)
}

func (t *Transport) hitlPolicyConfigValue(ctx context.Context) string {
	if active := t.activeHITLPolicy(ctx); active != "" {
		return active
	}
	return hitlPolicyDefaultValue
}

func (t *Transport) hitlPolicyConfigValues(ctx context.Context) libacp.SessionConfigValues {
	defaultName := "Default"
	defaultDescription := "Use Contenox's configured fallback policy"
	if name := strings.TrimSpace(t.deps.HITLDefaultPolicyName); name != "" {
		defaultDescription = "Use " + name
	}
	values := []libacp.SessionConfigValue{{
		Value:       hitlPolicyDefaultValue,
		Name:        defaultName,
		Description: defaultDescription,
	}}

	seen := map[string]struct{}{hitlPolicyDefaultValue: {}}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		values = append(values, libacp.SessionConfigValue{Value: name, Name: hitlPolicyDisplayName(name)})
	}
	for _, name := range t.deps.KnownPolicies {
		add(name)
	}
	add(t.activeHITLPolicy(ctx))
	return libacp.NewSessionConfigValues(values)
}

func (t *Transport) activeHITLPolicy(ctx context.Context) string {
	if t.deps.DB == nil {
		return ""
	}
	return clikv.ReadHITLPolicy(ctx, runtimetypes.New(t.deps.DB.WithoutTransaction()))
}

func (t *Transport) runtimeStates(ctx context.Context) []statetype.BackendRuntimeState {
	if t.deps.Engine == nil || t.deps.Engine.State == nil {
		return nil
	}
	states := t.deps.Engine.State.Get(ctx)
	out := make([]statetype.BackendRuntimeState, 0, len(states))
	for _, state := range states {
		out = append(out, state)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Backend.Type + "/" + out[i].Name)
		right := strings.ToLower(out[j].Backend.Type + "/" + out[j].Name)
		return left < right
	})
	return out
}

func (t *Transport) SetSessionConfigOption(ctx context.Context, req libacp.SetSessionConfigOptionRequest) (libacp.SetSessionConfigOptionResponse, error) {
	reportErr, reportChange, end := t.tracker().Start(ctx, "set_config_option", "acp_session", "session_id", string(req.SessionID), "config_id", req.ConfigID)
	defer end()

	sess, ok := t.sessionFor(req.SessionID)
	if !ok {
		err := libacp.NewErrorf(libacp.ErrInvalidParams, "unknown session %q", req.SessionID)
		reportErr(err)
		return libacp.SetSessionConfigOptionResponse{}, err
	}

	if err := t.setSessionConfigOption(ctx, sess, req.ConfigID, req.Value); err != nil {
		reportErr(err)
		return libacp.SetSessionConfigOptionResponse{}, err
	}

	reportChange(req.ConfigID, req.Value)
	return libacp.SetSessionConfigOptionResponse{
		ConfigOptions: t.sessionConfigOptions(ctx, sess),
	}, nil
}

func (t *Transport) setSessionConfigOption(ctx context.Context, sess *sessionEntry, configID, value string) error {
	switch configID {
	case configIDModel:
		if !configOptionHasValue(t.modelConfigOption(ctx, sess), value) {
			return libacp.NewErrorf(libacp.ErrInvalidParams, "unknown model option %q", value)
		}
		provider, model := splitModelConfigValue(value)
		if strings.TrimSpace(model) == "" {
			return libacp.NewErrorf(libacp.ErrInvalidParams, "model option %q has empty model", value)
		}
		sess.setModelSelection(provider, model)
		return nil

	case configIDHITLPolicy:
		if !configOptionHasValue(t.hitlPolicyConfigOption(ctx), value) {
			return libacp.NewErrorf(libacp.ErrInvalidParams, "unknown HITL policy option %q", value)
		}
		policy := value
		if policy == hitlPolicyDefaultValue {
			policy = ""
		}
		if t.deps.DB == nil {
			return fmt.Errorf("cannot set HITL policy: database unavailable")
		}
		cfgCtx := libtracker.WithNewRequestID(ctx)
		if err := clikv.SetHITLPolicy(cfgCtx, runtimetypes.New(t.deps.DB.WithoutTransaction()), policy); err != nil {
			return fmt.Errorf("set hitl policy: %w", err)
		}
		return nil

	case configIDThink:
		level, err := reasoning.Normalize(value)
		if err != nil {
			return libacp.NewError(libacp.ErrInvalidParams, err.Error())
		}
		if !configOptionHasValue(t.thinkConfigOption(sess), level) {
			return libacp.NewErrorf(libacp.ErrInvalidParams, "unknown think option %q", value)
		}
		sess.setThink(level)
		return nil

	case configIDTokenLimit:
		requested := 0
		if strings.TrimSpace(value) != "" && value != "0" {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return libacp.NewErrorf(libacp.ErrInvalidParams, "token-limit must be a non-negative integer, got %q", value)
			}
			requested = n
		}
		cap := t.modelContextCap(ctx, sess)
		eff := requested
		if cap > 0 && (eff == 0 || eff > cap) {
			eff = cap
		}
		sess.setEffectiveTokenLimit(eff)
		return nil

	default:
		return libacp.NewErrorf(libacp.ErrInvalidParams, "unknown config option %q", configID)
	}
}

func (t *Transport) sendConfigOptionUpdate(ctx context.Context, sid libacp.SessionID, sess *sessionEntry) {
	if t.conn == nil || sess == nil {
		return
	}
	t.sendUpdate(ctx, libacp.SessionNotification{
		SessionID: sid,
		Update: libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateConfigOption,
			ConfigOptions: t.sessionConfigOptions(ctx, sess),
		},
	})
}

func configOptionHasValue(option libacp.SessionConfigOption, value string) bool {
	for _, candidate := range option.Options.AllValues() {
		if candidate.Value == value {
			return true
		}
	}
	return false
}

func modelConfigValue(provider, model string) string {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" {
		return model
	}
	if model == "" {
		return provider
	}
	return provider + "/" + model
}

func modelConfigDisplayName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "(default)"
	}
	return strings.TrimPrefix(model, "models/")
}

func splitModelConfigValue(value string) (provider, model string) {
	value = strings.TrimSpace(value)
	if before, after, ok := strings.Cut(value, "/"); ok {
		return strings.TrimSpace(before), strings.TrimSpace(after)
	}
	return "", value
}

func hitlPolicyDisplayName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".json")
	name = strings.TrimPrefix(name, "hitl-policy-")
	if name == "" {
		return "Default"
	}
	return name
}

func describePulledModel(model statetype.ModelPullStatus) string {
	var parts []string
	if model.ContextLength > 0 {
		parts = append(parts, "context "+strconv.Itoa(model.ContextLength))
	}
	if model.MaxOutputTokens > 0 {
		parts = append(parts, "output ceiling "+strconv.Itoa(model.MaxOutputTokens))
	}
	if model.CanThink {
		parts = append(parts, "thinking")
	}
	return strings.Join(parts, ", ")
}
