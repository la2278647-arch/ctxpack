package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// sampleFits returns a fixed, deliberately unsorted slice so a test can assert
// the order a sort produced instead of whatever happened to arrive. The values
// are chosen so that the three sort keys produce three DIFFERENT orders:
//
//	pct-desc  : alpha, mango, ionic, zebra
//	window-desc: zebra, mango, ionic, alpha
//	name-asc  : alpha, ionic, mango, zebra
//
// alpha has the highest percentage but the smallest window, and zebra has the
// largest window but the lowest percentage, so every pair of keys disagrees
// somewhere. If two keys ever produced the same order, a test would pass even
// though the wrong comparator had been used.
func sampleFits() []fitEntry {
	return []fitEntry{
		{Model: "zebra", Used: 400, Limit: 2000, Fits: true, PctUsed: 20.0},
		{Model: "mango", Used: 900, Limit: 1500, Fits: true, PctUsed: 60.0},
		{Model: "ionic", Used: 450, Limit: 1000, Fits: true, PctUsed: 45.0},
		{Model: "alpha", Used: 760, Limit: 800, Fits: true, PctUsed: 95.0},
	}
}

// assertModelOrder checks the model names of a sorted slice against an
// expected order, reporting the actual order on failure so a wrong tie-break
// is diagnosable from the message alone.
func assertModelOrder(t *testing.T, fits []fitEntry, want []string) {
	t.Helper()
	if len(fits) != len(want) {
		t.Fatalf("got %d fits, want %d", len(fits), len(want))
	}
	got := make([]string, len(fits))
	for i, f := range fits {
		got[i] = f.Model
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %q, want %q (got %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}

// TestTopFitsTruncates is the only reason a caller passes n at all: a report
// that quietly returned every model would defeat capping one. Truncation must
// keep the prefix in place, so it cannot also reorder.
func TestTopFitsTruncates(t *testing.T) {
	got := topFits(sampleFits(), 2)
	if len(got) != 2 {
		t.Fatalf("topFits(n=2) returned %d entries, want 2", len(got))
	}
	if got[0].Model != "zebra" || got[1].Model != "mango" {
		t.Errorf("topFits(n=2) reordered the prefix: %v", modelsOf(got))
	}
}

// TestTopFitsKeepsAllWhenNotShrinking covers the inverse branch. Any n that
// does not strictly shrink the slice — zero, negative, exactly the length, or
// above it — must hand back the whole slice. Returning a shorter copy here
// would silently drop models nobody asked to drop.
func TestTopFitsKeepsAllWhenNotShrinking(t *testing.T) {
	src := sampleFits()
	for _, n := range []int{0, -1, -1000, len(src), len(src) + 5} {
		if got := topFits(src, n); len(got) != len(src) {
			t.Errorf("topFits(n=%d) returned %d entries, want all %d", n, len(got), len(src))
		}
	}
}

// TestTopFitsEmptyNeverPanics guards the empty input. A naive fits[:n] would
// turn an empty slice into an out-of-range panic, which would surface only as
// a crash on the first empty `tokens` report.
func TestTopFitsEmptyNeverPanics(t *testing.T) {
	for _, n := range []int{0, 1, 100} {
		if got := topFits(nil, n); got != nil {
			t.Errorf("topFits(nil, %d) = %v, want nil", n, got)
		}
	}
}

// TestSortFitsByPctDescending pins the first primary key: the model that is
// closest to its window sorts first, because that is the one worth warning
// about. alpha leads even though its window is the smallest of the four.
func TestSortFitsByPctDescending(t *testing.T) {
	fits := sampleFits()
	sortFits(fits, "pct")
	assertModelOrder(t, fits, []string{"alpha", "mango", "ionic", "zebra"})
}

// TestSortFitsByWindowDescending sorts the same sample on the second key. It
// must flip both ends of the percentage sort: zebra, with the largest window
// and the lowest percentage, now comes first.
func TestSortFitsByWindowDescending(t *testing.T) {
	fits := sampleFits()
	sortFits(fits, "window")
	assertModelOrder(t, fits, []string{"zebra", "mango", "ionic", "alpha"})
}

// TestSortFitsDefaultIsByName is the contract `tokens` relies on when the user
// passes no --sort: alphabetical, which is the only order a reader can
// reconstruct by hand. The expectation deliberately differs from the two sorts
// above — see the table in sampleFits — so each of the three tests fails if the
// wrong comparator is used.
func TestSortFitsDefaultIsByName(t *testing.T) {
	fits := sampleFits()
	sortFits(fits, "")
	assertModelOrder(t, fits, []string{"alpha", "ionic", "mango", "zebra"})
}

// TestSortFitsUnknownKeyFallsBackToName: an unrecognised --sort value must
// behave like the default rather than leaving the slice in arrival order,
// which would look deterministic while silently depending on map iteration.
func TestSortFitsUnknownKeyFallsBackToName(t *testing.T) {
	fits := sampleFits()
	sortFits(fits, "nonsense")
	assertModelOrder(t, fits, []string{"alpha", "ionic", "mango", "zebra"})
}

// TestSortFitsTieBreaksByNameDeterministically is the whole reason the
// tie-break exists. Without it sort.Slice leaves equal keys in whatever order
// they arrived in, so a report would reshuffle between identical runs.
func TestSortFitsTieBreaksByNameDeterministically(t *testing.T) {
	fits := []fitEntry{
		{Model: "zebra", PctUsed: 50.0},
		{Model: "apple", PctUsed: 50.0},
		{Model: "mango", PctUsed: 50.0},
	}
	sortFits(fits, "pct")
	assertModelOrder(t, fits, []string{"apple", "mango", "zebra"})
}

// TestSortFitsWindowTieBreaksByNameDeterministically is the same guarantee for
// the second key, which has its own tie-break line.
func TestSortFitsWindowTieBreaksByNameDeterministically(t *testing.T) {
	fits := []fitEntry{
		{Model: "zulu", Limit: 8000},
		{Model: "aardvark", Limit: 8000},
	}
	sortFits(fits, "window")
	assertModelOrder(t, fits, []string{"aardvark", "zulu"})
}

// TestFilterFitsMatchesByName is the only selector the --model flag goes
// through, so its contract is that exactly the named model survives.
func TestFilterFitsMatchesByName(t *testing.T) {
	got := filterFits(sampleFits(), "mango")
	if len(got) != 1 || got[0].Model != "mango" {
		t.Fatalf("filterFits(%q) = %v, want one mango entry", "mango", got)
	}
}

// TestFilterFitsUnknownNameReturnsEmpty not nil documents the difference: an
// empty slice marshals as [] in the JSON envelope while nil marshals as null,
// and a report that said null instead of an empty list would read as a
// failure rather than as "nothing matched".
func TestFilterFitsUnknownNameReturnsEmpty(t *testing.T) {
	got := filterFits(sampleFits(), "no-such-model")
	if len(got) != 0 {
		t.Fatalf("filterFits with an unknown name returned %v, want empty", got)
	}
	if got == nil {
		t.Error("filterFits returned nil; want an empty non-nil slice so JSON prints [] not null")
	}
}

// modelsOf is a test-only projection to keep assertion messages short.
func modelsOf(fits []fitEntry) []string {
	out := make([]string, len(fits))
	for i, f := range fits {
		out[i] = f.Model
	}
	return out
}

// TestOutputWriterStdoutForEmptyAndDash pins the two spellings that mean "the
// terminal": a bare --output with nothing after it, and an explicit -. Both
// must hand back os.Stdout and a closer that is safe to call, not a file
// literally named "-".
func TestOutputWriterStdoutForEmptyAndDash(t *testing.T) {
	for _, dest := range []string{"", "-"} {
		w, closeFn := outputWriter(dest)
		if w != os.Stdout {
			t.Errorf("outputWriter(%q) did not return os.Stdout", dest)
		}
		closeFn() // must be a no-op, never close the real stdout
	}
}

// TestOutputWriterCreatesAndClosesARealFile covers the file branch including
// the close function the caller must invoke. A caller that forgot it would
// leak a descriptor on every --output run, and the written bytes would not
// have been flushed to disk when the next command ran.
func TestOutputWriterCreatesAndClosesARealFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.xml")
	const payload = "<repository>ctxpack</repository>"

	w, closeFn := outputWriter(path)
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	closeFn()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back the written file failed: %v", err)
	}
	if string(data) != payload {
		t.Errorf("file holds %q, want %q", data, payload)
	}
}
