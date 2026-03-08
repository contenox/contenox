package contenoxcli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/contenox/contenox/internal/runtimestate"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtimetypes"
	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:     "model",
	Aliases: []string{"models"},
	Short:   "Manage LLM models (list live, add, remove).",
	Long: `Manage models available to LLM backends.

By default, 'model list' queries each registered backend in real-time and
shows the models it is currently serving. Use --declared to see only what
is recorded in the local database.

Examples:
  contenox model list
  contenox model list --declared
  contenox model add qwen2.5:7b
  contenox model remove qwen2.5:7b`,
}

var modelListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List models available from live backends (or --declared for DB view).",
	Long: `Query each registered backend in real time and show its available models.

Shows model name, backend it comes from, and capabilities discovered at runtime
(chat, embed, prompt, stream, context length).

Use --declared to show the models recorded in the local SQLite database instead
of performing live backend queries.

Examples:
  contenox model list
  contenox model list --declared`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		declared, _ := cmd.Flags().GetBool("declared")
		ctx := libtracker.WithNewRequestID(context.Background())
		db, _, err := openBackendDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		if declared {
			return printDeclaredModels(ctx, db)
		}
		return printLiveModels(ctx, db)
	},
}

// printLiveModels runs one backend reconciliation cycle and prints what each
// backend is actually serving right now.
func printLiveModels(ctx context.Context, db libdb.DBManager) error {
	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()

	state, err := runtimestate.New(ctx, db, bus, runtimestate.WithSkipDeleteUndeclaredModels())
	if err != nil {
		return fmt.Errorf("failed to initialize runtime state: %w", err)
	}

	// A single cycle contacts every backend and populates PulledModels.
	if err := state.RunBackendCycle(ctx); err != nil {
		// Non-fatal: partial results are still useful.
		fmt.Fprintf(os.Stderr, "warning: backend cycle error: %v\n", err)
	}

	rt := state.Get(ctx)
	if len(rt) == 0 {
		fmt.Println("No backends registered. Run: contenox backend add <name> --type <type>")
		return nil
	}

	// Stable sort by backend name.
	type entry struct {
		backendName string
		backendErr  string
		pulled      []string
		canChat     map[string]bool
		canEmbed    map[string]bool
		canPrompt   map[string]bool
		ctx         map[string]int
	}
	var entries []entry
	for _, bs := range rt {
		e := entry{
			backendName: bs.Name,
			backendErr:  bs.Error,
			canChat:     map[string]bool{},
			canEmbed:    map[string]bool{},
			canPrompt:   map[string]bool{},
			ctx:         map[string]int{},
		}
		for _, pm := range bs.PulledModels {
			e.pulled = append(e.pulled, pm.Model)
			e.canChat[pm.Model] = pm.CanChat
			e.canEmbed[pm.Model] = pm.CanEmbed
			e.canPrompt[pm.Model] = pm.CanPrompt
			e.ctx[pm.Model] = pm.ContextLength
		}
		sort.Strings(e.pulled)
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].backendName < entries[j].backendName })

	any := false
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BACKEND\tMODEL\tCHAT\tEMBED\tPROMPT\tCTX")
	for _, e := range entries {
		if e.backendErr != "" {
			fmt.Fprintf(w, "%s\t(unreachable: %s)\t\t\t\t\n", e.backendName, e.backendErr)
			continue
		}
		if len(e.pulled) == 0 {
			fmt.Fprintf(w, "%s\t(no models)\t\t\t\t\n", e.backendName)
			continue
		}
		for _, m := range e.pulled {
			any = true
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n",
				e.backendName, m,
				boolMark(e.canChat[m]),
				boolMark(e.canEmbed[m]),
				boolMark(e.canPrompt[m]),
				e.ctx[m],
			)
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if !any {
		fmt.Println("\nNo models found. Add a model with: contenox model add <model-name>")
	}
	return nil
}

// printDeclaredModels lists the models stored in the local SQLite database.
func printDeclaredModels(ctx context.Context, db libdb.DBManager) error {
	models, err := runtimetypes.New(db.WithoutTransaction()).ListAllModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}
	if len(models) == 0 {
		fmt.Println("No models declared. Run: contenox model add <model-name>")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tCHAT\tEMBED\tPROMPT\tCTX")
	for _, m := range models {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n",
			m.Model,
			boolMark(m.CanChat),
			boolMark(m.CanEmbed),
			boolMark(m.CanPrompt),
			m.ContextLength,
		)
	}
	return w.Flush()
}

var modelAddCmd = &cobra.Command{
	Use:   "add <model-name>",
	Short: "Declare a model for use by LLM backends.",
	Long: `Register a model name in the local database.

For Ollama backends, this also triggers download if the model is not yet pulled.
For OpenAI/Gemini/vLLM, the model name is validated against the backend at runtime.

Examples:
  contenox model add qwen2.5:7b
  contenox model add gemini-2.0-flash
  contenox model add gpt-4o`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, _, err := openBackendDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		modelName := args[0]
		store := runtimetypes.New(db.WithoutTransaction())
		existing, _ := store.GetModelByName(ctx, modelName)
		if existing != nil {
			fmt.Printf("Model %q is already declared.\n", modelName)
			return nil
		}
		if err := store.AppendModel(ctx, &runtimetypes.Model{Model: modelName}); err != nil {
			return fmt.Errorf("failed to add model: %w", err)
		}
		fmt.Printf("Model %q declared.\n", modelName)
		return nil
	},
}

var modelRemoveCmd = &cobra.Command{
	Use:     "remove <model-name>",
	Aliases: []string{"rm"},
	Short:   "Remove a declared model.",
	Long: `Unregister a model from the local database.

For Ollama-backed models this does not delete the model from Ollama itself,
only removes the declaration from Contenox.

Example:
  contenox model remove qwen2.5:7b`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, _, err := openBackendDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		modelName := args[0]
		if err := runtimetypes.New(db.WithoutTransaction()).DeleteModel(ctx, modelName); err != nil {
			return fmt.Errorf("failed to remove model %q: %w", modelName, err)
		}
		fmt.Printf("Model %q removed.\n", modelName)
		return nil
	},
}

func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "-"
}

func init() {
	modelListCmd.Flags().Bool("declared", false, "Show models recorded in the local database instead of querying live backends")
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelAddCmd)
	modelCmd.AddCommand(modelRemoveCmd)
}
