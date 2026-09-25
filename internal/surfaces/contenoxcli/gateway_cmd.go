package contenoxcli

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/contenox/contenox/internal/models/modelruntime"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/contenox/contenox/internal/trouble"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/contenox/contenox/libtracker"
	"github.com/spf13/cobra"
)

const defaultGatewayListen = "127.0.0.1:11435"

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Serve the model gateway clients point their runtime at.",
	Long: `Serve a model gateway: an Ollama-compatible HTTP API in front of the backends
this runtime already knows, metered per caller.

Every caller presents a license token as its bearer. The gateway verifies the
signature against your licensing authority, decrypts the claims, and — when a
token signing key is configured — checks the presented token against the key
ledger, so a key you minted here is the only kind that works, and revoking it
takes effect on the next request. Allowances in the claims are what a turn is
metered against: per model, a weekly output and input ceiling, an optional
five-hour burst window and an optional monthly spend ceiling.

Clients point at it the way they point at Ollama:
  contenox backend add gateway --type ollama --url http://127.0.0.1:11435

The authority is one of:
  --authority-key-file       your authority's SSH or PEM PUBLIC key, plus the
                             payload key the licenses were encrypted with
  --authority-private-key-file
                             the authority's own private key, which carries both
  (neither)                  the pinned Contenox authority compiled into an
                             official release build`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var gatewayServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the gateway until interrupted.",
	Long: `Serve the gateway until interrupted.

The key ledger is what makes a minted key authoritative and revocable. It is
read from the deployment's token signing key, ` + libtokenkey.EnvSigningKeyFile + `
or ` + libtokenkey.EnvSigningKey + `. Without one the gateway still verifies licenses,
but accepts any token your authority signed and can revoke nothing — which is
reported at startup rather than left to be discovered.`,
	Args: cobra.NoArgs,
	RunE: runGatewayServe,
}

type gatewayServeConfig struct {
	listen               string
	authorityKey         string
	authorityKeyFile     string
	privateKeyFile       string
	privateKeyPassphrase string
	payloadKey           string
	payloadKeyFile       string
	globalWeeklyTokens   int64
}

// meterLeasePath is where a deployment elects its one meter writer. It sits in
// the contenox data directory, so instances sharing a database and a home share
// the election; an instance that cannot see the file records nothing rather than
// adding a second charge to a turn the writer already counted.
func meterLeasePath(cmd *cobra.Command, contenoxDir string) string {
	if path := strings.TrimSpace(flagString(cmd, "meter-lease")); path != "" {
		return path
	}
	if strings.TrimSpace(contenoxDir) == "" {
		return ""
	}
	return filepath.Join(contenoxDir, "meter-writer.lease")
}

func runGatewayServe(cmd *cobra.Command, _ []string) error {
	cfg := gatewayServeConfig{
		listen:               flagString(cmd, "listen"),
		authorityKey:         flagString(cmd, "authority-key"),
		authorityKeyFile:     flagString(cmd, "authority-key-file"),
		privateKeyFile:       flagString(cmd, "authority-private-key-file"),
		privateKeyPassphrase: flagString(cmd, "authority-passphrase"),
		payloadKey:           flagString(cmd, "payload-key"),
		payloadKeyFile:       flagString(cmd, "payload-key-file"),
		globalWeeklyTokens:   flagInt64(cmd, "global-weekly-tokens"),
	}

	verifier, err := buildGatewayVerifier(cfg)
	if err != nil {
		return err
	}
	hasher, err := gatewayHasher()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx = libtracker.WithNewRequestID(ctx)
	contenoxDir, err := ResolveContenoxDir(cmd)
	if err != nil {
		return err
	}
	leasePath := flagString(cmd, "lease-file")
	if leasePath == "" {
		leasePath = filepath.Join(contenoxDir, "gateway.lease")
	}
	leaseTTL, _ := cmd.Flags().GetDuration("lease-ttl")
	ownership, err := acquireGatewayOwnership(ctx, leasePath, leaseTTL)
	if err != nil {
		return err
	}
	defer ownership.release()
	stopOnLoss := context.AfterFunc(ownership.ctx, stop)
	defer stopOnLoss()

	db, store, err := openConfigDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	opts, err := buildRunOpts(cmd, db, contenoxDir)
	if err != nil {
		return err
	}
	engine, err := modelruntime.Build(ctx, db, modelruntime.Config{
		DefaultModel: opts.EffectiveDefaultModel, DefaultProvider: opts.EffectiveDefaultProvider,
		ContextLength: opts.EffectiveContext, NoDeleteModels: opts.EffectiveNoDeleteModels,
		SkipBackendCycle: opts.EffectiveSkipBackendCycle, Tracing: opts.EffectiveTracing,
		Tracker: opts.EffectiveTracker,
	})
	if err != nil {
		return fmt.Errorf("failed to build the model runtime: %w", err)
	}
	defer engine.Stop()

	svc, err := gateway.New(gateway.Config{
		DB:                 db,
		Verifier:           verifier,
		Hasher:             hasher,
		Models:             engine.Models,
		Runtime:            engine.State,
		Bus:                engine.Bus,
		Tracker:            engine.Tracker,
		Trouble:            trouble.NewRecorder(db, engine.Tracker, nil),
		GlobalWeeklyTokens: cfg.globalWeeklyTokens,
		MeterLeasePath:     meterLeasePath(cmd, contenoxDir),
	})
	if err != nil {
		return fmt.Errorf("failed to build the gateway: %w", err)
	}

	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	svc.AddOpenAIProxyRoutes(mux)
	server := &http.Server{Addr: cfg.listen, Handler: ownership.guard(mux), BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 10 * time.Second}
	defer server.Close()

	go func() {
		if err := svc.SubscribeControlPlane(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(errOut, "warning: control plane subscription ended: %v\n", err)
		}
	}()
	go func() {
		if err := svc.StartUsageConsumer(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(errOut, "warning: usage consumer ended: %v\n", err)
		}
	}()
	go refreshModels(ctx, engine.State, errOut)

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	printGatewayStartup(out, cfg.listen, store, hasher != nil)

	select {
	case err := <-errCh:
		return fmt.Errorf("gateway listener failed: %w", err)
	case <-ctx.Done():
	}
	if ownership.ctx.Err() != nil {
		_ = server.Close()
		return context.Cause(ownership.ctx)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	fmt.Fprintln(out, "Gateway stopped.")
	return nil
}

func printGatewayStartup(out io.Writer, listen string, store runtimetypes.Store, ledger bool) {
	fmt.Fprintf(out, "Gateway listening on http://%s\n", listen)
	if !ledger {
		fmt.Fprintf(out, "warning: no token signing key configured (%s or %s), so no key ledger is\n"+
			"enforced: every token the authority signed is accepted and none can be revoked.\n",
			libtokenkey.EnvSigningKeyFile, libtokenkey.EnvSigningKey)
		return
	}
	active, err := store.CountActiveProxyKeys(context.Background(), time.Now().UTC())
	if err != nil {
		return
	}
	fmt.Fprintf(out, "Key ledger enforced: %d active key(s).\n", active)
}

func refreshModels(ctx context.Context, state *runtimestate.State, errOut io.Writer) {
	ticker := time.NewTicker(runtimestate.ReconcileDebounceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := state.ReconcileIfStale(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintf(errOut, "warning: model refresh failed: %v\n", err)
			}
		}
	}
}

// gatewayHasher reads the deployment's token signing key. An absent key is a
// finished deployment without a ledger, not an error; a key that is present and
// unusable is, because that deployment would reject every token it ever minted.
func gatewayHasher() (*libtokenkey.Hasher, error) {
	hasher, err := libtokenkey.FromEnv()
	if errors.Is(err, libtokenkey.ErrNoSigningKey) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("token signing key: %w", err)
	}
	return hasher, nil
}

// buildGatewayVerifier resolves the licensing authority a presented token is
// checked against: its private key when the operator holds it, otherwise its
// public key together with the payload key the licenses were encrypted with.
func buildGatewayVerifier(cfg gatewayServeConfig) (gateway.ClaimsVerifier, error) {
	if strings.TrimSpace(cfg.privateKeyFile) != "" {
		pem, err := os.ReadFile(cfg.privateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("--authority-private-key-file: %w", err)
		}
		passphrase := []byte(cfg.privateKeyPassphrase)
		verifier, err := liblicense.NewVerifierFromSSHPrivateKey(pem, passphrase)
		if err != nil {
			return nil, fmt.Errorf("--authority-private-key-file: %w", err)
		}
		return verifier, nil
	}

	authority, err := readKeyMaterial(cfg.authorityKey, cfg.authorityKeyFile, "--authority-key")
	if err != nil {
		return nil, err
	}
	payload, err := readKeyMaterial(cfg.payloadKey, cfg.payloadKeyFile, "--payload-key")
	if err != nil {
		return nil, err
	}

	if authority == "" {
		if payload == "" {
			pinned, err := liblicense.NewPinnedVerifier()
			if err != nil {
				return nil, fmt.Errorf("no licensing authority configured: pass --authority-private-key-file, or --authority-key-file with --payload-key-file: %w", err)
			}
			return pinned, nil
		}
		pinned, err := liblicense.NewPinnedVerifier(liblicense.WithPayloadDecryptionKey(payloadBytes(payload)))
		if err != nil {
			return nil, fmt.Errorf("no licensing authority configured: pass --authority-private-key-file, or --authority-key-file with --payload-key-file: %w", err)
		}
		return pinned, nil
	}
	if payload == "" {
		return nil, fmt.Errorf("--authority-key needs the payload key the licenses were encrypted with: pass --payload-key or --payload-key-file")
	}
	verifier, err := liblicense.NewVerifierFromSSHPublicKey([]byte(authority),
		liblicense.WithPayloadDecryptionKey(payloadBytes(payload)))
	if err != nil {
		return nil, fmt.Errorf("--authority-key: %w", err)
	}
	return verifier, nil
}

// payloadBytes reads a 32-byte payload key written as hex, base64 or raw bytes.
func payloadBytes(value string) []byte {
	value = strings.TrimSpace(value)
	if decoded, err := hex.DecodeString(value); err == nil {
		return decoded
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded
		}
	}
	return []byte(value)
}

func readKeyMaterial(value, path, flag string) (string, error) {
	if strings.TrimSpace(path) != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%s-file: %w", flag, err)
		}
		return strings.TrimSpace(string(raw)), nil
	}
	return strings.TrimSpace(value), nil
}

func flagString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}

func flagInt64(cmd *cobra.Command, name string) int64 {
	value, _ := cmd.Flags().GetInt64(name)
	return value
}

func init() {
	gatewayServeCmd.Flags().String("listen", defaultGatewayListen, "Address to serve the Ollama-compatible API on.")
	gatewayServeCmd.Flags().Int64("global-weekly-tokens", 0, "Operator cap on tokens proxied per model in a trailing week, across every key. Zero disables it.")
	gatewayServeCmd.Flags().String("meter-lease", "", "Path of the meter-writer lease shared by every gateway over one database (default <data-dir>/meter-writer.lease).")
	gatewayServeCmd.Flags().String("lease-file", "", "Server ownership lease (default <data-dir>/gateway.lease). Mount its parent directory shared by all contenders.")
	gatewayServeCmd.Flags().Duration("lease-ttl", 30*time.Second, "Server ownership lease lifetime; renewal failure stops serving.")
	gatewayServeCmd.Flags().String("authority-key", "", "The licensing authority's SSH or PEM public key.")
	gatewayServeCmd.Flags().String("authority-key-file", "", "Path to the authority's public key.")
	gatewayServeCmd.Flags().String("authority-private-key-file", "", "Path to the authority's SSH private key, which carries both the public key and the payload decryption key.")
	gatewayServeCmd.Flags().String("authority-passphrase", "", "Passphrase for --authority-private-key-file, when it is encrypted.")
	gatewayServeCmd.Flags().String("payload-key", "", "The 32-byte payload key the licenses were encrypted with, in hex or base64.")
	gatewayServeCmd.Flags().String("payload-key-file", "", "Path to the payload key.")

	gatewayCmd.AddCommand(gatewayServeCmd)
	rootCmd.AddCommand(gatewayCmd)
}
