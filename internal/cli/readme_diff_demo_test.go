package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDiffDemoReadmeListOutputMatches keeps the diff-demo README honest against
// what `diff --list` really prints.
//
// examples/diff-demo/README.md used to carry a section titled "`--list`
// deliberately hides deletions", with an example output of two paths. When
// --list was fixed to honour the pack's filters it started naming deletions too
// (the pack names them in <deleted>, so the list must agree), and the section
// became flatly wrong: it told readers their deletion would not appear, then
// showed an output that was no longer reproducible. The stale sentence had also
// already been copied into examples/ctxpack-self-budget8000.md, which packs this
// repository. This test rebuilds the demo's two-commit scenario the way make.sh
// does and compares the documented output with a real run.
func TestDiffDemoReadmeListOutputMatches(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "examples", "diff-demo", "README.md"))
	if err != nil {
		t.Fatalf("examples/diff-demo/README.md: %v", err)
	}
	documented, ok := documentedListOutput(string(readme))
	if !ok {
		t.Fatalf("README has no parseable --list example\nlooking for:\n  ctxpack diff <repo> --list --ref HEAD~1..HEAD\nfollowed by # lines")
	}
	if len(documented) == 0 {
		t.Fatalf("the documented --list output is empty")
	}

	got := diffDemoListOutput(t)

	if !reflect.DeepEqual(got, documented) {
		t.Errorf("--list prints %v, the README documents %v", got, documented)
	}
}

// documentedListOutput returns the "# "-prefixed lines of the fenced block that
// follows the `--list` example command in the README.
func documentedListOutput(readme string) ([]string, bool) {
	lines := strings.Split(readme, "\n")
	inBlock := false
	seenCmd := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "```sh" {
			inBlock = true
			continue
		}
		if trimmed == "```" {
			if inBlock && seenCmd {
				return out, true
			}
			inBlock = false
			continue
		}
		if !inBlock {
			continue
		}
		if strings.HasPrefix(trimmed, "ctxpack diff <repo> --list") {
			seenCmd = true
			continue
		}
		if seenCmd && strings.HasPrefix(trimmed, "#") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
		}
	}
	return nil, false
}

// diffDemoListOutput rebuilds the scenario examples/diff-demo/make.sh commits
// (revision one has src/app.go and src/legacy.go; revision two modifies app.go,
// adds src/util.go and deletes src/legacy.go) and returns the real --list
// output over the same range.
func diffDemoListOutput(t *testing.T) []string {
	t.Helper()
	src := t.TempDir()
	srcDir := filepath.Join(src, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(srcDir, "app.go"), "package app\n\nfunc Run() {}\n")
	mustWriteFile(t, filepath.Join(srcDir, "legacy.go"), "package app\n\nfunc Legacy() {}\n")

	gitInit(t, src)
	gitAddAll(t, src)
	gitCommit(t, src, "initial")

	mustWriteFile(t, filepath.Join(srcDir, "app.go"), "package app\n\nfunc Run() {}\n\nfunc Serve(req string) string { return \"ok: \" + req }\n")
	mustWriteFile(t, filepath.Join(srcDir, "util.go"), "package app\n\nfunc TitleCase(s string) string { return s }\n")
	if err := os.Remove(filepath.Join(srcDir, "legacy.go")); err != nil {
		t.Fatal(err)
	}
	gitAddAll(t, src)
	gitCommit(t, src, "add util, retire legacy")

	outCap := captureStdout(t)
	if code := cmdDiff([]string{src, "--list", "--ref", "HEAD~1..HEAD"}); code != 0 {
		t.Fatalf("cmdDiff --list --ref HEAD~1..HEAD exit = %d", code)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(outCap.Content()), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
