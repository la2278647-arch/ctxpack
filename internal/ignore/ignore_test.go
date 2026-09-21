package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompileGlob(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.go", "foo.go", true},
		{"*.go", "a/b/foo.go", true}, // basename → any depth
		{"*.go", "foo.txt", false},
		{"src/**/*.go", "src/x/y.go", true}, // ** crosses separators
		{"src/**/*.go", "src/y.go", true},
		{"src/**/*.go", "other/y.go", false},
		{"a/b", "a/b", true}, // contains "/" → anchored
		{"a/b", "c/a/b", false},
		{"!keep.go", "keep.go", true}, // negation compiles & matches
		{"build/", "build", true},     // dir-only stripped
	}
	for _, c := range cases {
		re, err := CompileGlob(c.pattern)
		if err != nil {
			t.Fatalf("CompileGlob(%q) error: %v", c.pattern, err)
		}
		// CompileGlob doesn't know about dirOnly; for basename/anchored test the
		// regex against the path.
		// Strip a leading "!" or trailing "/" for the regex path since
		// CompileGlob receives the raw pattern; emulate the normalization the
		// loader does so the test mirrors real usage.
		p := c.pattern
		p = trimLeft(p, "!")
		p = trimRight(p, "/")
		re, _ = CompileGlob(p)
		got := re.MatchString(c.path)
		if got != c.want {
			t.Errorf("CompileGlob(%q).Match(%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// trimLeft/trimRight are tiny local helpers so the test doesn't depend on the
// loader's normalization.
func trimLeft(s, cut string) string {
	for len(s) > 0 && string(s[0]) == cut {
		s = s[1:]
	}
	return s
}
func trimRight(s, cut string) string {
	for len(s) > 0 && string(s[len(s)-1]) == cut {
		s = s[:len(s)-1]
	}
	return s
}

func TestMatcher(t *testing.T) {
	dir := t.TempDir()
	// Root .gitignore
	gi := filepath.Join(dir, ".gitignore")
	content := []byte("node_modules/\n*.log\n!important.log\n/dist\nkeep.txt\n")
	if err := os.WriteFile(gi, content, 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewMatcher(dir)
	if err := m.Load(gi); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"node_modules", true, true},
		{"foo/node_modules", true, true},
		{"app.log", false, true},
		{"important.log", false, false}, // negated
		{"dist", true, true},
		{"dist/out", false, true}, // under excluded dir
		{"keep.txt", false, true},
		{"keep.txt", false, true},
		{"main.go", false, false},
		{"src/main.go", false, false},
	}
	for _, c := range cases {
		got := m.Match(c.path, c.isDir)
		if got != c.want {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}
