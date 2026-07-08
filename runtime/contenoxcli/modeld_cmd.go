package contenoxcli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var modeldCmd = &cobra.Command{
	Use:   "modeld",
	Short: "Manage the local modeld inference daemon.",
	Long: `Manage the local modeld inference daemon that serves GGUF (llama) and
OpenVINO models.

  contenox modeld install                       # download + verify the prebuilt daemon
  contenox modeld install --backend openvino    # require the openvino backend`,
}

var modeldInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download, verify, and install the prebuilt modeld daemon.",
	Long: `Resolve the newest protocol-compatible prebuilt modeld build for this
platform, download it, verify its checksum, and install it under
~/.contenox/modeld/. Non-interactive: suitable for scripts and installers.

The same install runs automatically when 'contenox setup' selects a local
provider; this command exposes it directly.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, _ := cmd.Flags().GetString("backend")
		if !isLocalModeldProvider(backend) {
			return fmt.Errorf("unsupported backend %q (supported: llama, openvino)", backend)
		}
		setupLocalModeld(cmd.OutOrStdout(), backend)
		return nil
	},
}

func init() {
	modeldInstallCmd.Flags().String("backend", "llama", "Inference backend the installed daemon must include: llama or openvino")
	modeldCmd.AddCommand(modeldInstallCmd)
}
