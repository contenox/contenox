package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/contenox/runtime/runtime/transport"
)

// backendFactory builds the transport.Service for one compiled-in local backend.
type backendFactory func() transport.Service

// backends holds every inference backend compiled into this build, keyed by
// name. Backend files register themselves from init() under their build tags
// (see backend_llama.go, backend_openvino.go); an untagged build registers
// nothing and selectBackend falls back to unavailableBackend.
var backends = map[string]backendFactory{}

// registerBackend records a compiled-in backend. Called from backend file
// init()s; the build tags decide which backends a given binary contains.
func registerBackend(name string, f backendFactory) {
	if _, dup := backends[name]; dup {
		panic("modeld: backend registered twice: " + name)
	}
	backends[name] = f
}

// preferredBackendOrder breaks ties when several backends are compiled in and
// CONTENOX_MODELD_BACKEND is not set: the accelerator-capable backend wins so a
// build that bothered to link OpenVINO uses it by default. Override per process
// with CONTENOX_MODELD_BACKEND.
var preferredBackendOrder = []string{"openvino", "llama"}

// selectBackend chooses the transport.Service this daemon serves. Selection:
//  1. CONTENOX_MODELD_BACKEND, if set and compiled in;
//  2. the only compiled-in backend, if exactly one;
//  3. preferredBackendOrder among several;
//  4. unavailableBackend when none is compiled in — the daemon still owns the
//     lease and answers health probes, it just cannot open sessions.
func selectBackend() (transport.Service, string) {
	if want := os.Getenv("CONTENOX_MODELD_BACKEND"); want != "" {
		if f, ok := backends[want]; ok {
			return f(), want
		}
		fmt.Fprintf(os.Stderr, "modeld: CONTENOX_MODELD_BACKEND=%q is not compiled into this build (have: %s); falling back\n", want, availableBackends())
	}

	names := availableBackendNames()
	switch len(names) {
	case 0:
		return unavailableBackend{}, "none"
	case 1:
		return backends[names[0]](), names[0]
	}

	chosen := names[0] // deterministic fallback if none is in the preference list
	for _, name := range preferredBackendOrder {
		if _, ok := backends[name]; ok {
			chosen = name
			break
		}
	}
	fmt.Fprintf(os.Stderr, "modeld: multiple backends compiled (%s); using %q (set CONTENOX_MODELD_BACKEND to override)\n", availableBackends(), chosen)
	return backends[chosen](), chosen
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
