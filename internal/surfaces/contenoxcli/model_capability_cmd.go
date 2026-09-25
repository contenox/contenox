package contenoxcli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/contenox/contenox/internal/models/modelcapability"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/spf13/cobra"
)

var modelCapabilityCmd = &cobra.Command{
	Use:   "capability",
	Short: "Manage manual provider/model capability overrides.",
	Long: `Manage what is stated about a provider/model pair that the provider's own
catalog does not report.

Two tiers, and the more specific one wins:

  --think, --vision     provider-level, applied to every backend of that type.
                        Use when all your OpenAI-compatible endpoints behave
                        alike.
  --backend <id|name>   backend-level facts: context window, output ceiling, the
                        capability set, and the upstream rate card. Use when one
                        backend differs — a reseller's own prices, or one
                        endpoint that gates a model differently.

Facts travel to the client on /api/show and to the meter that prices a turn.
They are folded into the model list when the runtime state next refreshes, so a
change takes effect on the following discovery cycle.

Examples:
  contenox model capability set openai gpt-5-mini --think true
  contenox model capability set openai gpt-5-mini --context 400k --max-output 128k
  contenox model capability set openai gpt-5-mini --capabilities completion,tools,vision
  contenox model capability set openai gpt-5-mini --input-price 1.25 --output-price 10
  contenox model capability set openai gpt-5-mini --context 1m --backend my-reseller
  contenox model capability show openai gpt-5-mini
  contenox model capability unset openai gpt-5-mini`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var modelCapabilitySetCmd = &cobra.Command{
	Use:   "set <provider> <model>",
	Short: "Set a manual capability override.",
	Long: `Set a manual provider/model override.

The --think flag records whether this provider/model supports reasoning request
controls, and --vision whether it accepts image input. Both are provider-level:
they apply to every backend of that type. A flag you omit leaves its setting as
it was.

--context, --max-output, --capabilities and the pricing flags are backend-level.
Pass --backend to write them to one backend, or let them fan out to every
backend of the given provider type. A capability list REPLACES the observed set
rather than adding to it, so --capabilities states the whole set.

Pricing is USD per million tokens and is what the monthly spend ceiling is
metered against: a model with no declared rate card costs nothing as far as the
meter is concerned.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, store, err := openConfigDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		provider, model := args[0], args[1]
		backendFlag, _ := cmd.Flags().GetString("backend")

		if err := applyCapabilityBooleans(ctx, store, cmd, provider, model, backendFlag); err != nil {
			return err
		}

		declared, err := modelFactsFromFlags(cmd)
		if err != nil {
			return err
		}
		if declared.IsZero() {
			if !cmd.Flags().Changed("think") && !cmd.Flags().Changed("vision") {
				return fmt.Errorf("provide at least one flag: --think, --vision, --context, --max-output, --capabilities or a pricing flag")
			}
			return nil
		}

		targets, err := declarationTargets(ctx, store, provider, backendFlag)
		if err != nil {
			return err
		}
		for _, backend := range targets {
			if err := store.SetLLMProviderModelFact(ctx, backend.ID, model, declared); err != nil {
				return fmt.Errorf("failed to set model facts for backend %s: %w", backend.Name, err)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Model facts set for %s on %s.\n", model, backendList(targets))
		return nil
	},
}

var modelCapabilityShowCmd = &cobra.Command{
	Use:   "show <provider> <model>",
	Short: "Show a manual capability override.",
	Long: `Print what is stated about a provider/model pair: the provider-level think and
vision settings, and the facts declared for each backend of that type.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, store, err := openConfigDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		provider, model := args[0], args[1]
		out := cmd.OutOrStdout()

		override, ok, err := modelcapability.New(store).Get(ctx, provider, model)
		if err != nil {
			return fmt.Errorf("failed to read capability override: %w", err)
		}
		if !ok || (override.CanThink == nil && override.CanVision == nil) {
			_, canonicalProvider, canonicalModel, keyErr := modelcapability.Key(provider, model)
			if keyErr != nil {
				return keyErr
			}
			fmt.Fprintf(out, "No provider-level capability override for %s/%s.\n", canonicalProvider, canonicalModel)
		} else {
			var parts []string
			if override.CanThink != nil {
				parts = append(parts, fmt.Sprintf("think=%t", *override.CanThink))
			}
			if override.CanVision != nil {
				parts = append(parts, fmt.Sprintf("vision=%t", *override.CanVision))
			}
			fmt.Fprintf(out, "Capability override for %s/%s: %s.\n", override.Provider, override.Model, strings.Join(parts, ", "))
		}

		backendFlag, _ := cmd.Flags().GetString("backend")
		targets, err := declarationTargets(ctx, store, provider, backendFlag)
		if err != nil {
			return err
		}
		for _, backend := range targets {
			declared, err := store.GetLLMProviderModelFacts(ctx, backend.ID)
			if err != nil {
				return fmt.Errorf("failed to read model facts for backend %s: %w", backend.Name, err)
			}
			facts, stated := declared[model]
			if !stated {
				fmt.Fprintf(out, "No model facts for %s on %s.\n", model, backend.Name)
				continue
			}
			fmt.Fprintf(out, "Model facts for %s on %s: %s.\n", model, backend.Name, describeModelFacts(facts))
		}
		return nil
	},
}

var modelCapabilityUnsetCmd = &cobra.Command{
	Use:   "unset <provider> <model>",
	Short: "Remove a manual capability override.",
	Long: `Remove what was stated about a provider/model pair, reverting to whatever the
provider catalog advertises.

The provider-level override and the facts of every backend of that type are
removed. With --backend, only that one backend's facts are removed and the
provider-level override is left alone.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, store, err := openConfigDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		provider, model := args[0], args[1]
		backendFlag, _ := cmd.Flags().GetString("backend")
		out := cmd.OutOrStdout()

		if backendFlag == "" {
			removed, err := modelcapability.New(store).Unset(ctx, provider, model)
			if err != nil {
				return fmt.Errorf("failed to remove capability override: %w", err)
			}
			_, canonicalProvider, _, keyErr := modelcapability.Key(provider, model)
			if keyErr != nil {
				return keyErr
			}
			if removed {
				fmt.Fprintf(out, "Capability override removed for %s/%s.\n", canonicalProvider, model)
			}
		}

		targets, err := declarationTargets(ctx, store, provider, backendFlag)
		if err != nil {
			return err
		}
		for _, backend := range targets {
			if err := store.DeleteLLMProviderModelFact(ctx, backend.ID, model); err != nil {
				return fmt.Errorf("failed to remove model facts for backend %s: %w", backend.Name, err)
			}
		}
		fmt.Fprintf(out, "Model facts removed for %s on %s.\n", model, backendList(targets))
		return nil
	},
}

func applyCapabilityBooleans(ctx context.Context, store runtimetypes.Store, cmd *cobra.Command, provider, model, backendFlag string) error {
	thinkRaw, _ := cmd.Flags().GetString("think")
	visionRaw, _ := cmd.Flags().GetString("vision")
	if strings.TrimSpace(thinkRaw) == "" && strings.TrimSpace(visionRaw) == "" {
		return nil
	}
	if backendFlag != "" {
		return fmt.Errorf("--think and --vision are provider-level and cannot be scoped to one backend; state the whole set with --capabilities")
	}
	svc := modelcapability.New(store)
	if strings.TrimSpace(thinkRaw) != "" {
		canThink, err := parseModelCapabilityBool("--think", thinkRaw)
		if err != nil {
			return err
		}
		if _, err := svc.SetThink(ctx, provider, model, canThink); err != nil {
			return fmt.Errorf("failed to set capability override: %w", err)
		}
	}
	if strings.TrimSpace(visionRaw) != "" {
		canVision, err := parseModelCapabilityBool("--vision", visionRaw)
		if err != nil {
			return err
		}
		if _, err := svc.SetVision(ctx, provider, model, canVision); err != nil {
			return fmt.Errorf("failed to set capability override: %w", err)
		}
	}
	return nil
}

func modelFactsFromFlags(cmd *cobra.Command) (runtimetypes.ModelFacts, error) {
	var facts runtimetypes.ModelFacts

	if raw, _ := cmd.Flags().GetString("context"); strings.TrimSpace(raw) != "" {
		size, err := parseContextSize(raw)
		if err != nil {
			return facts, fmt.Errorf("--context: %w", err)
		}
		facts.ContextLength = size
	}
	if raw, _ := cmd.Flags().GetString("max-output"); strings.TrimSpace(raw) != "" {
		size, err := parseContextSize(raw)
		if err != nil {
			return facts, fmt.Errorf("--max-output: %w", err)
		}
		facts.MaxOutputTokens = size
	}
	if raw, _ := cmd.Flags().GetString("capabilities"); strings.TrimSpace(raw) != "" {
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if !runtimetypes.ValidModelCapability(name) {
				return facts, fmt.Errorf("--capabilities: %q is not one of %s", name, strings.Join(runtimetypes.ModelCapabilities(), ", "))
			}
			facts.Capabilities = append(facts.Capabilities, name)
		}
	}

	pricing := &runtimetypes.ModelPricing{}
	priced := false
	for _, flag := range []struct {
		name  string
		field *float64
	}{
		{"input-price", &pricing.InputPerMillion},
		{"output-price", &pricing.OutputPerMillion},
		{"cache-read-price", &pricing.CacheReadPerMillion},
		{"cache-write-price", &pricing.CacheWritePerMillion},
		{"image-price", &pricing.PerImage},
		{"audio-price", &pricing.PerAudioMebibyte},
	} {
		raw, _ := cmd.Flags().GetString(flag.name)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || value < 0 {
			return facts, fmt.Errorf("--%s: %q is not a non-negative USD amount per million tokens", flag.name, raw)
		}
		*flag.field = value
		priced = true
	}
	if priced {
		facts.Pricing = pricing
	}
	if err := runtimetypes.ValidateModelFacts(facts); err != nil {
		return facts, err
	}
	return facts, nil
}

func declarationTargets(ctx context.Context, store runtimetypes.Store, provider, backendFlag string) ([]*runtimetypes.Backend, error) {
	backends, err := store.ListAllBackends(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list backends: %w", err)
	}
	canonical := modelrepo.CanonicalBackendType(provider)
	backendFlag = strings.TrimSpace(backendFlag)

	var targets []*runtimetypes.Backend
	for _, backend := range backends {
		if modelrepo.CanonicalBackendType(backend.Type) != canonical {
			continue
		}
		if backendFlag != "" && backend.ID != backendFlag && backend.Name != backendFlag {
			continue
		}
		targets = append(targets, backend)
	}
	if len(targets) == 0 {
		if backendFlag != "" {
			return nil, fmt.Errorf("no %s backend matches %q", canonical, backendFlag)
		}
		return nil, fmt.Errorf("no backend of type %q is configured", canonical)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	return targets, nil
}

func backendList(backends []*runtimetypes.Backend) string {
	names := make([]string, 0, len(backends))
	for _, backend := range backends {
		names = append(names, backend.Name)
	}
	return strings.Join(names, ", ")
}

func describeModelFacts(facts runtimetypes.ModelFacts) string {
	var parts []string
	if facts.ContextLength > 0 {
		parts = append(parts, fmt.Sprintf("context=%d", facts.ContextLength))
	}
	if facts.MaxOutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("max_output=%d", facts.MaxOutputTokens))
	}
	if len(facts.Capabilities) > 0 {
		parts = append(parts, "capabilities="+strings.Join(facts.Capabilities, ","))
	}
	if facts.Pricing != nil {
		parts = append(parts, fmt.Sprintf("prices=$%.4g/$%.4g/$%.4g/$%.4g per Mtok in/out/cache-read/cache-write",
			facts.Pricing.InputPerMillion, facts.Pricing.OutputPerMillion,
			facts.Pricing.CacheReadPerMillion, facts.Pricing.CacheWritePerMillion))
		if facts.Pricing.PerImage > 0 {
			parts = append(parts, fmt.Sprintf("$%.4g per image", facts.Pricing.PerImage))
		}
		if facts.Pricing.PerAudioMebibyte > 0 {
			parts = append(parts, fmt.Sprintf("$%.4g per MiB of audio", facts.Pricing.PerAudioMebibyte))
		}
	}
	return strings.Join(parts, ", ")
}

func parseModelCapabilityBool(flag, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", flag)
	}
}

func openModelCapabilityService(cmd *cobra.Command) (libdb.DBManager, modelcapability.Service, error) {
	db, store, err := openConfigDB(cmd)
	if err != nil {
		return nil, modelcapability.Service{}, err
	}
	return db, modelcapability.New(store), nil
}

func init() {
	modelCapabilitySetCmd.Flags().String("think", "", "Whether this provider/model supports thinking/reasoning controls (true or false).")
	modelCapabilitySetCmd.Flags().String("vision", "", "Whether this provider/model accepts image input (true or false).")
	modelCapabilitySetCmd.Flags().String("context", "", "Context window in tokens, bare int or shorthand (32k, 1m).")
	modelCapabilitySetCmd.Flags().String("max-output", "", "Upstream output ceiling in tokens, bare int or shorthand.")
	modelCapabilitySetCmd.Flags().String("capabilities", "", "The model's whole capability set, comma-separated: "+strings.Join(runtimetypes.ModelCapabilities(), ", ")+".")
	modelCapabilitySetCmd.Flags().String("input-price", "", "USD per million uncached input tokens.")
	modelCapabilitySetCmd.Flags().String("output-price", "", "USD per million output tokens.")
	modelCapabilitySetCmd.Flags().String("cache-read-price", "", "USD per million cached prompt tokens.")
	modelCapabilitySetCmd.Flags().String("cache-write-price", "", "USD per million tokens written to cache.")
	modelCapabilitySetCmd.Flags().String("image-price", "", "USD per image attachment, for a provider that bills images apart from the tokens it reports for them.")
	modelCapabilitySetCmd.Flags().String("audio-price", "", "USD per mebibyte of inline audio. At 128 kbps a mebibyte is about a minute, which is how a per-minute upstream rate is stated here.")
	modelCapabilitySetCmd.Flags().String("backend", "", "Backend id or name to write backend-level facts to; defaults to every backend of the provider type.")

	modelCapabilityShowCmd.Flags().String("backend", "", "Show facts for one backend id or name.")
	modelCapabilityUnsetCmd.Flags().String("backend", "", "Remove facts for one backend id or name, leaving the provider-level override alone.")

	modelCapabilityCmd.AddCommand(modelCapabilitySetCmd)
	modelCapabilityCmd.AddCommand(modelCapabilityShowCmd)
	modelCapabilityCmd.AddCommand(modelCapabilityUnsetCmd)
	modelCmd.AddCommand(modelCapabilityCmd)
}
