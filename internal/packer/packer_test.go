package packer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/la2278647-arch/ctxpack/internal/format"
	"github.com/la2278647-arch/ctxpack/internal/walker"
)

func TestPackBasic(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){println(\"hi\")}\n"))
	mustWrite(t, filepath.Join(dir, "README.md"), []byte("# Demo\n\nA demo project.\n"))

	bundle, err := Pack(dir, Options{
		Walker: walker.Options{RespectGitignore: true, ReadContent: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(bundle.Files))
	}
	if bundle.TotalTokens <= 0 {
		t.Fatal("expected nonzero total tokens")
	}
	xml := format.Render(bundle, format.XML)
	if !strings.Contains(xml, "<repository>") || !strings.Contains(xml, "main.go") {
		t.Fatalf("XML missing expected content:\n%s", xml)
	}
	md := format.Render(bundle, format.Markdown)
	if !strings.Contains(md, "```go") {
		t.Fatalf("Markdown missing go fence:\n%s", md)
	}
}

func TestPackBudget(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "README.md"), []byte("# Project\n\nshort readme\n"))
	mustWrite(t, filepath.Join(dir, "big.go"), []byte(strings.Repeat("// line\n", 2000)))

	bundle, err := Pack(dir, Options{
		Walker: walker.Options{RespectGitignore: true, ReadContent: true},
		Budget: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 1 || bundle.Files[0].Path != "README.md" {
		paths := make([]string, len(bundle.Files))
		for i, f := range bundle.Files {
			paths[i] = f.Path
		}
		t.Fatalf("budget kept %v, want only [README.md]", paths)
	}
}

func TestPackFilesAllowlist(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("package a\n"))
	mustWrite(t, filepath.Join(dir, "b.go"), []byte("package b\n"))
	mustWrite(t, filepath.Join(dir, "c.go"), []byte("package c\n"))

	bundle, err := Pack(dir, Options{
		Walker: walker.Options{RespectGitignore: true, ReadContent: true},
		Files:  []string{"a.go", "c.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(bundle.Files))
	}
}

func TestApplyBudgetReportsOmitted(t *testing.T) {
	files := []format.File{
		{Path: "LICENSE", Tokens: 10},
		{Path: "big.md", Tokens: 50},
		{Path: "main.go", Tokens: 10},
		{Path: "doc.md", Tokens: 30},
		{Path: "z.go", Tokens: 5},
	}
	kept, omitted := applyBudget(files, 25)
	if got := paths(kept); !equal(got, []string{"LICENSE", "main.go", "z.go"}) {
		t.Errorf("kept = %v, want LICENSE, main.go, z.go in path order", got)
	}
	if got := paths(omitted); !equal(got, []string{"big.md", "doc.md"}) {
		t.Errorf("omitted = %v, want big.md, doc.md", got)
	}
	if n := omittedTokens(omitted); n != 80 {
		t.Errorf("omittedTokens = %d, want 80", n)
	}
	if got := omittedPaths(omitted); len(got) != 2 || got[0] != "big.md" || got[1] != "doc.md" {
		t.Errorf("omittedPaths = %v", got)
	}
}

func TestApplyBudgetNothingFits(t *testing.T) {
	kept, omitted := applyBudget([]format.File{{Path: "a.go", Tokens: 50}}, 10)
	if len(kept) != 0 {
		t.Errorf("kept = %v, want empty", kept)
	}
	if len(omitted) != 1 || omitted[0].Path != "a.go" {
		t.Errorf("omitted = %v, want [a.go]", omitted)
	}
}

func TestApplyBudgetEmpty(t *testing.T) {
	kept, omitted := applyBudget(nil, 100)
	if kept != nil || omitted != nil {
		t.Errorf("empty input gave kept=%v omitted=%v", kept, omitted)
	}
	if got := omittedPaths(nil); got != nil {
		t.Errorf("omittedPaths(nil) = %v, want nil", got)
	}
}

func TestPriorityLadder(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"LICENSE", 1000},
		{"LICENSE.md", 1000},
		{"docs/contributing.md", 1000},
		{"README.rst", 1000},
		{"README", 1000},
		{"cmd/app/main.go", 400},
		{"web/index.ts", 400},
		{"web/index.js", 400},
		{"api/server.js", 400},
		{"manage.py", 400},
		{"mod.go", 400},
		{"api/v1/service.proto", 350},
		{"schema.graphql", 350},
		{"docs/guide.md", 300},
		{"docs/guide.rst", 300},
		{"notes.txt", 300},
		{"src/lib.go", 200},
		{"app.py", 200},
		{"config.yml", 150},
		{"a.json", 150},
		{"Dockerfile", 150},
		{"sub/UPPER.DOCKERFILE", 150},
		{"Makefile", 150},
		{"scripts/run.sh", 100},
		{"lib_test.go", 50},
		{"app.test.js", 50},
		{"lib.spec.ts", 50},
		{"test_auth.py", 50},
		{"lib.test.py", 50},
		{"test_", 10},
		{"data.bin", 10},
		// SECURITY.md is a policy doc; a file merely starting with "security"
		// is source code.
		{"SECURITY.md", 1000},
		{"securityscanner.go", 200},
		// Readmes match on the basename, not a prefix, so a file merely named
		// after readme is source.
		{"readme.md", 1000},
		{"readme.rst", 1000},
		{"docs/README.adoc", 1000},
		{"readme_helper.py", 200},
		{"readmes.py", 200},
		{"docs/READMEING.md", 300},
		// Entry points match on the basename too, so myindex.js is source.
		{"myindex.js", 200},
		{"notmain.go", 200},
		{"mymanage.py", 200},
		{"customserver.js", 200},
		{"src/App.tsx", 200},
		{"src/App.jsx", 200},
		{"index.jsx", 400},
		{"index.tsx", 400},
	}
	for _, tc := range cases {
		if got := priority(format.File{Path: tc.path}); got != tc.want {
			t.Errorf("priority(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
	// The documented order must actually hold as a total order.
	ladder := []string{
		"LICENSE", "main.go", "service.proto", "guide.md",
		"lib.go", "cfg.yml", "run.sh", "x_test.go", "data.bin",
	}
	for i := 0; i+1 < len(ladder); i++ {
		if priority(format.File{Path: ladder[i]}) <= priority(format.File{Path: ladder[i+1]}) {
			t.Errorf("priority(%q) = %d is not strictly above priority(%q) = %d",
				ladder[i], priority(format.File{Path: ladder[i]}),
				ladder[i+1], priority(format.File{Path: ladder[i+1]}))
		}
	}
}

func TestPackBudgetReportsOmitted(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "README.md"), []byte("# Project\n\nshort readme\n"))
	mustWrite(t, filepath.Join(dir, "big.go"), []byte(strings.Repeat("// line\n", 2000)))
	mustWrite(t, filepath.Join(dir, "big2.md"), []byte(strings.Repeat("word\n", 1500)))

	bundle, err := Pack(dir, Options{
		Walker: walker.Options{RespectGitignore: true, ReadContent: true},
		Budget: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Omitted) != 2 {
		t.Fatalf("omitted = %v, want two paths", bundle.Omitted)
	}
	if bundle.Omitted[0] != "big.go" || bundle.Omitted[1] != "big2.md" {
		t.Errorf("omitted = %v, want big.go then big2.md", bundle.Omitted)
	}
	if bundle.OmittedTokens <= 0 {
		t.Errorf("OmittedTokens = %d, want > 0", bundle.OmittedTokens)
	}
	// The bundle total must equal the kept files' total, and kept plus omitted
	// must account for every token the walk saw.
	if bundle.TotalTokens != sumTokens(bundle.Files) {
		t.Errorf("TotalTokens = %d, want sum of kept files (%d)",
			bundle.TotalTokens, sumTokens(bundle.Files))
	}
	all, err := Pack(dir, Options{Walker: walker.Options{RespectGitignore: true, ReadContent: true}})
	if err != nil {
		t.Fatal(err)
	}
	if all.TotalTokens != bundle.TotalTokens+bundle.OmittedTokens {
		t.Errorf("kept (%d) + omitted (%d) = %d, want %d (the whole walk)",
			bundle.TotalTokens, bundle.OmittedTokens,
			bundle.TotalTokens+bundle.OmittedTokens, all.TotalTokens)
	}
}

func TestPackNoBudgetHasNoOmitted(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("package a\n"))

	bundle, err := Pack(dir, Options{Walker: walker.Options{ReadContent: true}})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Omitted != nil {
		t.Errorf("Omitted = %v, want nil with no budget", bundle.Omitted)
	}
	if bundle.OmittedTokens != 0 {
		t.Errorf("OmittedTokens = %d, want 0", bundle.OmittedTokens)
	}
}

func TestPackAllowlistDropsAreNotOmitted(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("package a\n"))
	mustWrite(t, filepath.Join(dir, "b.go"), []byte("package b\n"))

	bundle, err := Pack(dir, Options{
		Walker: walker.Options{ReadContent: true},
		Budget: 100000,
		Files:  []string{"a.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 1 || bundle.Files[0].Path != "a.go" {
		t.Fatalf("files = %v", paths(bundle.Files))
	}
	// b.go was excluded by the caller, not by the budget, so it is not a cut.
	if len(bundle.Omitted) != 0 {
		t.Errorf("Omitted = %v, want empty: allowlist drops are not budget cuts", bundle.Omitted)
	}
}

func TestPackMissingRoot(t *testing.T) {
	if _, err := Pack(filepath.Join(t.TempDir(), "no-such-dir"), Options{}); err == nil {
		t.Fatal("Pack on a missing root must fail")
	}
}

func TestPackBinaryGetsPathTokens(t *testing.T) {
	// .sqlite is binary by extension but not in the default file denylist, so
	// it is packed — flagged binary, with no content.
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "data.sqlite"), []byte{0x53, 0x51, 0x4c, 0x69, 0x74, 0x65})
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("package a\n"))

	bundle, err := Pack(dir, Options{Walker: walker.Options{ReadContent: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(bundle.Files))
	}
	var binary *format.File
	for i := range bundle.Files {
		if bundle.Files[i].Binary {
			binary = &bundle.Files[i]
		}
	}
	if binary == nil {
		t.Fatalf("data.sqlite was not flagged binary: %v", paths(bundle.Files))
	}
	if binary.Content != "" {
		t.Errorf("a binary file must not carry content: %q", binary.Content)
	}
	if binary.Tokens <= 0 {
		t.Error("a binary file still costs path tokens and must not be free")
	}
}

func TestPackWithoutContentUsesPathTokens(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "big/file.go"), []byte(strings.Repeat("x\n", 5000)))

	// ReadContent:false is what repo_map uses; tokens must come from the path.
	noContent, err := Pack(dir, Options{Walker: walker.Options{ReadContent: false}})
	if err != nil {
		t.Fatal(err)
	}
	if len(noContent.Files) != 1 {
		t.Fatalf("got %d files", len(noContent.Files))
	}
	f := noContent.Files[0]
	if f.Content != "" {
		t.Errorf("Content = %q with ReadContent:false", f.Content)
	}
	if f.Tokens <= 0 {
		t.Error("expected path-derived tokens")
	}
	if f.Tokens > 50 {
		t.Errorf("tokens for an unread file = %d, want a small path estimate", f.Tokens)
	}

	// Reading the same file costs roughly as many tokens as its content.
	read, err := Pack(dir, Options{Walker: walker.Options{ReadContent: true}})
	if err != nil {
		t.Fatal(err)
	}
	if read.Files[0].Tokens <= f.Tokens {
		t.Errorf("tokens with content (%d) should exceed the path estimate (%d)",
			read.Files[0].Tokens, f.Tokens)
	}
}

func paths(files []format.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func sumTokens(files []format.File) int {
	total := 0
	for _, f := range files {
		total += f.Tokens
	}
	return total
}

func equal(a, b []string) bool {
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

func mustWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRootName(t *testing.T) {
	for _, tc := range []struct {
		root string
		want string
	}{
		// Absolute paths reduce to the directory's base name.
		{"C:\\Users\\alice\\repo", "repo"},
		{"/home/alice/repo", "repo"},
		// A trailing separator must not leave an empty label.
		{"C:\\Users\\alice\\repo\\", "repo"},
		{"repo", "repo"},
		// A bare separator resolves to the drive or filesystem root itself.
		// Trimming it to "" would give a bundle no root at all, so keep it.
		{string(filepath.Separator), string(filepath.Separator)},
		{"", ""},
	} {
		if got := rootName(tc.root); got != tc.want {
			t.Errorf("rootName(%q) = %q, want %q", tc.root, got, tc.want)
		}
	}
}

func TestPackRootIsBaseName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("package a\n"))

	bundle, err := Pack(dir, Options{Walker: walker.Options{ReadContent: true}})
	if err != nil {
		t.Fatal(err)
	}
	// The bundle names the directory, never the machine it was built on.
	if bundle.Root != filepath.Base(dir) {
		t.Errorf("root = %q, want %q", bundle.Root, filepath.Base(dir))
	}
	if bundle.Root == dir {
		t.Errorf("root leaked the absolute path: %q", bundle.Root)
	}
}
