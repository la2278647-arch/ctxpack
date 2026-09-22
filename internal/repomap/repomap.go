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
	cnt := counter.NewDefault()
	rootNode := &Node{Name: rootNodeName(res.Root), IsDir: true}
	totalTokens, totalBytes := 0, 0

	for _, fe := range res.Files {
		// Without content, size is the only remaining signal, and it is worth
		// far more than tokenising the file's own name.
		e := cnt.EstimateSize(fe.Size)
		if fe.Content != nil {
			e = cnt.EstimateBytes(fe.Content)
		}
		parts := strings.Split(fe.RelPath, "/")
		cur := rootNode
		cur.Tokens += e.Tokens
		cur.Bytes += int(fe.Size)
		for i, p := range parts {
			if p == "" {
				continue
			}
			isLeaf := i == len(parts)-1
			child := findChild(cur, p)
			if child == nil {
				child = &Node{Name: p, IsDir: !isLeaf}
				cur.Children = append(cur.Children, child)
			}
			child.Tokens += e.Tokens
			child.Bytes += int(fe.Size)
			if isLeaf {
				child.IsDir = false
			}
			cur = child
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

func findChild(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
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
