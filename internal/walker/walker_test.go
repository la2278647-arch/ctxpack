package walker

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWalkDefaults(t *testing.T) {
	dir := t.TempDir()
	// Source files kept.
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("package main\nfunc main(){}\n"))
	mustWrite(t, filepath.Join(dir, "b.txt"), []byte("hello\n"))
	mustWrite(t, filepath.Join(dir, "sub"), nil) // dir
	mustWrite(t, filepath.Join(dir, "sub", "c.go"), []byte("package sub\n"))
	// Skipped by default denylist.
	mustWrite(t, filepath.Join(dir, "node_modules"), nil)
	mustWrite(t, filepath.Join(dir, "node_modules", "x.js"), []byte("x\n"))
	mustWrite(t, filepath.Join(dir, "pic.png"), []byte("\x89PNG\r\n\x1a\n\x00\x00")) // binary ext
	mustWrite(t, filepath.Join(dir, ".env"), []byte("SECRET=1\n"))                   // hidden
	mustWrite(t, filepath.Join(dir, ".git"), nil)                                    // VCS dir
	mustWrite(t, filepath.Join(dir, ".git", "config"), []byte("x\n"))

	res, err := Walk(dir, Options{RespectGitignore: true, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Files {
		got = append(got, f.RelPath)
	}
	sort.Strings(got)
	want := []string{"a.go", "b.txt", "sub/c.go"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v (skipped=%d)", got, want, res.Skipped)
	}
}

func TestWalkIncludeExclude(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), []byte("a\n"))
	mustWrite(t, filepath.Join(dir, "a_test.go"), []byte("test\n"))
	mustWrite(t, filepath.Join(dir, "b.md"), []byte("md\n"))
	mustWrite(t, filepath.Join(dir, "node_modules"), nil)
	mustWrite(t, filepath.Join(dir, "node_modules", "x.js"), []byte("x\n"))

	res, err := Walk(dir, Options{
		Include:          []string{"*.go"},
		Exclude:          []string{"*_test.go"},
		RespectGitignore: false,
		ReadContent:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Files {
		got = append(got, f.RelPath)
	}
	if len(got) != 1 || got[0] != "a.go" {
		t.Fatalf("got %v, want [a.go]", got)
	}
}

func TestWalkMaxSize(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "small.txt"), []byte("small\n"))
	mustWrite(t, filepath.Join(dir, "big.txt"), make([]byte, 5000))
	res, err := Walk(dir, Options{MaxFileSize: 1000, RespectGitignore: false, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	var content []string
	for _, f := range res.Files {
		if f.Content != nil {
			content = append(content, f.RelPath)
		}
	}
	if len(content) != 1 || content[0] != "small.txt" {
		t.Fatalf("content-bearing files = %v, want [small.txt]", content)
	}
}

func mustWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if content == nil {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
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
