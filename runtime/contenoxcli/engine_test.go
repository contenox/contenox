package contenoxcli

import "testing"

func TestReadinessDefaults(t *testing.T) {
	cases := []struct {
		name         string
		opts         chatOpts
		wantModel    string
		wantProvider string
	}{
		{
			name: "explicit --model on fresh DB is credited",
			opts: chatOpts{
				EffectiveDefaultModel:    "phi-4-mini",
				EffectiveConfiguredModel: "",
			},
			wantModel: "phi-4-mini",
		},
		{
			name: "hardcoded fallback model on fresh DB is not credited",
			opts: chatOpts{
				EffectiveDefaultModel:    defaultModel,
				EffectiveConfiguredModel: "",
			},
			wantModel: "",
		},
		{
			name: "model from persisted config needs no override",
			opts: chatOpts{
				EffectiveDefaultModel:    "persisted",
				EffectiveConfiguredModel: "persisted",
			},
			wantModel: "",
		},
		{
			name: "explicit --provider on fresh DB is credited",
			opts: chatOpts{
				EffectiveDefaultProvider:    "ollama",
				EffectiveConfiguredProvider: "",
			},
			wantProvider: "ollama",
		},
		{
			name: "provider from persisted config needs no override",
			opts: chatOpts{
				EffectiveDefaultProvider:    "ollama",
				EffectiveConfiguredProvider: "ollama",
			},
			wantProvider: "",
		},
		{
			name: "model and provider flags both credited together",
			opts: chatOpts{
				EffectiveDefaultModel:    "phi-4-mini",
				EffectiveConfiguredModel: "",
				EffectiveDefaultProvider: "llama",
			},
			wantModel:    "phi-4-mini",
			wantProvider: "llama",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, provider := readinessDefaults(tc.opts)
			if model != tc.wantModel {
				t.Errorf("model = %q, want %q", model, tc.wantModel)
			}
			if provider != tc.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tc.wantProvider)
			}
		})
	}
}
