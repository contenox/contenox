package main

import (
	"testing"

	"github.com/contenox/runtime/modeld/capacity"
	"github.com/contenox/runtime/runtime/transport"
)

// TestUnit_SelectBackend covers the hardware-aware tie-break in selectBackend:
// with several backends compiled in, an accelerator-reporting backend beats the
// static preference order, ties fall back to that order, and an explicit
// CONTENOX_MODELD_BACKEND override always wins. Fake factories/probes keep this
// free of native dependencies.
func TestUnit_SelectBackend(t *testing.T) {
	origBackends := backends
	origAccel := backendHasAccel
	origOrder := preferredBackendOrder
	t.Cleanup(func() {
		backends = origBackends
		backendHasAccel = origAccel
		preferredBackendOrder = origOrder
	})

	fakeFactory := func(policy capacity.Policy) transport.Service { return unavailableBackend{} }
	yes := func() bool { return true }
	no := func() bool { return false }

	cases := []struct {
		name     string
		compiled map[string]func() bool // backend name -> accel probe (nil = no probe)
		order    []string
		override string
		want     string
	}{
		{
			name:     "single backend wins regardless of accelerator",
			compiled: map[string]func() bool{"llama": no},
			want:     "llama",
		},
		{
			name:     "accelerated backend beats static preference",
			compiled: map[string]func() bool{"llama": yes, "openvino": no},
			order:    []string{"openvino", "llama"},
			want:     "llama",
		},
		{
			name:     "openvino accelerator chosen",
			compiled: map[string]func() bool{"llama": no, "openvino": yes},
			order:    []string{"openvino", "llama"},
			want:     "openvino",
		},
		{
			name:     "both accelerated falls back to preference order",
			compiled: map[string]func() bool{"llama": yes, "openvino": yes},
			order:    []string{"openvino", "llama"},
			want:     "openvino",
		},
		{
			name:     "none accelerated falls back to preference order",
			compiled: map[string]func() bool{"llama": no, "openvino": no},
			order:    []string{"openvino", "llama"},
			want:     "openvino",
		},
		{
			name:     "explicit override wins over accelerator",
			compiled: map[string]func() bool{"llama": yes, "openvino": no},
			order:    []string{"openvino", "llama"},
			override: "openvino",
			want:     "openvino",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backends = map[string]backendFactory{}
			backendHasAccel = map[string]func() bool{}
			for name, probe := range tc.compiled {
				backends[name] = fakeFactory
				if probe != nil {
					backendHasAccel[name] = probe
				}
			}
			if tc.order != nil {
				preferredBackendOrder = tc.order
			} else {
				preferredBackendOrder = origOrder
			}
			t.Setenv("CONTENOX_MODELD_BACKEND", tc.override)

			if _, got := selectBackend(capacity.Policy{}); got != tc.want {
				t.Fatalf("selectBackend = %q, want %q", got, tc.want)
			}
		})
	}
}
