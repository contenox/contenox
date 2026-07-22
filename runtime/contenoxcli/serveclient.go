// serveclient.go is the first HTTP client in contenoxcli. Every other CLI
// verb that touches state (agent, backend, model, session, state, ...) opens
// the SQLite database (and, for state, the runtime engine) directly — see
// db_util.go's OpenDBAt. That works because everything those verbs read or
// write is durable and process-independent.
//
// Some state is neither: a pending human-in-the-loop approval
// (runtime/hitlservice) has a goroutine parked inside RequestApproval in the
// *serve* process, waiting on an in-memory channel. A future fleet instance
// (runtime/agentinstance) is a live subprocess serve's Manager owns. Writing
// the durable row (or the fleet's config) from a second process would change
// state without waking the goroutine or touching the running instance — the
// row/config would eventually be consistent, but nothing would happen until
// serve noticed on its own. So any CLI verb that needs THAT state must reach
// serve the same way any other API client does: over HTTP, through
// `/api/*`.
//
// serveClient is deliberately resource-agnostic (get/post move a JSON body
// in, an optional JSON body out, over an arbitrary path) so it is not
// approvals-specific: `contenox approvals` (fleet-consolidation.md slice C2)
// is its first caller, and `contenox fleet` (slice C3) is meant to build its
// verbs — list/show/stop/cancel/dispatch — on the same client rather than
// hand-rolling a second one.
package contenoxcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	// envServeURL overrides the base URL of the `contenox serve` instance a
	// serveClient talks to. Unset defaults to
	// http://<defaultServeAddr>:<defaultServePort> — serve's own bind
	// default (see serve_cmd.go) — so a loopback `contenox serve` started
	// with no configuration needs no client configuration either.
	envServeURL = "CONTENOX_SERVER_URL"
	// envServeToken supplies the bearer credential serve's ProtectAPI
	// expects (runtime/serverapi/local_security.go) when serve was started
	// with a TOKEN configured. Empty matches serve's own no-token loopback
	// default: reads pass, and browser-only CSRF rules do not apply to a
	// non-browser client like this one.
	envServeToken = "CONTENOX_SERVER_TOKEN"

	// serveClientTimeout bounds one HTTP round trip. Generous relative to a
	// local loopback call, but finite: a CLI command must eventually report
	// "serve is unreachable" instead of hanging next to a dead process.
	serveClientTimeout = 30 * time.Second
)

// serveClient is a minimal HTTP client for contenox serve's /api/* surface.
// See the package doc above for why it exists at all.
type serveClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// addServeClientFlags attaches --server/--token to cmd, typically a command
// group's parent (e.g. approvalsCmd) so every subcommand inherits them.
// Kept as a shared helper (not copy-pasted per command tree) so `contenox
// fleet` registers the identical two flags later.
func addServeClientFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("server", "",
		fmt.Sprintf("Base URL of a running 'contenox serve' (default http://%s:%s; also settable via %s)",
			defaultServeAddr, defaultServePort, envServeURL))
	cmd.PersistentFlags().String("token", "",
		fmt.Sprintf("Bearer token for the serve API, when serve was started with one configured (also settable via %s)",
			envServeToken))
}

// newServeClient resolves the base URL and token from --server/--token,
// falling back to CONTENOX_SERVER_URL/CONTENOX_SERVER_TOKEN, then to
// serve's own bind default (defaultServeAddr:defaultServePort, no token) —
// the zero-config loopback case every other `contenox serve` default
// assumes.
func newServeClient(cmd *cobra.Command) (*serveClient, error) {
	base, _ := cmd.Flags().GetString("server")
	defaulted := false
	if strings.TrimSpace(base) == "" {
		base = os.Getenv(envServeURL)
	}
	if strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("http://%s:%s", defaultServeAddr, defaultServePort)
		// Neither --server nor CONTENOX_SERVER_URL was given: this command is
		// about to talk to the default loopback serve. Surface WHICH instance it
		// hit — from an isolated HOME a bare `mission list`/`fleet stop` otherwise
		// silently reads or mutates whatever serve happens to own :32123, with no
		// hint in the output that a default was used.
		defaulted = true
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if _, err := url.ParseRequestURI(base); err != nil {
		return nil, fmt.Errorf("invalid server URL %q: %w", base, err)
	}

	token, _ := cmd.Flags().GetString("token")
	if strings.TrimSpace(token) == "" {
		token = os.Getenv(envServeToken)
	}

	if defaulted {
		// Hint on stderr only, so the stdout contract (e.g. `mission fire -q`
		// prints a bare correlatable id) is untouched. Shown only when defaulted:
		// an operator who set --server/CONTENOX_SERVER_URL already knows the target.
		fmt.Fprintf(cmd.ErrOrStderr(), "serve: %s (default; set --server or %s to target another instance)\n", base, envServeURL)
	}

	return &serveClient{
		baseURL: base,
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: serveClientTimeout},
	}, nil
}

// ServeError is returned by serveClient methods when serve answers with a
// non-2xx status. StatusCode lets callers map specific statuses to specific
// behavior (e.g. `contenox approvals answer` telling a 404 apart from a 409)
// without re-parsing the response body themselves.
type ServeError struct {
	StatusCode int
	Message    string
}

func (e *ServeError) Error() string {
	return fmt.Sprintf("serve: %s (status %d)", e.Message, e.StatusCode)
}

// serveAPIErrorBody mirrors the error envelope apiframework.Error writes
// (apiframework/errors.go's apiErrorResponse) so a non-2xx response reports
// serve's own message instead of a generic "request failed".
type serveAPIErrorBody struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// get issues a GET to path (relative to baseURL+"/api") and decodes a 2xx
// JSON response into out.
func (c *serveClient) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// post issues a POST to path with body JSON-encoded (nil for no body), and
// decodes a 2xx JSON response into out (nil to discard it).
func (c *serveClient) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// delete issues a DELETE to path and discards any 2xx body. `contenox fleet
// stop` (slice C3) is its first caller: DELETE /fleet/{id} is idempotent by the
// kernel contract, so the response body ("deleted") carries nothing the CLI
// needs beyond the 2xx.
func (c *serveClient) delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// do is the one place this client builds a request, authenticates it, and
// interprets the response. path is joined onto baseURL + "/api" — every
// route this client calls is mounted there (see serverapi.registerProductRoutes
// and runtime/contenoxcli/serve_cmd.go's rootMux.Handle("/api/", ...)).
func (c *serveClient) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api"+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect to %s: %w (is 'contenox serve' running?)", c.baseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response from %s: %w", c.baseURL, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		var apiErr serveAPIErrorBody
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Error.Message != "" {
			msg = apiErr.Error.Message
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &ServeError{StatusCode: resp.StatusCode, Message: msg}
	}

	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response from %s: %w", c.baseURL, err)
	}
	return nil
}
