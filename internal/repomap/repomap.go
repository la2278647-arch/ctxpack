// Package repomap builds a compact, token-aware tree outline of a repository.
// Each directory and file is annotated with a token estimate and byte size,
// giving an at-a-glance view of where the context budget is being spent.
package repomap

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/walker"
)

// Node is a tree node (file or directory).
type Node struct {
	Name     string
	IsDir    bool
	Tokens   int
	Bytes    int
	Children []*Node
}

// Build walks root and returns the token-annotated tree plus totals.
func Build(root string, opts walker.Options) (*Node, int, int, error) {
	res, err := walker.Walk(root, opts)
	if err != nil {
		return nil, 0, 0, err
	}
	rootNode := &Node{Name: rootNodeName(res.Root), IsDir: true}
	tree, totalTokens, totalBytes, err := fold(rootNode, res)
	return tree, totalTokens, totalBytes, nil
}

// fold adds every file of res to the tree rooted at rootNode and returns the
// root together with the totals across all of them. Split from Build so the
// folding can be tested against a fabricated walk result — a file and a
// directory may share a name, which the walker cannot produce on Windows.
//
// It cannot fail: Build is the only caller and it only calls it with the
// result of a successful Walk, so a test that reaches here has already passed
// every check that could return an error. The extra return is nil so the
// wrapping call above stays a one-liner.
func fold(rootNode *Node, res *walker.Result) (*Node, int, int, error) {
	cnt := counter.NewDefault()
	totalTokens, totalBytes := 0, 0

	for _, fe := range res.Files {
		// Without content, size is the only remaining signal, and it is worth
		// far more than tokenising the file's own name.
		e := cnt.EstimateSize(fe.Size)
		if fe.Content != nil {
			e = cnt.EstimateBytes(fe.Content)
		}
		cur := rootNode
		cur.Tokens += e.Tokens
		cur.Bytes += int(fe.Size)
		parts := strings.Split(fe.RelPath, "/")
		for i, p := range parts {
			if p == "" {
				continue
			}
			cur = childFor(cur, p, i == len(parts)-1)
			cur.Tokens += e.Tokens
			cur.Bytes += int(fe.Size)
		}
		totalTokens += e.Tokens
		totalBytes += int(fe.Size)
	}
	sortNodes(rootNode)
	return rootNode, totalTokens, totalBytes, nil
}

// rootNodeName labels the tree root with the walked directory's base name
// instead of the raw argument, so that "ctxpack map ." renders "ctxpack/"
// rather than "./".
func rootNodeName(root string) string {
	return strings.TrimRight(filepath.Base(root), string(filepath.Separator))
}

// childFor returns (creating it when needed) the child of n that holds p: the
// file node when p is the final path component, the directory node otherwise.
//
// The kind is part of the key, not just the name. A file and a directory may
// legally share a name on Linux and macOS, and matching on the name alone
// folded them into one node — the file disappeared as an entry, and its size
// was added on top of the directory's subtree total.
func childFor(n *Node, p string, isLeaf bool) *Node {
	for _, c := range n.Children {
		if c.Name == p && c.IsDir == !isLeaf {
			return c
		}
	}
	c := &Node{Name: p, IsDir: !isLeaf}
	n.Children = append(n.Children, c)
	return c
}

func sortNodes(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		if n.Children[i].IsDir != n.Children[j].IsDir {
			return n.Children[i].IsDir
		}
		return n.Children[i].Name < n.Children[j].Name
	})
	for _, c := range n.Children {
		sortNodes(c)
	}
}

// Render produces an indented tree string.
func Render(n *Node) string {
	var sb strings.Builder
	renderNode(&sb, n, "")
	return sb.String()
}

func renderNode(sb *strings.Builder, n *Node, prefix string) {
	fmt.Fprintf(sb, "%s%s  [%dt, %s]\n", prefix, display(n), n.Tokens, human(n.Bytes))
	for _, c := range n.Children {
		renderNode(sb, c, prefix+"  ")
	}
}

func display(n *Node) string {
	if n.IsDir {
		return n.Name + "/"
	}
	return n.Name
}

func human(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
}
