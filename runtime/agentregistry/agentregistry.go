// Package agentregistry is a read-only catalog client for the Agent Client
// Protocol (ACP) agent registry — the published list of ACP-speaking agents at
// https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json.
//
// It is a CATALOG to seed from, not an installer. contenox never downloads,
// extracts, or verifies an agent's archive, and never manages a binary
// directory: it only ever *invokes* agents the user already has installed, or
// that run-time fetchers like npx/uvx pull down themselves. Resolve therefore
// turns a catalog entry into a RunSpec (a bare command + args + env the runtime
// can spawn), never into anything on disk.
package agentregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultRegistryURL is the canonical location of the ACP agent registry
// catalog. Fetch GETs this unless Client.URL overrides it.
const DefaultRegistryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

// defaultTimeout bounds a single registry HTTP fetch. The catalog is a small
// static JSON document, so a short timeout is plenty and keeps `agent search`
// responsive when the CDN is unreachable (Fetch then falls back to cache).
const defaultTimeout = 15 * time.Second

// Distribution method names observed in the registry's `distribution` map.
const (
	MethodNPX    = "npx"
	MethodUVX    = "uvx"
	MethodBinary = "binary"
)

// Registry is the top-level catalog document.
type Registry struct {
	Version    string          `json:"version"`
	Agents     []RegistryAgent `json:"agents"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
}

// RegistryAgent is one catalog entry. Distribution is kept as raw JSON per
// method because each method ("npx"/"uvx"/"binary") has a different shape;
// Resolve decodes only the method it selects.
type RegistryAgent struct {
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	Version      string                     `json:"version"`
	Description  string                     `json:"description"`
	Repository   string                     `json:"repository,omitempty"`
	Authors      []string                   `json:"authors,omitempty"`
	License      string                     `json:"license,omitempty"`
	Website      string                     `json:"website,omitempty"`
	Icon         string                     `json:"icon,omitempty"`
	Distribution map[string]json.RawMessage `json:"distribution"`
}

// RunSpec is how to spawn an agent, resolved from a catalog entry for a
// concrete OS/arch. It is a run recipe only: Command is a bare executable the
// runtime hands to exec (npx/uvx, or a binary basename the user must have on
// PATH), never a path into anything contenox installed.
type RunSpec struct {
	Command string            // executable to spawn (e.g. "npx", "uvx", "goose")
	Args    []string          // arguments passed to Command
	Env     map[string]string // extra environment the agent expects (never nil)
	Method  string            // distribution method used: "npx" | "uvx" | "binary"
	Note    string            // human note, e.g. a PATH requirement for binary agents
}

// scriptDist is the shape of the "npx" and "uvx" distribution methods:
// a package spec plus optional extra args.
type scriptDist struct {
	Package string   `json:"package"`
	Args    []string `json:"args,omitempty"`
}

// binaryPlatform is one "<os>-<arch>" entry of the "binary" distribution
// method. Archive/SHA256 are intentionally ignored by Resolve — contenox does
// not download or verify anything; only Cmd/Args/Env feed the RunSpec.
type binaryPlatform struct {
	Archive string            `json:"archive"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args,omitempty"`
	SHA256  string            `json:"sha256,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Client fetches and caches the registry catalog. The zero value is not
// usable; construct one with NewClient.
type Client struct {
	URL       string       // catalog URL; defaults to DefaultRegistryURL
	CachePath string       // local cache file path (e.g. <dataDir>/agent-registry.json)
	HTTP      *http.Client // HTTP client; defaults to one with defaultTimeout
}

// NewClient returns a Client that caches the catalog at cachePath and fetches
// from DefaultRegistryURL.
func NewClient(cachePath string) *Client {
	return &Client{
		URL:       DefaultRegistryURL,
		CachePath: cachePath,
		HTTP:      &http.Client{Timeout: defaultTimeout},
	}
}

// Fetch returns the registry catalog.
//
// When refresh is false and a readable cache exists, the cache is returned
// without any network access — the common `agent search` / `agent add` path
// stays offline and fast. When refresh is true (or no usable cache exists), it
// GETs the catalog, writes it to the cache (best-effort), and returns it. On a
// network or HTTP error it falls back to the cache if one is present, so a
// transient CDN outage doesn't break the command; only when there is no cache
// to fall back to does the network error surface.
func (c *Client) Fetch(ctx context.Context, refresh bool) (*Registry, error) {
	if !refresh {
		if reg, err := c.readCache(); err == nil {
			return reg, nil
		}
	}

	reg, fetchErr := c.fetchRemote(ctx)
	if fetchErr == nil {
		// Best-effort cache write; a failure to persist must not fail the fetch.
		_ = c.writeCache(reg)
		return reg, nil
	}

	// Network/HTTP failure: fall back to any cached copy so the command still works.
	if reg, err := c.readCache(); err == nil {
		return reg, nil
	}
	return nil, fmt.Errorf("agentregistry: fetch %s: %w", c.url(), fetchErr)
}

func (c *Client) url() string {
	if c.URL != "" {
		return c.URL
	}
	return DefaultRegistryURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: defaultTimeout}
}

func (c *Client) fetchRemote(ctx context.Context) (*Registry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var reg Registry
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("decode registry: %w", err)
	}
	return &reg, nil
}

func (c *Client) readCache() (*Registry, error) {
	if c.CachePath == "" {
		return nil, fmt.Errorf("no cache path")
	}
	data, err := os.ReadFile(c.CachePath)
	if err != nil {
		return nil, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse cached registry %s: %w", c.CachePath, err)
	}
	return &reg, nil
}

func (c *Client) writeCache(reg *Registry) error {
	if c.CachePath == "" {
		return fmt.Errorf("no cache path")
	}
	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.CachePath, data, 0o644)
}

// List returns every catalog entry (a copy, safe to sort/filter).
func List(reg *Registry) []RegistryAgent {
	if reg == nil {
		return nil
	}
	out := make([]RegistryAgent, len(reg.Agents))
	copy(out, reg.Agents)
	return out
}

// Find returns the catalog entry with the given id (exact match).
func Find(reg *Registry, id string) (RegistryAgent, bool) {
	if reg == nil {
		return RegistryAgent{}, false
	}
	for _, a := range reg.Agents {
		if a.ID == id {
			return a, true
		}
	}
	return RegistryAgent{}, false
}

// Search returns catalog entries whose id, name, or description contains query
// (case-insensitive). An empty query returns the full catalog.
func Search(reg *Registry, query string) []RegistryAgent {
	if reg == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return List(reg)
	}
	var out []RegistryAgent
	for _, a := range reg.Agents {
		hay := strings.ToLower(a.ID + " " + a.Name + " " + a.Description)
		if strings.Contains(hay, q) {
			out = append(out, a)
		}
	}
	return out
}

// Resolve turns a catalog entry into a RunSpec for the given Go
// runtime.GOOS/runtime.GOARCH. It selects, in order of preference, npx, then
// uvx, then binary — the run-time fetchers first, since they need nothing
// pre-installed. (Registry entries carry exactly one method in practice; the
// ordering only matters as a tiebreak.)
//
//   - npx    → {Command:"npx", Args:["-y", <package>, <distArgs>...]}. The -y
//     flag lets npx self-install the package non-interactively on first run
//     instead of blocking on an install prompt.
//   - uvx    → {Command:"uvx", Args:[<package>, <distArgs>...]}. uvx fetches
//     and runs the package itself; no prompt to suppress.
//   - binary → the platform's cmd basename becomes Command (contenox does NOT
//     download the archive), with its args/env, and a Note that the user must
//     install the agent and have that binary on PATH. Returns a clear error if
//     no platform entry matches goos/goarch.
//
// Env is always non-nil (empty for npx/uvx that declare no env).
func Resolve(a RegistryAgent, goos, goarch string) (RunSpec, error) {
	if raw, ok := a.Distribution[MethodNPX]; ok {
		dist, err := decodeScriptDist(raw, MethodNPX)
		if err != nil {
			return RunSpec{}, err
		}
		args := append([]string{"-y", dist.Package}, dist.Args...)
		return RunSpec{
			Command: "npx",
			Args:    args,
			Env:     map[string]string{},
			Method:  MethodNPX,
		}, nil
	}

	if raw, ok := a.Distribution[MethodUVX]; ok {
		dist, err := decodeScriptDist(raw, MethodUVX)
		if err != nil {
			return RunSpec{}, err
		}
		args := append([]string{dist.Package}, dist.Args...)
		return RunSpec{
			Command: "uvx",
			Args:    args,
			Env:     map[string]string{},
			Method:  MethodUVX,
		}, nil
	}

	if raw, ok := a.Distribution[MethodBinary]; ok {
		return resolveBinary(a, raw, goos, goarch)
	}

	return RunSpec{}, fmt.Errorf(
		"agentregistry: agent %q has no supported distribution method (has: %s; supported: npx, uvx, binary)",
		a.ID, strings.Join(distributionMethods(a), ", "))
}

func decodeScriptDist(raw json.RawMessage, method string) (scriptDist, error) {
	var dist scriptDist
	if err := json.Unmarshal(raw, &dist); err != nil {
		return scriptDist{}, fmt.Errorf("agentregistry: decode %s distribution: %w", method, err)
	}
	if dist.Package == "" {
		return scriptDist{}, fmt.Errorf("agentregistry: %s distribution is missing a package", method)
	}
	return dist, nil
}

func resolveBinary(a RegistryAgent, raw json.RawMessage, goos, goarch string) (RunSpec, error) {
	var platforms map[string]binaryPlatform
	if err := json.Unmarshal(raw, &platforms); err != nil {
		return RunSpec{}, fmt.Errorf("agentregistry: decode binary distribution: %w", err)
	}

	key := PlatformKey(goos, goarch)
	plat, ok := platforms[key]
	if !ok {
		return RunSpec{}, fmt.Errorf(
			"agentregistry: agent %q has no binary distribution for %s (available: %s)",
			a.ID, key, strings.Join(sortedKeys(platforms), ", "))
	}
	if plat.Cmd == "" {
		return RunSpec{}, fmt.Errorf("agentregistry: agent %q binary distribution for %s has no cmd", a.ID, key)
	}

	binary := binaryBasename(plat.Cmd)
	env := plat.Env
	if env == nil {
		env = map[string]string{}
	}
	return RunSpec{
		Command: binary,
		Args:    plat.Args,
		Env:     env,
		Method:  MethodBinary,
		Note:    fmt.Sprintf("install %s and ensure %q is on PATH", a.Name, binary),
	}, nil
}

// PlatformKey maps a Go runtime.GOOS/GOARCH pair to the registry's
// "<os>-<arch>" distribution key (e.g. linux/amd64 -> "linux-x86_64",
// darwin/arm64 -> "darwin-aarch64"). GOOS tokens (linux, darwin, windows) match
// the registry directly; only the arch token is translated.
func PlatformKey(goos, goarch string) string {
	arch := goarch
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	return goos + "-" + arch
}

// binaryBasename extracts the bare executable name from a registry cmd, which
// may be a relative path into the (un-downloaded) archive such as "./goose",
// "./bin/devin", or a Windows path like "./goose-package\\goose.exe". Backslash
// separators are normalized so Windows cmds resolve on any host.
func binaryBasename(cmd string) string {
	normalized := strings.ReplaceAll(cmd, "\\", "/")
	return path.Base(normalized)
}

// distributionMethods returns the method names present on a, sorted.
func distributionMethods(a RegistryAgent) []string {
	return sortedKeys(a.Distribution)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
