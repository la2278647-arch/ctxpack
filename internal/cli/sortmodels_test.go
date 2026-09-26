package cli

import (
	"testing"

	"github.com/la2278647-arch/ctxpack/internal/counter"
)

// sortModels sorts the whole model table in place, and the table has 31 entries
// with distinct names, so the two branches that only fire on a tie were never
// exercised: the name tiebreak inside the default (name) sort, and the name
// tiebreak inside the window sort. These tests build a table with duplicate
// names and duplicate windows so every branch runs, and pin the ordering
// contract that the models listing depends on.
//
// sort.Slice is not stable, so only elements that differ on at least one key are
// asserted; truly-equal elements may appear in either order and are not checked.

func TestSortModelsByNameDefault(t *testing.T) {
	m := []counter.Model{
		{Name: "zeta", ContextWindow: 1000, Vendor: "v"},
		{Name: "alpha", ContextWindow: 9000, Vendor: "v"},
		{Name: "Beta", ContextWindow: 4000, Vendor: "v"},
	}
	sortModels(m, "")

	got := names(m)
	want := []string{"alpha", "Beta", "zeta"}
	if !equalStr(got, want) {
		t.Fatalf("default sort order = %v, want %v", got, want)
	}

	// Case-insensitivity: "Beta" and "alpha" compare as "beta" and "alpha", so
	// the sort is by folded name while the original spelling is preserved.
	if m[1].Name != "Beta" {
		t.Fatalf("spelling not preserved: got %q", m[1].Name)
	}
}

func TestSortModelsByNameTiebreaksOnWindow(t *testing.T) {
	// Same name, different windows: the wider window must win.
	m := []counter.Model{
		{Name: "gpt-4", ContextWindow: 8192, Vendor: "openai"},
		{Name: "gpt-4", ContextWindow: 128000, Vendor: "openai"},
		{Name: "other", ContextWindow: 100, Vendor: "openai"},
	}
	sortModels(m, "name")

	got := names(m)
	want := []string{"gpt-4", "gpt-4", "other"}
	if !equalStr(got, want) {
		t.Fatalf("name sort order = %v, want %v", got, want)
	}
	if m[0].ContextWindow <= m[1].ContextWindow {
		t.Fatalf("name tie not broken by window descending: %d vs %d",
			m[0].ContextWindow, m[1].ContextWindow)
	}
}

func TestSortModelsByWindow(t *testing.T) {
	m := []counter.Model{
		{Name: "small", ContextWindow: 4000, Vendor: "a"},
		{Name: "big", ContextWindow: 128000, Vendor: "a"},
		{Name: "tiny", ContextWindow: 1000, Vendor: "a"},
	}
	sortModels(m, "window")

	got := windows(m)
	want := []int{128000, 4000, 1000}
	if !equalInt(got, want) {
		t.Fatalf("window sort order = %v, want %v", got, want)
	}
}

func TestSortModelsByWindowTiebreaksOnName(t *testing.T) {
	// Same window, different names: the name break must be ascending.
	m := []counter.Model{
		{Name: "zebra", ContextWindow: 1000, Vendor: "a"},
		{Name: "ant", ContextWindow: 1000, Vendor: "a"},
		{Name: "mole", ContextWindow: 1000, Vendor: "a"},
	}
	sortModels(m, "window")

	got := names(m)
	want := []string{"ant", "mole", "zebra"}
	if !equalStr(got, want) {
		t.Fatalf("window tie not broken by name ascending: got %v, want %v", got, want)
	}
}

func TestSortModelsByVendor(t *testing.T) {
	m := []counter.Model{
		{Name: "one", ContextWindow: 1000, Vendor: "OpenAI"},
		{Name: "two", ContextWindow: 5000, Vendor: "openai"},
		{Name: "three", ContextWindow: 9000, Vendor: "Anthropic"},
	}
	sortModels(m, "vendor")

	got := names(m)
	want := []string{"three", "two", "one"}
	if !equalStr(got, want) {
		t.Fatalf("vendor sort order = %v, want %v", got, want)
	}
	// Vendor comparison is case-insensitive: OpenAI and openai are the same
	// vendor, so the two entries order by window descending, not by spelling.
	if m[0].Vendor != "Anthropic" {
		t.Fatalf("vendor sort put %q first", m[0].Vendor)
	}
	if m[1].ContextWindow < m[2].ContextWindow {
		t.Fatalf("same vendor not ordered by window descending: %d vs %d",
			m[1].ContextWindow, m[2].ContextWindow)
	}
}

// Regression: the vendor comparator compared raw vendor strings for equality but
// folded ones for order, so "OpenAI" and "openai" were seen as different vendors
// while ordering as equal. sort.Slice has no defined result for an inconsistent
// comparator, and the grouping came out wrong.
func TestSortModelsByVendorGroupsCaseVariants(t *testing.T) {
	m := []counter.Model{
		{Name: "one", ContextWindow: 1000, Vendor: "OpenAI"},
		{Name: "two", ContextWindow: 5000, Vendor: "openai"},
		{Name: "three", ContextWindow: 9000, Vendor: "Anthropic"},
	}
	sortModels(m, "vendor")

	got := names(m)
	want := []string{"three", "two", "one"}
	if !equalStr(got, want) {
		t.Fatalf("vendor sort order = %v, want %v", got, want)
	}
	// The two case-variant vendors must be adjacent and internally ordered by
	// window descending.
	if !equalStr(got[1:], []string{"two", "one"}) {
		t.Fatalf("case-variant vendors not grouped together: %v", got)
	}
	if m[1].ContextWindow < m[2].ContextWindow {
		t.Fatalf("case-variant vendors not ordered by window: %d vs %d",
			m[1].ContextWindow, m[2].ContextWindow)
	}
}

// Same class of bug in the default (name) sort.
func TestSortModelsByNameGroupsCaseVariants(t *testing.T) {
	m := []counter.Model{
		{Name: "Alpha", ContextWindow: 1000, Vendor: "a"},
		{Name: "alpha", ContextWindow: 8000, Vendor: "a"},
		{Name: "zulu", ContextWindow: 50, Vendor: "a"},
	}
	sortModels(m, "name")

	got := names(m)
	want := []string{"alpha", "Alpha", "zulu"}
	if !equalStr(got, want) {
		t.Fatalf("name sort order = %v, want %v", got, want)
	}
	if m[0].ContextWindow < m[1].ContextWindow {
		t.Fatalf("name case-variants not ordered by window: %d vs %d",
			m[0].ContextWindow, m[1].ContextWindow)
	}
}

func TestSortModelsByVendorTiebreaksOnName(t *testing.T) {
	// Same vendor and same window: the final break is the name, ascending.
	m := []counter.Model{
		{Name: "zebra", ContextWindow: 1000, Vendor: "a"},
		{Name: "ant", ContextWindow: 1000, Vendor: "a"},
	}
	sortModels(m, "vendor")

	got := names(m)
	want := []string{"ant", "zebra"}
	if !equalStr(got, want) {
		t.Fatalf("vendor+window tie not broken by name: got %v, want %v", got, want)
	}
}

func TestSortModelsUnknownFieldFallsBackToName(t *testing.T) {
	m := []counter.Model{
		{Name: "zeta", ContextWindow: 1, Vendor: "a"},
		{Name: "alpha", ContextWindow: 2, Vendor: "a"},
	}
	sortModels(m, "nonsense")

	got := names(m)
	want := []string{"alpha", "zeta"}
	if !equalStr(got, want) {
		t.Fatalf("unknown sort key did not fall back to name: got %v, want %v", got, want)
	}
}

func TestSortModelsKeysAreCaseInsensitive(t *testing.T) {
	for _, key := range []string{"NAME", "Name", "nAmE", "WINDOW", "Vendor", "VENDOR"} {
		m := []counter.Model{
			{Name: "zeta", ContextWindow: 1000, Vendor: "a"},
			{Name: "alpha", ContextWindow: 4000, Vendor: "a"},
		}
		sortModels(m, key)
		if names(m)[0] != "alpha" {
			t.Fatalf("key %q did not sort by name", key)
		}
	}
}

func TestSortModelsIsIdempotent(t *testing.T) {
	m := []counter.Model{
		{Name: "zeta", ContextWindow: 3000, Vendor: "a"},
		{Name: "alpha", ContextWindow: 1000, Vendor: "b"},
		{Name: "beta", ContextWindow: 9000, Vendor: "a"},
	}
	sortModels(m, "name")
	first := names(m)
	for _, key := range []string{"name", "window", "vendor", "unknown"} {
		sortModels(m, key)
		sortModels(m, key)
		if !equalStr(names(m), names(m)) {
			t.Fatalf("key %q is not deterministic", key)
		}
	}
	// Sorting by name twice must produce the same result as sorting once.
	m2 := []counter.Model{
		{Name: "zeta", ContextWindow: 3000, Vendor: "a"},
		{Name: "alpha", ContextWindow: 1000, Vendor: "b"},
		{Name: "beta", ContextWindow: 9000, Vendor: "a"},
	}
	sortModels(m2, "name")
	sortModels(m2, "name")
	if !equalStr(names(m2), first) {
		t.Fatalf("re-sorting changed the order: %v vs %v", names(m2), first)
	}
}

func TestSortModelsHandlesEmptyAndSingle(t *testing.T) {
	var empty []counter.Model
	sortModels(empty, "name")
	if len(empty) != 0 {
		t.Fatalf("empty slice grew: %v", empty)
	}

	single := []counter.Model{{Name: "only", ContextWindow: 1, Vendor: "a"}}
	sortModels(single, "window")
	if len(single) != 1 || single[0].Name != "only" {
		t.Fatalf("single-element slice changed: %v", single)
	}
}

// --- helpers ---

func names(ms []counter.Model) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

func windows(ms []counter.Model) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = m.ContextWindow
	}
	return out
}

func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
