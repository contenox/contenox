package contenoxcli

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/spf13/cobra"
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show locally metered harness usage by model.",
	RunE:  runLocalUsage,
}

func runLocalUsage(cmd *cobra.Command, _ []string) error {
	db, _, err := openConfigDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()
	scope, scopeID := runtimetypes.UsageScopeClient, gateway.LocalClientID
	if session := flagString(cmd, "session"); session != "" {
		scope, scopeID = runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(gateway.LocalClientID, session)
	}
	totals, err := runtimetypes.NewUsageStore(db).UsageByScope(cmd.Context(), scope, scopeID)
	if err != nil {
		return err
	}
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(totals)
	}
	if len(totals) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing metered yet.")
		return nil
	}
	for _, total := range totals {
		fmt.Fprintf(cmd.OutOrStdout(), "%-32s %s\n", total.Model, describeSnapshot(total.Snapshot))
	}
	return nil
}

func init() {
	usageCmd.Flags().String("session", "", "Limit usage to this session ID.")
	usageCmd.Flags().Bool("json", false, "Print usage as JSON.")
	rootCmd.AddCommand(usageCmd)
}
