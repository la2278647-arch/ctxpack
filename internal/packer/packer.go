// Package packer orchestrates the walker, token counter, and formatter into a
// single Pack entry point. When a token budget is supplied it greedily
// selects the most relevant files until the budget is reached, prioritizing
// documentation and entry points over large low-signal files.
package packer

import (
	"sort"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/format"
	"github.com/la2278647-arch/ctxpack/internal/walker"
)

// Options configures packing.
type Options struct {
	// Walker options.
	Walker walker.Options
	// Budget is the max tokens to include (0 = unlimited).
	Budget int
	// Files overrides selection: when non-empty, only these relative paths are
	// packed (used by diff mode). Ignored when empty.
	Files []string
	// Deleted carries paths a diff removed. Their content is unavailable, so
	// they are reported by name only; ignored when empty.
	Deleted []string
}

// policyDocs are the project-level documents a reader reaches for first. The
// readme prefix is matched separately below, because it covers readme.md,
// readme.rst and readme.adoc at once. These are exact basenames, with or
// without an extension, so securityscanner.go is not confused for SECURITY.md.
var policyDocs = map[string]bool{
	"license":            true,
	"license.md":         true,
	"licence":            true,
	"licence.md":         true,
	"copying":            true,
	"copying.md":         true,
	"contributing":       true,
	"contributing.md":    true,
	"contributing.txt":   true,
	"code_of_conduct":    true,
	"code_of_conduct.md": true,
	"codeofconduct.md":   true,
	"security":           true,
	"security.md":        true,
	"patents":            true,
	"patents.md":         true,
}

// Pack walks root, counts tokens, applies an optional budget, and returns a
// format.Bundle ready for rendering.
func Pack(root string, opts Options) (*format.Bundle, error) {
	res, err := walker.Walk(root, opts.Walker)
	if err != nil {
		return nil, err
	}
	cnt := counter.NewDefault()

	var files []format.File
	for _, fe := range res.Files {
		if len(opts.Files) > 0 && !inSet(fe.RelPath, opts.Files) {
			continue
		}
		f := format.File{
			Path:   fe.RelPath,
			Bytes:  int(fe.Size),
			Binary: fe.IsBinary,
		}
		if !fe.IsBinary && fe.Content != nil {
			e := cnt.EstimateBytes(fe.Content)
			f.Tokens = e.Tokens
			f.Content = string(fe.Content)
		} else {
			e := cnt.Estimate(fe.RelPath)
			f.Tokens = e.Tokens
		}
		files = append(files, f)
	}

	var omitted []format.File
	if opts.Budget > 0 {
		files, omitted = applyBudget(files, opts.Budget)
	} else {
		sort.SliceStable(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
	}

	bundle := &format.Bundle{
		Root:          res.Root,
		Files:         files,
		Skipped:       res.Skipped,
		Omitted:       omittedPaths(omitted),
		OmittedTokens: omittedTokens(omitted),
		Deleted:       opts.Deleted,
	}
	for _, f := range files {
		bundle.TotalTokens += f.Tokens
		bundle.TotalBytes += f.Bytes
	}
	return bundle, nil
}

// applyBudget greedily selects files by priority until the token budget is
// reached, then returns the selected subset in path order together with the
// files the budget left out. The omitted list is not dropped: a caller that
// sees "40 files, ~500 tokens" for a 5000-token budget needs to know what did
// not make the cut and what it would have cost.
func applyBudget(files []format.File, budget int) (kept, omitted []format.File) {
	type scored struct {
		f     format.File
		score int
	}
	scoredFiles := make([]scored, len(files))
	for i, f := range files {
		scoredFiles[i] = scored{f: f, score: priority(f)}
	}
	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score != scoredFiles[j].score {
			return scoredFiles[i].score > scoredFiles[j].score
		}
		return scoredFiles[i].f.Tokens < scoredFiles[j].f.Tokens
	})

	used := 0
	for _, s := range scoredFiles {
		if used+s.f.Tokens > budget {
			omitted = append(omitted, s.f)
			continue
		}
		kept = append(kept, s.f)
		used += s.f.Tokens
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Path < kept[j].Path })
	sort.SliceStable(omitted, func(i, j int) bool { return omitted[i].Path < omitted[j].Path })
	return kept, omitted
}

// omittedPaths reports where the budget cut. Paths only — an omitted file may
// be the largest thing in the tree, and its content is exactly what should not
// be carried.
func omittedPaths(files []format.File) []string {
	if len(files) == 0 {
		return nil
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths
}

func omittedTokens(files []format.File) int {
	total := 0
	for _, f := range files {
		total += f.Tokens
	}
	return total
}

// priority scores a file for budget selection. Higher = more important. The
// ladder is the order the docs promise: policy documents, entry points,
// documentation, source, config, scripts, and tests last.
func priority(f format.File) int {
	low := strings.ToLower(f.Path)
	base := low
	if i := strings.LastIndex(low, "/"); i >= 0 {
		base = low[i+1:]
	}
	if strings.HasPrefix(base, "readme") || policyDocs[base] {
		return 1000
	}
	switch {
	case strings.HasSuffix(low, "main.go") || strings.HasSuffix(low, "index.ts") ||
		strings.HasSuffix(low, "index.js") || strings.HasSuffix(low, "mod.go") ||
		strings.HasSuffix(low, "server.js") || strings.HasSuffix(low, "manage.py"):
		return 400
	case strings.HasSuffix(low, ".proto") || strings.HasSuffix(low, ".graphql") ||
		strings.HasSuffix(low, ".thrift"):
		return 350
	case strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".rst") ||
		strings.HasSuffix(low, ".txt"):
		return 300
	// The test case must precede the general source-code case: _test.go,
	// .test.js and test_*.py are also .go, .js and .py, so matching the
	// extension first would score every test as source and nothing would be
	// left to rank last.
	case strings.HasSuffix(low, "_test.go") || strings.HasSuffix(low, ".test.js") ||
		strings.HasSuffix(low, ".spec.ts") || strings.HasSuffix(low, ".test.ts") ||
		strings.HasSuffix(low, ".test.py") ||
		(strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")):
		return 50
	case strings.HasSuffix(low, ".go") || strings.HasSuffix(low, ".ts") ||
		strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".py") ||
		strings.HasSuffix(low, ".rs") || strings.HasSuffix(low, ".java") ||
		strings.HasSuffix(low, ".rb") || strings.HasSuffix(low, ".cs"):
		return 200
	case strings.HasSuffix(low, ".yml") || strings.HasSuffix(low, ".yaml") ||
		strings.HasSuffix(low, ".toml") || strings.HasSuffix(low, ".json") ||
		strings.HasSuffix(base, "dockerfile") || strings.HasSuffix(base, "makefile"):
		return 150
	case strings.HasSuffix(low, ".sh"):
		return 100
	}
	return 10
}

func inSet(s string, set []string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}
