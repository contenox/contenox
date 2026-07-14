package contenoxcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/internal/providerapi"
	"github.com/contenox/runtime/runtime/internal/setupapi"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	internalweb "github.com/contenox/runtime/runtime/internal/web"
	"github.com/contenox/runtime/runtime/providerservice"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/version"
)

// envSetupWebAddr overrides the loopback address the setup-web server binds.
const envSetupWebAddr = "CONTENOX_SETUP_WEB_ADDR"

const defaultSetupWebAddr = "127.0.0.1:32124"

// runSetupWeb is the browser sibling of the terminal setup wizard: it serves
// the embedded Beam UI with just enough API for onboarding (setup status,
// CLI config, backends, providers, runtime state) — no engine required — and
// returns once setup is complete, so an ACP client that launched it via the
// "browser" auth method can reconnect to a configured agent. Beam's wizard
// writes into the same global config/DB that `contenox acp` reads.
func runSetupWeb(ctx context.Context, out io.Writer, openBrowser bool) error {
	dbPath, err := globalDBPath()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}
	db, err := OpenDBAt(libtracker.WithNewRequestID(ctx), dbPath)
	if err != nil {
		return fmt.Errorf("open database %q: %w", dbPath, err)
	}
	defer db.Close()

	contenoxDir, err := globalContenoxDir()
	if err != nil {
		return fmt.Errorf("resolve contenox dir: %w", err)
	}
	workspaceID := ResolveWorkspaceID(contenoxDir)

	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()
	state, err := runtimestate.New(ctx, db, bus)
	if err != nil {
		return fmt.Errorf("runtime state: %w", err)
	}

	stateSvc := stateservice.New(state, db, workspaceID)
	backendSvc := backendservice.New(db)
	providerSvc := providerservice.New(db, workspaceID)

	apiMux := http.NewServeMux()
	setupapi.AddSetupRoutes(apiMux, stateSvc, nil)
	backendapi.AddStateRoutes(apiMux, stateSvc)
	backendapi.AddBackendRoutes(apiMux, backendSvc, stateSvc)
	providerapi.AddProviderRoutes(apiMux, providerSvc)

	rootMux := http.NewServeMux()
	serverapi.AddHealthRoutes(rootMux)
	serverapi.AddVersionRoutes(rootMux, version.Get(), uuid.NewString()[:8], "local")
	rootMux.Handle("/api/", http.StripPrefix("/api", apiMux))
	rootMux.Handle("/", internalweb.SPAHandler())

	addr := strings.TrimSpace(os.Getenv(envSetupWebAddr))
	if addr == "" {
		addr = defaultSetupWebAddr
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           rootMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	url := "http://" + addr
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "  Contenox setup is running in your browser: %s\n", url)
	fmt.Fprintln(out, "  Complete the wizard there; this command exits when setup is done (Ctrl-C to abort).")
	fmt.Fprintln(out, "")
	if openBrowser {
		openInBrowser(url)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return fmt.Errorf("setup web server: %w", err)
		case <-ticker.C:
			// Refresh (not just read) so backend reachability stays current
			// even when the user doesn't press the wizard's refresh button.
			res, err := stateSvc.Refresh(ctx)
			if err != nil {
				continue
			}
			if setupWebComplete(res) {
				fmt.Fprintln(out, "  ✓ Setup complete — you can close the browser tab and reconnect your editor.")
				// A beat for the wizard's final status poll before teardown.
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
}

// setupWebComplete mirrors Beam's own onboarding gate (Layout.tsx): defaults
// configured, at least one reachable backend, and no error-severity issues.
func setupWebComplete(res setupcheck.Result) bool {
	if strings.TrimSpace(res.DefaultModel) == "" {
		return false
	}
	if res.ReachableBackendCount <= 0 {
		return false
	}
	for _, issue := range res.Issues {
		if issue.Severity == "error" {
			return false
		}
	}
	return true
}

// openInBrowser is best effort: setup-web always prints the URL, so a failed
// launcher only costs the user a copy-paste.
func openInBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
