package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/contenox/runtime/modeld/capacity"
	"github.com/contenox/runtime/runtime/transport"
)

// backendFactory builds the transport.Service for one compiled-in local backend.
type backendFactory func(capacity.Policy) transport.Service

// backends holds every inference backend compiled into this build, keyed by
// name. Backend files register themselves from init() under their build tags
// (see backend_llama.go, backend_openvino.go); an untagged build registers
// nothing and selectBackend falls back to unavailableBackend.
var backends = map[string]backendFactory{}

// backendHasAccel holds the optional accelerator probe for each compiled-in
// backend, keyed by name. A backend registers a probe (from its init()) when it
// can report whether an accelerator device is actually present on this host;
// selectBackend uses it to prefer an accelerated backend when several are
// compiled in. A nil probe (or absent entry) means "no accelerator known".
var backendHasAccel = map[string]func() bool{}

// registerBackend records a compiled-in backend. Called from backend file
// init()s; the build tags decide which backends a given binary contains.
// hasAccel is an optional probe (may be nil) reporting whether this backend has
// an accelerator on the current host.
func registerBackend(name string, f backendFactory, hasAccel func() bool) {
	if _, dup := backends[name]; dup {
		panic("modeld: backend registered twice: " + name)
	}
	backends[name] = f
	if hasAccel != nil {
		backendHasAccel[name] = hasAccel
	}
}

// preferredBackendOrder breaks ties when several backends are compiled in and
// CONTENOX_MODELD_BACKEND is not set: the accelerator-capable backend wins so a
// build that bothered to link OpenVINO uses it by default. Override per process
// with CONTENOX_MODELD_BACKEND.
var preferredBackendOrder = []string{"openvino", "llama"}

// selectBackend chooses the transport.Service this daemon serves. Selection:
//  1. CONTENOX_MODELD_BACKEND, if set and compiled in;
//  2. the only compiled-in backend, if exactly one;
//  3. among several, a backend reporting an accelerator on this host wins, with
//     preferredBackendOrder breaking remaining ties;
//  4. unavailableBackend when none is compiled in — the daemon still owns the
//     lease and answers health probes, it just cannot open sessions.
func selectBackend(policy capacity.Policy) (transport.Service, string) {
	if want := os.Getenv("CONTENOX_MODELD_BACKEND"); want != "" {
		if f, ok := backends[want]; ok {
			return f(policy), want
		}
		fmt.Fprintf(os.Stderr, "modeld: CONTENOX_MODELD_BACKEND=%q is not compiled into this build (have: %s); falling back\n", want, availableBackends())
	}

	names := availableBackendNames()
	switch len(names) {
	case 0:
		return unavailableBackend{}, "none"
	case 1:
		return backends[names[0]](policy), names[0]
	}

	// Several backends compiled in and no override: prefer one that reports an
	// accelerator present on this host, so a universal binary uses the GPU path
	// that physically exists rather than the static preference. Probing loads the
	// backend's native libs (CUDA / OpenVINO) once at startup.
	candidates := names
	var accelerated []string
	for _, name := range names {
		if probe := backendHasAccel[name]; probe != nil && probe() {
			accelerated = append(accelerated, name)
		}
	}
	if len(accelerated) > 0 {
		candidates = accelerated
	}

	chosen := candidates[0] // deterministic fallback if none is in the preference list
	for _, name := range preferredBackendOrder {
		if slices.Contains(candidates, name) {
			chosen = name
			break
		}
	}
	if len(accelerated) > 0 {
		fmt.Fprintf(os.Stderr, "modeld: multiple backends compiled (%s); accelerator detected for %s; using %q (set CONTENOX_MODELD_BACKEND to override)\n", availableBackends(), strings.Join(accelerated, ", "), chosen)
	} else {
		fmt.Fprintf(os.Stderr, "modeld: multiple backends compiled (%s); no accelerator detected; using %q (set CONTENOX_MODELD_BACKEND to override)\n", availableBackends(), chosen)
	}
	return backends[chosen](policy), chosen
}

// availableBackendNames returns the registered backend names, sorted for
// deterministic selection and logging.
func availableBackendNames() []string {
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func availableBackends() string {
	names := availableBackendNames()
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// errNoBackend is returned by OpenSession when this modeld build has no local
// inference backend compiled in.
var errNoBackend = errors.New("modeld: no local inference backend compiled in this build (build with -tags 'openvino openvino_genai' or -tags llamanode)")

type unavailableBackend struct{}

func (unavailableBackend) OpenSession(context.Context, transport.OpenSessionRequest) (transport.Session, error) {
	return nil, errNoBackend
}

func (unavailableBackend) Describe(context.Context, transport.OpenSessionRequest) (transport.ModelInfo, error) {
	return transport.ModelInfo{}, errNoBackend
}

func (unavailableBackend) Embed(context.Context, transport.EmbedRequest) (transport.EmbedResult, error) {
	return transport.EmbedResult{}, errNoBackend
}
