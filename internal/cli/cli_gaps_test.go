package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These are the reachable branches left uncovered: the Run dispatcher never
// reached the doctor case, cmdDoctor never saw a positional argument, the
// tokens --model path never reported an overflow, and a diff --dry-run never
// had a budget that omitted anything.

// TestRunDispatchesDoctor drives the dispatcher that main.go actually calls.
// The other commands are already covered by TestRunDispatchesEveryCommand;
// doctor was the one case left unexercised, so a typo in its case label would
// have shipped as "unknown command" for a command the help text advertises.
func TestRunDispatchesDoctor(t *testing.T) {
	errs := captureStderr(t)
	out := captureStdout(t)
	if code := Run([]string{"doctor"}); code != 0 {
		t.Fatalf("Run(doctor) = %d, want 0; stderr: %s", code, errs.Content())
	}
	if !strings.Contains(out.Content(), "ctxpack diagnostics:") {
		t.Fatalf("the doctor report did not reach stdout through Run:\n%s", out.Content())
	}

	// JSON mode must parse, and artifacts stay on stdout: the report is the
	// only thing a client reads, so a stray status line would corrupt it.
	errs2 := captureStderr(t)
	out2 := captureStdout(t)
	if code := Run([]string{"doctor", "--format", "json"}); code != 0 {
		t.Fatalf("Run(doctor --format json) = %d, want 0; stderr: %s", code, errs2.Content())
	}
	var rep struct {
		Version    string `json:"version"`
		ModelCount int    `json:"model_count"`
	}
	if err := json.Unmarshal([]byte(out2.Content()), &rep); err != nil {
		t.Fatalf("the JSON report is not valid JSON: %v\n%s", err, out2.Content())
	}
	if rep.ModelCount == 0 {
		t.Errorf("the JSON report should name the registry size: %v", rep)
	}
	if got := errs2.Content(); got != "" {
		t.Errorf("doctor --format json wrote status to stderr:\n%s", got)
	}
}

// A near-miss spelling must not silently dispatch: the dispatcher's only
// obligation is to send "doctor" to cmdDoctor, so a wrong label must land in
// the unknown-command branch rather than somewhere harmless.
func TestRunDoesNotDispatchATypoOfDoctor(t *testing.T) {
	errs := captureStderr(t)
	if code := Run([]string{"docter"}); code != 2 {
		t.Fatalf("Run(docter) = %d, want 2", code)
	}
	if !strings.Contains(errs.Content(), "unknown command") {
		t.Errorf("stderr should say unknown command:\n%s", errs.Content())
	}
}

// A repository past the model's limit must say so. Every existing tokens test
// used a tiny fixture, so the OVERFLOW branch was the only one of the two marks
// never seen — and it is the mark an agent actually needs, because packing
// past a window silently truncates.
func TestTokensModelOverflow(t *testing.T) {
	src := t.TempDir()
	writeBigRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--model", "gpt-4"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "[OVERFLOW] gpt-4") {
		t.Errorf("a repository past gpt-4's limit should report OVERFLOW:\n%s", out)
	}
	// The percentage is not pinned: it comes from the tokenizer estimate, a
	// heuristic. The verdict is what an agent acts on, so that is the contract.
}

// The same content fits a large window, so OVERFLOW is a property of the pair,
// not of the repository. Without this the overflow test could pass because the
// fixture is broken rather than because the mark is right.
func TestTokensModelFitsALargeWindow(t *testing.T) {
	src := t.TempDir()
	writeBigRepo(t, src)
	c := captureStdout(t)

	code := cmdTokens([]string{src, "--model", "gpt-4o"})
	if code != 0 {
		t.Fatalf("cmdTokens exit = %d", code)
	}
	out := c.Content()
	if !strings.Contains(out, "[fits] gpt-4o") {
		t.Errorf("the same repository should fit gpt-4o:\n%s", out)
	}
	if strings.Contains(out, "OVERFLOW") {
		t.Errorf("no overflow expected for gpt-4o:\n%s", out)
	}
}

// diff --dry-run has its own budget summary, so it needs its own test: the
// omission line is the one thing a user reads to learn how much they lost to
// the cap. pack --dry-run is covered by TestPackDryRunWithBudget.
func TestDiffDryRunReportsBudgetOmissions(t *testing.T) {
	dir := newGitRepo(t)
	writeBigRepo(t, dir)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	for _, name := range gitListChanged(t, dir) {
		writeFile(t, dir, name, strings.Repeat("one more line of text. ", 40))
	}

	errs := captureStderr(t)
	out := captureStdout(t)

	code := cmdDiff([]string{dir, "--dry-run", "--budget", "100"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d; stderr: %s", code, errs.Content())
	}
	err := errs.Content()
	if !strings.Contains(err, "dry run vs") {
		t.Errorf("missing the dry-run header:\n%s", err)
	}
	if !strings.Contains(err, "omitted by the budget") {
		t.Errorf("a 100-token budget must report the omissions:\n%s", err)
	}
	if got := out.Content(); got != "" {
		t.Errorf("dry run must not write the bundle to stdout:\n%s", got)
	}
}

// A generous budget omits nothing, which proves the omission line above is a
// consequence of the cap rather than printed unconditionally.
func TestDiffDryRunReportsNoOmissionsWhenTheBudgetFits(t *testing.T) {
	dir := newGitRepo(t)
	writeBigRepo(t, dir)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	writeFile(t, dir, "a.txt", strings.Repeat("one more line of text. ", 40))

	errs := captureStderr(t)
	code := cmdDiff([]string{dir, "--dry-run", "--budget", "1000000"})
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d; stderr: %s", code, errs.Content())
	}
	err := errs.Content()
	if strings.Contains(err, "omitted by the budget") {
		t.Errorf("an unlimited budget must not claim omissions:\n%s", err)
	}
	if !strings.Contains(err, "dry run vs") {
		t.Errorf("missing the dry-run header:\n%s", err)
	}
}

// writeBigRepo fills dir with enough prose to clear gpt-4's effective limit
// (window 8192 minus the 4096 reply reserve = 4096 tokens) while staying
// well inside a comfortable window for the larger models.
func writeBigRepo(t *testing.T, dir string) {
	t.Helper()
	body := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 40)
	for i := 0; i < 12; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc%02d.txt", i))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// gitListChanged returns the tracked paths in a repo, so a test can rewrite
// them all and make every file a diff candidate.
func gitListChanged(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "ls-files")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var names []string
	for _, line := range strings.Split(string(b), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	if len(names) == 0 {
		t.Fatal("git ls-files returned no paths")
	}
	return names
}
