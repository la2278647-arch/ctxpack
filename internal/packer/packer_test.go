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

func mustWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
