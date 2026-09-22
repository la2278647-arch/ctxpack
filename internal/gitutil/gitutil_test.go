package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.invalid")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}

func put(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func srt(files []string) []string {
	out := append([]string(nil), files...)
	sort.Strings(out)
	return out
}

func eqWant(t *testing.T, got []string, want ...string) {
	t.Helper()
	if s := strings.Join(srt(got), ","); s != strings.Join(srt(want), ",") {
		t.Errorf("got [%s], want [%s]", strings.Join(srt(got), ", "), strings.Join(srt(want), ", "))
	}
}

func TestParsePorcelain(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n", nil},
		{"?? new.go", []string{"new.go"}},
		{" M tracked.go", []string{"tracked.go"}},
		{"M  a/b/c.go", []string{"a/b/c.go"}},
		{"A  added.go", []string{"added.go"}},
		{"R  old.go -> new.go", []string{"new.go"}},
		{"?? a", []string{"a"}},
		// Deletions are dropped: callers fall back to full-tree packing.
		{"D  gone.go", nil},
		{"AD staged-then-deleted", nil},
		// Empty paths are dropped.
		{"?? ", nil},
		{"?? \n M a.go", []string{"a.go"}},
	} {
		got := parsePorcelain(tc.in)
		eqWant(t, got, tc.want...)
	}
}

func TestParseNameStatus(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"M\ttracked.go", []string{"tracked.go"}},
		{"A\tadded.go", []string{"added.go"}},
		{"D\tgone.go", nil},
		{"T\ta.go", []string{"a.go"}},
		// Renames and copies: the meaningful path is the NEW one.
		{"R100\told/path.go\tnew/path.go", []string{"new/path.go"}},
		{"C075\tsrc/a.go\tdst/a.go", []string{"dst/a.go"}},
		{"M\ta.go\nR090\told/b.go\tnew/b.go", []string{"a.go", "new/b.go"}},
	} {
		got := parseNameStatus(tc.in)
		eqWant(t, got, tc.want...)
	}
}

func TestParseQuotedPaths(t *testing.T) {
	// git core.quotePath wraps non-ASCII paths in C-style quoting.
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"porcelain", `?? "caf\303\251.go"`, "café.go"},
		{"name-status", "M\t\"caf\\303\\251.go\"", "café.go"},
	} {
		var got []string
		if tc.name == "porcelain" {
			got = parsePorcelain(tc.in)
		} else {
			got = parseNameStatus(tc.in)
		}
		eqWant(t, got, tc.want)
	}
}

func TestDedup(t *testing.T) {
	got := dedup([]string{"a", "b", "a", "c", "b"})
	eqWant(t, got, "a", "b", "c")
	if len(got) != 3 {
		t.Fatalf("dedup kept %d entries: %v", len(got), got)
	}
}

func TestChangedFilesWorktree(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "tracked.go", "one\n")
	put(t, dir, "deleted.go", "x\n")
	put(t, dir, "kept.go", "k\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")

	put(t, dir, "tracked.go", "two\n")
	os.Remove(filepath.Join(dir, "deleted.go"))
	put(t, dir, filepath.Join("sub", "untracked.go"), "u\n")

	for _, ref := range []string{"", "HEAD", "WORKTREE", "main"} {
		got, err := ChangedFiles(dir, ref)
		if err != nil {
			t.Fatalf("ref %q: %v", ref, err)
		}
		eqWant(t, got, "tracked.go", filepath.ToSlash(filepath.Join("sub", "untracked.go")))
		if _, has := find(got, "deleted.go"); has {
			t.Errorf("ref %q reported a deleted file", ref)
		}
	}
}

func TestChangedFilesAgainstRefIncludesRenames(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "oldname.go", "one\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	// git mv refuses to create a destination directory itself.
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "mv", "oldname.go", filepath.Join("pkg", "newname.go"))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "rename")

	got, err := ChangedFiles(dir, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	eqWant(t, got, filepath.ToSlash(filepath.Join("pkg", "newname.go")))
}

func TestChangedFilesCleanTree(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")

	got, err := ChangedFiles(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("clean tree reported changes: %v", got)
	}
}

func TestChangedFilesNotARepo(t *testing.T) {
	if _, err := ChangedFiles(t.TempDir(), ""); err != ErrNotARepo {
		t.Errorf("err = %v, want ErrNotARepo", err)
	}
}

// A corrupt index makes `git status` fail while `git rev-parse --git-dir`
// still succeeds, so this is a repository-level failure, not the
// "not a repository" one. The error must surface rather than be swallowed.
func TestChangedFilesReportsAStatusError(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")

	idx := filepath.Join(dir, ".git", "index")
	if err := os.WriteFile(idx, []byte("this is not a git index file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ChangedFiles(dir, "")
	if err == nil {
		t.Fatal("wanted an error for a corrupt index")
	}
	if err == ErrNotARepo {
		t.Errorf("a corrupt index is a repository error, not %q", ErrNotARepo)
	}
	if _, gerr := git(dir, "rev-parse", "--git-dir"); gerr != nil {
		t.Fatalf("rev-parse must still succeed, which is what makes the branch reachable: %v", gerr)
	}
}

// An unresolvable ref fails `git diff`, which is again neither "not a repo"
// nor an empty change set.
func TestChangedFilesReportsADiffError(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")

	_, err := ChangedFiles(dir, "ref-that-does-not-exist")
	if err == nil {
		t.Fatal("wanted an error for an unresolvable ref")
	}
	if err == ErrNotARepo {
		t.Errorf("an unresolvable ref is a diff error, not %q", ErrNotARepo)
	}
}

// unquote's fallback is for paths that start with a quote but are not valid
// Go string literals. They must come back unchanged rather than truncated.
func TestUnquoteReturnsMalformedPathsUnchanged(t *testing.T) {
	for _, p := range []string{
		`"unclosed`, // no closing quote
		`"\q"`,      // \q is not a valid escape sequence
		`"\8"`,      // 8 is not an octal digit
		`"a"b"c"`,   // trailing junk
	} {
		if got := unquote(p); got != p {
			t.Errorf("unquote(%q) = %q, want it unchanged", p, got)
		}
	}
}

func find(files []string, want string) (string, bool) {
	for _, f := range files {
		if f == want {
			return f, true
		}
	}
	return "", false
}
