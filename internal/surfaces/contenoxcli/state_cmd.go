package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/substrate"
	"github.com/contenox/contenox/libtracker"
	"github.com/spf13/cobra"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Inspect captured execution state from past chain runs.",
	Long: `Browse per-request execution captures persisted to KV by the engine's
inspector chain. Each chain run produces a request ID and a sequence of
captured step records (CapturedStateUnit) that survive process restart.

  contenox state list             # list request IDs that have captured state
  contenox state show <reqID>     # print step rows for a request
  contenox state show <reqID> --raw   # JSON dump of the captured units
  contenox state clear            # delete captured state; keep sessions`,
}

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List request IDs with captured execution state.",
	Long: `List every request ID for which the engine's inspector persisted execution
state to KV. Use a listed ID with 'contenox state show' to inspect its steps.
Prints "(no captured state)" when nothing has been recorded.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, _, err := openConfigDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := libtracker.WithNewRequestID(context.Background())
		kv, releaseKV, err := substrate.OpenKV(ctx, db)
		if err != nil {
			return err
		}
		defer releaseKV()
		inspector := taskengine.NewKVInspector(taskengine.NewSimpleInspector(), kv, libtracker.NoopTracker{})

		ids, err := inspector.GetStatefulRequests(ctx)
		if err != nil {
			return fmt.Errorf("list stateful requests: %w", err)
		}
		if len(ids) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "(no captured state)")
			return nil
		}
		for _, id := range ids {
			fmt.Fprintln(cmd.OutOrStdout(), id)
		}
		return nil
	},
}

var stateShowCmd = &cobra.Command{
	Use:   "show <reqID>",
	Short: "Print captured execution steps for a request.",
	Long: `Print the captured step records for one request as a table showing each task,
its handler, retry index, duration, transition, and status. Pass --raw to dump
the captured units as JSON instead. Use 'contenox state list' to find request
IDs.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reqID := args[0]
		raw, _ := cmd.Flags().GetBool("raw")

		db, _, err := openConfigDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := libtracker.WithNewRequestID(context.Background())
		kv, releaseKV, err := substrate.OpenKV(ctx, db)
		if err != nil {
			return err
		}
		defer releaseKV()
		inspector := taskengine.NewKVInspector(taskengine.NewSimpleInspector(), kv, libtracker.NoopTracker{})

		units, err := inspector.GetExecutionStateByRequestID(ctx, reqID)
		if err != nil {
			return fmt.Errorf("read state for %s: %w", reqID, err)
		}
		if len(units) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "(no captured state for %s)\n", reqID)
			return nil
		}

		if raw {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(units)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TASK\tHANDLER\tRETRY\tDURATION\tTRANSITION\tSTATUS")
		for _, u := range units {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
				u.TaskID, u.TaskHandler, u.RetryIndex, formatStateDuration(u.Duration), u.Transition, formatStateStatus(u))
		}
		return w.Flush()
	},
}

var stateClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete all captured execution state.",
	Long: `Delete the inspector's per-request execution traces. This does not delete
sessions, messages, checkpoints, approvals, configuration, or model data.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		db, _, err := openConfigDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := libtracker.WithNewRequestID(cmd.Context())
		kv, releaseKV, err := substrate.OpenKV(ctx, db)
		if err != nil {
			return err
		}
		defer releaseKV()
		inspector := taskengine.NewKVInspector(taskengine.NewSimpleInspector(), kv, libtracker.NoopTracker{})
		removed, err := inspector.ClearExecutionState(ctx)
		if err != nil {
			return fmt.Errorf("clear captured state: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Cleared captured execution state for %d requests. Sessions and checkpoints were not changed.\n", removed)
		return nil
	},
}

func formatStateDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	return d.String()
}

func formatStateStatus(u taskengine.CapturedStateUnit) string {
	switch {
	case u.TimedOut:
		return "TIMED-OUT"
	case u.Cancelled:
		return "CANCELLED"
	case u.Error.Error != "":
		return "ERROR: " + u.Error.Error
	default:
		return "OK"
	}
}

func init() {
	stateShowCmd.Flags().Bool("raw", false, "Print captured units as JSON instead of a table.")
	stateCmd.AddCommand(stateListCmd)
	stateCmd.AddCommand(stateShowCmd)
	stateCmd.AddCommand(stateClearCmd)
}
