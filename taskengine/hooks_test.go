package taskengine_test

import (
	"context"
	"sort"
	"testing"

	"github.com/contenox/contenox/taskengine"
)

// resolveHookNames is tested indirectly via the exported behaviour through
// MacroEnv and SimpleEnv, but we also exercise it directly by constructing a
// minimal HookProvider stub.

func sortedNames(names []string) []string {
	cp := append([]string(nil), names...)
	sort.Strings(cp)
	return cp
}

func TestResolveHookNames_Nil_ReturnsAll(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), nil, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Errorf("nil allowlist: expected 3, got %d: %v", len(names), names)
	}
}

func TestResolveHookNames_Empty_ReturnsNothing(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), []string{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("empty allowlist: expected 0, got %d: %v", len(names), names)
	}
}

func TestResolveHookNames_Star_ReturnsAll(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), []string{"*"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Errorf("[*]: expected 3, got %d: %v", len(names), names)
	}
}

func TestResolveHookNames_Exact_Match(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), []string{"hook_a"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "hook_a" {
		t.Errorf("[hook_a]: expected [hook_a], got %v", names)
	}
}

func TestResolveHookNames_Exact_Miss(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), []string{"unknown"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("[unknown]: expected empty, got %v", names)
	}
}

func TestResolveHookNames_StarExclude_RemovesEntry(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), []string{"*", "!hook_b"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	got := sortedNames(names)
	if len(got) != 2 || got[0] != "hook_a" || got[1] != "hook_c" {
		t.Errorf("[*, !hook_b]: expected [hook_a hook_c], got %v", got)
	}
}

func TestResolveHookNames_StarExcludeMiss_ReturnsAll(t *testing.T) {
	repo := stubRepo()
	names, err := taskengine.ExportedResolveHookNames(context.Background(), []string{"*", "!hook_x"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Errorf("[*, !hook_x]: expected 3, got %d: %v", len(names), names)
	}
}
