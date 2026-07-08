package contenoxcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// vscodeExtensionID is the Marketplace identifier the launcher enables proposed
// APIs for. The proposed VS Code build (package-vscode-proposed) declares the
// chatSessionsProvider / chatParticipantPrivate / languageModelProxy proposals,
// which VS Code only activates when launched with --enable-proposed-api for this
// extension — that is what this launcher does.
const vscodeExtensionID = "contenox.contenox-runtime"

var codeCmd = &cobra.Command{
	Use:   "code [vscode args...]",
	Short: "Launch VS Code with Contenox's proposed API enabled.",
	Long: `Launch VS Code (or a compatible editor) with Contenox's proposed API
enabled, then forward every remaining argument straight through — it behaves like
an alias for the editor binary.

This lets the native Contenox agent-session chat (the proposed extension build)
run on stable VS Code without GitHub Copilot: proposed APIs are only active when
the editor is launched with --enable-proposed-api for this extension.

  contenox code .                 # open the current directory
  contenox code --new-window .    # every argument is forwarded verbatim

The editor binary defaults to "code"; set CONTENOX_CODE_BIN to use a compatible
editor (e.g. codium, code-insiders).
Use CONTENOX_NATIVE_AGENT_SESSIONS=0 to disable the native agent-session chat.
`,
	// Pass every argument (including flags) straight through to the editor.
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		bin := os.Getenv("CONTENOX_CODE_BIN")
		if bin == "" {
			bin = "code"
		}
		path, err := exec.LookPath(bin)
		if err != nil {
			return fmt.Errorf("editor %q not found in PATH; set CONTENOX_CODE_BIN to your VS Code binary: %w", bin, err)
		}

		editorArgs := append([]string{"--enable-proposed-api=" + vscodeExtensionID}, args...)
		editor := exec.Command(path, editorArgs...)
		editor.Env = append(os.Environ(), "CONTENOX_NATIVE_AGENT_SESSIONS=1")
		editor.Stdin, editor.Stdout, editor.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := editor.Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			return fmt.Errorf("launching %s: %w", bin, err)
		}
		return nil
	},
}
