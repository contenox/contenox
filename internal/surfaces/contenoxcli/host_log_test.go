package contenoxcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/liblog"
	"github.com/spf13/cobra"
)

// beam must be reserved, or `contenox beam` with a typo'd flag would be injected
// as a chat prompt and spend a model call saying nothing.
func TestUnit_beamIsReservedSubcommand(t *testing.T) {
	if !reservedSubcommands["beam"] {
		t.Fatal(`"beam" must be reserved`)
	}
	if !firstNonFlagIsReserved([]string{"beam"}) {
		t.Fatal(`expected "beam" to be recognized as a reserved subcommand`)
	}
}

// The surface log must not depend on the telemetry toggle: a beam that is not
// opted into telemetry still needs somewhere to write its own diagnostics.
func TestUnit_SurfaceLogIsSeparateFromTelemetry(t *testing.T) {
	dir := t.TempDir()
	cmd := &cobra.Command{Use: "beam"}
	cmd.Flags().String("log-dir", dir, "")

	w, err := openHostLog(cmd, acpProfileBeam.name)
	if err != nil {
		t.Fatalf("openHostLog: %v", err)
	}
	defer w.Close()
	if _, err := os.Stat(w.Path()); err != nil {
		t.Fatalf("log file was not created: %v", err)
	}
	if filepath.Base(w.Path()) == "telemetry.log" {
		t.Fatalf("surface log collided with telemetry.log: %q", w.Path())
	}
	// Dated, so "what happened on Tuesday" is a question about filenames.
	if !strings.HasPrefix(filepath.Base(w.Path()), acpProfileBeam.name+"-") {
		t.Fatalf("surface log is not dated: %q", w.Path())
	}
}

// A surface boots its log before the database exists, then adopts the stored
// bounds. This pins that second step, which is the only reason config keys can
// bound a file that was already open.
func TestUnit_SurfaceStoredSettingsReachTheLiveLog(t *testing.T) {
	dir := t.TempDir()
	cmd := &cobra.Command{Use: "beam"}
	cmd.Flags().String("log-dir", dir, "")

	w, err := openHostLog(cmd, acpProfileBeam.name)
	if err != nil {
		t.Fatalf("openHostLog: %v", err)
	}
	defer w.Close()
	if w.MaxBytes() != liblog.DefaultMaxBytes {
		t.Fatalf("expected the boot default, got %d", w.MaxBytes())
	}

	// What logSettingsFromConfig would have produced for these stored values.
	w.Reconfigure(50<<20, 4, 7*24*time.Hour)
	if w.MaxBytes() != 50<<20 || w.MaxFiles() != 4 || w.MaxAge() != 7*24*time.Hour {
		t.Fatalf("stored settings did not reach the live log: %d/%d/%s", w.MaxBytes(), w.MaxFiles(), w.MaxAge())
	}
}

// `config set` is where a bad bound must be refused: at boot the only options
// are to ignore it or refuse to start, and by then nobody is watching.
func TestUnit_LogConfigIsValidatedAtSetTime(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"log-max-size", "enormous"},
		{"log-max-size", "-1MB"},
		{"log-max-files", "many"},
		{"log-max-files", "-2"},
		{"log-max-age-days", "3.5"},
	} {
		if _, err := normalizeLogConfig(tc.key, tc.value); err == nil {
			t.Fatalf("normalizeLogConfig(%q, %q) = nil error, want a refusal", tc.key, tc.value)
		}
	}
}

// A size is stored canonically so `config get` reads back what the log uses.
func TestUnit_LogSizeIsStoredCanonically(t *testing.T) {
	got, err := normalizeLogConfig("log-max-size", "  50 mb ")
	if err != nil {
		t.Fatalf("normalizeLogConfig: %v", err)
	}
	if got != "50MB" {
		t.Fatalf("stored %q, want the canonical %q", got, "50MB")
	}
}

// Keys this validator does not own must pass through untouched, or adding a
// log key would start rewriting unrelated config values.
func TestUnit_LogValidatorIgnoresOtherKeys(t *testing.T) {
	got, err := normalizeLogConfig("default-model", "qwen3:8b")
	if err != nil {
		t.Fatalf("unexpected error for an unrelated key: %v", err)
	}
	if got != "" {
		t.Fatalf("validator rewrote an unrelated key to %q", got)
	}
}
