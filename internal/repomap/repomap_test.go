package repomap

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/la2278647-arch/ctxpack/internal/walker"
)

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

func countNodes(n *Node) int {
	if n == nil {
		return 0
	}
	c := 1
	for _, k := range n.Children {
		c += countNodes(k)
	}
	return c
}

func byName(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func leaf(tree *Node) *Node {
	// The only file in the tree.
	for _, c := range tree.Children {
		if c.IsDir {
			return leaf(c)
		}
		return c
	}
	return nil
}

func TestBuildTotalsAndRollup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "hello readme\n")
	writeFile(t, dir, filepath.Join("src", "main.go"), "package main\n")
	writeFile(t, dir, filepath.Join("src", "util.go"), "package main\nfunc f() {}\n")
	writeFile(t, dir, filepath.Join("docs", "guide.md"), "guide\n")

	root, tokens, bytes, err := Build(dir, walker.Options{ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if !root.IsDir || countNodes(root) != 7 {
		t.Fatalf("root = %+v nodes = %d, want 7", root, countNodes(root))
	}
	// Totals must equal the root node, which must equal the sum of its files.
	if root.Tokens != tokens || root.Bytes != bytes {
		t.Errorf("root (%dt/%dB) != returned totals (%dt/%dB)",
			root.Tokens, root.Bytes, tokens, bytes)
	}
	if bytes != 13+13+25+6 {
		t.Errorf("bytes = %d, want 57", bytes)
	}
	// An intermediate directory must hold exactly its subtree.
	src := byName(root, "src")
	if src == nil || !src.IsDir {
		t.Fatalf("src missing: %s", Render(root))
	}
	wantTokens := byName(src, "main.go").Tokens + byName(src, "util.go").Tokens
	wantBytes := 13 + 25
	if src.Tokens != wantTokens || src.Bytes != wantBytes {
		t.Errorf("src = %dt/%dB, want %dt/%dB", src.Tokens, src.Bytes, wantTokens, wantBytes)
	}
}

func TestBuildCountsBySizeWhenContentNotRead(t *testing.T) {
	// The MCP repo_map path sets ReadContent:false. Token counts must still
	// track file size; estimating from the path name gives ~1 token for a
	// 40 KiB file and makes the map useless.
	dir := t.TempDir()
	writeFile(t, dir, "x", strings.Repeat("abcdefghij", 4096)) // 40960 bytes

	withTok, _, _, err := Build(dir, walker.Options{ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	withoutTok, _, _, err := Build(dir, walker.Options{ReadContent: false})
	if err != nil {
		t.Fatal(err)
	}
	a, b := leaf(withTok), leaf(withoutTok)
	if b.Tokens < a.Tokens/2 {
		t.Errorf("ReadContent:false gave %d tokens vs %d with content; counts must scale with size",
			b.Tokens, a.Tokens)
	}
}

func TestBuildSkipsHiddenAndVCS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.go", "package a\n")
	writeFile(t, dir, ".env", "SECRET=1\n")
	writeFile(t, dir, filepath.Join(".git", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, dir, filepath.Join(".github", "workflow.yml"), "name: ci\n")

	root, _, _, err := Build(dir, walker.Options{ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := countNodes(root); n != 2 {
		t.Fatalf("nodes = %d, want 2 (root + keep.go): %s", n, Render(root))
	}
	if got := Render(root); strings.Contains(got, ".env") || strings.Contains(got, ".git") || strings.Contains(got, ".github") {
		t.Errorf("hidden or VCS content leaked: %s", got)
	}
}

func TestBuildMissingRoot(t *testing.T) {
	if _, _, _, err := Build(filepath.Join(t.TempDir(), "no-such-dir"), walker.Options{}); err == nil {
		t.Fatal("expected an error for a missing root")
	}
}

func TestSortDirsBeforeFiles(t *testing.T) {
	n := &Node{
		Name: "root", IsDir: true,
		Children: []*Node{
			{Name: "zeta", IsDir: false},
			{Name: "alpha", IsDir: false},
			{Name: "beta", IsDir: true},
			{Name: "gamma", IsDir: true},
		},
	}
	sortNodes(n)
	var got []string
	for _, c := range n.Children {
		got = append(got, c.Name)
	}
	if strings.Join(got, ",") != "beta,gamma,alpha,zeta" {
		t.Errorf("sort order = %v", got)
	}
}

func TestFileAndDirShareName(t *testing.T) {
	// Windows will not allow a file and a directory to share a name in one
	// directory, so this collision is unreachable here. On Linux the walker
	// can return both, and findChild would merge them into a single node.
	if runtime.GOOS == "windows" {
		t.Skip("Windows forbids a file and directory sharing a name")
	}
	dir := t.TempDir()
	writeFile(t, dir, "a", "root-level file named a\n")
	writeFile(t, dir, filepath.Join("a", "b.go"), "package a\n")

	root, _, _, err := Build(dir, walker.Options{ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	// Both must survive: a file "a" and a directory "a/".
	got := Render(root)
	if !strings.Contains(got, "a/ ") || !strings.Contains(got, "\na ") {
		t.Errorf("file/dir name collision lost an entry: %s", got)
	}
}

func TestRender(t *testing.T) {
	tree := &Node{Name: "root", IsDir: true, Tokens: 10, Bytes: 100, Children: []*Node{
		{Name: "d", IsDir: true, Tokens: 5, Bytes: 50, Children: []*Node{
			{Name: "f", IsDir: false, Tokens: 5, Bytes: 50},
		}},
		{Name: "e", IsDir: false, Tokens: 5, Bytes: 50},
	}}
	want := "root/  [10t, 100B]\n  d/  [5t, 50B]\n    f  [5t, 50B]\n  e  [5t, 50B]\n"
	if got := Render(tree); got != want {
		t.Errorf("Render:\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderEmptyTree(t *testing.T) {
	if got := Render(&Node{Name: "empty", IsDir: true}); got != "empty/  [0t, 0B]\n" {
		t.Errorf("empty render = %q", got)
	}
}

func TestRootNodeName(t *testing.T) {
	cases := []struct {
		root string
		want string
	}{
		{filepath.FromSlash("C:/tmp/ctxpack"), "ctxpack"},
		{filepath.FromSlash("C:/tmp/ctxpack/"), "ctxpack"},
		{filepath.FromSlash("/home/u/proj"), "proj"},
		{filepath.FromSlash("C:/tmp/.repo"), ".repo"},
	}
	for _, tc := range cases {
		if got := rootNodeName(tc.root); got != tc.want {
			t.Errorf("rootNodeName(%q) = %q, want %q", tc.root, got, tc.want)
		}
	}
}

func TestBuildRootLabelIsBaseName(t *testing.T) {
	// "ctxpack map ." used to render the tree root as "./"; the label must be
	// the walked directory's base name regardless of how the caller wrote it.
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package main\n")

	for _, arg := range []string{
		dir,
		dir + string(filepath.Separator),
		dir + string(filepath.Separator) + ".",
	} {
		root, _, _, err := Build(arg, walker.Options{ReadContent: true})
		if err != nil {
			t.Fatalf("arg %q: %v", arg, err)
		}
		want := filepath.Base(dir)
		if root.Name != want {
			t.Errorf("arg %q: root.Name = %q, want %q", arg, root.Name, want)
		}
		if got := Render(root); !strings.HasPrefix(got, want+"/") {
			t.Errorf("arg %q: Render = %q, want it to start with %q", arg, got, want+"/")
		}
	}
}

func TestHuman(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
	} {
		if got := human(tc.n); got != tc.want {
			t.Errorf("human(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
