package contenoxcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/contenox/contenox/internal/modeld/modelstore"
	"github.com/contenox/contenox/internal/models/modeldprobe"
	"github.com/contenox/contenox/internal/models/modelregistry"
	"github.com/contenox/contenox/internal/models/modelrepo/modeldconn"
	"github.com/contenox/contenox/internal/transport"
	"github.com/spf13/cobra"
)

var modelPullCmd = &cobra.Command{
	Use:   "pull <name>",
	Short: "Download a registry model for native modeld inference.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := modelregistry.New(nil)
		desc, err := reg.Resolve(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("%w; use 'contenox model registry-list' to list downloadable models", err)
		}
		if err := modelstore.EnsureModelAvailable(cmd.Context(), reg, desc.Name, modelstore.AutoPullOptions{
			DataRoot: modeldprobe.DefaultDataRoot(), ProgressOut: cmd.OutOrStdout(),
		}); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Model %s available in %s\n", desc.Name, modelstore.Dir(modeldprobe.DefaultDataRoot(), ""))
		return nil
	},
}

var modelRegistryListCmd = &cobra.Command{
	Use:   "registry-list",
	Short: "List downloadable native models.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		models, err := modelregistry.New(nil).List(cmd.Context())
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tENGINE\tBYTES\tDESCRIPTION")
		for _, m := range models {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", m.Name, m.BackendType(), m.SizeBytes, m.Label())
		}
		return w.Flush()
	},
}

func promptModeldModel(out io.Writer, scanner *bufio.Scanner, defaultModel string) string {
	root := modeldprobe.DefaultDataRoot()
	models, err := modelstore.NewAdmin(modelstore.Dir(root, "")).ListModels(context.Background())
	if err == nil && len(models) > 0 {
		names := make([]string, 0, len(models))
		for _, m := range models {
			names = append(names, m.Name)
		}
		return promptOllamaModelMenu(out, scanner, names, preselectOllamaModel(names, defaultModel))
	}
	fmt.Fprintf(out, "  No native models found in %s.\n", modelstore.Dir(root, ""))
	fmt.Fprintf(out, "  Download one with: contenox model pull %s\n", defaultModel)
	return promptLine(out, scanner, fmt.Sprintf("  Model [%s]", defaultModel), defaultModel)
}

func init() {
	modelCmd.AddCommand(modelPullCmd, modelRegistryListCmd, newNativeShowCmd(), newNativePSCmd(), newNativeStopCmd())
	modelListCmd.Flags().Bool("local", false, "List installed native models without contacting a worker.")
	for _, cmd := range []*cobra.Command{
		{Use: modelPullCmd.Use, Short: modelPullCmd.Short, Args: modelPullCmd.Args, RunE: modelPullCmd.RunE},
		newNativeListCmd(), newNativeShowCmd(), newNativePSCmd(), newNativeStopCmd(),
	} {
		reservedSubcommands[cmd.Name()] = true
		for _, alias := range cmd.Aliases {
			reservedSubcommands[alias] = true
		}
		rootCmd.AddCommand(cmd)
	}
}

func newNativeListCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List installed native models without starting a worker.", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return printNativeModels(cmd) }}
	cmd.Flags().Bool("json", false, "Print installed model metadata as JSON.")
	return cmd
}

func printNativeModels(cmd *cobra.Command) error {
	models, err := modelstore.NewAdmin(modelstore.Dir(modeldprobe.DefaultDataRoot(), "")).ListModels(cmd.Context())
	if err != nil {
		return err
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	if models == nil {
		models = []modelstore.NodeModel{}
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(models)
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tENGINE\tBYTES\tMODEL CONTEXT")
	for _, m := range models {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", m.Name, m.Type, m.SizeBytes, m.ContextLength)
	}
	return w.Flush()
}

func newNativeShowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "show <model>", Short: "Show a native model's worker-resolved capabilities and capacity.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := modelstore.NewAdmin(modelstore.Dir(modeldprobe.DefaultDataRoot(), "")).ListModels(cmd.Context())
			if err != nil {
				return err
			}
			ref := modeldconn.ModelRef{Name: args[0]}
			for _, model := range models {
				if model.Name == ref.Name {
					ref.Type, ref.Digest = model.Type, model.Digest
					break
				}
			}
			if ref.Type == "" {
				return fmt.Errorf("native model %q is not installed; use 'contenox pull <name>'", args[0])
			}
			info, err := modeldconn.Describe(cmd.Context(), ref, transport.Config{})
			if err != nil {
				return err
			}
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintf(w, "Model\t%s\nRuntime\t%s\nDevice\t%s %s\nModel context\t%d\nEffective context\t%d\nTools\t%t\nThinking\t%t\nVision\t%t\n", args[0], info.RuntimeName, info.DeviceKind, info.DeviceID, info.ModelMaxContext, info.EffectiveContext, info.ChatTemplateSupportsToolCalls, info.ChatTemplateSupportsThinking, info.SupportsVision)
			if info.Reason != "" {
				fmt.Fprintf(w, "Capacity reason\t%s\n", info.Reason)
			}
			return w.Flush()
		}}
	cmd.Flags().Bool("json", false, "Print the worker's full model report as JSON.")
	return cmd
}

func newNativePSCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ps", Short: "Show the local worker's resident model without starting it.", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := modeldconn.Status(cmd.Context())
			if errors.Is(err, modeldprobe.ErrNotRunning) || errors.Is(err, modeldprobe.ErrNotInstalled) {
				status = transport.DaemonStatus{}
			} else if err != nil {
				return err
			}
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
			}
			if status.Active == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "No resident native model.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "MODEL\tENGINE\tSTATE\tGENERATION")
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", status.Active.ModelName, status.Backend, status.State, status.Active.Generation)
			return w.Flush()
		}}
	cmd.Flags().Bool("json", false, "Print the worker's resident slot status as JSON.")
	return cmd
}

func newNativeStopCmd() *cobra.Command {
	return &cobra.Command{Use: "stop <model>", Short: "Unload the named resident native model; keep the worker running.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := modeldconn.StopModel(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unloaded %s\n", args[0])
			return nil
		}}
}
