package liblicense

import "strings"

// Meter semantics: a weekly grant is measured on OUTPUT (completion) tokens,
// with INPUT (prompt) tokens metered separately under their own cap, so a
// context-heavy or cache-read-heavy session does not spend the grant that
// measures what the model wrote. The two keys are per model and are spelled
// with the raw model name, so two models can never collide on a folded form.

// OutputAllowanceKey is the canonical claim key for a model's weekly output
// allowance.
func OutputAllowanceKey(model string) string { return "output_allowance:" + model }

// InputAllowanceKey is the canonical claim key for a model's weekly input cap.
func InputAllowanceKey(model string) string { return "input_allowance:" + model }

// OutputAllowanceFor reads a model's weekly output allowance. A licence minted
// before output and input were split carries only the legacy total allowance,
// which is honoured as the output allowance for as long as such a token is
// live. Zero means the claim states no output limit for the model.
func OutputAllowanceFor(cl *Claims, model string) int64 {
	if cl == nil || model == "" {
		return 0
	}
	if v := cl.GetInt64(OutputAllowanceKey(model), 0); v > 0 {
		return v
	}
	return LegacyAllowanceFor(cl, model)
}

// InputAllowanceFor reads a model's weekly input cap. Zero means the claim
// states no input limit, as a licence minted before the split does not.
func InputAllowanceFor(cl *Claims, model string) int64 {
	if cl == nil || model == "" {
		return 0
	}
	return cl.GetInt64(InputAllowanceKey(model), 0)
}

// LegacyAllowanceFor reads the pre-split total allowance under either spelling
// the issuer has used: "token_allowance:<model>", and the folded form that
// replaces "/" and "-" with "_".
func LegacyAllowanceFor(cl *Claims, model string) int64 {
	if cl == nil || model == "" {
		return 0
	}
	if v := cl.GetInt64("token_allowance:"+model, 0); v > 0 {
		return v
	}
	folded := strings.ReplaceAll(strings.ReplaceAll(model, "/", "_"), "-", "_")
	return cl.GetInt64("token_allowance_"+folded, 0)
}

// DefaultInputAllowanceMultiplier is the input cap applied when a plan states
// no multiplier: five times the model's output allowance.
const DefaultInputAllowanceMultiplier = int64(5)

// CacheDiscountMultiplierKey is the canonical claim key for a model's prompt
// cache discount multiplier in basis points (500 = 0.05).
func CacheDiscountMultiplierKey(model string) string {
	return "cache_discount_multiplier:" + model
}

// CacheDiscountMultiplierFor reads a model's prompt cache discount as a
// fraction. A claim stating none gets the 0.10 default.
func CacheDiscountMultiplierFor(cl *Claims, model string) float64 {
	if cl == nil || model == "" {
		return 0.10
	}
	if _, ok := cl.Get(CacheDiscountMultiplierKey(model)); ok {
		bp := cl.GetInt64(CacheDiscountMultiplierKey(model), 1000)
		if bp >= 0 && bp <= 10000 {
			return float64(bp) / 10000.0
		}
	}
	return 0.10
}

// ThinkingDiscountMultiplierKey is the canonical claim key for the share of a
// model's thinking tokens that consumes its output and burst allowances.
func ThinkingDiscountMultiplierKey(model string) string {
	return "thinking_discount_multiplier:" + model
}

// ThinkingDiscountMultiplierFor reads the allowance share of a thinking token.
// An absent or invalid claim charges the whole token.
func ThinkingDiscountMultiplierFor(cl *Claims, model string) float64 {
	if cl == nil || model == "" {
		return 1
	}
	if _, ok := cl.Get(ThinkingDiscountMultiplierKey(model)); ok {
		bp := cl.GetInt64(ThinkingDiscountMultiplierKey(model), 10000)
		if bp >= 0 && bp <= 10000 {
			return float64(bp) / 10000.0
		}
	}
	return 1
}

// FiveHourAllowanceKey is the canonical claim key for a model's rolling
// five-hour token ceiling.
func FiveHourAllowanceKey(model string) string { return "five_hour_allowance:" + model }

// FiveHourAllowanceFor reads a model's rolling five-hour token limit. Zero
// means the claim states no burst limit.
func FiveHourAllowanceFor(cl *Claims, model string) int64 {
	if cl == nil || model == "" {
		return 0
	}
	return cl.GetInt64(FiveHourAllowanceKey(model), 0)
}

// ImageAllowanceKey is the canonical claim key for a model's weekly ceiling on
// image input, counted in image attachments rather than tokens: an image's token
// cost is the provider's business, but how many were sent is ours to count.
func ImageAllowanceKey(model string) string { return "image_allowance:" + model }

// ImageAllowanceFor reads a model's weekly image ceiling. Zero means the claim
// states no image limit.
func ImageAllowanceFor(cl *Claims, model string) int64 {
	if cl == nil || model == "" {
		return 0
	}
	return cl.GetInt64(ImageAllowanceKey(model), 0)
}

// AudioAllowanceKey is the canonical claim key for a model's weekly ceiling on
// inline audio, in mebibytes: the wire carries audio as bytes and no duration,
// so a mebibyte is the largest unit that can be counted exactly.
func AudioAllowanceKey(model string) string { return "audio_allowance:" + model }

// AudioAllowanceFor reads a model's weekly audio ceiling in mebibytes. Zero
// means the claim states no audio limit.
func AudioAllowanceFor(cl *Claims, model string) int64 {
	if cl == nil || model == "" {
		return 0
	}
	return cl.GetInt64(AudioAllowanceKey(model), 0)
}

// MonthlyBudgetUSDKey is the canonical claim key for a model's monthly spend
// ceiling, in microdollars.
func MonthlyBudgetUSDKey(model string) string { return "monthly_budget_usd:" + model }

// MonthlyBudgetUSDFor reads a model's monthly spend ceiling in USD. Zero means
// the claim states no budget.
func MonthlyBudgetUSDFor(cl *Claims, model string) float64 {
	if cl == nil || model == "" {
		return 0
	}
	if micro := cl.GetInt64(MonthlyBudgetUSDKey(model), 0); micro > 0 {
		return float64(micro) / 1_000_000.0
	}
	return 0
}
