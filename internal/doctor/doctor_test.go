package doctor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/la2278647-arch/ctxpack/internal/counter"
)

// TestGather checks the snapshot a real machine produces. Every assertion is
// relative to what the registry and runtime actually report, so the test keeps
// passing when models are added or the Go version moves.
func TestGather(t *testing.T) {
	r := Gather()
	if r.Version == "" {
		t.Error("version is empty")
	}
	if r.GoVersion == "" {
		t.Error("go_version is empty")
	}
	if r.Platform == "" || !strings.Contains(r.Platform, "/") {
		t.Errorf("platform = %q, want GOOS/GOARCH", r.Platform)
	}
	models := counter.Models()
	if r.ModelCount != len(models) {
		t.Errorf("model_count = %d, want %d", r.ModelCount, len(models))
	}
	if r.ModelVendors != len(r.Vendors) {
		t.Errorf("model_vendors = %d, want %d", r.ModelVendors, len(r.Vendors))
	}
}

// TestGatherVendorBreakdownSumsToTotal pins the invariant that the breakdown
// accounts for every registered model. If a vendor were silently dropped the
// counts would disagree and a report would look smaller than the registry is.
func TestGatherVendorBreakdownSumsToTotal(t *testing.T) {
	r := Gather()
	total := 0
	for _, v := range r.Vendors {
		total += v.Models
	}
	if total != r.ModelCount {
		t.Errorf("vendor breakdown sums to %d, want model_count %d", total, r.ModelCount)
	}
}

func sample() []counter.Model {
	return []counter.Model{
		{Name: "a1", Vendor: "alpha", ContextWindow: 1000},
		{Name: "a2", Vendor: "alpha", ContextWindow: 1000},
		{Name: "a3", Vendor: "alpha", ContextWindow: 1000},
		{Name: "b1", Vendor: "beta", ContextWindow: 2000},
		{Name: "b2", Vendor: "beta", ContextWindow: 2000},
		{Name: "c1", Vendor: "charlie", ContextWindow: 3000},
		{Name: "d1", Vendor: "delta", ContextWindow: 4000},
		{Name: "e1", Vendor: "echo", ContextWindow: 5000},
		{Name: "e2", Vendor: "echo", ContextWindow: 5000},
	}
}

// TestVendorBreakdownSortOrder pins the ordering contract: model count
// descending, vendor name ascending for ties. That tie-break is what makes a
// truncated report deterministic across runs.
func TestVendorBreakdownSortOrder(t *testing.T) {
	vs := VendorBreakdown(sample())
	if len(vs) != 5 {
		t.Fatalf("got %d vendors, want 5", len(vs))
	}
	want := []string{"alpha", "beta", "echo", "charlie", "delta"}
	for i, name := range want {
		if vs[i].Vendor != name {
			t.Errorf("position %d = %q, want %q", i, vs[i].Vendor, name)
		}
	}
}

func report() Report {
	return Report{
		Version:      "ctxpack 0.1.8 (windows/amd64, go1.21.0, commit abc1234, built 2026-09-26)",
		GoVersion:    "go1.21.0",
		Platform:     "windows/amd64",
		Git:          "C:/tools/git/cmd/git.exe",
		GitVersion:   "git version 2.39.0",
		ModelCount:   31,
		ModelVendors: 3,
		Vendors: []VendorCount{
			{Vendor: "alpha", Models: 20},
			{Vendor: "beta", Models: 8},
			{Vendor: "gamma", Models: 3},
		},
	}
}

func TestTextHappyPath(t *testing.T) {
	var sb bytes.Buffer
	report().Text(&sb, 0)
	out := sb.String()
	for _, want := range []string{
		"ctxpack diagnostics:",
		"  Version:   ctxpack 0.1.8",
		"  Go:        go1.21.0",
		"  Platform:  windows/amd64",
		"  Git:       C:/tools/git/cmd/git.exe (git version 2.39.0)",
		"  Models:    31 models, 3 vendors",
		"  Vendor breakdown:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q\n%s", want, out)
		}
	}
	// Vendor lines are %-15s padded, so match on name and count rather than on
	// the exact number of spaces between them. Padding is presentation.
	assertVendorLine(t, out, "alpha", 20)
	assertVendorLine(t, out, "beta", 8)
	assertVendorLine(t, out, "gamma", 3)
}

// assertVendorLine finds a line that names vendor and reports n models,
// ignoring the padding between the two. Padding is presentation, so asserting
// an exact string here would make the test fail whenever the width changed.
func assertVendorLine(t *testing.T, out, vendor string, n int) {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, vendor) && strings.HasSuffix(trimmed, fmt.Sprintf("%d model(s)", n)) {
			return
		}
	}
	t.Errorf("no vendor line for %s (%d model(s)):\n%s", vendor, n, out)
}

// TestTextGitMissing covers the branch a user hits when `diff` cannot work at
// all. The message has to say git is missing AND why that matters, otherwise
// the report is a dead end.
func TestTextGitMissing(t *testing.T) {
	r := report()
	r.Git = ""
	r.GitError = "exec: \"git\": executable file not found"
	r.GitVersion = ""
	var sb bytes.Buffer
	r.Text(&sb, 0)
	out := sb.String()
	if !strings.Contains(out, "Git:       not found (diff command will not work)") {
		t.Errorf("missing the git-absent line:\n%s", out)
	}
	if strings.Contains(out, "Git:       C:/tools") {
		t.Errorf("reported a git path that is absent:\n%s", out)
	}
}

// TestTextGitVersionUnknown covers the rarer half: git is on PATH but
// `git --version` failed. The path is still worth printing, so it must not be
// replaced by the "not found" message.
func TestTextGitVersionUnknown(t *testing.T) {
	r := report()
	r.GitVersion = ""
	r.GitVersionError = "boom"
	var sb bytes.Buffer
	r.Text(&sb, 0)
	out := sb.String()
	if !strings.Contains(out, "  Git:       C:/tools/git/cmd/git.exe (unknown)") {
		t.Errorf("missing the unknown-version line:\n%s", out)
	}
}

// TestTextTopTruncatesButKeepsTotal is the contract the CLI's --top relies on:
// the vendor list may shrink, the totals must not. A truncated report that
// also truncated the counts would understate the registry's size.
func TestTextTopTruncatesButKeepsTotal(t *testing.T) {
	var full bytes.Buffer
	var short bytes.Buffer
	r := report()
	r.Text(&full, 0)
	r.Text(&short, 1)
	if strings.Count(full.String(), " model(s)") != 3 {
		t.Fatalf("full output has %d vendor lines, want 3", strings.Count(full.String(), " model(s)"))
	}
	if strings.Count(short.String(), " model(s)") != 1 {
		t.Fatalf("--top 1 output has %d vendor lines, want 1", strings.Count(short.String(), " model(s)"))
	}
	if !strings.Contains(short.String(), "  Models:    31 models, 3 vendors") {
		t.Errorf("--top 1 changed the totals:\n%s", short.String())
	}
}

// TestTopVendorsEdges pins the boundary behaviour. The important case is that
// an out-of-range or non-positive top returns the whole list rather than an
// empty one: a caller asking for "all vendors" must not get zero.
func TestTopVendorsEdges(t *testing.T) {
	vs := report().Vendors
	if got := len(TopVendors(vs, 0)); got != 3 {
		t.Errorf("top=0 gave %d vendors, want 3", got)
	}
	if got := len(TopVendors(vs, -5)); got != 3 {
		t.Errorf("top=-5 gave %d vendors, want 3", got)
	}
	if got := len(TopVendors(vs, 3)); got != 3 {
		t.Errorf("top=len gave %d vendors, want 3", got)
	}
	if got := len(TopVendors(vs, 100)); got != 3 {
		t.Errorf("top>len gave %d vendors, want 3", got)
	}
	top := TopVendors(vs, 2)
	if len(top) != 2 || top[0].Vendor != "alpha" || top[1].Vendor != "beta" {
		t.Errorf("top=2 gave %v, want [alpha beta]", top)
	}
	if got := len(TopVendors(nil, 2)); got != 0 {
		t.Errorf("top on nil gave %d, want 0", got)
	}
}

// TestTextNoVendors covers a registry with no models: the breakdown heading
// must not appear, because an empty heading would look like a bug.
func TestTextNoVendors(t *testing.T) {
	r := report()
	r.Vendors = nil
	r.ModelCount = 0
	r.ModelVendors = 0
	var sb bytes.Buffer
	r.Text(&sb, 0)
	out := sb.String()
	if strings.Contains(out, "Vendor breakdown:") {
		t.Errorf("empty registry printed a breakdown heading:\n%s", out)
	}
	if !strings.Contains(out, "  Models:    0 models, 0 vendors") {
		t.Errorf("empty registry did not report zero:\n%s", out)
	}
}

// TestReportJSON checks that the report marshals to the same keys the CLI's
// `doctor --json` always produced, so an agent that parsed the CLI output keeps
// working, and that the vendor breakdown is always complete regardless of a
// truncation request. Truncation is a presentation concern, not a data one.
func TestReportJSON(t *testing.T) {
	r := report()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for _, key := range []string{"version", "go_version", "platform", "git", "git_version", "model_count", "model_vendors", "vendors"} {
		if _, ok := data[key]; !ok {
			t.Errorf("JSON missing key %q: %s", key, b)
		}
	}
	if data["model_count"].(float64) != 31 {
		t.Errorf("model_count = %v, want 31", data["model_count"])
	}
	vendors, _ := data["vendors"].([]any)
	if len(vendors) != 3 {
		t.Errorf("JSON has %d vendors, want 3", len(vendors))
	}
	// Errors that never occurred must be absent, not present as empty strings.
	for _, key := range []string{"git_error", "git_version_error"} {
		if _, ok := data[key]; ok {
			t.Errorf("JSON has unexpected key %q: %s", key, b)
		}
	}
}

// TestReportJSONWithErrors covers the inverse: when git is missing, the error
// key must be present and the version key must be absent.
func TestReportJSONWithErrors(t *testing.T) {
	r := report()
	r.Git = "not found"
	r.GitError = "exec: \"git\": executable file not found"
	r.GitVersion = ""
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if data["git_error"] == nil {
		t.Errorf("JSON missing git_error: %s", b)
	}
	if _, ok := data["git_version"]; ok {
		t.Errorf("JSON has git_version for a missing git: %s", b)
	}
}

// TestReportTextIsDeterministic runs the renderer twice and requires byte
// identical output. Map iteration would have made the vendor order vary; a
// truncated report that changed from run to run is unreadable.
func TestReportTextIsDeterministic(t *testing.T) {
	r := report()
	var a, b bytes.Buffer
	r.Text(&a, 2)
	r.Text(&b, 2)
	if a.String() != b.String() {
		t.Error("text output differs between identical calls")
	}
}
