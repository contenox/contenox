package main

import "testing"

func TestUnit_IsLoopbackEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{"ipv4 loopback", "127.0.0.1:8080", true},
		{"ipv4 loopback other host part", "127.0.0.2:8080", true},
		{"ipv6 loopback", "[::1]:8080", true},
		{"localhost hostname", "localhost:8080", true},
		{"localhost mixed case", "LocalHost:8080", true},
		{"all interfaces (empty host)", ":8080", false},
		{"lan address", "192.168.1.5:8080", false},
		{"public address", "203.0.113.7:8080", false},
		{"hostname that is not localhost", "modeld-node.internal:8080", false},
		{"no port suffix falls back to raw host", "127.0.0.1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLoopbackEndpoint(tc.endpoint); got != tc.want {
				t.Fatalf("isLoopbackEndpoint(%q) = %v, want %v", tc.endpoint, got, tc.want)
			}
		})
	}
}

func TestUnit_MinHotContextZeroDisablesFloor(t *testing.T) {
	policy, err := resolvePolicy(t.TempDir(), "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if policy.MinHotContextTokens != -1 {
		t.Fatalf("zero must disable the floor, got %d", policy.MinHotContextTokens)
	}
}
