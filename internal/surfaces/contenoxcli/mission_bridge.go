package contenoxcli

import (
	"os"

	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/spf13/cobra"
)

func init() {
	missionCmd.AddCommand(&cobra.Command{
		Use:    "bridge <socket>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return missiontools.ProxyMCP(cmd.Context(), args[0], os.Getenv(missiontools.BridgeTokenEnv), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	})
}
