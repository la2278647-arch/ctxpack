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

func TestFoldFileAndDirShareName(t *testing.T) {
	// A file and a directory may share a name — legal on Linux and macOS, not
	// on Windows. Matching on the name alone folded the two into one node, so
	// the file vanished as an entry and its size was stacked on top of the
	// directory's subtree total. fold is called directly so the case runs on
	// every platform.
	a := []byte("root file a\n")           // 12 bytes, the file named a
	b := []byte("package a\nfunc b(){}\n") // 21 bytes, a/b.go
	cases := []struct {
		name  string
		files []walker.FileEntry
	}{
		{"file visited first", []walker.FileEntry{
			{RelPath: "a", Size: int64(len(a)), Content: a},
			{RelPath: "a/b.go", Size: int64(len(b)), Content: b},
		}},
		{"directory visited first", []walker.FileEntry{
			{RelPath: "a/b.go", Size: int64(len(b)), Content: b},
			{RelPath: "a", Size: int64(len(a)), Content: a},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, totalTokens, totalBytes, err := fold(
				&Node{Name: "repo", IsDir: true},
				&walker.Result{Root: "/tmp/repo", Files: tc.files},
			)
			if err != nil {
				t.Fatal(err)
			}
			dir := byKind(root, "a", true)
			leaf := byKind(root, "a", false)
			if dir == nil || leaf == nil {
				t.Fatalf("collision lost an entry:\n%s", Render(root))
			}
			// The file must stay a leaf: the old code hung the directory's
			// children off it.
			if len(leaf.Children) != 0 {
				t.Errorf("file node has %d children, want 0:\n%s", len(leaf.Children), Render(root))
			}
			if leaf.Bytes != len(a) {
				t.Errorf("file bytes = %d, want %d", leaf.Bytes, len(a))
			}
			if dir.Bytes != len(b) {
				t.Errorf("directory bytes = %d, want %d (its subtree only)", dir.Bytes, len(b))
			}
			if totalBytes != len(a)+len(b) {
				t.Errorf("total bytes = %d, want %d: both files counted once",
					totalBytes, len(a)+len(b))
			}
			if len(dir.Children) != 1 || dir.Children[0].Name != "b.go" {
				t.Fatalf("directory children = %+v, want one b.go", dir.Children)
			}
			if dir.Children[0].Bytes != len(b) {
				t.Errorf("b.go bytes = %d, want %d", dir.Children[0].Bytes, len(b))
			}
			// Every token belongs to exactly one of the two nodes.
			if totalTokens != leaf.Tokens+dir.Tokens {
				t.Errorf("total tokens = %d, want %d", totalTokens, leaf.Tokens+dir.Tokens)
			}
			if got := Render(root); !strings.Contains(got, "  a/ ") || !strings.Contains(got, "\n  a ") {
				t.Errorf("render must show both the directory and the file:\n%s", got)
			}
		})
	}
}

func TestFoldCountsFromSizeWhenContentAbsent(t *testing.T) {
	// ReadContent:false is what repo_map passes, so the size-only path must
	// still fold correctly and must not count a path name as tokens.
	root, _, totalBytes, err := fold(
		&Node{Name: "repo", IsDir: true},
		&walker.Result{
			Root:  "/tmp/repo",
			Files: []walker.FileEntry{{RelPath: "src/large.go", Size: 40000}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if totalBytes != 40000 {
		t.Errorf("total bytes = %d, want 40000", totalBytes)
	}
	if root.Tokens < 9000 {
		t.Errorf("tokens = %d for 40 KiB, want roughly a quarter of the bytes", root.Tokens)
	}
}

func byKind(n *Node, name string, wantDir bool) *Node {
	for _, c := range n.Children {
		if c.Name == name && c.IsDir == wantDir {
			return c
		}
	}
	return nil
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
