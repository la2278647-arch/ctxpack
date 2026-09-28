package cli

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/repomap"
)

// testFlagSet mirrors the flags cmdPack registers, since whether reorderArgs
// carries a value depends on which flags are registered.
func testFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var includes stringList
	fs.Var(&includes, "include", "")
	fs.StringVar(new(string), "format", "xml", "")
	fs.StringVar(new(string), "o", "", "")
	fs.StringVar(new(string), "output", "", "")
	fs.StringVar(new(string), "model", "", "")
	fs.StringVar(new(string), "ref", "", "")
	fs.Bool("hidden", false, "")
	fs.Bool("no-gitignore", false, "")
	fs.Int("budget", 0, "")
	fs.Int64("max-size", 0, "")
	return fs
}

// --- reorderArgs ---

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
}

func gitAddAll(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestReorderArgs(t *testing.T) {
	fs := testFlagSet()
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		// The documented usage: path first, flags after.
		{[]string{"./repo", "--format", "markdown"}, []string{"--format", "markdown", "./repo"}},
		// Flags before the path also work.
		{[]string{"--format", "json", "./repo"}, []string{"--format", "json", "./repo"}},
		// --flag=value is already self-contained.
		{[]string{"./repo", "--format=json"}, []string{"--format=json", "./repo"}},
		// -o shorthand carries its value.
		{[]string{"./repo", "-o", "out.md"}, []string{"-o", "out.md", "./repo"}},
		// Repeated multi-value flags.
		{[]string{"./repo", "--include", "*.go", "--include", "*.md"},
			[]string{"--include", "*.go", "--include", "*.md", "./repo"}},
		// Int flag carrying its value.
		{[]string{"./repo", "--budget", "5000", "--format", "text"},
			[]string{"--budget", "5000", "--format", "text", "./repo"}},
		// Interleaved.
		{[]string{"--hidden", "./repo", "--budget", "100", "--format", "json"},
			[]string{"--hidden", "--budget", "100", "--format", "json", "./repo"}},
		// Flag with no value left over.
		{[]string{"./repo", "--format"}, []string{"--format", "./repo"}},
		// Bare positional.
		{[]string{"./repo"}, []string{"./repo"}},
		{[]string{}, []string{}},
		// Multiple positionals stay in order and end up last.
		{[]string{"a", "b", "--budget", "1", "c"}, []string{"--budget", "1", "a", "b", "c"}},
		// A boolean flag does not swallow the path that follows it.
		{[]string{"--hidden", "./repo", "--format", "json"},
			[]string{"--hidden", "--format", "json", "./repo"}},
		// --flag=value is unaffected by the boolean check.
		{[]string{"./repo", "--budget=500"}, []string{"--budget=500", "./repo"}},
	} {
		got := reorderArgs(fs, tc.in)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("reorderArgs(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- parseFormat ---

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"xml", "xml"},
		{"XML", "xml"},
		{"md", "markdown"},
		{"markdown", "markdown"},
		{"MARKDOWN", "markdown"},
		{"json", "json"},
		{"JSON", "json"},
		{"text", "text"},
		{"txt", "text"},
		{"plain", "text"},
		{"raw", "text"},
	} {
		got, err := parseFormat(tc.in)
		if err != nil {
			t.Errorf("parseFormat(%q) errored: %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("parseFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// An unknown format is a mistake, not a reason to guess XML.
	for _, bad := range []string{"", "yaml", "html", "jons", "csv"} {
		if _, err := parseFormat(bad); err == nil {
			t.Errorf("parseFormat(%q) accepted an unknown format", bad)
		}
	}
}

func TestPackRejectsUnknownFormat(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	for _, f := range []string{"yaml", "jons", "csv"} {
		if code := cmdPack([]string{src, "--format", f}); code != 2 {
			t.Errorf("--format %s: exit = %d, want 2", f, code)
		}
	}
}

func TestPackAcceptsValidFormats(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	for _, f := range []string{"xml", "json", "markdown", "text", "md", "txt"} {
		if code := cmdPack([]string{src, "-o", filepath.Join(t.TempDir(), "o."+f), "--format", f}); code != 0 {
			t.Errorf("--format %s: exit = %d, want 0", f, code)
		}
	}
}

// --- number formatting ---

func TestHumanTokens(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{10000, "10.0k"},
		{999999, "1000.0k"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	} {
		if got := humanTokens(tc.n); got != tc.want {
			t.Errorf("humanTokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{53, "53 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{127200, "124.2 KB"},
		{1024 * 1024, "1.0 MB"},
	} {
		if got := humanBytes(tc.n); got != tc.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// --- env defaults ---

func TestEnvStrDefault(t *testing.T) {
	t.Setenv("CTXPACK_FORMAT", "")
	if got := envStrDefault("CTXPACK_FORMAT", "xml"); got != "xml" {
		t.Errorf("unset: got %q, want the fallback", got)
	}
	t.Setenv("CTXPACK_FORMAT", "markdown")
	if got := envStrDefault("CTXPACK_FORMAT", "xml"); got != "markdown" {
		t.Errorf("set: got %q, want markdown", got)
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("CTXPACK_BUDGET", "")
	if got := envInt("CTXPACK_BUDGET"); got != 0 {
		t.Errorf("unset: got %d", got)
	}
	t.Setenv("CTXPACK_BUDGET", "12345")
	if got := envInt("CTXPACK_BUDGET"); got != 12345 {
		t.Errorf("valid: got %d", got)
	}
	t.Setenv("CTXPACK_BUDGET", "not-a-number")
	if got := envInt("CTXPACK_BUDGET"); got != 0 {
		t.Errorf("invalid: got %d, want 0 (the fallback)", got)
	}
}

// --- annotateFit ---

func TestAnnotateFit(t *testing.T) {
	got := annotateFit(5000, "gpt-4o")
	if !strings.HasPrefix(got, "<!-- fit:") {
		t.Errorf("annotateFit = %q, want an HTML comment", got)
	}
	if !strings.Contains(got, "model=gpt-4o") {
		t.Errorf("annotateFit = %q, missing the model name", got)
	}
	// A typo must not pretend to be a fit annotation.
	unknown := annotateFit(5000, "no-such-model-xyz")
	if !strings.Contains(unknown, "unknown model") {
		t.Errorf("annotateFit(unknown) = %q, want an unknown-model note", unknown)
	}
}

// --- writeOutput ---

func TestWriteOutputToFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.xml")
	if err := writeOutput(dest, "<hello/>"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "<hello/>" {
		t.Errorf("wrote %q, want <hello/>", b)
	}
}

func TestWriteOutputRefusesNoDir(t *testing.T) {
	if err := writeOutput(filepath.Join(t.TempDir(), "no", "such", "dir", "f"), "x"); err == nil {
		t.Error("expected an error writing through a missing directory")
	}
}

// --- stringList ---

func TestStringList(t *testing.T) {
	var sl stringList
	if err := sl.Set("a"); err != nil {
		t.Fatal(err)
	}
	if err := sl.Set("b"); err != nil {
		t.Fatal(err)
	}
	if sl.String() != "a,b" {
		t.Errorf("String() = %q", sl.String())
	}
	if len(sl) != 2 {
		t.Errorf("len = %d", len(sl))
	}
}

// --- Run dispatch ---

func TestRunDispatchExitCodes(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"help"}, 0},
		{[]string{"--help"}, 0},
		{[]string{"-h"}, 0},
		{[]string{"help", "pack"}, 0},
		{[]string{"help", "bogus"}, 2},
		{[]string{"help", "--help"}, 0},
		{[]string{"help", "-h"}, 0},
		{[]string{"version"}, 0},
		{[]string{"--version"}, 0},
		{[]string{"-v"}, 0},
		{[]string{"version", "--json"}, 0},
		{[]string{"mcp", "--version"}, 2},
		{[]string{"mcp", "--list"}, 2},
		{[]string{"models"}, 0},
		{[]string{"models", "-h"}, 0},
		{[]string{"totally-unknown"}, 2},
	} {
		if got := Run(tc.args); got != tc.want {
			t.Errorf("Run(%v) = %d, want %d", tc.args, got, tc.want)
		}
	}
}

func TestRunVersionJSON(t *testing.T) {
	outCap := captureStdout(t)
	if code := Run([]string{"version", "--json"}); code != 0 {
		t.Fatalf("Run(version --json) exit = %d", code)
	}
	var v struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		OS      string `json:"os"`
	}
	if err := json.Unmarshal([]byte(outCap.Content()), &v); err != nil {
		t.Fatalf("version --json output is not valid JSON: %v\n%s", err, outCap.Content())
	}
	if v.Name != "ctxpack" || v.Version == "" || v.OS == "" {
		t.Errorf("version --json = %+v, want a complete build identity", v)
	}
	if got := Run([]string{"version", "--json", "--json"}); got != 2 {
		t.Errorf("Run(version --json --json) = %d, want 2 (extra flags rejected)", got)
	}
}

// TestVersionJSONAllAliases pins that the three version spellings accept
// --json and produce byte-identical output, so a script cannot depend on one
// alias and break when a user types another.
func TestVersionJSONAllAliases(t *testing.T) {
	var outs []string
	for _, alias := range []string{"version", "--version", "-v"} {
		outCap := captureStdout(t)
		if code := Run([]string{alias, "--json"}); code != 0 {
			t.Fatalf("Run(%s --json) exit = %d", alias, code)
		}
		out := outCap.Content()
		var v map[string]any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("%s --json is not valid JSON: %v\n%s", alias, err, out)
		}
		if len(outs) > 0 && out != outs[0] {
			t.Errorf("%s --json differs from --version --json:\n%s\nvs\n%s", alias, out, outs[0])
		}
		outs = append(outs, out)
	}
}

// TestCommandDashFooHelpExitsZero pins that `-h` on every subcommand returns 0
// and prints the shared help, instead of the flag parser's default of exit 2.
func TestCommandHelpExitsZero(t *testing.T) {
	for _, cmd := range []string{"pack", "diff", "map", "tokens", "models", "doctor"} {
		for _, flag := range []string{"-h", "--help"} {
			outCap := captureStdout(t)
			var code int
			switch cmd {
			case "pack":
				code = cmdPack([]string{flag})
			case "diff":
				code = cmdDiff([]string{flag})
			case "map":
				code = cmdMap([]string{flag})
			case "tokens":
				code = cmdTokens([]string{flag})
			case "models":
				code = cmdModels([]string{flag})
			case "doctor":
				code = cmdDoctor([]string{flag})
			}
			if code != 0 {
				t.Errorf("cmd%s(%s) = %d, want 0", cmd, flag, code)
			}
			if out := outCap.Content(); !strings.Contains(out, "USAGE") {
				t.Errorf("cmd%s(%s) did not print the help text", cmd, flag)
			}
		}
	}
}

// TestVersionShort pins `version --short`: a bare semantic version on its own
// line, identical across the three aliases, and rejected when combined with
// anything else.
func TestVersionShort(t *testing.T) {
	var outs []string
	for _, alias := range []string{"version", "--version", "-v"} {
		outCap := captureStdout(t)
		if code := Run([]string{alias, "--short"}); code != 0 {
			t.Fatalf("Run(%s --short) exit = %d", alias, code)
		}
		out := strings.TrimSpace(outCap.Content())
		if regexp.MustCompile(`^\d+\.\d+\.\d+$`).FindString(out) == "" {
			t.Errorf("%s --short = %q, want a bare semantic version", alias, out)
		}
		if len(outs) > 0 && out != outs[0] {
			t.Errorf("%s --short differs: %q vs %q", alias, out, outs[0])
		}
		outs = append(outs, out)
	}
	for _, args := range [][]string{{"version", "--short", "--json"}, {"version", "--json", "--short"}, {"version", "--short", "x"}} {
		if code := Run(args); code != 2 {
			t.Errorf("Run(%v) = %d, want 2 (only one of --json/--short)", args, code)
		}
	}
}

// --- pack end to end ---

func writeRepo(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"README.md": "hello\n",
		"src/a.go":  "package a\n",
		"src/b.go":  "package a\n",
		"notes.txt": "notes\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPackWritesFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.json")

	code := cmdPack([]string{src, "-o", out, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if !strings.Contains(string(b), `"files"`) || !strings.Contains(string(b), "src/a.go") {
		t.Errorf("output does not look like a JSON bundle:\n%s", string(b))
	}
}

func TestPackRespectsInclude(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.json")

	code := cmdPack([]string{src, "-o", out, "--format", "json", "--include", "*.go"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "src/a.go") || strings.Contains(s, "README.md") || strings.Contains(s, "notes.txt") {
		t.Errorf("--include *.go leaked non-Go files:\n%s", s)
	}
}

func TestPackAnnotatesModelFit(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.xml")

	code := cmdPack([]string{src, "-o", out, "--format", "xml", "--model", "gpt-4o"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "<!-- fit:") {
		t.Errorf("missing the fit annotation prefix:\n%s", string(b))
	}
}

func TestPackBudgetCapsFiles(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	// With a huge budget everything fits.
	code := cmdPack([]string{src, "-o", filepath.Join(t.TempDir(), "big.json"),
		"--format", "json", "--budget", "999999"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
}

// TestPackBudgetZeroMeansUnlimited pins that an explicit --budget 0 is the
// same as the default: no cap, so nothing is omitted by the budget.
func TestPackBudgetZeroMeansUnlimited(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text", "--budget", "0"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if strings.Contains(strings.ToLower(out), "omitted by budget") {
		t.Errorf("--budget 0 must not omit anything by budget:\n%s", out)
	}
	// Every file the repo holds must be present.
	for _, f := range []string{"README.md", "src/a.go", "src/b.go", "notes.txt"} {
		if !strings.Contains(out, "==== "+f) {
			t.Errorf("--budget 0 pack is missing %s", f)
		}
	}
}

func TestPackMissingPathFails(t *testing.T) {
	code := cmdPack([]string{filepath.Join(t.TempDir(), "no-such-dir"),
		"-o", filepath.Join(t.TempDir(), "x.json")})
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a missing path", code)
	}
}

// TestPackBudgetExcludeCombined pins that --budget selects from what
// --exclude left: the excluded file never competes (the mirror of
// TestPackBudgetIncludeCombined).
func TestPackBudgetExcludeCombined(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("b", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "small.go"), []byte("x"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "json", "--budget", "1", "--exclude", "*.txt"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	var env struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, c.Content())
	}
	if len(env.Files) != 1 || env.Files[0].Path != "small.go" {
		t.Errorf("budget+exclude must pack only small.go, got %+v", env.Files)
	}
}

// TestPackBudgetIncludeCombined pins that --budget selects from the
// --include-filtered set: a file outside the include never enters, so the
// budget works on a pre-narrowed candidate pool.
func TestPackBudgetIncludeCombined(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("b", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "small.go"), []byte("x"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "json", "--budget", "1", "--include", "*.go"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	var env struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, c.Content())
	}
	if len(env.Files) != 1 || env.Files[0].Path != "small.go" {
		t.Errorf("budget+include must pack only small.go, got %+v", env.Files)
	}
}

// TestPackBudgetXMLOmitted pins the command-level xml output under a budget:
// the omitted element names what the budget dropped (the xml sibling of
// TestPackBudgetJSONOmitted).
func TestPackBudgetXMLOmitted(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("b", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "small.go"), []byte("x"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "xml", "--budget", "1"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "<omitted") {
		t.Errorf("xml budget must emit an omitted element:\n%s", out)
	}
	if !strings.Contains(out, "big.txt") {
		t.Errorf("xml omitted element must name the dropped file:\n%s", out)
	}
}

// TestPackBudgetJSONOmitted pins the command-level json envelope under a
// budget: the packed files, plus an omitted list naming what the budget
// dropped, so a script can see both sides of the cut.
func TestPackBudgetJSONOmitted(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("b", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "small.go"), []byte("x"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "json", "--budget", "1"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	var env struct {
		Files         []any `json:"files"`
		Omitted       []any `json:"omitted"`
		OmittedTokens int   `json:"omitted_tokens"`
		TotalTokens   int   `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, c.Content())
	}
	if len(env.Files) != 1 {
		t.Errorf("budget 1 should keep exactly one file, got %d", len(env.Files))
	}
	if len(env.Omitted) != 1 {
		t.Errorf("omitted = %v, want one entry", env.Omitted)
	}
	if env.OmittedTokens <= 0 {
		t.Errorf("omitted_tokens = %d, want > 0", env.OmittedTokens)
	}
	if env.TotalTokens <= 0 {
		t.Errorf("total_tokens = %d, want > 0", env.TotalTokens)
	}
}

// TestPackMaxSizeZeroMeansUnlimited pins that an explicit --max-size 0 reads
// whole files like the default, while a positive cap still lists the file but
// omits its content.
func TestPackMaxSizeZeroMeansUnlimited(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte("hello, world\n"), 0o644)
	os.WriteFile(filepath.Join(src, "small.txt"), []byte("SMALL\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text", "--max-size", "0"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if out := c.Content(); !strings.Contains(out, "hello, world") {
		t.Errorf("--max-size 0 must include file content, got:\n%s", out)
	}

	c = captureStdout(t)
	// big.txt is 13 bytes, small.txt is 6: a 10-byte cap omits the big one's
	// content but keeps the small one's.
	if code := cmdPack([]string{src, "--format", "text", "--max-size", "10"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "==== big.txt") {
		t.Errorf("--max-size 10 must still list big.txt:\n%s", out)
	}
	if strings.Contains(out, "hello, world") {
		t.Errorf("--max-size 10 must omit big.txt's content:\n%s", out)
	}
	if !strings.Contains(out, "SMALL") {
		t.Errorf("--max-size 10 must keep a file under the cap (small.txt is \"SMALL\\n\"):\n%s", out)
	}
}

func TestPackBadFlagFails(t *testing.T) {
	if code := cmdPack([]string{t.TempDir(), "--budget", "not-a-number"}); code != 2 {
		t.Errorf("exit = %d, want 2 for an unparseable flag", code)
	}
}

// TestPackModelAnnotatesXML pins --model at the pack command level: the xml
// output carries a fit comment naming the model, and an unknown model is a
// note, not an error (pack annotates; it does not filter like tokens).
func TestPackModelAnnotatesXML(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--model", "gpt-4o"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "<!-- fit:") || !strings.Contains(out, "model=gpt-4o") {
		t.Errorf("known model must produce a fit comment naming it:\n%.200s", out)
	}

	c = captureStdout(t)
	if code := cmdPack([]string{src, "--model", "no-such-model-xyz"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if out := c.Content(); !strings.Contains(out, "unknown model") {
		t.Errorf("unknown model must produce a note, not silence:\n%.200s", out)
	}
}

// TestPackExcludeFiltersFiles pins --exclude at the pack command level: the
// excluded files stay out of the bundle while everything else comes in.
func TestPackExcludeFiltersFiles(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "a_test.go"), []byte("package a\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text", "--exclude", "*_test.go"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "==== a.go") {
		t.Errorf("--exclude *_test.go must keep a.go:\n%s", out)
	}
	if strings.Contains(out, "==== a_test.go") {
		t.Errorf("--exclude *_test.go must drop a_test.go:\n%s", out)
	}
}

// TestPackIncludeFiltersFiles pins --include at the pack command level: only
// matching files enter the bundle (the walk flags are shared with map, but
// pack's application of them deserves its own integration guard).
func TestPackIncludeFiltersFiles(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("b\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text", "--include", "*.go"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "==== a.go") {
		t.Errorf("--include *.go must pack a.go:\n%s", out)
	}
	if strings.Contains(out, "==== b.txt") {
		t.Errorf("--include *.go must drop b.txt:\n%s", out)
	}
}

// TestPackDepthLimitsBundle pins pack --depth: a two-level directory is not
// traversed at depth 1, so its file stays out of the bundle (the pack-side
// mirror of TestMapDepthLimitsNestedDirs / TestTokensDepthLimitsTotal).
func TestPackDepthLimitsBundle(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "deep", "b.txt"), []byte("deep\n"), 0o644)
	os.WriteFile(filepath.Join(src, "root.go"), []byte("package main\n"), 0o644)

	full := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	shallow := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text", "--depth", "1"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if !strings.Contains(full.Content(), "sub/deep/b.txt") {
		t.Fatalf("full pack must include the deep file:\n%s", full.Content())
	}
	if strings.Contains(shallow.Content(), "b.txt") {
		t.Errorf("--depth 1 must keep the deep file out of the bundle:\n%s", shallow.Content())
	}
	if !strings.Contains(shallow.Content(), "root.go") {
		t.Errorf("--depth 1 must keep the root file:\n%s", shallow.Content())
	}
}

// TestPackRespectsGitignoreByDefault pins the default gitignore handling at
// the pack level (same walker as map, own integration guard): an excluded
// file stays out of the bundle.
func TestPackRespectsGitignoreByDefault(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(src, "secret.txt"), []byte("s\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "text"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if strings.Contains(out, "==== secret.txt") {
		t.Errorf("default pack must respect .gitignore:\n%s", out)
	}
	if !strings.Contains(out, "==== keep.go") {
		t.Errorf("default pack must keep normal files:\n%s", out)
	}
}

// TestPackOutputToDirectoryFails pins that -o pointing at an existing
// directory is a runtime error (exit 1), never a truncation of the directory.
func TestPackOutputToDirectoryFails(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	os.MkdirAll(filepath.Join(src, "outdir"), 0o755)

	errCap := captureStderr(t)
	if code := cmdPack([]string{src, "-o", filepath.Join(src, "outdir")}); code != 1 {
		t.Fatalf("cmdPack exit = %d, want 1", code)
	}
	if !strings.Contains(errCap.Content(), "ctxpack:") {
		t.Errorf("stderr should carry the error:\n%s", errCap.Content())
	}
}

// TestPackOutputOverwritesExisting pins that -o overwrites an existing file
// silently (os.Create truncates; no prompt, no error), so re-runs are safe.
func TestPackOutputOverwritesExisting(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	out := filepath.Join(src, "out.json")
	os.WriteFile(out, []byte("old"), 0o644)

	if code := cmdPack([]string{src, "--format", "json", "-o", out}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("output must be overwritten with the new json: %v\n%s", err, data)
	}
}

// TestPackOutputDashIsStdout pins that pack honors --output - as stdout, the
// last of the output-dash trio (models and diff already pinned): no file
// named "-" is created and the json lands on stdout.
func TestPackOutputDashIsStdout(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "json", "-o", "-"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Errorf("-o - must print the json to stdout: %v\n%s", err, c.Content())
	}
	if _, err := os.Stat(filepath.Join(src, "-")); err == nil {
		t.Errorf("-o - must not create a file named '-'")
	}
}

// TestPackEmptyDirYieldsEmptyBundle pins that packing an empty directory is
// not an error: a well-formed envelope with no files and zero tokens, exit 0.
func TestPackEmptyDirYieldsEmptyBundle(t *testing.T) {
	src := t.TempDir()
	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "json"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	var env struct {
		Files       *[]any `json:"files"`
		TotalTokens int    `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, c.Content())
	}
	got := 0
	if env.Files != nil {
		got = len(*env.Files)
	}
	if got != 0 || env.TotalTokens != 0 {
		t.Errorf("empty dir pack = %d files, %d tokens; want 0, 0", got, env.TotalTokens)
	}
}

// --- diff requires git ---

// TestDiffBadRefFails pins that an unresolvable --ref reports the git error.
func TestDiffEmptyRepoListsUntrackedWorktree(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "f.txt"), []byte("hi\n"), 0o644)
	gitInit(t, src) // no commits: HEAD does not exist

	for _, args := range [][]string{
		{src, "--list"},
		{src, "--list", "--ref", "HEAD"},
	} {
		outCap := captureStdout(t)
		if code := cmdDiff(args); code != 0 {
			t.Fatalf("cmdDiff(%v) exit = %d, want 0 in a repo with no commits", args, code)
		}
		out := outCap.Content()
		if !strings.Contains(out, "f.txt") {
			t.Errorf("cmdDiff(%v) must list the untracked file in an empty repo:\n%s", args, out)
		}
	}
}

func TestDiffBadRefFails(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")

	errCap := captureStderr(t)
	code := cmdDiff([]string{src, "--ref", "not-a-real-ref"})
	if code == 0 {
		t.Fatal("cmdDiff with an unresolvable ref must fail")
	}
	if !strings.Contains(errCap.Content(), "ctxpack:") {
		t.Errorf("bad ref did not report the git error on stderr:\n%s", errCap.Content())
	}
}

// TestDiffRefHEADEqualsWorktree pins that `--ref HEAD` is the same as the
// default (working-tree changes, incl. untracked files) on a repo with
// commits — gitutil special-cases HEAD alongside "" and "WORKTREE".
func TestDiffRefHEADEqualsWorktree(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "untracked.txt"), []byte("new\n"), 0o644)

	dflt := captureStdout(t)
	if code := cmdDiff([]string{src, "--list"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	head := captureStdout(t)
	if code := cmdDiff([]string{src, "--list", "--ref", "HEAD"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if !strings.Contains(dflt.Content(), "untracked.txt") ||
		!strings.Contains(head.Content(), "untracked.txt") {
		t.Errorf("both default and --ref HEAD must list the untracked file:\ndefault:\n%s\nhead:\n%s",
			dflt.Content(), head.Content())
	}
}

// TestDiffNoChangesJSONEmitsEnvelope pins that an empty diff still yields a
// parseable json envelope on stdout (never a bare stderr note), so a script
// can always json-parse the output; --quiet silences the note.
func TestDiffNoChangesJSONEmitsEnvelope(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src) // no files, no commits: nothing changed vs WORKTREE

	c := captureStdout(t)
	errCap := captureStderr(t)
	if code := cmdDiff([]string{src, "--format", "json"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var env struct {
		Files *[]any `json:"files"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("empty diff json must be a parseable envelope: %v\n%s", err, c.Content())
	}
	if !strings.Contains(errCap.Content(), "no changed files") {
		t.Errorf("stderr should carry the no-changes note:\n%s", errCap.Content())
	}

	c2 := captureStdout(t)
	errCap2 := captureStderr(t)
	if code := cmdDiff([]string{src, "--format", "json", "--quiet"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if err := json.Unmarshal([]byte(c2.Content()), &env); err != nil {
		t.Fatalf("quiet empty diff must still emit the envelope: %v", err)
	}
	if errCap2.Content() != "" {
		t.Errorf("--quiet must silence the no-changes note:\n%s", errCap2.Content())
	}
}

// TestDiffNoChangesTextNotesOnStderr pins the non-json empty diff: a stderr
// note, empty stdout, exit 0 — scripts parsing stdout for text stay empty,
// while the human sees why.
func TestDiffNoChangesTextNotesOnStderr(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src) // no files, no commits

	c := captureStdout(t)
	errCap := captureStderr(t)
	if code := cmdDiff([]string{src}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if c.Content() != "" {
		t.Errorf("text empty diff should leave stdout empty, got:\n%s", c.Content())
	}
	if !strings.Contains(errCap.Content(), "no changed files") {
		t.Errorf("stderr should carry the note:\n%s", errCap.Content())
	}

	// --quiet silences the note for text too, like the json path.
	errCap2 := captureStderr(t)
	if code := cmdDiff([]string{src, "--quiet"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if errCap2.Content() != "" {
		t.Errorf("--quiet must silence the empty-diff note:\n%s", errCap2.Content())
	}
}

// TestDiffSingleRefIncludesWorktree pins that a single revision ref (not a
// range) diffs against the working tree, so untracked files are included —
// the mirror of TestDiffRangeExcludesWorktree.
func TestDiffSingleRefIncludesWorktree(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("one\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "one")
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("two\n"), 0o644)
	gitAddAll(t, src)
	gitCommit(t, src, "two")
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("one\nthree\n"), 0o644)
	os.WriteFile(filepath.Join(src, "untracked.txt"), []byte("new\n"), 0o644)

	c := captureStdout(t)
	if code := cmdDiff([]string{src, "--ref", "HEAD~1", "--list"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "a.txt") {
		t.Errorf("single ref must include the working-tree change:\n%s", out)
	}
	if !strings.Contains(out, "untracked.txt") {
		t.Errorf("single ref must include untracked files:\n%s", out)
	}
}

// TestDiffRangeExcludesWorktree pins that a two-revision range (`--ref A..B`)
// compares the two commits only: a file changed in that window is listed,
// while an untracked working-tree file is not — the range semantics that feed
// `diff --ref HEAD~1..HEAD` from a script.
func TestDiffRangeExcludesWorktree(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("one\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "one")
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("two\n"), 0o644)
	gitAddAll(t, src)
	gitCommit(t, src, "two")
	os.WriteFile(filepath.Join(src, "untracked.txt"), []byte("new\n"), 0o644)

	outCap := captureStdout(t)
	if code := cmdDiff([]string{src, "--ref", "HEAD~1..HEAD", "--list"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "b.txt") {
		t.Errorf("range must list the file added in that window:\n%s", out)
	}
	if strings.Contains(out, "untracked.txt") {
		t.Errorf("range must exclude working-tree files:\n%s", out)
	}
}

func TestDiffOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	if code := cmdDiff([]string{dir}); code != 1 {
		t.Errorf("exit = %d, want 1 outside a git repo", code)
	}
}

// TestPackPlainDirectoryWorks pins that pack (unlike diff) does not need git:
// any directory — a plain folder, a downloaded archive, a temp dir — packs
// fine, so the tool stays useful where git never was.
func TestPackPlainDirectoryWorks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{dir, "--format", "text"}); code != 0 {
		t.Fatalf("cmdPack exit = %d, want 0 for a plain directory", code)
	}
	out := c.Content()
	if !strings.Contains(out, "==== a.go") {
		t.Errorf("plain-directory pack must include the file:\n%s", out)
	}
}

// TestDiffDryRunBudgetShowsOmitted pins that diff --dry-run --budget reports
// what the budget dropped in the dry-run summary.
func TestDiffDryRunBudgetShowsOmitted(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "f.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	errCap := captureStderr(t)
	if code := cmdDiff([]string{src, "--dry-run", "--budget", "1"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "dry run") {
		t.Errorf("stderr missing the dry-run header:\n%s", out)
	}
	if !strings.Contains(out, "omitted by the budget") {
		t.Errorf("stderr missing the omitted summary:\n%s", out)
	}
}

func TestDiffDryRunShowsFiles(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	// Make a change so there are changed files.
	os.WriteFile(filepath.Join(src, "new.go"), []byte("package main\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	errCap := captureStderr(t)
	code := cmdDiff([]string{src, "--dry-run"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "dry run") {
		t.Errorf("stderr missing 'dry run'\n%s", out)
	}
	if !strings.Contains(out, "1 file") {
		t.Errorf("stderr missing the singular file count '1 file':\n%s", out)
	}
	if strings.Contains(out, "1 files") {
		t.Errorf("stderr must not say '1 files':\n%s", out)
	}
}

// TestDiffDryRunQuietKeepsReport pins that --quiet does not swallow diff's
// dry-run report either (mirror of TestPackDryRunQuietKeepsReport): quiet
// silences status lines, while a dry run's report is the command's output.
func TestDiffDryRunQuietKeepsReport(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	errCap := captureStderr(t)
	if code := cmdDiff([]string{src, "--dry-run", "--quiet"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "dry run vs") {
		t.Errorf("--dry-run --quiet must keep the report on stderr:\n%s", out)
	}
}

// TestDiffMaxSizeZeroUnlimited pins that diff --max-size 0 is the same as the
// default (read everything), closing out the max-size 0 series across
// pack/tokens/map/diff.
func TestDiffMaxSizeZeroUnlimited(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "f.txt"), []byte(strings.Repeat("b", 500)), 0o644)

	dflt := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	zero := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json", "--max-size", "0"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var envDefault, envZero struct {
		TotalTokens int `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(dflt.Content()), &envDefault); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(zero.Content()), &envZero); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envZero.TotalTokens != envDefault.TotalTokens {
		t.Errorf("--max-size 0 must equal the default total (%d), got %d",
			envDefault.TotalTokens, envZero.TotalTokens)
	}
}

// TestDiffMaxSizeOmitsContent pins diff --max-size: a changed file over the
// cap is still in the diff, but its body is not read, so the token total
// drops far below the full-read count.
func TestDiffMaxSizeOmitsContent(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "f.txt"), []byte(strings.Repeat("b", 500)), 0o644)

	full := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	capped := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json", "--max-size", "10"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var envFull, envCapped struct {
		TotalTokens int `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(full.Content()), &envFull); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(capped.Content()), &envCapped); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envCapped.TotalTokens >= envFull.TotalTokens {
		t.Errorf("--max-size 10 must cut the total below %d, got %d",
			envFull.TotalTokens, envCapped.TotalTokens)
	}
}

// TestDiffListHiddenExcludesByDefault pins diff's hidden handling: a modified
// dotfile is out of the default list and in with --hidden (the same walker
// semantics map has, at the diff layer).
func TestDiffListHiddenExcludesByDefault(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, ".hidden.txt"), []byte("x\n"), 0o644)
	gitAddAll(t, src)
	gitCommit(t, src, "dotfile")
	os.WriteFile(filepath.Join(src, ".hidden.txt"), []byte("x\ny\n"), 0o644)

	dflt := captureStdout(t)
	if code := cmdDiff([]string{src, "--list"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	hidden := captureStdout(t)
	if code := cmdDiff([]string{src, "--list", "--hidden"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if strings.Contains(dflt.Content(), ".hidden.txt") {
		t.Errorf("default diff must exclude the dotfile:\n%s", dflt.Content())
	}
	if !strings.Contains(hidden.Content(), ".hidden.txt") {
		t.Errorf("--hidden diff must list the dotfile:\n%s", hidden.Content())
	}
}

// TestDiffDepthLimitsNested pins diff --depth's boundary: a change two levels
// deep is out of the list at depth 1 and in at depth 2 (one-level changes are
// in at depth 1, matching the map/tokens semantics).
func TestDiffDepthLimitsNested(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "deep", "x.txt"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "a.go"), []byte("package a\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "sub", "deep", "x.txt"), []byte("x\ny\n"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "a.go"), []byte("package a\n\nvar a = 1\n"), 0o644)

	shallow := captureStdout(t)
	if code := cmdDiff([]string{src, "--list", "--depth", "1"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := shallow.Content()
	if !strings.Contains(out, "sub/a.go") {
		t.Errorf("--depth 1 must list the one-level change:\n%s", out)
	}
	if strings.Contains(out, "deep/x.txt") {
		t.Errorf("--depth 1 must not list the two-level change:\n%s", out)
	}

	deeper := captureStdout(t)
	if code := cmdDiff([]string{src, "--list", "--depth", "2"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if !strings.Contains(deeper.Content(), "deep/x.txt") {
		t.Errorf("--depth 2 must list the two-level change:\n%s", deeper.Content())
	}
}

// TestDiffBudgetJSONOmitted pins the diff json envelope under a budget: the
// dropped change is named in the omitted list, so a script sees both sides of
// the cut (the diff-side mirror of TestPackBudgetJSONOmitted).
func TestDiffBudgetJSONOmitted(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "f.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	c := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json", "--budget", "1"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var env struct {
		Files         *[]any `json:"files"`
		Omitted       []any  `json:"omitted"`
		OmittedTokens int    `json:"omitted_tokens"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, c.Content())
	}
	got := 0
	if env.Files != nil {
		got = len(*env.Files)
	}
	if got != 0 {
		t.Errorf("budget 1 must drop the changed file; files = %d", got)
	}
	if len(env.Omitted) != 1 {
		t.Errorf("omitted = %v, want one entry", env.Omitted)
	}
	if env.OmittedTokens <= 0 {
		t.Errorf("omitted_tokens = %d, want > 0", env.OmittedTokens)
	}
}

// TestDiffBudgetTinyDropsFiles pins diff --budget: a budget too small for
// any changed file yields an empty bundle (exit 0, not an error), while a
// generous budget keeps every changed file — the same priority-selection
// contract pack has.
func TestDiffBudgetTinyDropsFiles(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "f.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	caseFor := func(budget, wantFiles int) {
		c := captureStdout(t)
		args := []string{src, "--format", "json"}
		if budget >= 0 {
			args = append(args, "--budget", strconv.Itoa(budget))
		}
		if code := cmdDiff(args); code != 0 {
			t.Fatalf("cmdDiff(%v) exit = %d", args, code)
		}
		var env struct {
			Files *[]any `json:"files"`
		}
		if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, c.Content())
		}
		got := 0
		if env.Files != nil {
			got = len(*env.Files)
		}
		if got != wantFiles {
			t.Errorf("budget=%v: %d files, want %d", budget, got, wantFiles)
		}
	}

	caseFor(1, 0)  // too small for any file
	caseFor(0, 1)  // 0 means no cap, like pack --budget 0
	caseFor(-1, 1) // no budget: everything
}

// TestDiffOutputDashIsStdout pins that diff honors --output - as stdout (the
// mirror of TestModelsOutputDashIsStdout): a file named "-" must never be
// created, and the json lands on stdout.
func TestDiffOutputDashIsStdout(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	c := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json", "-o", "-"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Errorf("-o - must print the json to stdout: %v\n%s", err, c.Content())
	}
	if _, err := os.Stat(filepath.Join(src, "-")); err == nil {
		t.Errorf("-o - must not create a file named '-'")
	}
}

// TestDiffOutputWritesFile pins the -o file branch for diff, mirroring the
// --output tests the other commands have: json goes to the file, exit 0.
func TestDiffOutputWritesFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	out := filepath.Join(t.TempDir(), "diff.json")
	code := cmdDiff([]string{src, "--format", "json", "-o", out})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("output file is not valid JSON: %v\n%s", err, data)
	}
}

func TestDiffDryRunNoOutputFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "new.go"), []byte("package main\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	out := filepath.Join(t.TempDir(), "diff.json")
	code := cmdDiff([]string{src, "--dry-run", "-o", out})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("dry run should not create output file, but %s exists", out)
	}
}

func TestDiffListOnly(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "new.go"), []byte("package main\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--list"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "modified.go") {
		t.Errorf("stdout missing modified.go:\n%s", out)
	}
	if strings.Contains(out, "tokens") {
		t.Errorf("list mode should not show tokens:\n%s", out)
	}
}

// TestDiffListIgnoresFormat pins that --list wins over --format: it names
// paths (for `while read f; do` loops), so a JSON request must not make it
// emit an envelope instead.
func TestDiffListIgnoresFormat(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "new.go"), []byte("package main\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	outCap := captureStdout(t)
	if code := cmdDiff([]string{src, "--list", "--format", "json"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "modified.go") {
		t.Errorf("--list --format json must still name the file:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		// Expected: plain path lines, no JSON envelope.
		return
	}
	t.Errorf("--list --format json emitted a JSON envelope; list must win:\n%s", out)
}

// A deletion has no content to pack, but a diff must still report it: a removed
// file that the bundle never mentions would look like it never existed.
func TestDiffListHonoursInclude(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "new.go"), []byte("package main\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	// Two changed files in src/, one elsewhere.
	os.WriteFile(filepath.Join(src, "src", "a.go"), []byte("package a\n\nvar a = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "src", "b.go"), []byte("package a\n\nvar b = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("notes\n\nmore notes\n"), 0o644)

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--list", "--include", "src/*"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "src/a.go") || !strings.Contains(out, "src/b.go") {
		t.Errorf("expected both src changes in the list:\n%s", out)
	}
	if strings.Contains(out, "notes.txt") {
		t.Errorf("--include src/* must exclude notes.txt:\n%s", out)
	}
	if strings.Contains(out, "new.go") {
		t.Errorf("new.go is not changed and must not be listed:\n%s", out)
	}
}

// TestDiffListIncludeExcludeCombined pins that --include and --exclude work
// together on diff --list: the include narrows the candidate set and the
// exclude carves it further (the mirror of the map walker conflict test).
func TestDiffListIncludeExcludeCombined(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n\nvar a = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "a_test.go"), []byte("package a\n\nvar t = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("notes\n\nmore\n"), 0o644)

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--list", "--include", "*.go", "--exclude", "*_test.go"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "a.go") {
		t.Errorf("combined filters must keep a.go:\n%s", out)
	}
	if strings.Contains(out, "a_test.go") {
		t.Errorf("exclude must drop a_test.go:\n%s", out)
	}
	if strings.Contains(out, "notes.txt") {
		t.Errorf("include must drop notes.txt:\n%s", out)
	}
}

// TestDiffListHonoursExclude pins diff --list --exclude: a dropped glob keeps
// the file out of the list while the rest stays (the mirror of the include test).
func TestDiffListHonoursExclude(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n\nvar x = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "drop.txt"), []byte("notes\n\nmore\n"), 0o644)

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--list", "--exclude", "*.txt"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "keep.go") {
		t.Errorf("--exclude *.txt must keep keep.go:\n%s", out)
	}
	if strings.Contains(out, "drop.txt") {
		t.Errorf("--exclude *.txt must drop drop.txt:\n%s", out)
	}
}

func TestDiffListReportsDeletions(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "gone.txt"), []byte("gone\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)
	os.Remove(filepath.Join(src, "gone.txt"))

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--list"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "modified.go") {
		t.Errorf("stdout missing the modified file:\n%s", out)
	}
	if !strings.Contains(out, "gone.txt") {
		t.Errorf("--list must name the deleted file too:\n%s", out)
	}
}

func TestDiffListNothingInScope(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	outCap := captureStdout(t)
	errCap := captureStderr(t)
	code := cmdDiff([]string{src, "--list", "--exclude", "*"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if out := outCap.Content(); out != "" {
		t.Errorf("nothing in scope, so stdout must be empty:\n%s", out)
	}
	if out := errCap.Content(); !strings.Contains(out, "no changed files in scope") {
		t.Errorf("stderr should say nothing is in scope:\n%s", out)
	}
	if out := errCap.Content(); !strings.Contains(out, "--include / --exclude") {
		t.Errorf("stderr should point at the filters:\n%s", out)
	}
}

func TestDiffListMatchesPackedFiles(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "src", "a.go"), []byte("package a\n\nvar a = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "src", "b.go"), []byte("package a\n\nvar b = 1\n"), 0o644)

	listCap := captureStdout(t)
	if code := cmdDiff([]string{src, "--list"}); code != 0 {
		t.Fatalf("cmdDiff --list exit = %d", code)
	}
	var listed []string
	for _, line := range strings.Split(strings.TrimSpace(listCap.Content()), "\n") {
		if line != "" {
			listed = append(listed, line)
		}
	}

	outCap := captureStdout(t)
	if code := cmdDiff([]string{src, "--format", "json"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var bundle struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
		Deleted []string `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(outCap.Content()), &bundle); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	packed := append([]string{}, bundle.Deleted...)
	for _, f := range bundle.Files {
		packed = append(packed, f.Path)
	}
	sort.Strings(packed)
	sort.Strings(listed)
	if !reflect.DeepEqual(listed, packed) {
		t.Errorf("--list = %v, pack paths = %v: the two must agree", listed, packed)
	}
}

// A diff that filters one changed file out must tell the reader the packed
// count, not the pre-filter count: <fileCount> is the packed count, so the
// header has to say the same thing.
func TestDiffHeaderCountsPackedFiles(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "src", "a.go"), []byte("package a\n\nvar a = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "src", "b.go"), []byte("package a\n\nvar b = 1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("notes\n\nmore\n"), 0o644)

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--include", "src/*"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	header := regexp.MustCompile(`<!-- ctxpack diff vs "[^"]+": (\d+) files? -->`).FindStringSubmatch(out)
	if header == nil {
		t.Fatalf("output has no diff header:\n%s", out)
	}
	headerCount, _ := strconv.Atoi(header[1])
	if !regexp.MustCompile(`<fileCount>2</fileCount>`).MatchString(out) {
		t.Errorf("fileCount is not 2 with --include src/*:\n%s", out)
	}
	if headerCount != 2 {
		t.Errorf("header says %d files, <fileCount> says 2: they must agree:\n%s",
			headerCount, out)
	}
}

func TestDiffReportsDeletions(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "new.go"), []byte("package main\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)
	os.Remove(filepath.Join(src, "new.go"))

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--format", "markdown"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := outCap.Content()
	if !strings.Contains(out, "modified.go") {
		t.Errorf("stdout missing the modified file:\n%s", out)
	}
	if !strings.Contains(out, "## Deleted (1 file)") {
		t.Errorf("stdout missing the deleted section:\n%s", out)
	}
	if strings.Contains(out, "## Deleted (1 files)") {
		t.Errorf("deleted section must not pluralise a single file:\n%s", out)
	}
	if !strings.Contains(out, "`new.go`") {
		t.Errorf("deleted section missing new.go:\n%s", out)
	}
}

func TestDiffJSONCarriesDeletions(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "gone.go"), []byte("package gone\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)
	os.Remove(filepath.Join(src, "gone.go"))

	outCap := captureStdout(t)
	code := cmdDiff([]string{src, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	var bundle struct {
		Files   []struct{ Path string } `json:"files"`
		Deleted []string                `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(outCap.Content()), &bundle); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(bundle.Deleted) != 1 || bundle.Deleted[0] != "gone.go" {
		t.Errorf("deleted = %v, want [gone.go]", bundle.Deleted)
	}
}

func TestDiffDryRunListsDeletions(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	os.WriteFile(filepath.Join(src, "gone.go"), []byte("package gone\n"), 0o644)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)
	os.Remove(filepath.Join(src, "gone.go"))

	errCap := captureStderr(t)
	code := cmdDiff([]string{src, "--dry-run"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "1 file deleted") {
		t.Errorf("dry run missing the singular deletion count:\n%s", out)
	}
	if strings.Contains(out, "1 files deleted") {
		t.Errorf("dry run must not pluralise a single deletion:\n%s", out)
	}
	if !strings.Contains(out, "D gone.go") {
		t.Errorf("dry run missing the deleted path:\n%s", out)
	}
}

// --- quiet ---

// TestDiffQuietSuppressesStatus pins --quiet on diff: the "ctxpack diff vs
// <ref>" status line goes to stderr and --quiet silences it, while the stdout
// JSON stays intact — the README's "diff --quiet still emits its JSON" claim.
func TestDiffQuietSuppressesStatus(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")
	os.WriteFile(filepath.Join(src, "modified.go"), []byte("package main\n\nvar x = 1\n"), 0o644)

	outCap := captureStdout(t)
	errCap := captureStderr(t)
	if code := cmdDiff([]string{src, "--quiet", "--format", "json"}); code != 0 {
		t.Fatalf("cmdDiff exit = %d", code)
	}
	if err := errCap.Content(); err != "" {
		t.Errorf("with --quiet, stderr should be empty, got:\n%s", err)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(outCap.Content()), &env); err != nil {
		t.Errorf("--quiet must not break the json output: %v", err)
	}
}

// TestPackQuietTextSuppressesWrote pins that --quiet silences the wrote line
// for the text format too, closing out the quiet format series across
// json/xml/markdown/text.
func TestPackQuietTextSuppressesWrote(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.txt")

	errCap := captureStderr(t)
	if code := cmdPack([]string{src, "-o", out, "--format", "text", "--quiet"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if got := errCap.Content(); got != "" {
		t.Errorf("with --quiet, stderr should be empty, got:\n%s", got)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("the output file must still be written: %v", err)
	}
}

// TestPackQuietMarkdownSuppressesWrote pins that --quiet silences the wrote
// line for the markdown format too (json and xml are covered by the sibling
// tests), closing out the quiet format series.
func TestPackQuietMarkdownSuppressesWrote(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.md")

	errCap := captureStderr(t)
	if code := cmdPack([]string{src, "-o", out, "--format", "markdown", "--quiet"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if got := errCap.Content(); got != "" {
		t.Errorf("with --quiet, stderr should be empty, got:\n%s", got)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("the output file must still be written: %v", err)
	}
}

// TestPackQuietXMLSuppressesWrote pins that --quiet silences the wrote line
// for the xml format too (the json path is covered by
// TestPackQuietSuppressesWrote).
func TestPackQuietXMLSuppressesWrote(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.xml")

	errCap := captureStderr(t)
	if code := cmdPack([]string{src, "-o", out, "--format", "xml", "--quiet"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if got := errCap.Content(); got != "" {
		t.Errorf("with --quiet, stderr should be empty, got:\n%s", got)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("the output file must still be written: %v", err)
	}
}

func TestPackQuietSuppressesWrote(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.json")

	// Without --quiet: the "wrote" message goes to stderr.
	errCap := captureStderr(t)
	code := cmdPack([]string{src, "-o", out, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if !strings.Contains(errCap.Content(), "wrote") {
		t.Errorf("without --quiet, stderr missing 'wrote':\n%s", errCap.Content())
	}

	// With --quiet: stderr must be empty.
	errCap = captureStderr(t)
	code = cmdPack([]string{src, "-o", out, "--format", "json", "--quiet"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d with --quiet", code)
	}
	if got := errCap.Content(); got != "" {
		t.Errorf("with --quiet, stderr should be empty, got %d bytes:\n%s", len(got), got)
	}
}

func TestPackQuietShortFlag(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.json")

	errCap := captureStderr(t)
	code := cmdPack([]string{src, "-o", out, "-q"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d with -q", code)
	}
	if got := errCap.Content(); got != "" {
		t.Errorf("with -q, stderr should be empty, got %d bytes:\n%s", len(got), got)
	}
}

// TestPackDryRunExcludeFilters pins that pack --dry-run --exclude reports the
// count after the exclusion (the mirror of TestPackDryRunIncludeFilters).
func TestPackDryRunExcludeFilters(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	errCap := captureStderr(t)
	if code := cmdPack([]string{src, "--dry-run", "--exclude", "*.txt"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "1 file") {
		t.Errorf("dry-run exclude *.txt must report 1 file:\n%s", out)
	}
	if strings.Contains(out, "big.txt") {
		t.Errorf("dry-run exclude *.txt must not list the excluded file:\n%s", out)
	}
}

// TestPackDryRunIncludeFilters pins that pack --dry-run --include reports the
// filtered file count, so the summary reflects what would actually pack.
func TestPackDryRunIncludeFilters(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	errCap := captureStderr(t)
	if code := cmdPack([]string{src, "--dry-run", "--include", "*.go"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "1 file") {
		t.Errorf("dry-run include *.go must report 1 file:\n%s", out)
	}
	if strings.Contains(out, "big.txt") {
		t.Errorf("dry-run include *.go must not list the excluded file:\n%s", out)
	}
}

func TestPackDryRunShowsFiles(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errCap := captureStderr(t)

	code := cmdPack([]string{src, "--dry-run"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "dry run:") {
		t.Errorf("stderr missing 'dry run:'\n%s", out)
	}
	if !strings.Contains(out, "files,") {
		t.Errorf("stderr missing file count\n%s", out)
	}
}

// TestPackDryRunQuietKeepsReport pins that --quiet does not swallow the
// dry-run report: quiet silences the "wrote" status line for --output, while
// a dry run's report is the command's main output, so it stays on stderr.
func TestPackDryRunQuietKeepsReport(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errCap := captureStderr(t)

	if code := cmdPack([]string{src, "--dry-run", "--quiet"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "dry run:") {
		t.Errorf("--dry-run --quiet must keep the report on stderr:\n%s", out)
	}
}

func TestPackDryRunNoOutputFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "bundle.json")

	code := cmdPack([]string{src, "--dry-run", "-o", out})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("dry run should not create output file, but %s exists", out)
	}
}

func TestPackDryRunWithBudget(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errCap := captureStderr(t)

	code := cmdPack([]string{src, "--dry-run", "--budget", "10"})
	if code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := errCap.Content()
	if !strings.Contains(out, "omitted") {
		t.Errorf("stderr missing 'omitted' for budget-constrained dry run\n%s", out)
	}
}

// --- tokens --model ---

func TestTokensModelShowsOneModel(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--model", "gpt-4o"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "gpt-4o") {
		t.Errorf("output missing gpt-4o:\n%s", out)
	}
	if strings.Contains(out, "Per-model fit") {
		t.Errorf("with --model, the 'Per-model fit' header should be omitted:\n%s", out)
	}
	// Count model lines: there should be exactly one.
	lines := strings.Split(out, "\n")
	count := 0
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 model line with --model, got %d:\n%s", count, out)
	}
}

func TestTokensModelUnknownFails(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errCap := captureStderr(t)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--model", "not-a-real-model"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown model", code)
	}
	if !strings.Contains(errCap.Content(), "unknown model") {
		t.Errorf("stderr missing 'unknown model':\n%s", errCap.Content())
	}
	if c.Content() != "" {
		t.Errorf("stdout should be empty on failure:\n%s", c.Content())
	}
}

func TestTokensModelJSONFilters(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--json", "--model", "gpt-4o"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v\n%s", err, c.Content())
	}
	if len(env.Fits) != 1 {
		t.Fatalf("expected 1 fit entry with --model, got %d", len(env.Fits))
	}
	if env.Fits[0].Model != "gpt-4o" {
		t.Errorf("fit model = %q, want gpt-4o", env.Fits[0].Model)
	}
}

// TestTokensModelUnknownCSV pins that an unknown --model in csv mode behaves
// exactly like json mode — an empty result, exit 0 — so a script can rely on
// the shape of the output rather than a special-case exit code. (The text
// mode deliberately differs: it validates the name and exits 2.)
func TestTokensModelUnknownCSV(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src, "--csv", "--model", "not-a-model"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d, want 0 for an unknown model in csv mode", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != 1 || lines[0] != "model,used,limit,fits,pct_used" {
		t.Fatalf("unknown --model in csv mode should print only the header, got:\n%s", c.Content())
	}
}

// --- models --vendor ---

func TestModelsVendorShowsOnlyThatVendor(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--vendor", "anthropic"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "claude-3-haiku") {
		t.Errorf("missing anthropic model:\n%s", out)
	}
	if strings.Contains(out, "gpt-4o") {
		t.Errorf("leaked openai model:\n%s", out)
	}
	if strings.Contains(out, "gemini") {
		t.Errorf("leaked google model:\n%s", out)
	}
}

func TestModelsVendorCaseInsensitive(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--vendor", "OpenAI"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "gpt-4o") {
		t.Errorf("missing openai model (case-insensitive):\n%s", out)
	}
	if strings.Contains(out, "claude") {
		t.Errorf("leaked anthropic model:\n%s", out)
	}
}

func TestModelsVendorUnknownFails(t *testing.T) {
	errCap := captureStderr(t)
	c := captureStdout(t)

	code := cmdModels([]string{"--vendor", "nonexistent"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown vendor", code)
	}
	if !strings.Contains(errCap.Content(), "no models for vendor") {
		t.Errorf("stderr missing 'no models for vendor':\n%s", errCap.Content())
	}
	if c.Content() != "" {
		t.Errorf("stdout should be empty on failure:\n%s", c.Content())
	}
}

func TestModelsVendorJSONFilters(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--json", "--vendor", "meta"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	var env modelsEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v\n%s", err, c.Content())
	}
	if len(env.Models) != 2 {
		t.Fatalf("expected 2 models for meta, got %d", len(env.Models))
	}
	if env.Models[0].Vendor != "meta" {
		t.Errorf("vendor = %q, want meta", env.Models[0].Vendor)
	}
}

// TestModelsVendorSortWindow pins that a --sort given alongside --vendor
// re-orders the filtered set, not the whole table: every returned model is
// from the vendor, and windows are non-increasing (with name tiebreak).
func TestModelsVendorSortWindow(t *testing.T) {
	c := captureStdout(t)

	if code := cmdModels([]string{"--json", "--vendor", "anthropic", "--sort", "window"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	var env modelsEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v\n%s", err, c.Content())
	}
	if len(env.Models) == 0 {
		t.Fatal("no anthropic models")
	}
	prev := int(^uint(0) >> 1) // max int
	for _, m := range env.Models {
		if m.Vendor != "anthropic" {
			t.Fatalf("vendor filter leaked %q", m.Vendor)
		}
		if m.ContextWindow > prev {
			t.Errorf("sort=window is not non-increasing: %d after %d", m.ContextWindow, prev)
		}
		prev = m.ContextWindow
	}
}

func TestModelsFormatText(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--format", "text"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Known models") {
		t.Errorf("output missing 'Known models'\n%s", out)
	}
}

func TestModelsFormatJSON(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--format", "json"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	var env modelsEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if len(env.Models) == 0 {
		t.Error("JSON models array is empty")
	}
}

func TestModelsCSVHeaderAndRows(t *testing.T) {
	for _, args := range [][]string{{"--format", "csv"}, {"--csv"}} {
		c := captureStdout(t)
		if code := cmdModels(args); code != 0 {
			t.Fatalf("cmdModels(%v) exit = %d", args, code)
		}
		lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
		if len(lines) == 0 || lines[0] != "name,context_window,vendor" {
			t.Fatalf("cmdModels(%v) header = %q, want name,context_window,vendor", args, lines[0])
		}
		if len(lines) != len(counter.Models())+1 {
			t.Errorf("cmdModels(%v) rows = %d, want %d", args, len(lines)-1, len(counter.Models()))
		}
		for _, row := range lines[1:] {
			parts := strings.Split(row, ",")
			if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
				t.Errorf("cmdModels(%v) row %q is not name,window,vendor", args, row)
			}
			if _, err := strconv.Atoi(parts[1]); err != nil {
				t.Errorf("cmdModels(%v) row %q has a non-numeric window", args, row)
			}
		}
	}
}

func TestModelsCSVFollowsFilters(t *testing.T) {
	c := captureStdout(t)
	if code := cmdModels([]string{"--csv", "--vendor", "anthropic", "--top", "2"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("vendor+top csv rows = %d, want 2:\n%s", len(lines)-1, c.Content())
	}
	for _, row := range lines[1:] {
		if !strings.HasSuffix(row, ",anthropic") {
			t.Errorf("row %q does not end in ,anthropic", row)
		}
	}
}

// TestModelsJSONTakesPrecedenceOverCSV pins the conflict resolution: when both
// --json and --csv are given, JSON wins (it is checked first), so a script
// that sets a format flag and also wants JSON is not surprised by a switch.
func TestModelsJSONTakesPrecedenceOverCSV(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--csv"},
		{"--csv", "--json"},
		{"--format", "csv", "--json"},
		{"--format", "json", "--csv"},
	} {
		c := captureStdout(t)
		if code := cmdModels(args); code != 0 {
			t.Fatalf("cmdModels(%v) exit = %d", args, code)
		}
		trimmed := strings.TrimSpace(c.Content())
		if !strings.HasPrefix(trimmed, "{") {
			t.Errorf("cmdModels(%v) must output JSON, got:\n%s", args, trimmed)
		}
		var env modelsEnvelope
		if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
			t.Errorf("cmdModels(%v) output is not parseable JSON: %v", args, err)
		}
	}
}

// TestMapTopTakesPrecedenceOverCSV pins that --top (a flat table) beats --csv
// when both are given: the top branch runs before the csv branch.
func TestMapTopTakesPrecedenceOverCSV(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "3", "--csv"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Top 3 files") {
		t.Errorf("--top --csv must render the top table, got:\n%s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "path,tokens,bytes") {
		t.Errorf("--top --csv rendered CSV; top must take precedence:\n%s", out)
	}
}

func TestModelsFormatUnknown(t *testing.T) {
	code := cmdModels([]string{"--format", "xml"})
	if code != 2 {
		t.Fatalf("cmdModels exit = %d, want 2", code)
	}
}

func TestModelsOutputWritesFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.txt")

	code := cmdModels([]string{"--output", dst})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "Known models") {
		t.Errorf("output file missing 'Known models'\n%s", string(data))
	}
}

func TestModelsOutputJSON(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.json")

	code := cmdModels([]string{"--json", "--output", dst})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	var env modelsEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
}

// TestModelsOutputDashIsStdout pins that `--output -` means stdout at the
// command level (outputWriter already treats "-" as the terminal): a file
// named "-" must never be created, and the csv must land on stdout.
func TestModelsOutputDashIsStdout(t *testing.T) {
	dir := t.TempDir()
	c := captureStdout(t)
	if code := cmdModels([]string{"--csv", "--output", "-"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := strings.TrimSpace(c.Content())
	if !strings.HasPrefix(out, "name,context_window,vendor") {
		t.Errorf("--output - must print the csv to stdout, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "-")); err == nil {
		t.Errorf("--output - must not create a file named '-'")
	}
}

func TestModelsTopShowsFewer(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--top", "5"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Skip the header line "Known models..."
	modelLines := 0
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" {
			modelLines++
		}
	}
	if modelLines != 5 {
		t.Errorf("expected 5 model lines, got %d\n%s", modelLines, out)
	}
}

// TestModelsTopIgnoresSort pins that --top truncates by window descending and
// then returns, exactly as the CLI documents — a --sort given alongside is
// ignored, so a script cannot be surprised by a vendor-ordered top list.
func TestModelsTopIgnoresSort(t *testing.T) {
	c := captureStdout(t)
	if code := cmdModels([]string{"--top", "2", "--sort", "vendor", "--json"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	var env modelsEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if len(env.Models) != 2 {
		t.Fatalf("top=2 returned %d models, want 2", len(env.Models))
	}
	// The top branch sorts by window descending; the largest registered window
	// must lead, regardless of the --sort vendor request.
	maxWin := 0
	for _, m := range counter.Models() {
		if m.ContextWindow > maxWin {
			maxWin = m.ContextWindow
		}
	}
	if env.Models[0].ContextWindow != maxWin {
		t.Errorf("top[0].context_window = %d, want the largest %d (--sort vendor must be ignored)",
			env.Models[0].ContextWindow, maxWin)
	}
}

// TestModelsCSVTopTruncates pins --top in csv mode: header + N rows, the
// first being the largest window (top ranks by window and ignores sort).
func TestModelsCSVTopTruncates(t *testing.T) {
	c := captureStdout(t)
	if code := cmdModels([]string{"--csv", "--top", "2"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("--top 2 csv should be header + 2 rows, got %d lines:\n%s", len(lines), c.Content())
	}
	first := strings.Split(lines[1], ",")
	if len(first) != 3 {
		t.Fatalf("first data row malformed: %q", lines[1])
	}
	maxWin := 0
	for _, m := range counter.Models() {
		if m.ContextWindow > maxWin {
			maxWin = m.ContextWindow
		}
	}
	win, err := strconv.Atoi(first[1])
	if err != nil || win != maxWin {
		t.Errorf("csv top[0].context_window = %v (%v), want the largest %d", first[1], err, maxWin)
	}
}

// TestModelsTopZeroMeansAll pins that --top 0 is the same as no --top: the
// full table, matching the budget/depth/max-size convention where 0 = off.
func TestModelsTopZeroMeansAll(t *testing.T) {
	c := captureStdout(t)
	if code := cmdModels([]string{"--csv", "--top", "0"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != len(counter.Models())+1 {
		t.Errorf("--top 0 csv = %d lines, want %d (header + all models)",
			len(lines), len(counter.Models())+1)
	}
}

func TestModelsTopJSON(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--top", "3", "--json"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	var env modelsEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if len(env.Models) != 3 {
		t.Errorf("expected 3 models, got %d", len(env.Models))
	}
	// The first model should have the largest context window.
	if env.Models[0].ContextWindow < env.Models[2].ContextWindow {
		t.Errorf("models not sorted by context window: %d < %d",
			env.Models[0].ContextWindow, env.Models[2].ContextWindow)
	}
}

func TestModelsTopAll(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--top", "100"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	modelLines := 0
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" {
			modelLines++
		}
	}
	if modelLines != 30 {
		t.Errorf("expected 30 model lines (all models), got %d", modelLines)
	}
}

func TestModelsSortByName(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--sort", "name"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	// With --sort name, the first model should start with "claude" (alphabetical).
	if !strings.Contains(out, "claude-3-haiku") {
		t.Errorf("expected claude-3-haiku first with --sort name:\n%s", out)
	}
	// Check it's not the original registration order (gpt-3.5-turbo first).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 1 {
		firstLine := strings.TrimSpace(lines[1])
		if strings.HasPrefix(firstLine, "gpt-3.5") {
			t.Errorf("--sort name should not start with gpt-3.5-turbo:\n%s", out)
		}
	}
}

func TestModelsSortByWindow(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--sort", "window"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) <= 1 {
		t.Fatalf("expected model lines, got:\n%s", out)
	}
	// With --sort window, the largest window model should come first.
	// gemini-1.5-pro has 2000000, the largest.
	firstLine := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(firstLine, "gemini-1.5-pro") {
		t.Errorf("expected gemini-1.5-pro first with --sort window, got:\n%s", out)
	}
}

// TestModelsCSVSortByVendor pins --sort vendor in csv mode: the first data
// row carries the alphabetically smallest vendor (alibaba), like the text
// table.
func TestModelsCSVSortByVendor(t *testing.T) {
	c := captureStdout(t)
	if code := cmdModels([]string{"--csv", "--sort", "vendor"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + rows:\n%s", c.Content())
	}
	first := strings.Split(lines[1], ",")
	if len(first) != 3 || first[2] != "alibaba" {
		t.Errorf("csv sort=vendor first row = %q, want vendor alibaba", lines[1])
	}
}

func TestModelsSortByVendor(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--sort", "vendor"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) <= 1 {
		t.Fatalf("expected model lines, got:\n%s", out)
	}
	// With --sort vendor, alibaba comes first alphabetically (a-l-i < a-n-t).
	firstLine := strings.TrimSpace(lines[1])
	if !strings.Contains(firstLine, "alibaba") {
		t.Errorf("expected alibaba vendor first with --sort vendor:\n%s", out)
	}
}

func TestModelsSortJSON(t *testing.T) {
	c := captureStdout(t)

	code := cmdModels([]string{"--json", "--sort", "window"})
	if code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	var env struct {
		Models []struct {
			Name          string `json:"name"`
			ContextWindow int    `json:"context_window"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if len(env.Models) == 0 {
		t.Fatal("no models returned")
	}
	// First model should have the largest context window.
	if env.Models[0].Name != "gemini-1.5-pro" {
		t.Errorf("first model = %q, want gemini-1.5-pro", env.Models[0].Name)
	}
}

// TestModelsJSONSortByVendor pins --sort vendor in json mode: the first model
// carries the alphabetically smallest vendor (alibaba), like the text/csv
// tables.
func TestModelsJSONSortByVendor(t *testing.T) {
	c := captureStdout(t)
	if code := cmdModels([]string{"--json", "--sort", "vendor"}); code != 0 {
		t.Fatalf("cmdModels exit = %d", code)
	}
	var env struct {
		Models []struct {
			Vendor string `json:"vendor"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if len(env.Models) == 0 {
		t.Fatal("no models returned")
	}
	if env.Models[0].Vendor != "alibaba" {
		t.Errorf("first vendor = %q, want alibaba", env.Models[0].Vendor)
	}
}

// --- map --sort ---

// TestMapMaxSizeZeroUnlimited pins that map --max-size 0 is the same as the
// default (read everything), closing out the max-size 0 trio across
// pack/tokens/map.
func TestMapMaxSizeZeroUnlimited(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	dflt := captureStdout(t)
	if code := cmdMap([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	zero := captureStdout(t)
	if code := cmdMap([]string{src, "--json", "--max-size", "0"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	var envDefault, envZero struct {
		TotalTokens int `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(dflt.Content()), &envDefault); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(zero.Content()), &envZero); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envZero.TotalTokens != envDefault.TotalTokens {
		t.Errorf("--max-size 0 must equal the default total (%d), got %d",
			envDefault.TotalTokens, envZero.TotalTokens)
	}
}

// TestMapMaxSizeHiddenCombined pins that --hidden and --max-size combine: the
// dotfile is included but capped, so the total is below the full read of it.
func TestMapMaxSizeHiddenCombined(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, ".env"), []byte(strings.Repeat("s", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	full := captureStdout(t)
	if code := cmdMap([]string{src, "--hidden", "--json"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	capped := captureStdout(t)
	if code := cmdMap([]string{src, "--hidden", "--max-size", "10", "--json"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	var envFull, envCapped struct {
		TotalTokens int `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(full.Content()), &envFull); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(capped.Content()), &envCapped); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envCapped.TotalTokens >= envFull.TotalTokens {
		t.Errorf("--hidden --max-size 10 must cut the total below %d, got %d",
			envFull.TotalTokens, envCapped.TotalTokens)
	}
}

// TestMapMaxSizeUsesEstimate pins map's --max-size: a file over the cap is
// still listed, but with the bytes/4 estimate instead of a content read, so
// the flag constrains reading (and the tokens column) on map too.
func TestMapMaxSizeUsesEstimate(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	full := captureStdout(t)
	if code := cmdMap([]string{src}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	capped := captureStdout(t)
	if code := cmdMap([]string{src, "--max-size", "10"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if !strings.Contains(capped.Content(), "big.txt") {
		t.Fatalf("--max-size must still list the file:\n%s", capped.Content())
	}
	if full.Content() == capped.Content() {
		t.Errorf("--max-size 10 must change the tokens estimate for a 500-byte file:\nfull:\n%s\ncapped:\n%s",
			full.Content(), capped.Content())
	}
}

// TestMapTopZeroShowsTree pins that map --top 0 is the same as no --top: the
// full tree, not the flat top list (top only takes effect when N > 0).
func TestMapTopZeroShowsTree(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "0"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("--top 0 must show the tree, not a flat list:\n%s", out)
	}
}

// TestMapSortNameDirsFirst pins the README's "name (default, dirs first)"
// promise: a directory sorts ahead of a file even when its name is larger.
func TestMapSortNameDirsFirst(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "zzz_dir"), 0o755)
	os.WriteFile(filepath.Join(src, "zzz_dir", "inner.go"), []byte("package z\n"), 0o644)
	os.WriteFile(filepath.Join(src, "aaa_file.txt"), []byte("a\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--sort", "name"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if strings.Index(out, "zzz_dir") > strings.Index(out, "aaa_file.txt") {
		t.Errorf("dirs must sort before files regardless of name:\n%s", out)
	}
}

func TestMapSortByNameDefault(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header:\n%s", out)
	}
}

func TestMapSortByTokens(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "small.go"), []byte("package s\n"), 0o644)
	os.WriteFile(filepath.Join(src, "big.go"), []byte(strings.Repeat("// filler line\n", 40)), 0o644)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--sort", "tokens"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header with --sort tokens:\n%s", out)
	}
	// The largest file by tokens must be listed before the smaller one.
	if strings.Index(out, "big.go") > strings.Index(out, "small.go") {
		t.Errorf("sort=tokens must put the larger file first:\n%s", out)
	}
}

func TestMapSortByBytes(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "small.go"), []byte("package s\n"), 0o644)
	os.WriteFile(filepath.Join(src, "big.go"), []byte(strings.Repeat("// filler line\n", 40)), 0o644)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--sort", "bytes"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header with --sort bytes:\n%s", out)
	}
	if strings.Index(out, "big.go") > strings.Index(out, "small.go") {
		t.Errorf("sort=bytes must put the larger file first:\n%s", out)
	}
}

// --- map --depth ---

// TestMapSortUnknownFallsBackToName pins that an unknown --sort value for map
// is not an error: the walk falls back to name ordering (a before b), like the
// models command's unknown sort keys.
func TestMapSortUnknownFallsBackToName(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "b.go"), []byte("package b\n"), 0o644)
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--sort", "bogus"}); code != 0 {
		t.Fatalf("cmdMap exit = %d, want 0", code)
	}
	out := c.Content()
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Fatalf("unknown sort must not drop files:\n%s", out)
	}
	// Name ordering lists a.go's line before b.go's.
	if strings.Index(out, "a.go") > strings.Index(out, "b.go") {
		t.Errorf("unknown sort did not fall back to name order:\n%s", out)
	}
}

func TestMapDepthLimitsTraversal(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	// With --depth 0 (unlimited), we should see all files.
	code := cmdMap([]string{src, "--depth", "0"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header with --depth 0:\n%s", out)
	}
}

// TestMapDepthExcludeCombined pins that --exclude still applies inside the
// depth-bounded walk: a one-level txt is dropped while a go file stays.
func TestMapDepthExcludeCombined(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "a.txt"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "b.go"), []byte("package b\n"), 0o644)
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("z\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--depth", "1", "--exclude", "*.txt"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "b.go") {
		t.Errorf("depth 1 exclude *.txt must keep the go file:\n%s", out)
	}
	if strings.Contains(out, "a.txt") || strings.Contains(out, "root.txt") {
		t.Errorf("depth 1 exclude *.txt must drop both txt files:\n%s", out)
	}
}

// TestMapDepthHiddenCombined pins that --hidden and --depth work together: a
// dotfile one level down is included when both flags are given (hidden lifts
// the dotfile skip, depth still bounds the traversal).
func TestMapDepthHiddenCombined(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", ".hidden.go"), []byte("package h\n"), 0o644)
	os.WriteFile(filepath.Join(src, "root.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--hidden", "--depth", "1"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, ".hidden.go") {
		t.Errorf("--hidden --depth 1 must include the one-level dotfile:\n%s", out)
	}
	if !strings.Contains(out, "root.go") {
		t.Errorf("--hidden --depth 1 must keep the root file:\n%s", out)
	}
}

// TestMapDepthLimitsNestedDirs pins the depth boundary at the CLI layer using
// the documented semantics (a directory at depth 1 is one level below, so at
// --depth 1 its own files reach the map but a directory below it does not).
func TestMapDepthLimitsNestedDirs(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "a.go"), []byte("package sub\n"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "deep", "b.go"), []byte("package deep\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--depth", "1"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "a.go") {
		t.Errorf("--depth 1 must reach one-level files:\n%s", out)
	}
	if strings.Contains(out, "b.go") {
		t.Errorf("--depth 1 must not reach a two-level file:\n%s", out)
	}

	c = captureStdout(t)
	if code := cmdMap([]string{src, "--depth", "2"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if out := c.Content(); !strings.Contains(out, "b.go") {
		t.Errorf("--depth 2 must reach a two-level file:\n%s", out)
	}
}

func TestMapDepthPositive(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--depth", "1"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header with --depth 1:\n%s", out)
	}
}

// TestMapTopExcludeCombined pins that --top ranks what --exclude left: the
// excluded file never competes (the mirror of TestMapTopIncludeCombined).
func TestMapTopExcludeCombined(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)
	os.WriteFile(filepath.Join(src, "mid.go"), []byte(strings.Repeat("b", 100)), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "1", "--exclude", "*.txt"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "mid.go") {
		t.Errorf("top 1 exclude *.txt must name mid.go:\n%s", out)
	}
	if strings.Contains(out, "big.txt") {
		t.Errorf("top 1 exclude *.txt must drop the excluded file:\n%s", out)
	}
}

// TestMapTopNoGitignoreCombined pins that --top ranks the gitignored file
// once --no-gitignore lifts the filter: the flat list then shows it first.
func TestMapTopNoGitignoreCombined(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(src, "secret.txt"), []byte(strings.Repeat("s", 500)), 0o644)
	os.WriteFile(filepath.Join(src, "small.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "1", "--no-gitignore"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "secret.txt") {
		t.Errorf("top 1 no-gitignore must name the gitignored big file:\n%s", out)
	}
}

// TestMapTopMaxSizeCombined pins that --top and --max-size combine: the flat
// top list still names the largest file, but with the estimate when capped.
func TestMapTopMaxSizeCombined(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)
	os.WriteFile(filepath.Join(src, "small.go"), []byte(strings.Repeat("b", 10)), 0o644)

	full := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "1"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	capped := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "1", "--max-size", "10"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if !strings.Contains(full.Content(), "big.txt") || !strings.Contains(capped.Content(), "big.txt") {
		t.Errorf("both runs must name big.txt as top 1:\nfull:\n%s\ncapped:\n%s",
			full.Content(), capped.Content())
	}
	// The capped run reports the estimate, which is smaller than the read.
	if strings.Contains(capped.Content(), "143") {
		t.Errorf("--max-size 10 must report the estimate, not the full read:\n%s", capped.Content())
	}
}

// TestMapTopIncludeCombined pins that --top ranks only what --include left:
// the flat top list is drawn from the filtered set.
func TestMapTopIncludeCombined(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.go"), []byte(strings.Repeat("a", 500)), 0o644)
	os.WriteFile(filepath.Join(src, "mid.go"), []byte(strings.Repeat("b", 100)), 0o644)
	os.WriteFile(filepath.Join(src, "small.txt"), []byte(strings.Repeat("c", 20)), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--top", "1", "--include", "*.go"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "big.go") {
		t.Errorf("top 1 include *.go must name big.go:\n%s", out)
	}
	if strings.Contains(out, "small.txt") || strings.Contains(out, "mid.go") {
		t.Errorf("top 1 include *.go must only name the largest go file:\n%s", out)
	}
}

// --- map --top ---

func TestMapTopShowsFlatList(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--top", "5"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Top ") {
		t.Errorf("output missing 'Top' header:\n%s", out)
	}
	if !strings.Contains(out, "by tokens") {
		t.Errorf("default top header must say 'by tokens' (the default sortBy of name is not a ranking key):\n%s", out)
	}
	if !strings.Contains(out, "TOKENS") || !strings.Contains(out, "BYTES") {
		t.Errorf("output missing column headers:\n%s", out)
	}
}

// TestMapPathIsFileFails pins that the walk commands take a directory: a file
// path is a usage-level runtime error (exit 1), not a silent empty tree, and
// the message names the problem.
func TestMapPathIsFileFails(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	file := filepath.Join(src, "README.md")

	errCap := captureStderr(t)
	if code := cmdMap([]string{file}); code != 1 {
		t.Fatalf("cmdMap with a file path = %d, want 1", code)
	}
	if !strings.Contains(errCap.Content(), "not a directory") {
		t.Errorf("file-path error should say it is not a directory, got:\n%s", errCap.Content())
	}
}

// TestMapIncludeNoGitignoreCombined pins that --include does not override
// .gitignore either: a gitignored file only enters when --no-gitignore joins.
func TestMapIncludeNoGitignoreCombined(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(src, "secret.txt"), []byte(strings.Repeat("s", 200)), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--include", "*.txt"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if strings.Contains(c.Content(), "secret.txt") {
		t.Errorf("include alone must not override gitignore:\n%s", c.Content())
	}

	c = captureStdout(t)
	if code := cmdMap([]string{src, "--include", "*.txt", "--no-gitignore"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if !strings.Contains(c.Content(), "secret.txt") {
		t.Errorf("include plus no-gitignore must include the gitignored file:\n%s", c.Content())
	}
}

// TestMapIncludeDoesNotExposeDotfiles pins that --include cannot sneak a
// hidden file past the default dotfile exclusion: .env needs --hidden even
// when a glob names it directly. The explicit flag stays the only way in.
func TestMapIncludeDoesNotExposeDotfiles(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.txt"), []byte("x\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--include", "*.env"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if strings.Contains(out, ".env") {
		t.Errorf("--include must not expose a hidden file without --hidden:\n%s", out)
	}
	if strings.Contains(out, "keep.txt") {
		t.Errorf("keep.txt does not match *.env and must be filtered out:\n%s", out)
	}
}

// TestPackNoGitignoreHiddenCombined pins the two "lift the guard" flags on
// pack: together they bring a gitignored dotfile into the bundle (the pack
// mirror of TestMapNoGitignoreHiddenCombined).
func TestPackNoGitignoreHiddenCombined(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte(".env\n"), 0o644)
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdPack([]string{src, "--format", "json", "--no-gitignore", "--hidden"}); code != 0 {
		t.Fatalf("cmdPack exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, ".env") {
		t.Errorf("both flags together must include the gitignored dotfile:\n%s", out)
	}
	if !strings.Contains(out, "keep.go") {
		t.Errorf("both flags together must not drop normal files:\n%s", out)
	}
}

// TestMapNoGitignoreHiddenCombined pins that the two "lift the guard" flags
// together bring even a gitignored dotfile into the walk: --no-gitignore
// ignores the .gitignore rules and --hidden includes dotfiles.
func TestMapNoGitignoreHiddenCombined(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte(".env\n"), 0o644)
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.txt"), []byte("x\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--no-gitignore", "--hidden"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, ".env") {
		t.Errorf("both flags together must include the gitignored dotfile:\n%s", out)
	}
	if !strings.Contains(out, "keep.txt") {
		t.Errorf("both flags together must not drop normal files:\n%s", out)
	}
}

// TestMapHiddenDoesNotOverrideGitignore pins that an explicit .gitignore
// exclusion wins over --hidden: --hidden only lifts the default dotfile skip,
// not a named ignore.
func TestMapHiddenDoesNotOverrideGitignore(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte(".env\n"), 0o644)
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.txt"), []byte("x\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--hidden"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if strings.Contains(out, ".env") {
		t.Errorf("--hidden must not override a .gitignore exclusion:\n%s", out)
	}
	if !strings.Contains(out, "keep.txt") {
		t.Errorf("--hidden must not drop normal files:\n%s", out)
	}
}

// TestMapHiddenExcludesDotfilesByDefault pins --hidden at the CLI layer: a
// dotfile like .env is skipped by default, included with --hidden, and .git
// stays out either way (VCS metadata is never walked).
func TestMapHiddenExcludesDotfilesByDefault(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(src, ".git", "config"), []byte("[core]\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if out := c.Content(); strings.Contains(out, ".env") {
		t.Errorf(".env must be excluded by default:\n%s", out)
	}

	c = captureStdout(t)
	if code := cmdMap([]string{src, "--hidden"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, ".env") {
		t.Errorf("--hidden must include .env:\n%s", out)
	}
	if strings.Contains(out, ".git/") || strings.Contains(out, ".git ") {
		t.Errorf(".git must never be walked, even with --hidden:\n%s", out)
	}
}

// TestMapRespectsGitignoreByDefault pins the default: without --no-gitignore,
// a .gitignore exclusion keeps the file out of the walk (the mirror of
// TestMapNoGitignoreIncludesGitignoredFiles).
func TestMapRespectsGitignoreByDefault(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(src, "secret.txt"), []byte("s\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if strings.Contains(out, "secret.txt") {
		t.Errorf("default walk must respect .gitignore:\n%s", out)
	}
	if !strings.Contains(out, "keep.go") {
		t.Errorf("default walk must keep normal files:\n%s", out)
	}
}

// TestMapNoGitignoreIncludesGitignoredFiles pins the flag at the CLI layer:
// with .gitignore excluding a file, --no-gitignore walks it anyway (the
// built-in denylist still applies; only .gitignore files are ignored).
func TestMapNoGitignoreIncludesGitignoredFiles(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(src, "secret.txt"), []byte("s\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--no-gitignore"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "secret.txt") {
		t.Errorf("--no-gitignore must walk the gitignored file:\n%s", out)
	}
	if !strings.Contains(out, "keep.go") {
		t.Errorf("--no-gitignore must not drop normal files:\n%s", out)
	}
}

// TestMapIncludePathGlob pins that an --include glob may span a directory
// ("src/*" matches files under src/), not just basenames.
func TestMapIncludePathGlob(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "src"), 0o755)
	os.WriteFile(filepath.Join(src, "src", "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "root.go"), []byte("package r\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--include", "src/*"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "a.go") {
		t.Errorf("src/* must match the nested file:\n%s", out)
	}
	if strings.Contains(out, "root.go") {
		t.Errorf("src/* must not match a root-level file:\n%s", out)
	}
}

// TestMapExcludePathGlob pins that --exclude may span a directory too, the
// mirror of TestMapIncludePathGlob: "src/*" drops the nested file but keeps
// root-level ones.
func TestMapExcludePathGlob(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "src"), 0o755)
	os.WriteFile(filepath.Join(src, "src", "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "root.go"), []byte("package r\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--exclude", "src/*"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if strings.Contains(out, "a.go") {
		t.Errorf("src/* must drop the nested file:\n%s", out)
	}
	if !strings.Contains(out, "root.go") {
		t.Errorf("src/* must keep root-level files:\n%s", out)
	}
}

// TestMapMultipleIncludeFlagsMerge pins the "repeatable" promise in the help
// text: two --include globs are a union, not a last-wins replacement.
func TestMapMultipleIncludeFlagsMerge(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(src, "b.go"), []byte("package b\n"), 0o644)
	os.WriteFile(filepath.Join(src, "c.txt"), []byte("c\n"), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--include", "*.go", "--include", "c.*"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	for _, want := range []string{"a.go", "b.go", "c.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("repeated --include must union the globs, missing %s:\n%s", want, out)
		}
	}
}

// TestMapIncludeNoMatch pins that an --include matching nothing is not an
// error: the walk yields an empty tree, exit 0, so a script can rely on the
// shape of the output rather than exit codes to detect an empty result.
func TestMapIncludeNoMatch(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--include", "*.nonexistent-ext"}); code != 0 {
		t.Fatalf("cmdMap exit = %d, want 0 for an empty match", code)
	}
	out := c.Content()
	if !strings.Contains(out, "0 tokens") {
		t.Errorf("empty --include match should report an empty tree, got:\n%s", out)
	}
	if strings.Contains(out, "README.md") {
		t.Errorf("--include matching nothing leaked a file:\n%s", out)
	}
}

// TestMapJSONTreeSortsByTokens pins that --sort tokens orders the JSON tree
// largest-first too (the sibling of the sort=bytes test).
func TestMapJSONTreeSortsByTokens(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.go"), []byte(strings.Repeat("a", 500)), 0o644)
	os.WriteFile(filepath.Join(src, "small.txt"), []byte(strings.Repeat("b", 20)), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--json", "--sort", "tokens"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	var env struct {
		Tree struct {
			Children []struct {
				Name   string `json:"name"`
				Tokens int    `json:"tokens"`
			} `json:"children"`
		} `json:"tree"`
	}
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, c.Content())
	}
	if len(env.Tree.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(env.Tree.Children))
	}
	if env.Tree.Children[0].Name != "big.go" || env.Tree.Children[0].Tokens < env.Tree.Children[1].Tokens {
		t.Errorf("sort=tokens must put the larger file first, got %+v / %+v",
			env.Tree.Children[0], env.Tree.Children[1])
	}
}

// TestMapJSONTreeSortsBySortBy pins that --sort orders the JSON tree the
// same way it orders the text outline: children come largest-first when
// sorting by bytes.
func TestMapJSONTreeSortsBySortBy(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)
	os.WriteFile(filepath.Join(src, "small.txt"), []byte(strings.Repeat("b", 20)), 0o644)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--json", "--sort", "bytes"}); code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	var env mapEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, c.Content())
	}
	if env.Tree == nil || len(env.Tree.Children) == 0 {
		t.Fatal("JSON tree has no children")
	}
	if env.Tree.Children[0].Name != "big.txt" {
		t.Errorf("sort=bytes should put the largest file first, got %q",
			env.Tree.Children[0].Name)
	}
}

// TestMapJSONIgnoresTop pins that --top does not disturb the JSON envelope:
// the json branch runs first, so the full tree is always returned, and a
// script passing --top by mistake still gets complete data.
func TestMapJSONIgnoresTop(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	for _, args := range [][]string{{src, "--top", "1", "--json"}, {src, "--json", "--top", "1"}} {
		c := captureStdout(t)
		if code := cmdMap(args); code != 0 {
			t.Fatalf("cmdMap(%v) exit = %d", args, code)
		}
		var env mapEnvelope
		if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
			t.Fatalf("cmdMap(%v) output is not valid JSON: %v\n%s", args, err, c.Content())
		}
		if env.Tree == nil {
			t.Fatalf("cmdMap(%v) json is missing the full tree", args)
		}
		if strings.Contains(c.Content(), "Top ") {
			t.Errorf("cmdMap(%v) rendered the top table despite --json", args)
		}
	}
}

func TestMapTopFewerThanAvailable(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--top", "100"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Top ") {
		t.Errorf("output missing 'Top' header:\n%s", out)
	}
}

func TestMapTopByBytes(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--top", "3", "--sort", "bytes"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "by bytes") {
		t.Errorf("output missing 'by bytes':\n%s", out)
	}
}

// --- map --csv ---

func TestMapCSVOutputsHeader(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--csv"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least header + 1 row, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "path,tokens,bytes" {
		t.Errorf("CSV header = %q, want path,tokens,bytes", lines[0])
	}
}

func TestMapCSVHasOneRowPerFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--csv"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// writeRepo creates 4 files, so we expect 5 lines (header + 4).
	if len(lines) != 5 {
		t.Errorf("got %d lines, want 5:\n%s", len(lines), out)
	}
}

func TestMapCSVSortableByBytes(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--csv", "--sort", "bytes"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines:\n%s", out)
	}
	// The largest file by bytes should be first (after header).
	// Just verify it's valid CSV.
	if !strings.Contains(lines[1], ",") {
		t.Errorf("first data row not CSV:\n%s", lines[1])
	}
}

// --- doctor ---

func TestDoctorShowsVersion(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "ctxpack diagnostics:") {
		t.Errorf("output missing 'ctxpack diagnostics:' header:\n%s", out)
	}
	if !strings.Contains(out, "Version:") {
		t.Errorf("output missing 'Version:' line:\n%s", out)
	}
}

func TestDoctorShowsPlatform(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Platform:") {
		t.Errorf("output missing 'Platform:' line:\n%s", out)
	}
}

func TestDoctorShowsModelCount(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Models:") {
		t.Errorf("output missing 'Models:' line:\n%s", out)
	}
	if !strings.Contains(out, "30 models") {
		t.Errorf("output missing '30 models':\n%s", out)
	}
}

func TestDoctorTopShowsFewerVendors(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{"--top", "2"})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Vendor breakdown:") {
		t.Errorf("output missing 'Vendor breakdown:':\n%s", out)
	}
	// With --top 2, exactly 2 vendor lines should appear.
	lines := strings.Split(out, "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if strings.Contains(line, " model(s)") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 vendor lines with --top 2, got %d:\n%s", count, out)
	}
}

func TestDoctorTopJSON(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{"--json", "--top", "2"})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	var info map[string]any
	if err := json.Unmarshal([]byte(c.Content()), &info); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	// --top only affects text output; JSON always shows full data.
	if info["model_count"] != float64(30) {
		t.Errorf("model_count = %v, want 30", info["model_count"])
	}
	if info["model_vendors"] != float64(7) {
		t.Errorf("model_vendors = %v, want 7", info["model_vendors"])
	}
}

func TestDoctorTopAll(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{"--top", "10"})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	// With --top 10 (and only 7 vendors), all vendors should appear.
	lines := strings.Split(out, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, " model(s)") {
			count++
		}
	}
	if count != 7 {
		t.Errorf("expected 7 vendor lines with --top 10, got %d:\n%s", count, out)
	}
}

// TestDoctorTopZeroShowsAllVendors pins that doctor --top 0 is the same as
// no --top: every vendor line, closing out the top-0 series.
func TestDoctorTopZeroShowsAllVendors(t *testing.T) {
	c := captureStdout(t)
	if code := cmdDoctor([]string{"--top", "0"}); code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(out, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, " model(s)") {
			count++
		}
	}
	if count != 7 {
		t.Errorf("expected 7 vendor lines with --top 0, got %d:\n%s", count, out)
	}
}

func TestDoctorJSON(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{"--json"})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if _, ok := data["version"]; !ok {
		t.Errorf("JSON missing 'version' key")
	}
	if data["model_count"].(float64) != 30 {
		t.Errorf("JSON model_count = %v, want 30", data["model_count"])
	}
	for _, k := range []string{"go_version", "platform", "git_version", "model_vendors", "vendors"} {
		if _, ok := data[k]; !ok {
			t.Errorf("JSON missing key %q", k)
		}
	}
}

func TestDoctorRejectsExtraArgs(t *testing.T) {
	c := captureStderr(t)

	// Two distinct rejections: flag.Parse rejects an unknown flag, the NArg
	// check rejects a positional argument. The messages differ, so assert both
	// rather than naming the test after the one it used to exercise.
	for _, args := range [][]string{
		{"--bogus"},
		{"."},
		{"--top", "3", "someroot"},
	} {
		if code := cmdDoctor(args); code != 2 {
			t.Errorf("cmdDoctor(%v) exit = %d, want 2", args, code)
		}
	}

	got := c.Content()
	if !strings.Contains(got, "flag provided but not defined") {
		t.Errorf("an unknown flag was not reported as such:\n%s", got)
	}
	if !strings.Contains(got, "unexpected argument") {
		t.Errorf("a positional argument was not reported as such:\n%s", got)
	}
}

func TestDoctorFormatText(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{"--format", "text"})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "ctxpack diagnostics:") {
		t.Errorf("output missing 'ctxpack diagnostics:'\n%s", out)
	}
}

func TestDoctorFormatJSON(t *testing.T) {
	c := captureStdout(t)

	code := cmdDoctor([]string{"--format", "json"})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	out := c.Content()
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if _, ok := data["version"]; !ok {
		t.Error("JSON missing 'version' key")
	}
}

func TestDoctorFormatUnknown(t *testing.T) {
	code := cmdDoctor([]string{"--format", "xml"})
	if code != 2 {
		t.Fatalf("cmdDoctor exit = %d, want 2", code)
	}
}

// TestDoctorOutputDashIsStdout pins that doctor honors --output - as stdout,
// closing out the output-dash trio (models, diff, pack) across every command
// that takes -o: no file named "-" is created.
func TestDoctorOutputDashIsStdout(t *testing.T) {
	dir := t.TempDir()
	c := captureStdout(t)
	if code := cmdDoctor([]string{"--json", "-o", "-"}); code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Errorf("-o - must print the json to stdout: %v\n%s", err, c.Content())
	}
	if _, err := os.Stat(filepath.Join(dir, "-")); err == nil {
		t.Errorf("-o - must not create a file named '-'")
	}
}

func TestDoctorOutputWritesFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.txt")

	code := cmdDoctor([]string{"--output", dst})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "ctxpack diagnostics:") {
		t.Errorf("output file missing 'ctxpack diagnostics:'\n%s", string(data))
	}
}

// TestDoctorOutputShortFlag pins the -o shorthand. cmdDoctor was the only
// command that registered --output without it, so `ctxpack doctor -o file`
// failed with "flag provided but not defined: -o" even though the help text
// advertises `-o, --output FILE ... (all commands)`.
func TestDoctorOutputShortFlag(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "short.txt")

	code := cmdDoctor([]string{"-o", dst})
	if code != 0 {
		t.Fatalf("cmdDoctor -o exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "ctxpack diagnostics:") {
		t.Errorf("output file missing 'ctxpack diagnostics:'\n%s", string(data))
	}
}

func TestDoctorOutputJSON(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.json")

	code := cmdDoctor([]string{"--json", "--output", dst})
	if code != 0 {
		t.Fatalf("cmdDoctor exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
}

// --- tokens --sort ---

func TestTokensSortByNameDefault(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Per-model fit") {
		t.Errorf("output missing 'Per-model fit':\n%s", out)
	}
	// Default sort by name should start with 'claude' or 'deepseek' etc.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d", len(lines))
	}
}

// TestTokensSortByNameFirst pins the default/name sort: the first model row
// is the alphabetically smallest name (claude-3-haiku).
func TestTokensSortByNameFirst(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)
	if code := cmdTokens([]string{src, "--sort", "name"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	first := ""
	for _, line := range strings.Split(c.Content(), "\n") {
		if strings.Contains(line, "[") && strings.Contains(line, "/") {
			first = line
			break
		}
	}
	if !strings.HasPrefix(first, "  [fits] claude-3-haiku") {
		t.Errorf("sort=name must put claude-3-haiku first, got %q:\n%s", first, c.Content())
	}
}

func TestTokensSortByPct(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--sort", "pct"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Per-model fit") {
		t.Errorf("output missing 'Per-model fit':\n%s", out)
	}
	// The first model row must carry the highest pct_used.
	firstPct := -1.0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "(") && strings.Contains(line, "%)") {
			if i := strings.Index(line, "("); i >= 0 {
				if p, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(line[i+1:]), "%)"), 64); err == nil {
					firstPct = p
					break
				}
			}
		}
	}
	if firstPct < 0 {
		t.Fatalf("could not find a pct row in:\n%s", out)
	}
	maxPct := 0.0
	for _, m := range counter.Models() {
		f := counter.FitsModel(counter.Estimate{Tokens: envTokensOf(t, src)}, m, fitReserve)
		if f.PctUsed > maxPct {
			maxPct = f.PctUsed
		}
	}
	if math.Abs(firstPct-maxPct) > 0.5 {
		t.Errorf("sort=pct first row pct = %v, want ~%v", firstPct, maxPct)
	}
}

// envTokensOf returns the total token estimate for a directory, reused by the
// pct-order assertion above.
func envTokensOf(t *testing.T, src string) int {
	t.Helper()
	c := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	return env.TotalTokens
}

// TestTokensDepthLimitsTotal pins tokens --depth: at depth 1 a two-level
// directory is not traversed, so its tokens drop out of the total.
func TestTokensDepthLimitsTotal(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "deep", "b.txt"), []byte(strings.Repeat("b", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "root.go"), []byte("package main\n"), 0o644)

	full := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	shallow := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--depth", "1"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envFull, envShallow tokensEnvelope
	if err := json.Unmarshal([]byte(full.Content()), &envFull); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(shallow.Content()), &envShallow); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envShallow.TotalTokens >= envFull.TotalTokens {
		t.Errorf("--depth 1 must cut the total below %d, got %d",
			envFull.TotalTokens, envShallow.TotalTokens)
	}
}

// TestTokensHiddenIncludesDotfiles pins tokens --hidden: a dotfile is
// excluded by default and counted with --hidden, so the total rises.
func TestTokensHiddenIncludesDotfiles(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	dflt := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	hidden := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--hidden"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envDefault, envHidden tokensEnvelope
	if err := json.Unmarshal([]byte(dflt.Content()), &envDefault); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(hidden.Content()), &envHidden); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envHidden.TotalTokens <= envDefault.TotalTokens {
		t.Errorf("--hidden must raise the total above %d, got %d",
			envDefault.TotalTokens, envHidden.TotalTokens)
	}
}

// TestTokensMaxSizeZeroUnlimited pins that tokens --max-size 0 is the same as
// the default (read everything), the tokens-side mirror of the pack test.
func TestTokensMaxSizeZeroUnlimited(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	dflt := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	zero := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--max-size", "0"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envDefault, envZero tokensEnvelope
	if err := json.Unmarshal([]byte(dflt.Content()), &envDefault); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(zero.Content()), &envZero); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envZero.TotalTokens != envDefault.TotalTokens {
		t.Errorf("--max-size 0 must equal the default total (%d), got %d",
			envDefault.TotalTokens, envZero.TotalTokens)
	}
}

// TestTokensMaxSizeUsesEstimate pins that tokens honors --max-size like map:
// a file over the cap is estimated from its byte size instead of read, so the
// total drops below the full-read count.
func TestTokensMaxSizeUsesEstimate(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	full := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	capped := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--max-size", "10"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envFull, envCapped tokensEnvelope
	if err := json.Unmarshal([]byte(full.Content()), &envFull); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(capped.Content()), &envCapped); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envCapped.TotalTokens >= envFull.TotalTokens {
		t.Errorf("--max-size 10 must cut the total below %d, got %d",
			envFull.TotalTokens, envCapped.TotalTokens)
	}
}

// TestTokensRespectsGitignoreByDefault pins the default gitignore handling at
// the tokens level, closing out the map/pack/tokens trio: an excluded file is
// not part of the counted tree.
func TestTokensRespectsGitignoreByDefault(t *testing.T) {
	src := t.TempDir()
	gitInit(t, src)
	os.WriteFile(filepath.Join(src, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(src, "secret.txt"), []byte(strings.Repeat("s", 200)), 0o644)
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)

	full := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	withFlag := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--no-gitignore"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envDefault, envNoGit tokensEnvelope
	if err := json.Unmarshal([]byte(full.Content()), &envDefault); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(withFlag.Content()), &envNoGit); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envDefault.TotalTokens >= envNoGit.TotalTokens {
		t.Errorf("default total (%d) must be below the --no-gitignore total (%d)",
			envDefault.TotalTokens, envNoGit.TotalTokens)
	}
}

// TestTokensExcludeFiltersTotal pins tokens --exclude: a dropped file leaves
// the counted tree, shrinking the total (the mirror of TestTokensIncludeFiltersTotal).
func TestTokensExcludeFiltersTotal(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "keep.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(src, "drop.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	full := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	filtered := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--exclude", "*.txt"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envFull, envFiltered tokensEnvelope
	if err := json.Unmarshal([]byte(full.Content()), &envFull); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(filtered.Content()), &envFiltered); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envFiltered.TotalTokens >= envFull.TotalTokens {
		t.Errorf("--exclude *.txt must cut the total below %d, got %d",
			envFull.TotalTokens, envFiltered.TotalTokens)
	}
}

func TestTokensIncludeFiltersTotal(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	full := captureStdout(t)
	if code := cmdTokens([]string{src, "--json"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	filtered := captureStdout(t)
	if code := cmdTokens([]string{src, "--json", "--include", "*.go"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var envFull, envFiltered tokensEnvelope
	if err := json.Unmarshal([]byte(full.Content()), &envFull); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if err := json.Unmarshal([]byte(filtered.Content()), &envFiltered); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if envFiltered.TotalTokens >= envFull.TotalTokens {
		t.Errorf("--include *.go must cut the total below %d, got %d",
			envFull.TotalTokens, envFiltered.TotalTokens)
	}
}

func TestTokensModelWithTopOverFilter(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src, "--json", "--model", "gpt-4o", "--top", "5"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if len(env.Fits) != 1 {
		t.Fatalf("expected the single filtered fit, got %d", len(env.Fits))
	}
	if env.Fits[0].Model != "gpt-4o" {
		t.Errorf("fit model = %q, want gpt-4o", env.Fits[0].Model)
	}
}

// TestTokensJSONReserveField pins the reserve_tokens field in the json
// envelope: a script can compute the fit threshold itself (limit = window -
// reserve) from the published value, and it must match the documented 4096.
func TestTokensJSONReserveField(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src, "--json", "--model", "gpt-4o"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if env.ReserveTokens != 4096 {
		t.Errorf("reserve_tokens = %d, want the documented 4096", env.ReserveTokens)
	}
	if len(env.Fits) != 1 {
		t.Fatalf("expected 1 fit, got %d", len(env.Fits))
	}
	if env.Fits[0].Window != env.Fits[0].Limit+env.ReserveTokens {
		t.Errorf("window (=%d) must equal limit (=%d) + reserve (=%d)",
			env.Fits[0].Window, env.Fits[0].Limit, env.ReserveTokens)
	}
}

// TestTokensTopZeroMeansAll pins that --top 0 is the same as no --top for
// tokens too: every model fit, matching the models command.
func TestTokensTopZeroMeansAll(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)
	if code := cmdTokens([]string{src, "--csv", "--top", "0"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != len(counter.Models())+1 {
		t.Errorf("--top 0 csv = %d lines, want %d (header + all models)",
			len(lines), len(counter.Models())+1)
	}
}

// TestTokensCSVTopTruncates pins that --top works in csv mode too: exactly
// the N largest windows, header included, with the largest first.
func TestTokensCSVTopTruncates(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src, "--csv", "--top", "3"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != 4 {
		t.Fatalf("--top 3 csv should be header + 3 rows, got %d lines:\n%s", len(lines), c.Content())
	}
	first := strings.Split(lines[1], ",")
	if len(first) != 5 {
		t.Fatalf("first data row malformed: %q", lines[1])
	}
	// The largest registered window must lead (see TestTokensTopJSON).
	maxWin := 0
	for _, m := range counter.Models() {
		if m.ContextWindow > maxWin {
			maxWin = m.ContextWindow
		}
	}
	limit, err := strconv.Atoi(first[2])
	if err != nil || limit != maxWin-fitReserve {
		t.Errorf("csv top[0].limit = %v (%v), want the largest window minus reserve %d",
			first[2], err, maxWin-fitReserve)
	}
}

// TestTokensCSVSortsByPct pins that --sort pct orders the csv rows the same
// way it orders the text table: descending pct_used.
func TestTokensCSVSortsByPct(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src, "--csv", "--sort", "pct"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + rows:\n%s", c.Content())
	}
	prev := 1.0 // pct_used values exceed 1.0 for the smallest windows
	for _, row := range lines[1:] {
		parts := strings.Split(row, ",")
		if len(parts) != 5 {
			t.Fatalf("row %q has %d fields, want 5", row, len(parts))
		}
		pct, err := strconv.ParseFloat(parts[4], 64)
		if err != nil {
			t.Fatalf("row %q has a non-numeric pct_used", row)
		}
		if pct > prev {
			t.Errorf("csv --sort pct is not descending: %v after %v", pct, prev)
		}
		prev = pct
	}
}

func TestTokensSortByWindow(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--sort", "window"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Per-model fit") {
		t.Errorf("output missing 'Per-model fit':\n%s", out)
	}
	// The first model row is the largest context window.
	first := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[") && strings.Contains(line, "/") {
			first = line
			break
		}
	}
	if !strings.Contains(first, "gemini") {
		t.Errorf("sort=window must put the largest window first, got %q:\n%s", first, out)
	}
	if !strings.Contains(out, "gemini-2.5-pro") {
		t.Errorf("expected gemini-2.5-pro among the sorted rows:\n%s", out)
	}
}

func TestTokensSortJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--json", "--sort", "pct"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	var data tokensEnvelope
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if len(data.Fits) == 0 {
		t.Error("JSON fits array is empty")
	}
}

// --- map/tokens --output ---

func TestMapOutputWritesFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	dst := filepath.Join(t.TempDir(), "out.txt")

	code := cmdMap([]string{src, "--output", dst})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "Repository:") {
		t.Errorf("output file missing 'Repository:'\n%s", string(data))
	}
}

func TestTokensOutputWritesFile(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	dst := filepath.Join(t.TempDir(), "out.txt")

	code := cmdTokens([]string{src, "--output", dst})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "Path:") {
		t.Errorf("output file missing 'Path:'\n%s", string(data))
	}
}

func TestMapOutputJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	dst := filepath.Join(t.TempDir(), "out.json")

	code := cmdMap([]string{src, "--json", "--output", dst})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	var env mapEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if env.Root == "" {
		t.Error("JSON root is empty")
	}
}

func TestMapOutputShortFlag(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	dst := filepath.Join(t.TempDir(), "out.txt")

	code := cmdMap([]string{src, "-o", dst})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("output file not created: %v", err)
	}
}

// --- map --format ---

func TestMapFormatText(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--format", "text"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing 'Repository:'\n%s", out)
	}
}

func TestMapFormatJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	var env mapEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if env.Root == "" {
		t.Error("JSON root is empty")
	}
}

func TestMapFormatCSV(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--format", "csv"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.HasPrefix(out, "path,tokens,bytes") {
		t.Errorf("CSV output missing header:\n%s", out)
	}
}

func TestMapFormatUnknown(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	code := cmdMap([]string{src, "--format", "xml"})
	if code != 2 {
		t.Fatalf("cmdMap exit = %d, want 2", code)
	}
}

func TestMapFormatAlias(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	// --format json should be equivalent to --json
	code := cmdMap([]string{src, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	var env mapEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("--format json did not produce JSON: %v\n%s", err, out)
	}
}

// --- tokens --format ---

func TestTokensFormatText(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--format", "text"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Path:") {
		t.Errorf("output missing 'Path:'\n%s", out)
	}
}

// TestTokensJSONHasVendor pins that each fit in the json envelope carries the
// model's vendor, matching the MCP count_tokens envelope (CLI/MCP parity).
func TestTokensJSONHasVendor(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src, "--json", "--model", "gpt-4o"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(c.Content()), &env); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if len(env.Fits) != 1 {
		t.Fatalf("expected 1 fit, got %d", len(env.Fits))
	}
	if env.Fits[0].Vendor != "openai" {
		t.Errorf("fit vendor = %q, want openai", env.Fits[0].Vendor)
	}
	if env.Fits[0].Window != 128000 {
		t.Errorf("fit window = %d, want 128000 (gpt-4o)", env.Fits[0].Window)
	}
}

func TestTokensFormatJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if env.Path == "" {
		t.Error("JSON path is empty")
	}
}

func TestTokensCSVHeaderAndRows(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	for _, args := range [][]string{{src, "--format", "csv"}, {src, "--csv"}} {
		c := captureStdout(t)
		if code := cmdTokens(args); code != 0 {
			t.Fatalf("cmdTokens(%v) exit = %d", args, code)
		}
		lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
		if lines[0] != "model,used,limit,fits,pct_used" {
			t.Fatalf("cmdTokens(%v) header = %q, want model,used,limit,fits,pct_used", args, lines[0])
		}
		if len(lines) != len(counter.Models())+1 {
			t.Errorf("cmdTokens(%v) rows = %d, want %d", args, len(lines)-1, len(counter.Models()))
		}
		for _, row := range lines[1:] {
			parts := strings.Split(row, ",")
			if len(parts) != 5 {
				t.Errorf("cmdTokens(%v) row %q has %d fields, want 5", args, row, len(parts))
				continue
			}
			if _, err := strconv.Atoi(parts[1]); err != nil {
				t.Errorf("cmdTokens(%v) row %q has a non-numeric used", args, row)
			}
			if parts[3] != "true" && parts[3] != "false" {
				t.Errorf("cmdTokens(%v) row %q has a non-boolean fits", args, row)
			}
		}
	}
}

func TestTokensCSVSingleModel(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)
	if code := cmdTokens([]string{src, "--csv", "--model", "gpt-4o"}); code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(c.Content()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row, got %d lines:\n%s", len(lines), c.Content())
	}
	if !strings.HasPrefix(lines[1], "gpt-4o,") {
		t.Errorf("row = %q, want it to start with gpt-4o,", lines[1])
	}
}

func TestTokensFormatUnknown(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	code := cmdTokens([]string{src, "--format", "xml"})
	if code != 2 {
		t.Fatalf("cmdTokens exit = %d, want 2", code)
	}
}

func TestTokensFormatAlias(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	// --format json should be equivalent to --json
	code := cmdTokens([]string{src, "--format", "json"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("--format json did not produce JSON: %v\n%s", err, out)
	}
}

// --- tokens --top ---

func TestTokensTopShowsFewerModels(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--top", "3", "--sort", "window"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	modelLines := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			modelLines++
		}
	}
	if modelLines != 3 {
		t.Errorf("expected 3 model lines, got %d\n%s", modelLines, out)
	}
}

func TestTokensTopJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--top", "5", "--json"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	var env tokensEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if len(env.Fits) != 5 {
		t.Errorf("expected 5 fits, got %d", len(env.Fits))
	}
	// --top's documented contract is "N largest by context window"; the first
	// entry must carry the largest window in the registry. This used to be the
	// first N entries of the registration order (gpt-3.5-turbo & co).
	maxWin := 0
	for _, m := range counter.Models() {
		if m.ContextWindow > maxWin {
			maxWin = m.ContextWindow
		}
	}
	if env.Fits[0].Limit != maxWin-fitReserve {
		t.Errorf("top[0].limit = %d, want the largest window minus reserve (%d)",
			env.Fits[0].Limit, maxWin-fitReserve)
	}
}

func TestTokensTopAll(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--top", "100"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	modelLines := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			modelLines++
		}
	}
	if modelLines != 30 {
		t.Errorf("expected 30 model lines (all models), got %d", modelLines)
	}
}

func TestCountFiles(t *testing.T) {
	// nil node counts zero.
	if got := countFiles(nil); got != 0 {
		t.Errorf("countFiles(nil) = %d, want 0", got)
	}
	// A single file counts one.
	if got := countFiles(&repomap.Node{Name: "a.go", IsDir: false}); got != 1 {
		t.Errorf("countFiles(file) = %d, want 1", got)
	}
	// An empty directory counts zero.
	if got := countFiles(&repomap.Node{Name: "dir", IsDir: true}); got != 0 {
		t.Errorf("countFiles(empty dir) = %d, want 0", got)
	}
	// A directory with two files counts two.
	dir := &repomap.Node{Name: "dir", IsDir: true, Children: []*repomap.Node{
		{Name: "a.go", IsDir: false},
		{Name: "b.go", IsDir: false},
	}}
	if got := countFiles(dir); got != 2 {
		t.Errorf("countFiles(dir with 2 files) = %d, want 2", got)
	}
	// Nested directories recurse.
	root := &repomap.Node{Name: "root", IsDir: true, Children: []*repomap.Node{
		{Name: "a.go", IsDir: false},
		{Name: "pkg", IsDir: true, Children: []*repomap.Node{
			{Name: "b.go", IsDir: false},
			{Name: "sub", IsDir: true, Children: []*repomap.Node{
				{Name: "c.go", IsDir: false},
			}},
		}},
	}}
	if got := countFiles(root); got != 3 {
		t.Errorf("countFiles(nested) = %d, want 3", got)
	}
}
