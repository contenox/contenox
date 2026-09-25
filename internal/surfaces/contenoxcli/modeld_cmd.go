package contenoxcli

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/contenox/internal/modeldinstall"
	"github.com/contenox/contenox/internal/models/modeldprobe"
	"github.com/spf13/cobra"
)

var modeldCmd = &cobra.Command{
	Use:   "modeld",
	Short: "Install or inspect the local inference worker.",
}

var modeldInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download and verify a compatible native worker from the release index.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		backend, _ := cmd.Flags().GetString("backend")
		if backend != "llama" && backend != "openvino" {
			return fmt.Errorf("backend must be llama or openvino")
		}
		result, err := modeldinstall.EnsureInstalled(cmd.Context(), backend, modeldinstall.Options{
			BaseURL:       flagString(cmd, "base-url"),
			DataRoot:      flagString(cmd, "data-root"),
			ClientVersion: cliVersion(),
			Progress:      cmd.ErrOrStderr(),
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "modeld %s: %s\n", result.Version, result.LauncherPath)
		return nil
	},
}

var modeldStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Probe the local worker lease and inference endpoint.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		status := modeldprobe.New(flagString(cmd, "data-root")).Probe(cmd.Context())
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				State    string `json:"state"`
				Binary   string `json:"binary,omitempty"`
				Endpoint string `json:"endpoint,omitempty"`
				Backend  string `json:"backend,omitempty"`
			}{status.State.String(), status.Binary, status.Endpoint, status.Backend})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", status.State)
		if status.Binary != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Binary: %s\n", status.Binary)
		}
		if status.Endpoint != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Endpoint: %s\n", status.Endpoint)
		}
		return nil
	},
}

func init() {
	modeldCmd.PersistentFlags().String("data-root", modeldprobe.DefaultDataRoot(), "Worker data directory.")
	modeldInstallCmd.Flags().String("backend", "llama", "Required native backend: llama or openvino.")
	modeldInstallCmd.Flags().String("base-url", modeldinstall.DefaultBaseURL, "Release index base URL.")
	modeldStatusCmd.Flags().Bool("json", false, "Print status as JSON.")
	modeldCmd.AddCommand(modeldInstallCmd, modeldStatusCmd)
	reservedSubcommands["modeld"] = true
	rootCmd.AddCommand(modeldCmd)
}
