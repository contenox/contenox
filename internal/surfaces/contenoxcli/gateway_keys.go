package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/substrate"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/contenox/contenox/libtracker"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	defaultKeyTTL  = 720 * time.Hour
	digestPrefix   = 12
	keyPageSize    = 200
	revocationNote = "operator revocation"
)

var gatewayKeyCmd = &cobra.Command{
	Use:   "key",
	Short: "Mint, list and revoke the keys clients present.",
	Long: `Mint, list and revoke the keys clients present to the gateway.

A key is a license token: the allowances a caller is metered against live in its
claims, and the ledger records only the token's digest, so the token itself
exists nowhere on this machine after it is printed. Minting therefore prints the
token exactly once — store it where the caller can read it, because a lost token
cannot be recovered, only replaced.

These commands work on the same database the gateway serves from, so pass the
same --db the gateway runs with. Minting also needs the deployment's token
signing key, so that the key it prints is one the gateway can verify.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var gatewayKeyCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Mint a key and record it in the ledger.",
	Long: `Mint a license for one client and record its digest.

Allowances are per model and apply to every model named by --models, so a key
that may use several models states each ceiling for each of them. Sizes accept
the k/m shorthand (400k, 5m); --monthly-budget is in USD.

The token goes to stdout and the summary to stderr, so this captures the token
alone:

  contenox gateway key create --client laptop-alex --models qwen3:8b > alex.key

The client id is the durable identity the meter is keyed on. Re-minting for the
same client continues that client's metering rather than resetting it, which is
what makes replacing a lost key safe.

--cache-discount states a cached prompt token's share of the input ceilings: 0.25
means a quarter of it counts, which is what a plan that reuses context heavily is
usually worth. Left unstated, a cached token counts at a tenth.
--thinking-discount states a thinking token's share of the output and burst
ceilings; left unstated, a thinking token counts in full.

--image-allowance bounds image attachments rather than the tokens they cost,
because the count is what the gateway can take for itself; what an image costs is
whatever rate card the operator states for the model. --audio-allowance bounds
inline audio the same way, in mebibytes: the wire carries audio as bytes and no
duration, so bytes are the largest unit that can be counted exactly.

--claims-file takes a JSON object of raw claims, for anything the flags do not
cover: a default model, a plan's feature flags.`,
	Args: cobra.NoArgs,
	RunE: runGatewayKeyCreate,
}

var gatewayKeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List minted keys, newest first.",
	Long: `List the ledger: every key minted for this deployment, newest first, with the
digest that revoke takes. Only digests are stored, so a key can never be shown as
a token again — replacing one is the only recovery.`,
	Args: cobra.NoArgs,
	RunE: runGatewayKeyList,
}

var gatewayKeyRevokeCmd = &cobra.Command{
	Use:   "revoke [key-digest]",
	Short: "Revoke a key so the gateway refuses it.",
	Long: `Mark minted keys revoked, which the gateway refuses from the next request on.

Name the key by the digest "key list" prints — the short form is enough, as
long as it names only one key — or cut off one client with
--client, which revokes every key that client still holds unrevoked.

Revocation is durable: it is the ledger row, so every gateway reading this
database refuses the key whether or not it is running. Add --broadcast to also
publish the cutoff on the message bus, which reaches a gateway that keeps its own
database — publishing by digest, never by client, so a broadcast revocation cuts
off exactly the keys you revoked and nothing else.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runGatewayKeyRevoke,
}

var gatewayUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show what the meter has counted.",
	Long: `Show a scope's newest metered totals: the tokens and realized cost through the
last turn charged.

Without --client this reports the deployment. A window that has rolled reads as
zero, which is the refill: the meter keeps the newest total per scope and no
history, so this answers "how much is spent against the ceiling" and not "what
happened last week". --by-model reports the deployment's all-time totals per
model instead, and needs no --model.

A cost of zero on a model nobody declared pricing for is not a free model: the
monthly spend ceiling is metered from the rate cards you state with
'contenox model capability set --input-price …', so with none stated the ceiling
never trips.`,
	Args: cobra.NoArgs,
	RunE: runGatewayUsage,
}

func runGatewayKeyCreate(cmd *cobra.Command, _ []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())

	issuer, err := gatewayIssuer(cmd)
	if err != nil {
		return err
	}
	hasher, err := gatewayHasher()
	if err != nil {
		return err
	}
	if hasher == nil {
		return fmt.Errorf("no token signing key configured (%s or %s): without one the gateway cannot verify or revoke the key it would print",
			libtokenkey.EnvSigningKeyFile, libtokenkey.EnvSigningKey)
	}

	client := strings.TrimSpace(flagString(cmd, "client"))
	if client == "" {
		return fmt.Errorf("--client is required: it is the durable identity the meter is keyed on")
	}
	ttl, err := parseTTL(flagString(cmd, "ttl"))
	if err != nil {
		return err
	}
	claims, err := gatewayClaims(cmd, client, ttl)
	if err != nil {
		return err
	}

	bearer, err := issuer.Issue(claims)
	if err != nil {
		return fmt.Errorf("failed to issue the license: %w", err)
	}
	digest, err := hasher.Hash(libtokenkey.PurposeProxyKey, bearer)
	if err != nil {
		return fmt.Errorf("failed to hash the minted token: %w", err)
	}

	db, store, err := openConfigDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	expires := time.Now().UTC().Add(ttl)
	if _, err := store.RecordProxyKey(ctx, runtimetypes.ProxyKey{
		KeyHash: digest, ClientID: client,
		Tier: flagString(cmd, "tier"), ExpiresAt: expires,
	}); err != nil {
		return fmt.Errorf("failed to record the minted key: %w", err)
	}

	errOut := cmd.ErrOrStderr()
	fmt.Fprintf(errOut, "Minted key %s for %s, expires %s.\n", shortDigest(digest), client, expires.Format(time.RFC3339))
	fmt.Fprintf(errOut, "Allowed models: %s\n", claims.GetString("allowed_models", "*"))
	fmt.Fprintf(errOut, "This token is shown once and only its digest is stored; a lost token is replaced, not recovered.\n")
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", bearer)
	return nil
}

func gatewayIssuer(cmd *cobra.Command) (*liblicense.Issuer, error) {
	path := strings.TrimSpace(flagString(cmd, "authority-private-key-file"))
	if path == "" {
		return nil, fmt.Errorf("--authority-private-key-file is required: minting signs the license, so it needs the authority's private key")
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("--authority-private-key-file: %w", err)
	}
	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(pem, []byte(flagString(cmd, "authority-passphrase")))
	if err != nil {
		return nil, fmt.Errorf("--authority-private-key-file: %w", err)
	}
	return issuer, nil
}

func gatewayClaims(cmd *cobra.Command, client string, ttl time.Duration) (liblicense.Claims, error) {
	claims := liblicense.NewClaims(uuid.NewString(), flagString(cmd, "issuer"), client)
	expires := time.Now().UTC().Add(ttl)
	claims.ExpiresAt = &expires

	models := splitList(flagString(cmd, "models"))
	if len(models) == 0 {
		models = []string{"*"}
	}
	claims.Set("allowed_models", strings.Join(models, ","))

	if def := strings.TrimSpace(flagString(cmd, "default-model")); def != "" {
		claims.Set("default_model", def)
	}

	if path := strings.TrimSpace(flagString(cmd, "claims-file")); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return claims, fmt.Errorf("--claims-file: %w", err)
		}
		stated := map[string]string{}
		if err := json.Unmarshal(raw, &stated); err != nil {
			return claims, fmt.Errorf("--claims-file: expected a JSON object of claim name to value: %w", err)
		}
		for name, value := range stated {
			claims.Set(name, value)
		}
	}

	for _, allowance := range []struct {
		flag  string
		key   func(string) string
		claim func(flag, raw string) (int64, error)
	}{
		{"output-allowance", liblicense.OutputAllowanceKey, tokenClaim},
		{"input-allowance", liblicense.InputAllowanceKey, tokenClaim},
		{"five-hour-allowance", liblicense.FiveHourAllowanceKey, tokenClaim},
		{"monthly-budget", liblicense.MonthlyBudgetUSDKey, usdClaim},
		{"cache-discount", liblicense.CacheDiscountMultiplierKey, discountClaim},
		{"thinking-discount", liblicense.ThinkingDiscountMultiplierKey, discountClaim},
		{"image-allowance", liblicense.ImageAllowanceKey, countClaim},
		{"audio-allowance", liblicense.AudioAllowanceKey, countClaim},
	} {
		raw := strings.TrimSpace(flagString(cmd, allowance.flag))
		if raw == "" {
			continue
		}
		if len(models) == 1 && models[0] == "*" {
			return claims, fmt.Errorf("--%s needs --models: an allowance is per model, so name the models it applies to", allowance.flag)
		}
		value, err := allowance.claim(allowance.flag, raw)
		if err != nil {
			return claims, err
		}
		for _, model := range models {
			claims.SetInt64(allowance.key(model), value)
		}
	}

	// An input cap follows from the output cap unless one is stated, the way a
	// plan has always meant it. Without this a key that states only an output
	// allowance has no input ceiling at all, which is unbounded spend on the
	// half of a turn nobody bounded.
	if strings.TrimSpace(flagString(cmd, "input-allowance")) == "" {
		if raw := strings.TrimSpace(flagString(cmd, "output-allowance")); raw != "" {
			output, err := tokenClaim("output-allowance", raw)
			if err != nil {
				return claims, err
			}
			for _, model := range models {
				claims.SetInt64(liblicense.InputAllowanceKey(model), output*liblicense.DefaultInputAllowanceMultiplier)
			}
		}
	}
	return claims, nil
}

func tokenClaim(flag, raw string) (int64, error) {
	tokens, err := parseContextSize(raw)
	if err != nil {
		return 0, fmt.Errorf("--%s: %w", flag, err)
	}
	return int64(tokens), nil
}

func countClaim(flag, raw string) (int64, error) {
	count, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("--%s: %q is not a non-negative whole number", flag, raw)
	}
	return count, nil
}

func usdClaim(flag, raw string) (int64, error) {
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil || amount < 0 {
		return 0, fmt.Errorf("--%s: %q is not a non-negative USD amount", flag, raw)
	}
	return int64(amount * 1_000_000), nil
}

func discountClaim(flag, raw string) (int64, error) {
	share, err := strconv.ParseFloat(strings.TrimSuffix(raw, "%"), 64)
	if err != nil || share < 0 {
		return 0, fmt.Errorf("--%s: %q is not a non-negative share", flag, raw)
	}
	if strings.HasSuffix(raw, "%") {
		share /= 100
	}
	if share > 1 {
		return 0, fmt.Errorf("--%s: %q is more than one whole token", flag, raw)
	}
	return int64(share * 10_000), nil
}

func runGatewayKeyList(cmd *cobra.Command, _ []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	db, store, err := openConfigDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	client := strings.TrimSpace(flagString(cmd, "client"))
	limit, _ := cmd.Flags().GetInt("limit")

	keys, err := ledgerKeys(ctx, store, client)
	if err != nil {
		return err
	}
	if client == "" && len(keys) > limit {
		keys = keys[:limit]
	}

	out := cmd.OutOrStdout()
	if len(keys) == 0 {
		fmt.Fprintln(out, "No keys minted.")
		return nil
	}
	now := time.Now().UTC()
	for _, key := range keys {
		fmt.Fprintf(out, "%s  %-20s %-12s %s → %s  %s\n",
			shortDigest(key.KeyHash), key.ClientID, key.Tier,
			key.IssuedAt.Format("2006-01-02"), key.ExpiresAt.Format("2006-01-02"),
			keyState(key, now))
	}
	return nil
}

func ledgerKeys(ctx context.Context, store runtimetypes.Store, client string) ([]*runtimetypes.ProxyKey, error) {
	var found []*runtimetypes.ProxyKey
	var cursor *time.Time
	for {
		page, err := store.ListProxyKeys(ctx, cursor, keyPageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to list keys: %w", err)
		}
		for _, key := range page {
			if client == "" || key.ClientID == client {
				found = append(found, key)
			}
		}
		if len(page) < keyPageSize || client == "" {
			return found, nil
		}
		last := page[len(page)-1].CreatedAt
		cursor = &last
	}
}

func resolveLedgerKey(ctx context.Context, store runtimetypes.Store, digest string) (*runtimetypes.ProxyKey, error) {
	if key, err := store.GetProxyKeyByHash(ctx, digest); err == nil {
		return key, nil
	}
	keys, err := ledgerKeys(ctx, store, "")
	if err != nil {
		return nil, err
	}
	var matches []*runtimetypes.ProxyKey
	for _, key := range keys {
		if strings.HasPrefix(key.KeyHash, digest) {
			matches = append(matches, key)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no key in this ledger starts with %q", digest)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("%q matches %d keys, so it names none of them: use a longer prefix", digest, len(matches))
	}
}

func keyState(key *runtimetypes.ProxyKey, now time.Time) string {
	switch {
	case key.RevokedAt != nil:
		return "revoked " + key.RevokedAt.Format("2006-01-02")
	case !now.Before(key.ExpiresAt):
		return "expired"
	default:
		return "active"
	}
}

func runGatewayKeyRevoke(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	db, store, err := openConfigDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	client := strings.TrimSpace(flagString(cmd, "client"))
	digest := ""
	if len(args) == 1 {
		digest = strings.TrimSpace(args[0])
	}

	var targets []*runtimetypes.ProxyKey
	switch {
	case digest != "" && client == "":
		key, err := resolveLedgerKey(ctx, store, digest)
		if err != nil {
			return err
		}
		targets = append(targets, key)
	case digest == "" && client != "":
		keys, err := ledgerKeys(ctx, store, client)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, key := range keys {
			if key.RevokedAt == nil && now.Before(key.ExpiresAt) {
				targets = append(targets, key)
			}
		}
	default:
		return fmt.Errorf("name one key digest, or every key of one client with --client")
	}

	out := cmd.OutOrStdout()
	if len(targets) == 0 {
		fmt.Fprintf(out, "No active key to revoke for %s.\n", client)
		return nil
	}

	var revoked []*runtimetypes.ProxyKey
	for _, key := range targets {
		changed, err := store.RevokeProxyKey(ctx, key.KeyHash)
		if err != nil {
			return fmt.Errorf("failed to revoke %s: %w", shortDigest(key.KeyHash), err)
		}
		if changed {
			revoked = append(revoked, key)
		}
	}
	fmt.Fprintf(out, "Revoked %d key(s).\n", len(revoked))

	if broadcast, _ := cmd.Flags().GetBool("broadcast"); broadcast {
		if len(revoked) == 0 {
			fmt.Fprintln(out, "Nothing to broadcast.")
			return nil
		}
		if err := broadcastRevocations(ctx, db, revoked); err != nil {
			return err
		}
		fmt.Fprintf(out, "Broadcast %d cutoff(s) on the message bus.\n", len(revoked))
	}
	return nil
}

// broadcastRevocations publishes one cutoff per revoked key, by digest. It never
// publishes a client-wide block: the cache would refuse every model that client
// asks for, including keys this command did not revoke.
func broadcastRevocations(ctx context.Context, db libdb.DBManager, keys []*runtimetypes.ProxyKey) error {
	bus, err := substrate.OpenBus(ctx, db.WithoutTransaction())
	if err != nil {
		return fmt.Errorf("failed to open the message bus: %w", err)
	}
	defer bus.Close()

	for _, key := range keys {
		event, err := json.Marshal(gateway.ProxyControlRevocationEvent{
			KeyHash:   key.KeyHash,
			Model:     "",
			Reason:    revocationNote,
			ExpiresAt: key.ExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("failed to encode the revocation: %w", err)
		}
		if err := bus.Publish(ctx, gateway.SubjectProxyControlRevocation, event); err != nil {
			return fmt.Errorf("failed to broadcast the revocation: %w", err)
		}
	}
	return nil
}

func runGatewayUsage(cmd *cobra.Command, _ []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	db, _, err := openConfigDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	usage := runtimetypes.NewUsageStore(db)
	out := cmd.OutOrStdout()

	if byModel, _ := cmd.Flags().GetBool("by-model"); byModel {
		totals, err := usage.UsageByModel(ctx)
		if err != nil {
			return fmt.Errorf("failed to read usage: %w", err)
		}
		if len(totals) == 0 {
			fmt.Fprintln(out, "Nothing metered yet.")
			return nil
		}
		for _, total := range totals {
			fmt.Fprintf(out, "%-32s %s\n", total.Model, describeSnapshot(total.Snapshot))
		}
		return nil
	}

	model := strings.TrimSpace(flagString(cmd, "model"))
	if model == "" {
		return fmt.Errorf("--model is required: every ceiling is metered per model, so say which one")
	}
	scope := runtimetypes.UsageScope{
		Scope:      runtimetypes.UsageScopeGlobal,
		Model:      model,
		WindowKind: flagString(cmd, "window"),
	}
	if client := strings.TrimSpace(flagString(cmd, "client")); client != "" {
		scope.Scope, scope.ScopeID = runtimetypes.UsageScopeClient, client
	}

	snapshot, err := usage.ReadUsage(ctx, scope, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to read usage: %w", err)
	}
	fmt.Fprintf(out, "%s, %s, %s window since %s: %s\n",
		scopeLabel(scope), model, scope.WindowKind,
		snapshot.WindowStart.Format(time.RFC3339), describeSnapshot(snapshot))
	return nil
}

func scopeLabel(scope runtimetypes.UsageScope) string {
	if scope.Scope == runtimetypes.UsageScopeClient {
		return "client " + scope.ScopeID
	}
	return "deployment"
}

func describeSnapshot(snapshot runtimetypes.UsageSnapshot) string {
	described := fmt.Sprintf("output %d (thinking %d, effective %d), input %d (effective %d), total %d, cost $%.4f",
		snapshot.CompletionTokens, snapshot.ThinkingTokens, snapshot.EffectiveOutput,
		snapshot.PromptTokens, snapshot.EffectiveInput,
		snapshot.TotalTokens, float64(snapshot.CostMicrodollars)/1_000_000)
	if snapshot.ImageCount > 0 {
		described += fmt.Sprintf(", images %d", snapshot.ImageCount)
	}
	if snapshot.AudioBytes > 0 {
		described += fmt.Sprintf(", audio %.2f MiB", float64(snapshot.AudioBytes)/runtimetypes.Mebibyte)
	}
	return described
}

func shortDigest(digest string) string {
	if len(digest) <= digestPrefix {
		return digest
	}
	return digest[:digestPrefix]
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseTTL(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultKeyTTL, nil
	}
	if last := raw[len(raw)-1]; last == 'd' || last == 'w' {
		n, err := strconv.ParseFloat(raw[:len(raw)-1], 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("--ttl: %q is not a positive number of %s", raw, map[byte]string{'d': "days", 'w': "weeks"}[last])
		}
		if last == 'd' {
			return time.Duration(n * float64(24*time.Hour)), nil
		}
		return time.Duration(n * float64(7*24*time.Hour)), nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		return 0, fmt.Errorf("--ttl: %q is not a positive duration (720h, 30d, 4w)", raw)
	}
	return ttl, nil
}

func init() {
	gatewayKeyCreateCmd.Flags().String("client", "", "The durable client identity the key is minted for. Required.")
	gatewayKeyCreateCmd.Flags().String("models", "*", "Comma-separated models the key may use, or * for whatever the gateway serves.")
	gatewayKeyCreateCmd.Flags().String("default-model", "", "Model the caller should use when it names none; a key stating nothing but this may use it alone.")
	gatewayKeyCreateCmd.Flags().String("ttl", "", "How long the key is valid: a Go duration (720h) or days/weeks (30d, 4w). Default 720h.")
	gatewayKeyCreateCmd.Flags().String("tier", "", "Free-form label recorded on the ledger row, for your own reporting.")
	gatewayKeyCreateCmd.Flags().String("issuer", "contenox", "Value written as the license issuer.")
	gatewayKeyCreateCmd.Flags().String("output-allowance", "", "Weekly output-token ceiling per model (5m, 400k).")
	gatewayKeyCreateCmd.Flags().String("input-allowance", "", "Weekly input-token ceiling per model (25m).")
	gatewayKeyCreateCmd.Flags().String("five-hour-allowance", "", "Burst ceiling per model over five hours (400k).")
	gatewayKeyCreateCmd.Flags().String("monthly-budget", "", "Monthly spend ceiling per model, in USD (20).")
	gatewayKeyCreateCmd.Flags().String("image-allowance", "", "Weekly image-attachment ceiling per model (100).")
	gatewayKeyCreateCmd.Flags().String("audio-allowance", "", "Weekly inline-audio ceiling per model, in mebibytes (10).")
	gatewayKeyCreateCmd.Flags().String("cache-discount", "", "Share of a cached prompt token that counts against the input ceilings, per model (0.25 or 25%).")
	gatewayKeyCreateCmd.Flags().String("thinking-discount", "", "Share of a thinking token that counts against the output and burst ceilings, per model (0.25 or 25%).")
	gatewayKeyCreateCmd.Flags().String("claims-file", "", "JSON object of raw claims to merge, for anything the flags do not cover.")
	gatewayKeyCreateCmd.Flags().String("authority-private-key-file", "", "The authority's SSH private key, which signs the license. Required.")
	gatewayKeyCreateCmd.Flags().String("authority-passphrase", "", "Passphrase for the authority private key, when it is encrypted.")

	gatewayKeyListCmd.Flags().Int("limit", 50, "How many ledger rows to print.")
	gatewayKeyListCmd.Flags().String("client", "", "Only this client's keys.")

	gatewayKeyRevokeCmd.Flags().String("client", "", "Revoke every key this client still holds unrevoked.")
	gatewayKeyRevokeCmd.Flags().Bool("broadcast", false, "Also publish the cutoff on the message bus, reaching gateways with their own database.")

	gatewayUsageCmd.Flags().String("client", "", "Metered client to report; the deployment when omitted.")
	gatewayUsageCmd.Flags().String("model", "", "Model to report. Required unless --by-model.")
	gatewayUsageCmd.Flags().String("window", runtimetypes.UsageWindowWeek, "Window to report: 5h, week, month or total.")
	gatewayUsageCmd.Flags().Bool("by-model", false, "Report the deployment's all-time totals, one line per model.")

	gatewayKeyCmd.AddCommand(gatewayKeyCreateCmd)
	gatewayKeyCmd.AddCommand(gatewayKeyListCmd)
	gatewayKeyCmd.AddCommand(gatewayKeyRevokeCmd)
	gatewayCmd.AddCommand(gatewayKeyCmd)
	gatewayCmd.AddCommand(gatewayUsageCmd)
}
