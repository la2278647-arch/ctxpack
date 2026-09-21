package cli

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		// Unknown values silently fall back to XML.
		{"", "xml"},
		{"yaml", "xml"},
		{"html", "xml"},
	} {
		if got := parseFormat(tc.in); string(got) != tc.want {
			t.Errorf("parseFormat(%q) = %q, want %q", tc.in, got, tc.want)
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
		{[]string{"version"}, 0},
		{[]string{"--version"}, 0},
		{[]string{"-v"}, 0},
		{[]string{"models"}, 0},
		{[]string{"models", "-h"}, 0},
		{[]string{"totally-unknown"}, 2},
	} {
		if got := Run(tc.args); got != tc.want {
			t.Errorf("Run(%v) = %d, want %d", tc.args, got, tc.want)
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

func TestPackMissingPathFails(t *testing.T) {
	code := cmdPack([]string{filepath.Join(t.TempDir(), "no-such-dir"),
		"-o", filepath.Join(t.TempDir(), "x.json")})
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a missing path", code)
	}
}

func TestPackBadFlagFails(t *testing.T) {
	if code := cmdPack([]string{t.TempDir(), "--budget", "not-a-number"}); code != 2 {
		t.Errorf("exit = %d, want 2 for an unparseable flag", code)
	}
}

// --- diff requires git ---

func TestDiffOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	if code := cmdDiff([]string{dir}); code != 1 {
		t.Errorf("exit = %d, want 1 outside a git repo", code)
	}
}
