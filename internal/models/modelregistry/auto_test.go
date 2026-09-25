package modelregistry

import (
	"errors"
	"strings"
	"testing"
)

func TestUnit_SelectAuto_OriginAndMemory(t *testing.T) {
	for _, tc := range []struct {
		origin string
		gib    int64
		want   string
	}{
		{"china", 12, "qwen3.5-4b"},
		{"china", 16, "qwen3.5-9b"},
		{"china", 24, "qwen3.5-9b"},
		{"china", 32, "qwen3.5-9b"},
		{"china", 48, "qwen3.8-27b"},
		{"us", 12, "gemma4-e4b"},
		{"us", 16, "gemma4-12b"},
		{"us", 24, "gemma4-26b-a4b"},
		{"us", 48, "granite4.1-30b"},
		{"eu", 16, "ministral3-8b"},
		{"eu", 24, "ministral3-14b"},
		{"eu", 48, "ministral3-14b"},
		{"", 16, "gemma4-12b"},
		{"", 24, "gemma4-26b-a4b"},
	} {
		t.Run(tc.origin+tc.want, func(t *testing.T) {
			budget := (tc.gib << 30) * 8 / 10
			got, err := SelectAuto(tc.origin, "llama", budget)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tc.want {
				t.Fatalf("got %s, want %s", got.Name, tc.want)
			}
			if !got.SupportsAgenticTools() {
				t.Fatalf("%s is not curated as agentic", got.Name)
			}
			if hot := got.AutoHotContext(budget); hot < MinAgenticHotContext {
				t.Fatalf("%s carries only %d usable tokens", got.Name, hot)
			}
			if got.EstimatedResidentBytes() > budget {
				t.Fatalf("selected model exceeds budget")
			}
		})
	}
}

// TestUnit_SelectAuto_RefusesBelowTheAgenticBaseline pins the floor: a machine
// that cannot hold an agentic model's working context gets a decision to make,
// not a model picked for the size of its window.
func TestUnit_SelectAuto_RefusesBelowTheAgenticBaseline(t *testing.T) {
	for _, tc := range []struct {
		origin string
		gib    int64
		names  string
	}{
		{"china", 3, "minicpm5-2b"},
		{"us", 8, "gemma4-e4b"},
		{"eu", 12, "ministral3-8b"},
		{"auto", 3, "minicpm5-2b"},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			_, err := SelectAuto(tc.origin, "llama", (tc.gib<<30)*8/10)
			if !errors.Is(err, ErrNoAutoModel) {
				t.Fatalf("got %v, want ErrNoAutoModel", err)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Fatalf("error must name what would work: %v", err)
			}
			if !strings.Contains(err.Error(), "hosted backend") {
				t.Fatalf("error must name the remedies: %v", err)
			}
		})
	}
	_, err := SelectAuto("gus", "llama", 48<<30)
	if !errors.Is(err, ErrNoAutoModel) || !strings.Contains(err.Error(), "at any budget") {
		t.Fatalf("an origin with no agentic model must say so: %v", err)
	}
}

// TestUnit_SelectAuto_NeverPicksAToolChatModel pins that a large window on a
// model that cannot sustain a loop is not a capability: the smoke-class entries
// reach far more hot context than the models chosen above them.
func TestUnit_SelectAuto_NeverPicksAToolChatModel(t *testing.T) {
	for _, gib := range []int64{12, 24, 48} {
		budget := (gib << 30) * 8 / 10
		for _, origin := range []string{"china", "us", "eu", "auto"} {
			got, err := SelectAuto(origin, "llama", budget)
			if errors.Is(err, ErrNoAutoModel) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.AgenticTier != AgenticTierAgentic {
				t.Fatalf("%s %dGiB picked %s (%s)", origin, gib, got.Name, got.AgenticTier)
			}
		}
	}
	smallBudget := int64(12<<30) * 8 / 10
	small, err := SelectAuto("china", "llama", smallBudget)
	if err != nil {
		t.Fatal(err)
	}
	if small.Name != "qwen3.5-4b" {
		t.Fatalf("got %s, want the 4B class", small.Name)
	}
	smoke := curatedModels["qwen3.5-0.8b"]
	if hot := smoke.AutoHotContext(smallBudget); hot <= small.AutoHotContext(smallBudget) {
		t.Fatalf("this test is only meaningful while the smoke model has the larger window: %d vs %d",
			hot, small.AutoHotContext(smallBudget))
	}
}

func TestUnit_SelectAuto_RefusesFallback(t *testing.T) {
	for _, tc := range []struct {
		origin, backend string
		budget          int64
	}{
		{"gus", "llama", 1 << 30},
		{"eu", "openvino", 128 << 30},
		{"gus", "openvino", 128 << 30},
	} {
		if _, err := SelectAuto(tc.origin, tc.backend, tc.budget); !errors.Is(err, ErrNoAutoModel) {
			t.Fatalf("%+v: got %v", tc, err)
		}
	}
	for _, tc := range []struct {
		origin, backend string
		budget          int64
	}{
		{"unknown", "llama", 32 << 30}, {"us", "remote", 32 << 30}, {"us", "llama", 0},
	} {
		if _, err := SelectAuto(tc.origin, tc.backend, tc.budget); err == nil {
			t.Fatalf("accepted invalid selection %+v", tc)
		}
	}
}

func TestUnit_SelectAuto_CompatibleOpenVINO(t *testing.T) {
	for _, origin := range []string{"us", "china"} {
		d, err := SelectAuto(origin, "openvino", 24<<30)
		if err != nil {
			t.Fatal(err)
		}
		if d.Origin != origin || d.BackendType() != "openvino" || d.ToolProtocol == "" || d.Vision {
			t.Fatalf("invalid tool-enabled text selection: %+v", d)
		}
	}
}

func TestUnit_AutoHotContext_ResidentBudget(t *testing.T) {
	for _, d := range curatedModels {
		if !d.AutoEligible {
			continue
		}
		if d.AutoContextLimit <= 0 || !d.AutoKVProfile.Valid() {
			t.Fatalf("missing metadata for %s", d.Name)
		}
		for _, gib := range []int64{8, 16, 24, 32, 48} {
			budget := (gib << 30) * 8 / 10
			hot := d.AutoHotContext(budget)
			if hot == 0 {
				continue
			}
			weights := d.SizeBytes + d.MMProjSizeBytes
			used := weights + max(weights/4, int64(256<<20)) + d.AutoKVProfile.KVBytesForContext(hot)
			if used > budget || hot > d.AutoContextLimit || hot > 280*1024 {
				t.Fatalf("%s: hot=%d used=%d budget=%d", d.Name, hot, used, budget)
			}
		}
	}
	if (ModelDescriptor{SizeBytes: 1}).AutoHotContext(48<<30) != 0 {
		t.Fatal("unknown metadata must not promise hot context")
	}
}

// TestUnit_SelectAuto_CapabilityBeforeContext pins the ordering: among models
// that clear the gate the larger one wins, so a bigger window on a smaller model
// does not decide the pick.
func TestUnit_SelectAuto_CapabilityBeforeContext(t *testing.T) {
	const gib int64 = 16
	budget := (gib << 30) * 8 / 10
	for _, origin := range []string{"china", "auto"} {
		got, err := SelectAuto(origin, "llama", budget)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range curatedModels {
			if !d.SupportsAgenticTools() || d.BackendType() != "llama" || (origin != "auto" && d.Origin != origin) {
				continue
			}
			if d.AutoHotContext(budget) < MinAgenticHotContext {
				continue
			}
			if d.SizeBytes > got.SizeBytes {
				t.Fatalf("%s %dGiB chose %s (%d bytes) over the larger %s (%d bytes), both eligible",
					origin, gib, got.Name, got.SizeBytes, d.Name, d.SizeBytes)
			}
		}
	}
}

// TestUnit_SelectAutoType_KVTypeBuysTheWindow pins the reason the KV type is an
// input to selection: a 6 GB-class machine runs the same model at double the
// window under a q8_0 cache, and a budget that fits nothing at f16 fits a 4B
// class model at q8_0. The caller must pass what the worker will allocate.
func TestUnit_SelectAutoType_KVTypeBuysTheWindow(t *testing.T) {
	small := int64(2_900_000_000)
	if _, err := SelectAutoType("", "llama", small, "f16"); !errors.Is(err, ErrNoAutoModel) {
		t.Fatalf("%.1fGiB at f16 should fit nothing agentic, got %v", float64(small)/(1<<30), err)
	}
	got, err := SelectAutoType("", "llama", small, "q8_0")
	if err != nil {
		t.Fatalf("%.1fGiB at q8_0 must fit a model: %v", float64(small)/(1<<30), err)
	}
	if got.Name != "minicpm5-2b" {
		t.Fatalf("got %s, want the 2B class", got.Name)
	}
	if hot := got.AutoHotContextType(small, "q8_0"); hot < MinAgenticHotContext {
		t.Fatalf("planned hot %d is below the floor", hot)
	}
	if hot := got.AutoHotContextType(small, "f16"); hot >= MinAgenticHotContext {
		t.Fatalf("this case is only meaningful while f16 is what fails: %d", hot)
	}

	// The same budget with a quantized cache also reaches a larger model.
	mid := int64(12<<30) * 8 / 10
	f16, err := SelectAutoType("", "llama", mid, "f16")
	if err != nil {
		t.Fatalf("12GiB at f16: %v", err)
	}
	q8, err := SelectAutoType("", "llama", mid, "q8_0")
	if err != nil {
		t.Fatalf("12GiB at q8_0: %v", err)
	}
	if q8.SizeBytes <= f16.SizeBytes {
		t.Fatalf("q8_0 picked %s (%d bytes), not larger than f16's %s (%d bytes)", q8.Name, q8.SizeBytes, f16.Name, f16.SizeBytes)
	}

	// An unset type is f16, which is what the worker runs without the variable.
	if lenient, err := SelectAutoType("", "llama", small, ""); !errors.Is(err, ErrNoAutoModel) {
		t.Fatalf("empty KV type must be judged as f16, got %v (%s)", err, lenient.Name)
	}
}

// TestUnit_NoAgenticModelError_NamesTheCostAndTheBlocker pins the refusal a 6 GB
// machine sees: the breakdown of what would have to fit, and the model that does
// fit but the agentic gate will not take.
func TestUnit_NoAgenticModelError_NamesTheCostAndTheBlocker(t *testing.T) {
	_, err := SelectAutoType("", "llama", 2<<30, "q8_0")
	if !errors.Is(err, ErrNoAutoModel) {
		t.Fatalf("got %v", err)
	}
	for _, want := range []string{"weights", "runtime allowance", "KV", "kv=q8_0", "tool-chat"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal must mention %q: %v", want, err)
		}
	}
}
