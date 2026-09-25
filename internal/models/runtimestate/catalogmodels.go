package runtimestate

import (
	"strings"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
)

const observedDisplayNameMetaKey = "display_name"

func observedModelFromPullStatus(model ModelPullStatus) modelrepo.ObservedModel {
	name := strings.TrimSpace(model.Model)
	if name == "" {
		name = strings.TrimSpace(model.Name)
	}

	meta := map[string]string{}
	if display := strings.TrimSpace(model.Name); display != "" && display != name {
		meta[observedDisplayNameMetaKey] = display
	}
	if len(meta) == 0 {
		meta = nil
	}

	return modelrepo.ObservedModel{
		Name:          name,
		ContextLength: model.ContextLength,
		ModifiedAt:    model.ModifiedAt,
		Size:          model.Size,
		Digest:        model.Digest,
		CapabilityConfig: modelrepo.CapabilityConfig{
			ContextLength:    model.ContextLength,
			MaxOutputTokens:  model.MaxOutputTokens,
			CanChat:          model.CanChat,
			CanEmbed:         model.CanEmbed,
			CanPrompt:        model.CanPrompt,
			CanStream:        model.CanStream,
			CanThink:         model.CanThink,
			CanVision:        model.CanVision,
			CanAudio:         model.CanAudio,
			AudioExtension:   model.AudioExtension,
			SessionExtension: model.SessionExtension,
		},
		Meta: meta,
	}
}

func declaredModelsByName(models []*runtimetypes.Model) map[string]*runtimetypes.Model {
	byName := make(map[string]*runtimetypes.Model, len(models))
	for _, model := range models {
		byName[model.Model] = model
	}
	return byName
}

func declaredModelNames(models []*runtimetypes.Model) []string {
	names := make([]string, 0, len(models))
	for _, model := range models {
		names = append(names, model.Model)
	}
	return names
}

func pullStatusFromObservedModel(model modelrepo.ObservedModel) ModelPullStatus {
	displayName := model.Name
	if model.Meta != nil {
		if display := strings.TrimSpace(model.Meta[observedDisplayNameMetaKey]); display != "" {
			displayName = display
		}
	}

	return ModelPullStatus{
		Name:             displayName,
		Model:            model.Name,
		ModifiedAt:       model.ModifiedAt,
		Size:             model.Size,
		Digest:           model.Digest,
		ContextLength:    model.ContextLength,
		MaxOutputTokens:  model.MaxOutputTokens,
		CanChat:          model.CanChat,
		CanEmbed:         model.CanEmbed,
		CanPrompt:        model.CanPrompt,
		CanStream:        model.CanStream,
		CanThink:         model.CanThink,
		CanVision:        model.CanVision,
		CanAudio:         model.CanAudio,
		AudioExtension:   model.CapabilityConfig.AudioExtension,
		SessionExtension: model.CapabilityConfig.SessionExtension,
	}
}
