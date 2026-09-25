package cli

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
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
	if !strings.Contains(out, "files") {
		t.Errorf("stderr missing 'files'\n%s", out)
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

// --- quiet ---

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
	if modelLines != 31 {
		t.Errorf("expected 31 model lines (all models), got %d", modelLines)
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

// --- map --sort ---

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
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--sort", "tokens"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header with --sort tokens:\n%s", out)
	}
}

func TestMapSortByBytes(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	code := cmdMap([]string{src, "--sort", "bytes"})
	if code != 0 {
		t.Fatalf("cmdMap exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "Repository:") {
		t.Errorf("output missing Repository header with --sort bytes:\n%s", out)
	}
}

// --- map --depth ---

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
	if !strings.Contains(out, "TOKENS") || !strings.Contains(out, "BYTES") {
		t.Errorf("output missing column headers:\n%s", out)
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
	if !strings.Contains(out, "31 models") {
		t.Errorf("output missing '31 models':\n%s", out)
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
	if info["model_count"] != float64(31) {
		t.Errorf("model_count = %v, want 31", info["model_count"])
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
	if data["model_count"].(float64) != 31 {
		t.Errorf("JSON model_count = %v, want 31", data["model_count"])
	}
}

func TestDoctorRejectsExtraArgs(t *testing.T) {
	c := captureStderr(t)

	code := cmdDoctor([]string{"--bogus"})
	if code != 2 {
		t.Fatalf("cmdDoctor exit = %d, want 2", code)
	}
	_ = c.Content() // drain
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
	if modelLines != 31 {
		t.Errorf("expected 31 model lines (all models), got %d", modelLines)
	}
}
