package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/la2278647-arch/ctxpack/internal/counter"
)

// --- stream capture ---
//
// fmt.Printf reads os.Stdout at call time, so swapping the *os.File is enough
// to capture output. os.Pipe is used because os.Stdout is a *os.File, not an
// io.Writer, so a *bytes.Buffer cannot be assigned to it.

type capturedStream struct {
	old  *os.File
	r    *os.File
	w    *os.File
	buf  *bytes.Buffer
	done chan struct{}
}

func capture(t *testing.T, old *os.File, set func(*os.File)) *capturedStream {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	set(w)
	c := &capturedStream{old: old, r: r, w: w, buf: &bytes.Buffer{}, done: make(chan struct{})}
	go func() {
		_, _ = io.Copy(c.buf, c.r)
		close(c.done)
	}()
	t.Cleanup(func() {
		set(c.old)
		_ = c.w.Close()
		<-c.done
		_ = c.r.Close()
	})
	return c
}

// Content returns everything written so far and closes the writer.
func (c *capturedStream) Content() string {
	_ = c.w.Close()
	<-c.done
	return c.buf.String()
}

func captureStdout(t *testing.T) *capturedStream {
	t.Helper()
	return capture(t, os.Stdout, func(f *os.File) { os.Stdout = f })
}

func captureStderr(t *testing.T) *capturedStream {
	t.Helper()
	return capture(t, os.Stderr, func(f *os.File) { os.Stderr = f })
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- Run dispatch ---

func TestRunDispatchesEveryCommand(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	captureStdout(t)
	captureStderr(t)

	if code := Run(nil); code != 0 {
		t.Errorf("Run(nil) = %d, want 0", code)
	}
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"pack", src, "-o", filepath.Join(t.TempDir(), "p.xml")}, 0},
		{[]string{"map", src}, 0},
		{[]string{"tokens", src}, 0},
		{[]string{"models"}, 0},
		{[]string{"pack", filepath.Join(t.TempDir(), "missing"), "-o", filepath.Join(t.TempDir(), "x")}, 1},
	} {
		if got := Run(tc.args); got != tc.want {
			t.Errorf("Run(%v) = %d, want %d", tc.args, got, tc.want)
		}
	}
}

func TestRunDispatchesDiff(t *testing.T) {
	captureStdout(t)
	captureStderr(t)
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")
	if code := Run([]string{"diff", dir, "-o", filepath.Join(t.TempDir(), "d.xml")}); code != 0 {
		t.Errorf("Run(diff) = %d, want 0", code)
	}
}

func TestRunUnknownCommandReportsAndShowsHelp(t *testing.T) {
	errBuf := captureStderr(t)
	if code := Run([]string{"definitely-not-a-command"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	out := errBuf.Content()
	if !strings.Contains(out, "unknown command") {
		t.Errorf("stderr = %q, want an unknown-command message", out)
	}
	if !strings.Contains(out, "USAGE") {
		t.Error("stderr should fall through to the help text")
	}
}

// --- map ---

func TestMapPrintsTree(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdMap([]string{src}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out := c.Content()
	for _, want := range []string{
		"Repository: ", "Files: ~",
		// The tree shows basenames, one level at a time.
		"README.md", "a.go", "b.go", "notes.txt", "src/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map output missing %q:\n%s", want, out)
		}
	}
	// The tree is relative to the given path, so the root is its base name.
	if !strings.Contains(out, filepath.Base(src)) {
		t.Errorf("map output missing the root name:\n%s", out)
	}
}

func TestMapHonoursIncludeAndExclude(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--include", "*.go"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "a.go") {
		t.Errorf("--include *.go dropped a Go file:\n%s", out)
	}
	if strings.Contains(out, "README.md") || strings.Contains(out, "notes.txt") {
		t.Errorf("--include *.go leaked non-Go files:\n%s", out)
	}

	c2 := captureStdout(t)
	if code := cmdMap([]string{src, "--exclude", "*.md"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out2 := c2.Content()
	if strings.Contains(out2, "README.md") {
		t.Errorf("--exclude *.md kept README.md:\n%s", out2)
	}
	if !strings.Contains(out2, "a.go") {
		t.Errorf("--exclude *.md dropped a Go file:\n%s", out2)
	}
}

func TestMapDefaultsToTheWorkingDirectory(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(src); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	c := captureStdout(t)
	if code := cmdMap(nil); code != 0 {
		t.Fatalf("exit = %d, want 0 for a bare 'map'", code)
	}
	if out := c.Content(); !strings.Contains(out, "Repository: "+filepath.Base(src)) {
		t.Errorf("map with no path did not use the working directory:\n%s", out)
	}
}

func TestMapFailsOnAMissingPath(t *testing.T) {
	captureStderr(t)
	if code := cmdMap([]string{filepath.Join(t.TempDir(), "no-such-dir")}); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestMapRejectsAnUnknownFlag(t *testing.T) {
	captureStderr(t)
	if code := cmdMap([]string{t.TempDir(), "--no-such-flag"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// --- tokens ---

func TestTokensPrintsTheSummary(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)

	if code := cmdTokens([]string{src}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out := c.Content()
	for _, want := range []string{
		"Path:", "Tokens:", "Bytes:", "Per-model fit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tokens output missing %q:\n%s", want, out)
		}
	}
}

func TestTokensListsEveryModelExactlyOnce(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	c := captureStdout(t)
	if code := cmdTokens([]string{src}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out := c.Content()

	marks := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  [") {
			marks++
		}
	}
	models := counter.Models()
	if marks != len(models) {
		t.Errorf("tokens listed %d models, want %d:\n%s", marks, len(models), out)
	}
	for _, m := range models {
		if !strings.Contains(out, m.Name) {
			t.Errorf("tokens output missing model %q", m.Name)
		}
	}
}

// TestTokensMarksOverflowAndFits builds a repository large enough to exceed the
// smallest registered context window and small enough to fit the largest, so
// both marks appear in one run.
func TestTokensMarksOverflowAndFits(t *testing.T) {
	src := t.TempDir()
	for i := 0; i < 40; i++ {
		writeFile(t, src, fmt.Sprintf("doc%02d.txt", i),
			strings.Repeat("the quick brown fox jumps over the lazy dog. ", 150))
	}
	c := captureStdout(t)
	if code := cmdTokens([]string{src}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "OVERFLOW") {
		t.Errorf("a large repo should overflow the smallest model:\n%s", out)
	}
	if !strings.Contains(out, "fits]") {
		t.Errorf("the same repo should still fit the largest model:\n%s", out)
	}
}

// --max-size does not drop files: a file over the limit is still listed, it
// just gets no content read. The tests below pin that, because the flag used
// to be documented as "skip".
func TestMaxSizeStillListsLargeFiles(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "big.txt", strings.Repeat("x", 2048))
	writeFile(t, src, "tiny.txt", "y")

	c := captureStdout(t)
	if code := cmdMap([]string{src, "--max-size", "100"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "big.txt") {
		t.Errorf("--max-size dropped the large file instead of listing it:\n%s", out)
	}
	if !strings.Contains(out, "tiny.txt") {
		t.Errorf("--max-size dropped a small file too:\n%s", out)
	}
	// The 2048-byte file is still accounted for in the total.
	if !strings.Contains(out, "2.0 KB") {
		t.Errorf("the large file's bytes are not counted:\n%s", out)
	}
}

func TestMaxSizeDropsContentFromThePack(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "big.txt", strings.Repeat("x", 2048))
	writeFile(t, src, "tiny.txt", "hello")

	out := filepath.Join(t.TempDir(), "b.json")
	captureStderr(t)
	if code := cmdPack([]string{src, "-o", out, "--format", "json", "--max-size", "100"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Files []struct {
			Path    string `json:"path"`
			Bytes   int    `json:"bytes"`
			Content string `json:"content,omitempty"`
		} `json:"files"`
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byName := map[string]struct {
		content string
		bytes   int
	}{}
	for _, f := range doc.Files {
		byName[f.Path] = struct {
			content string
			bytes   int
		}{f.Content, f.Bytes}
	}
	big, ok := byName["big.txt"]
	if !ok {
		t.Fatalf("big.txt is missing from the bundle:\n%s", b)
	}
	if big.content != "" {
		t.Errorf("big.txt got content despite --max-size 100: %q", big.content)
	}
	if big.bytes != 2048 {
		t.Errorf("big.txt bytes = %d, want 2048", big.bytes)
	}
	small := byName["tiny.txt"]
	if small.content != "hello" {
		t.Errorf("tiny.txt lost its content: %q", small.content)
	}
	if doc.Skipped == 0 {
		t.Error("the skipped counter should record the file that was not read")
	}
}

func TestTokensFailsOnAMissingPath(t *testing.T) {
	captureStderr(t)
	if code := cmdTokens([]string{filepath.Join(t.TempDir(), "no-such-dir")}); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestTokensRejectsAnUnknownFlag(t *testing.T) {
	captureStderr(t)
	if code := cmdTokens([]string{t.TempDir(), "--no-such-flag"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// --- diff ---

func TestDiffPackagesTheChangedFiles(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "README.md", "first\n")
	writeFile(t, dir, "src/a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")

	writeFile(t, dir, "src/a.go", "package a\n// edited\n")
	writeFile(t, dir, "src/b.go", "package a\n")

	captureStderr(t)
	out := filepath.Join(t.TempDir(), "d.xml")
	if code := cmdDiff([]string{dir, "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "<!-- ctxpack diff vs") {
		t.Errorf("missing the diff header:\n%s", s[:min(120, len(s))])
	}
	if !strings.Contains(s, "src/a.go") || !strings.Contains(s, "src/b.go") {
		t.Errorf("changed files missing from the diff:\n%s", s)
	}
	if strings.Contains(s, "README.md") {
		t.Errorf("an unchanged file leaked into the diff:\n%s", s)
	}
}

func TestDiffAgainstARef(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "one")
	writeFile(t, dir, "b.go", "package b\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "two")

	out := filepath.Join(t.TempDir(), "d.xml")
	if code := cmdDiff([]string{dir, "--ref", "HEAD~1", "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "b.go") {
		t.Errorf("the file added in the last commit is missing:\n%s", s)
	}
	if strings.Contains(s, "a.go") {
		t.Errorf("a.go was committed before the ref and should be absent:\n%s", s)
	}
	if !strings.Contains(s, `diff vs "HEAD~1"`) {
		t.Errorf("the header should name the ref:\n%s", s[:min(120, len(s))])
	}
}

func TestDiffWithNoChanges(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")

	errBuf := captureStderr(t)
	captureStdout(t)
	if code := cmdDiff([]string{dir}); code != 0 {
		t.Fatalf("exit = %d, want 0 when nothing changed", code)
	}
	if out := errBuf.Content(); !strings.Contains(out, "no changed files") {
		t.Errorf("stderr = %q, want a no-changes message", out)
	}
}

func TestDiffOutsideARepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	errBuf := captureStderr(t)
	if code := cmdDiff([]string{dir}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out := errBuf.Content(); !strings.Contains(out, "not a git repository") {
		t.Errorf("stderr = %q, want a not-a-repo message", out)
	}
}

func TestDiffRejectsAnUnknownFormat(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")

	errBuf := captureStderr(t)
	if code := cmdDiff([]string{dir, "--format", "yaml"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if out := errBuf.Content(); !strings.Contains(out, "unknown format") {
		t.Errorf("stderr = %q, want an unknown-format message", out)
	}
}

func TestDiffHonoursModelAnnotation(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")

	out := filepath.Join(t.TempDir(), "d.xml")
	if code := cmdDiff([]string{dir, "--model", "gpt-4o", "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, _ := os.ReadFile(out)
	if s := string(b); !strings.Contains(s, "<!-- fit:") || !strings.Contains(s, "model=gpt-4o") {
		t.Errorf("the fit annotation is missing:\n%s", s[:min(160, len(s))])
	}
}

func TestDiffWriteFailureReports(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")

	captureStderr(t)
	bad := filepath.Join(t.TempDir(), "no", "such", "dir", "d.xml")
	if code := cmdDiff([]string{dir, "-o", bad}); code != 1 {
		t.Errorf("exit = %d, want 1 for a bad output path", code)
	}
}

func TestDiffRejectsAnUnknownFlag(t *testing.T) {
	errBuf := captureStderr(t)
	if code := cmdDiff([]string{t.TempDir(), "--no-such-flag"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if out := errBuf.Content(); !strings.Contains(out, "not defined: -no-such-flag") {
		t.Errorf("stderr = %q, want the flag error", out)
	}
}

func TestDiffReportsAChangeSetError(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")

	errBuf := captureStderr(t)
	// A ref that cannot be resolved makes `git diff` fail, which is neither
	// "not a repo" nor an empty change set.
	if code := cmdDiff([]string{dir, "--ref", "ref-that-does-not-exist"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out := errBuf.Content(); !strings.HasPrefix(out, "ctxpack:") {
		t.Errorf("stderr = %q, want a ctxpack: error", out)
	}
}

func TestDiffJSONWithModelReportsFitToStderr(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")

	errBuf := captureStderr(t)
	out := filepath.Join(t.TempDir(), "d.json")
	if code := cmdDiff([]string{dir, "--format", "json", "--model", "gpt-4o", "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Errorf("the JSON diff is not valid JSON:\n%s", string(b[:min(90, len(b))]))
	}
	s := errBuf.Content()
	if !strings.Contains(s, "<!-- fit:") || !strings.Contains(s, "model=gpt-4o") {
		t.Errorf("the fit note should go to stderr for JSON output:\n%s", s)
	}
}

// --- JSON must stay JSON ---

func TestPackJSONWithModelStaysValidJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "b.json")
	captureStderr(t)
	if code := cmdPack([]string{src, "-o", out, "--format", "json", "--model", "gpt-4o"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Errorf("the JSON bundle is not valid JSON:\n%s", string(b[:min(90, len(b))]))
	}
	var doc struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Files) == 0 {
		t.Error("the bundle has no files")
	}
}

func TestPackJSONWithoutModelStaysValidJSON(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "b.json")
	if code := cmdPack([]string{src, "-o", out, "--format", "json"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, _ := os.ReadFile(out)
	if !json.Valid(b) {
		t.Errorf("the JSON bundle is not valid JSON:\n%s", string(b[:min(90, len(b))]))
	}
}

func TestPackXMLWithModelKeepsTheHeader(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	out := filepath.Join(t.TempDir(), "b.xml")
	if code := cmdPack([]string{src, "-o", out, "--format", "xml", "--model", "gpt-4o"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, _ := os.ReadFile(out)
	if s := string(b); !strings.HasPrefix(s, "<!-- fit:") {
		t.Errorf("the XML output lost its fit header:\n%s", s[:min(120, len(s))])
	}
}

func TestDiffJSONStaysValidJSON(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.go", "package a\n// x\n")

	errBuf := captureStderr(t)
	out := filepath.Join(t.TempDir(), "d.json")
	if code := cmdDiff([]string{dir, "--format", "json", "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Errorf("the JSON diff is not valid JSON:\n%s", string(b[:min(90, len(b))]))
	}
	// The header must not be dropped either: it goes to stderr.
	if out := errBuf.Content(); !strings.Contains(out, "diff vs") {
		t.Errorf("stderr = %q, want the diff summary", out)
	}
}

// --- pack stderr summaries ---

func TestPackReportsAPlainSummary(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errBuf := captureStderr(t)
	out := filepath.Join(t.TempDir(), "b.xml")
	if code := cmdPack([]string{src, "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	out_s := errBuf.Content()
	if !strings.Contains(out_s, "wrote ") {
		t.Errorf("stderr = %q, want a wrote line", out_s)
	}
	if strings.Contains(out_s, "omitted") {
		t.Errorf("nothing was omitted, so stderr must not say so:\n%s", out_s)
	}
}

func TestPackReportsAnOmittedSummary(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errBuf := captureStderr(t)
	out := filepath.Join(t.TempDir(), "b.xml")
	if code := cmdPack([]string{src, "-o", out, "--budget", "5"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out_s := errBuf.Content(); !strings.Contains(out_s, "omitted by the budget") {
		t.Errorf("stderr = %q, want the omitted-by-budget line", out_s)
	}
}

func TestPackWriteFailureReports(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	errBuf := captureStderr(t)
	bad := filepath.Join(t.TempDir(), "no", "such", "dir", "b.xml")
	if code := cmdPack([]string{src, "-o", bad}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out := errBuf.Content(); !strings.HasPrefix(out, "ctxpack:") {
		t.Errorf("stderr = %q, want a ctxpack: error", out)
	}
}

func TestPackHonoursEnvDefaults(t *testing.T) {
	src := t.TempDir()
	writeRepo(t, src)
	t.Setenv("CTXPACK_FORMAT", "json")
	out := filepath.Join(t.TempDir(), "b.json")
	if code := cmdPack([]string{src, "-o", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, _ := os.ReadFile(out)
	if !json.Valid(b) {
		t.Errorf("CTXPACK_FORMAT=json was ignored:\n%s", string(b[:min(90, len(b))]))
	}

	// A budget coming from the environment must be applied too.
	t.Setenv("CTXPACK_FORMAT", "")
	t.Setenv("CTXPACK_BUDGET", "5")
	errBuf := captureStderr(t)
	if code := cmdPack([]string{src, "-o", filepath.Join(t.TempDir(), "c.xml")}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out := errBuf.Content(); !strings.Contains(out, "omitted by the budget") {
		t.Errorf("CTXPACK_BUDGET=5 was ignored:\n%s", out)
	}
}

// --- mcp ---

func TestRunMCPDispatchesToTheServer(t *testing.T) {
	captureStdout(t)
	captureStderr(t)
	// os.Stdin is pointed at a pipe whose writer is already closed, so the real
	// runMCP closure drives mcp.Serve to EOF and returns without blocking.
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
	})
	if code := Run([]string{"mcp"}); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

type readFailer struct{}

func (readFailer) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestMCPReportsAReadFailure(t *testing.T) {
	errBuf := captureStderr(t)
	var out bytes.Buffer
	if code := cmdMCP(nil, readFailer{}, &out); code != 1 {
		t.Fatalf("exit = %d, want 1 for a broken input stream", code)
	}
	if got := errBuf.Content(); !strings.HasPrefix(got, "ctxpack mcp:") {
		t.Errorf("stderr = %q, want the mcp error prefix", got)
	}
}

func TestMCPServesAnEmptySession(t *testing.T) {
	r, w := io.Pipe()
	var out bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- cmdMCP(nil, r, &out) }()
	// Closing the writer side with no input means the scanner sees EOF.
	_ = w.Close()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit = %d, want 0 on a clean EOF", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not return on EOF")
	}
	if got := out.String(); got != "" {
		t.Errorf("an empty session should produce no output, got %q", got)
	}
}

func TestMCPHandlesOneRoundTrip(t *testing.T) {
	r, w := io.Pipe()
	var out bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- cmdMCP(nil, r, &out)
	}()

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	if _, err := w.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not return after EOF")
	}
	resp := out.String()
	if !strings.Contains(resp, "2024-11-05") {
		t.Errorf("missing the protocol version:\n%s", resp)
	}
	if !strings.Contains(resp, "ctxpack") {
		t.Errorf("the response does not identify itself:\n%s", resp)
	}
}

func TestMCPRejectsAnUnknownFlag(t *testing.T) {
	errBuf := captureStderr(t)
	var out bytes.Buffer
	if code := cmdMCP([]string{"--no-such-flag"}, io.NopCloser(strings.NewReader("")), &out); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got := errBuf.Content(); !strings.Contains(got, "not defined: -no-such-flag") {
		t.Errorf("stderr = %q, want an unknown-flag message", got)
	}
}

// --- helpers ---

func TestTakesValue(t *testing.T) {
	fs := testFlagSet()
	for _, tc := range []struct {
		flag string
		want bool
	}{
		{"--format", true},  // string flag
		{"-o", true},        // shorthand string
		{"--budget", true},  // int flag
		{"--hidden", false}, // bool flag
		{"--no-gitignore", false},
		{"--unknown-flag", false}, // not registered
		{"--format=json", true},   // the = is trimmed before lookup
	} {
		if got := takesValue(fs, tc.flag); got != tc.want {
			t.Errorf("takesValue(%q) = %v, want %v", tc.flag, got, tc.want)
		}
	}
}

func TestReorderArgsSkipsAnUnregisteredFlagValue(t *testing.T) {
	fs := testFlagSet()
	got := reorderArgs(fs, []string{"--unknown-flag", "./repo"})
	if strings.Join(got, " ") != "--unknown-flag ./repo" {
		t.Errorf("got %v: the value must not be carried as the flag's", got)
	}
}

func TestWriteOutputDashGoesToStdout(t *testing.T) {
	c := captureStdout(t)
	if err := writeOutput("-", "dash-out"); err != nil {
		t.Fatal(err)
	}
	if err := writeOutput("", "empty-out"); err != nil {
		t.Fatal(err)
	}
	if got := c.Content(); got != "dash-outempty-out" {
		t.Errorf("writeOutput got %q, want both writes on stdout", got)
	}
}

func TestAnnotateFitOverflow(t *testing.T) {
	got := annotateFit(500000, "gpt-4o")
	if !strings.Contains(got, "OVERFLOW") {
		t.Errorf("annotateFit(500000) = %q, want OVERFLOW", got)
	}
	if strings.Contains(got, "FITS") {
		t.Errorf("annotateFit(500000) says FITS:\n%s", got)
	}
	fits := annotateFit(1000, "gpt-4o")
	if !strings.Contains(fits, "FITS") || strings.Contains(fits, "OVERFLOW") {
		t.Errorf("annotateFit(1000) = %q", fits)
	}
}

func TestParseFormatRejectsEmpty(t *testing.T) {
	if _, err := parseFormat(""); err == nil {
		t.Error("parseFormat(\"\") accepted an empty format")
	}
}

// --- git helpers ---

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// A private HOME keeps the real git config out of the test, and
	// GIT_CONFIG_NOSYSTEM stops a machine-wide config from surprising it.
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.invalid")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}
